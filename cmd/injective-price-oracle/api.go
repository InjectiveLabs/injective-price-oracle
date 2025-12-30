package main

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/InjectiveLabs/metrics"

	api_health_rpc "github.com/InjectiveLabs/injective-price-oracle/api/gen/grpc/health/pb"
	api_health "github.com/InjectiveLabs/injective-price-oracle/api/gen/grpc/health/server"
	api_health_service "github.com/InjectiveLabs/injective-price-oracle/api/gen/health"
	api_health_http_server "github.com/InjectiveLabs/injective-price-oracle/api/gen/http/health/server"
	api_http_server "github.com/InjectiveLabs/injective-price-oracle/api/gen/http/injective_price_oracle_api/server"
	swaggerHTTPServer "github.com/InjectiveLabs/injective-price-oracle/api/gen/http/swagger/server"
	api_server_service "github.com/InjectiveLabs/injective-price-oracle/api/gen/injective_price_oracle_api"
	swaggerEndpoints "github.com/InjectiveLabs/injective-price-oracle/api/gen/swagger"
	"github.com/InjectiveLabs/injective-price-oracle/internal/service/health"
	"github.com/InjectiveLabs/injective-price-oracle/internal/service/oracle"
	"github.com/InjectiveLabs/injective-price-oracle/internal/service/swagger"

	log "github.com/InjectiveLabs/suplog"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	cli "github.com/jawher/mow.cli"
	"github.com/pkg/errors"
	"github.com/rs/cors"
	goahttp "goa.design/goa/v3/http"
	goaMiddleware "goa.design/goa/v3/middleware"
	"google.golang.org/grpc"
)

// apiCmd action runs the service
//
// $ injective-price-oracle api
func apiCmd(cmd *cli.Cmd) {
	var (
		// Metrics
		statsdPrefix   *string
		statsdAddr     *string
		statsdAgent    *string
		statsdStuckDur *string
		statsdMocking  *string
		statsdDisabled *string

		grpcWebListenAddress  *string
		grpcWebRequestTimeout *string
		apiKey                *string
	)

	initStatsdOptions(
		cmd,
		&statsdPrefix,
		&statsdAddr,
		&statsdAgent,
		&statsdStuckDur,
		&statsdMocking,
		&statsdDisabled,
	)

	iniAPIOptions(
		cmd,
		&grpcWebListenAddress,
		&grpcWebRequestTimeout,
		&apiKey,
	)

	cmd.Action = func() {
		ctx := context.Background()
		ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		startMetricsGathering(
			statsdPrefix,
			statsdAddr,
			statsdAgent,
			statsdStuckDur,
			statsdMocking,
			statsdDisabled,
		)

		grpcWebMux := goahttp.NewMuxer()

		requestTimeout, err := time.ParseDuration(*grpcWebRequestTimeout)
		panicIf(err)
		grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(TimeoutInterceptor(requestTimeout)))

		apiSvc := oracle.NewAPIService(*apiKey)

		// Initialize and register Health Service
		healthSvc := health.NewHealthService(log.DefaultLogger, metrics.Tags{
			"svc": "health",
		})
		log.Infof("created API service")

		grpcHealthRouter := api_health.New(
			api_health_service.NewEndpoints(healthSvc),
			nil,
		)

		api_health_rpc.RegisterHealthServer(grpcServer, grpcHealthRouter)

		// http health api
		healthHTTPRouter := api_health_http_server.New(
			api_health_service.NewEndpoints(healthSvc),
			grpcWebMux,
			goahttp.RequestDecoder,
			goahttp.ResponseEncoder,
			newErrorHandler(log.DefaultLogger),
			nil,
		)

		api_health_http_server.Mount(grpcWebMux, healthHTTPRouter)

		// http api
		apiRouter := api_http_server.New(
			api_server_service.NewEndpoints(apiSvc),
			grpcWebMux,
			goahttp.RequestDecoder,
			goahttp.ResponseEncoder,
			newErrorHandler(log.DefaultLogger),
			nil,
			DecodeInjectivePriceOracleAPIProbeRequest,
		)

		api_http_server.Mount(grpcWebMux, apiRouter)

		swaggerSvc := swagger.NewSwaggerService()

		swaggerRouter := swaggerHTTPServer.New(
			swaggerEndpoints.NewEndpoints(swaggerSvc),
			grpcWebMux,
			goahttp.RequestDecoder,
			goahttp.ResponseEncoder,
			newErrorHandler(log.DefaultLogger),
			nil,
			nil,
			nil,
		)
		swaggerHTTPServer.Mount(grpcWebMux, swaggerRouter)

		// only need to serve Grpc-Web
		handlerWithCors := cors.New(cors.Options{
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{
				http.MethodHead,
				http.MethodGet,
				http.MethodPost,
				http.MethodPut,
				http.MethodPatch,
				http.MethodDelete,
			},
			AllowedHeaders:     []string{"*"},
			AllowCredentials:   false,
			OptionsPassthrough: false,
		})

		grpcWeb := grpcweb.WrapServer(grpcServer)
		mountGRPCWebServices(grpcWebMux, grpcWeb, grpcweb.ListGRPCResources(grpcServer), 10*time.Second)

		httpSrv := &http.Server{
			Addr:         *grpcWebListenAddress,
			Handler:      handlerWithCors.Handler(grpcWebMux),
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  10 * time.Second,
		}

		// Start server in goroutine
		go func() {
			log.Infof("injective price oracle api starts listening on %s", *grpcWebListenAddress)
			if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.WithError(err).Fatalln("failed to start HTTP server")
			}
		}()

		<-ctx.Done()

		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelShutdown()

		grpcServer.GracefulStop()

		if err = httpSrv.Shutdown(shutdownCtx); err != nil {
			log.WithError(err).Error("HTTP server graceful shutdown failed")
		}

		log.Info("Shutdown complete")
	}

}

