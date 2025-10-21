package oracle

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	log "github.com/InjectiveLabs/suplog"

	injectivepriceoracleapi "github.com/InjectiveLabs/injective-price-oracle/api/gen/injective_price_oracle_api"
)

type apiSvc struct {
}

type APIService interface {
	Probe(ctx context.Context, payload *injectivepriceoracleapi.ProbePayload) (res *injectivepriceoracleapi.ProbeResponse, err error)
}

func NewAPIService() APIService {
	return &apiSvc{}
}

func (s *apiSvc) Probe(ctx context.Context, payload *injectivepriceoracleapi.ProbePayload) (res *injectivepriceoracleapi.ProbeResponse, err error) {
	feedCfg, err := ParseDynamicFeedConfig(payload.Content)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"payload": payload.Content,
		}).Errorln("failed to parse dynamic feed config")
		return nil, fmt.Errorf("failed to parse dynamic feed config: %w", err)
	}

	if err = validateFeedConfig(feedCfg); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"feed_config": feedCfg,
		}).Errorln("invalid feed config")
		return nil, fmt.Errorf("invalid feed config: %w", err)
	}

	pricePuller, err := NewDynamicPriceFeed(feedCfg)
	if err != nil {
		log.WithError(err).Fatalln("failed to init new dynamic price feed")
		return nil, fmt.Errorf("failed to init new dynamic price feed: %w", err)
	}

	pullerLogger := log.WithFields(log.Fields{
		"provider_name": pricePuller.ProviderName(),
		"symbol":        pricePuller.Symbol(),
		"oracle_type":   pricePuller.OracleType().String(),
	})

	answer, err := pricePuller.PullPrice(ctx)
	if err != nil {
		pullerLogger.WithError(err).Errorln("failed to pull price")
		return nil, fmt.Errorf("failed to pull price: %w", err)
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
