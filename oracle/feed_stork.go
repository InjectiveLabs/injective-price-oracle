package oracle

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/InjectiveLabs/metrics"
	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"
	"github.com/gorilla/websocket"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	log "github.com/xlab/suplog"
)

var _ PricePuller = &storkPriceFeed{}

type StorkFeedConfig struct {
	ProviderName string `toml:"provider"`
	Ticker       string `toml:"ticker"`
	PullInterval string `toml:"pullInterval"`
	Url          string `toml:"url"`
	Header       string `toml:"header"`
	Message      string `toml:"message"`
	OracleType   string `toml:"oracleType"`
}

type StorkConfig struct {
}

type storkPriceFeed struct {
	ticker       string
	providerName string
	interval     time.Duration
	url          string
	header       string
	message      string

	runNonce int32

	logger  log.Logger
	svcTags metrics.Tags

	oracleType oracletypes.OracleType
}

func ParseStorkFeedConfig(body []byte) (*StorkFeedConfig, error) {
	var config StorkFeedConfig
	if err := toml.Unmarshal(body, &config); err != nil {
		err = errors.Wrap(err, "failed to unmarshal TOML config")
		return nil, err
	}

	return &config, nil
}

// NewStorkPriceFeed returns price puller
func NewStorkPriceFeed(cfg *StorkFeedConfig) (PricePuller, error) {
	pullInterval := 1 * time.Minute
	if len(cfg.PullInterval) > 0 {
		interval, err := time.ParseDuration(cfg.PullInterval)
		if err != nil {
			err = errors.Wrapf(err, "failed to parse pull interval: %s (expected format: 60s)", cfg.PullInterval)
			return nil, err
		}

		if interval < 1*time.Second {
			err = errors.Wrapf(err, "failed to parse pull interval: %s (minimum interval = 1s)", cfg.PullInterval)
			return nil, err
		}

		pullInterval = interval
	}

	var oracleType oracletypes.OracleType
	if cfg.OracleType == "" {
		oracleType = oracletypes.OracleType_PriceFeed
	} else {
		tmpType, exist := oracletypes.OracleType_value[cfg.OracleType]
		if !exist {
			return nil, fmt.Errorf("oracle type does not exist: %s", cfg.OracleType)
		}

		oracleType = oracletypes.OracleType(tmpType)
	}

	feed := &storkPriceFeed{
		ticker:       cfg.Ticker,
		providerName: cfg.ProviderName,
		interval:     pullInterval,
		url:          cfg.Url,
		header:       cfg.Header,
		message:      cfg.Message,
		oracleType:   oracleType,

		logger: log.WithFields(log.Fields{
			"svc":      "oracle",
			"dynamic":  true,
			"provider": cfg.ProviderName,
		}),

		svcTags: metrics.Tags{
			"provider": cfg.ProviderName,
		},
	}

	return feed, nil
}

func (f *storkPriceFeed) Interval() time.Duration {
	return f.interval
}

func (f *storkPriceFeed) Symbol() string {
	return f.ticker
}

func (f *storkPriceFeed) Provider() FeedProvider {
	return FeedProviderStork
}

func (f *storkPriceFeed) ProviderName() string {
	return f.providerName
}

func (f *storkPriceFeed) OracleType() oracletypes.OracleType {
	return oracletypes.OracleType_Stork
}

func (f *storkPriceFeed) PullAssetPair(ctx context.Context) (assetPair oracletypes.AssetPair, err error) {
	metrics.ReportFuncCall(f.svcTags)
	doneFn := metrics.ReportFuncTiming(f.svcTags)
	defer doneFn()

	// Parse the URL
	u, err := url.Parse(f.url)
	if err != nil {
		log.Fatal("Error parsing URL:", err)
	}
	header := http.Header{}
	header.Add("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(f.header)))

	dialer := websocket.DefaultDialer
	dialer.EnableCompression = true

	// Connect to the WebSocket server
	conn, resp, err := dialer.Dial(u.String(), header)
	if err != nil {
		if resp != nil {
			log.Printf("Handshake failed with status: %d\n", resp.StatusCode)
			for k, v := range resp.Header {
				log.Printf("%s: %v\n", k, v)
			}
		}
		log.Fatal("Error connecting to WebSocket:", err)
	}
	defer conn.Close()

	log.Println("Connected to WebSocket server:", resp.Status)

	// create ctrl C signal call
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	err = conn.WriteMessage(websocket.TextMessage, []byte(f.message))
	if err != nil {
		log.Fatal("Error writing message:", err)
	}

	// Infinite loop
	go func() {
		for {
			// count := fasle
			var msgResp messageResponse
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Println("Error reading message:", err)
				return
			}
			// count++
			time.Sleep(1 * time.Second)
			log.Printf("Received message: %s\n", message)
			// if count !=
			if err = json.Unmarshal(message, &msgResp); err != nil {
				log.Println("Error unmarshal message:", err)
				return
			}
			println("check message type:", msgResp.Type)
		}

	}()

	<-interrupt
	log.Println("Interrupt received, closing connection")

	return oracletypes.AssetPair{}, nil
}

func (f *storkPriceFeed) PullPrice(ctx context.Context) (
	price decimal.Decimal,
	err error,
) {
	return zeroPrice, nil
}

type messageResponse struct {
	Type    string          `json:"type"`
	TraceID string          `json:"trace_id"`
	Data    map[string]Data `json:"data"`
}

type Data struct {
	Timestamp     int64         `json:"timestamp"`
	AssetID       string        `json:"asset_id"`
	SignatureType string        `json:"signature_type"`
	Trigger       string        `json:"trigger"`
	Price         string        `json:"price"`
	SignedPrices  []SignedPrice `json:"signed_prices"`
}

type SignedPrice struct {
	PublisherKey         string               `json:"publisher_key"`
	ExternalAssetID      string               `json:"external_asset_id"`
	SignatureType        string               `json:"signature_type"`
	Price                string               `json:"price"`
	TimestampedSignature TimestampedSignature `json:"timestamped_signature"`
}

type TimestampedSignature struct {
	Signature Signature `json:"signature"`
	Timestamp int64     `json:"timestamp"`
	MsgHash   string    `json:"msg_hash"`
}

type Signature struct {
	R string `json:"r"`
	S string `json:"s"`
	V string `json:"v"`
}