func DecodeInjectivePriceOracleAPIProbeRequest(mr *multipart.Reader, payload **api_server_service.ProbePayload) error {
	var (
		part *multipart.Part
		err  error
		p    = &api_server_service.ProbePayload{}
	)

	for {
		part, err = mr.NextPart()
		if err == io.EOF {
			break
		}
		if part.FormName() == "content" {
			data, err := io.ReadAll(part)
			if err != nil {
				return err
			}
			p.Content = data
		}
	}

	*payload = p
	return nil
}

func mountGRPCWebServices(
	mux goahttp.Muxer,
	grpcWeb *grpcweb.WrappedGrpcServer,
	grpcResources []string,
	requestTimeout time.Duration,
) {
	for _, res := range grpcResources {
		currentResource := res

		log.Infof("[GRPC Web] HTTP POST mounted on %s", currentResource)

		mux.Handle("POST", currentResource, func(resp http.ResponseWriter, req *http.Request) {
			if !grpcWeb.IsGrpcWebRequest(req) {
				resp.WriteHeader(400)
				resp.Write([]byte(fmt.Sprintf("not a GRPC web request on %s", currentResource)))
				return
			}

			ctx, cancel := context.WithTimeout(req.Context(), requestTimeout)
			defer cancel()

			grpcWeb.HandleGrpcWebRequest(resp, req.WithContext(ctx))
		})
	}
}

func newErrorHandler(logger log.Logger) func(context.Context, http.ResponseWriter, error) {
	type stackTracer interface {
		StackTrace() errors.StackTrace
	}

	return func(ctx context.Context, w http.ResponseWriter, err error) {
		id, ok := ctx.Value(goaMiddleware.RequestIDKey).(string)
		if !ok {
			return
		}
		logFields := log.Fields{
			"request_id": id,
		}
		if errWithStack, ok := err.(stackTracer); ok {
			logFields["stack_frames"] = len(errWithStack.StackTrace())
		}
		logger.WithFields(logFields).Warningln(err)
		fmt.Fprintf(w, "request (%s) processing internal error: %v", id, err)
	}
}

func TimeoutInterceptor(timeout time.Duration) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		return handler(ctx, req)
	}
}
