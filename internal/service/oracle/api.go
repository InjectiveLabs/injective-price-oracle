package oracle

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"goa.design/goa/v3/security"

	log "github.com/InjectiveLabs/suplog"

	injectivepriceoracleapi "github.com/InjectiveLabs/injective-price-oracle/api/gen/injective_price_oracle_api"
)

type apiSvc struct {
	APIKey string
}

type APIService interface {
	APIKeyAuth(ctx context.Context, key string, schema *security.APIKeyScheme) (context.Context, error)
	Probe(ctx context.Context, payload *injectivepriceoracleapi.ProbePayload) (res *injectivepriceoracleapi.ProbeResponse, err error)
}

func NewAPIService(APIKey string) APIService {
	return &apiSvc{
		APIKey: APIKey,
	}
}

// APIKeyAuth verifies the API key sent by the client
func (s *apiSvc) APIKeyAuth(ctx context.Context, key string, _ *security.APIKeyScheme) (context.Context, error) {
	if s.APIKey != key {
		return nil, injectivepriceoracleapi.MakeUnauthorized(fmt.Errorf("invalid API key"))
	}

	return ctx, nil
}

// Probe validates the dynamic feed config and attempts to pull price once
func (s *apiSvc) Probe(ctx context.Context, payload *injectivepriceoracleapi.ProbePayload) (res *injectivepriceoracleapi.ProbeResponse, err error) {
	feedCfg, err := ParseDynamicFeedConfig(payload.Content)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"payload": payload.Content,
		}).Errorln("failed to parse dynamic feed config")
		return nil, injectivepriceoracleapi.MakeInternal(fmt.Errorf("failed to parse dynamic feed config: %w", err))
	}

	if err = validateFeedConfig(feedCfg); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"feed_config": feedCfg,
		}).Errorln("invalid feed config")
		return nil, injectivepriceoracleapi.MakeInternal(fmt.Errorf("invalid feed config: %w", err))
	}

	pricePuller, err := NewDynamicPriceFeed(feedCfg)
	if err != nil {
		log.WithError(err).Errorln("failed to init new dynamic price feed")
		return nil, injectivepriceoracleapi.MakeInternal(fmt.Errorf("failed to init new dynamic price feed: %w", err))
	}

	pullerLogger := log.WithFields(log.Fields{
		"provider_name": pricePuller.ProviderName(),
		"symbol":        pricePuller.Symbol(),
		"oracle_type":   pricePuller.OracleType().String(),
	})

	answer, err := pricePuller.PullPrice(ctx)
	if err != nil {
		pullerLogger.WithError(err).Errorln("failed to pull price")
		return nil, injectivepriceoracleapi.MakeInternal(fmt.Errorf("failed to pull price: %w", err))
	}

	return &injectivepriceoracleapi.ProbeResponse{
		Result: answer.Price.String(),
	}, nil
}

func validateFeedConfig(feedCfg *FeedConfig) error {
	if feedCfg == nil {
		return errors.New("feed config is nil")
	}

	if feedCfg.ProviderName == "" {
		return errors.New("provider name is empty in feed config")
	}

	if feedCfg.Ticker == "" {
		return errors.New("ticker is empty in feed config")
	}

	if feedCfg.ObservationSource == "" {
		return errors.New("observation source is empty in feed config")
	}

	if feedCfg.PullInterval == "" {
		return errors.New("pull interval is empty in feed config")
	}

	return nil
}
