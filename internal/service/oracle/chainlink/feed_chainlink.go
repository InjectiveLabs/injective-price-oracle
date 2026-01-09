package chainlink

import (
	"context"
	"fmt"
	"time"

	"github.com/InjectiveLabs/metrics"
	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"
	log "github.com/InjectiveLabs/suplog"
	"github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"

	"github.com/InjectiveLabs/injective-price-oracle/internal/service/oracle/types"
)

type chainlinkPriceFeed struct {
	chainlinkFetcher ChainLinkFetcher
	providerName     string
	ticker           string
	feedID           string
	interval         time.Duration
	logger           log.Logger
	svcTags          metrics.Tags
	oracleType       oracletypes.OracleType
}

func ParseChainlinkFeedConfig(body []byte) (*types.FeedConfig, error) {
	var config types.FeedConfig
	if err := toml.Unmarshal(body, &config); err != nil {
		err = errors.Wrap(err, "failed to unmarshal TOML config")
		return nil, err
	}
	return &config, nil
}

// NewChainlinkPriceFeed returns price puller for Chainlink Data Streams
func NewChainlinkPriceFeed(chainlinkFetcher ChainLinkFetcher, cfg *types.FeedConfig) (types.PricePuller, error) {
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
		oracleType = oracletypes.OracleType_ChainlinkDataStreams
	} else {
		tmpType, exist := oracletypes.OracleType_value[cfg.OracleType]
		if !exist {
			return nil, fmt.Errorf("oracle type does not exist: %s", cfg.OracleType)
		}

		oracleType = oracletypes.OracleType(tmpType)
	}
	feed := &chainlinkPriceFeed{
		chainlinkFetcher: chainlinkFetcher,
		ticker:           cfg.Ticker,
		providerName:     cfg.ProviderName,
		feedID:           cfg.FeedID,
		interval:         pullInterval,
		oracleType:       oracleType,
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
func (f *chainlinkPriceFeed) Provider() types.FeedProvider {
	return FeedProviderChainlink
}
func (f *chainlinkPriceFeed) ProviderName() string {
	return f.providerName
}
func (f *chainlinkPriceFeed) Symbol() string {
	return f.ticker
}
func (f *chainlinkPriceFeed) Interval() time.Duration {
	return f.interval
}
func (f *chainlinkPriceFeed) OracleType() oracletypes.OracleType {
	return oracletypes.OracleType_ChainlinkDataStreams
}
func (f *chainlinkPriceFeed) GetAssetPair() *oracletypes.AssetPair {
	return nil
}

func (f *chainlinkPriceFeed) PullPrice(_ context.Context) (
	priceData types.PriceData,
	err error,
) {
	metrics.ReportFuncCallAndTimingWithErr(f.svcTags)(&err)

	ts := time.Now()

	chainlinkRepot := f.chainlinkFetcher.ChainlinkReport(f.feedID)
	if chainlinkRepot == nil {
		return nil, nil
	}

	chainlinkData := &ChainlinkPriceData{
		Ticker:          f.ticker,
		ProviderName:    f.providerName,
		Symbol:          f.ticker,
		Timestamp:       ts,
		OracleType:      f.OracleType(),
		ChainlinkReport: chainlinkRepot,
	}

	return chainlinkData, nil
}

func (f *chainlinkPriceFeed) GetFeedID() string {
	return f.feedID
}
