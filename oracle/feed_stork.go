package oracle

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/InjectiveLabs/metrics"
	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"
	log "github.com/InjectiveLabs/suplog"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"
)

var _ PricePuller = &storkPriceFeed{}

type storkPriceFeed struct {
	storkFetcher StorkFetcher
	providerName string
	ticker       string
	tickers      []string
	interval     time.Duration

	logger  log.Logger
	svcTags metrics.Tags

	oracleType oracletypes.OracleType
}

func ParseStorkFeedConfig(body []byte) (*FeedConfig, error) {
	var config FeedConfig
	if err := toml.Unmarshal(body, &config); err != nil {
		err = errors.Wrap(err, "failed to unmarshal TOML config")
		return nil, err
	}

	return &config, nil
}

// NewStorkPriceFeed returns price puller
func NewStorkPriceFeed(storkFetcher StorkFetcher, cfg *FeedConfig) (PricePuller, error) {
	pullInterval := 1 * time.Minute
	if len(cfg.PullInterval) > 0 {
		interval, err := time.ParseDuration(cfg.PullInterval)
		if err != nil {
			err = errors.Wrapf(err, "failed to parse pull interval: %s (expected format: 60s)", cfg.PullInterval)
			return nil, err
		}

		if interval < time.Second {
			err = errors.Wrapf(err, "failed to parse pull interval: %s (minimum interval = 30s)", cfg.PullInterval)
			return nil, err
		}

		pullInterval = interval
	}

	var oracleType oracletypes.OracleType
	if cfg.OracleType == "" {
		oracleType = oracletypes.OracleType_Stork
	} else {
		tmpType, exist := oracletypes.OracleType_value[cfg.OracleType]
		if !exist {
			return nil, fmt.Errorf("oracle type does not exist: %s", cfg.OracleType)
		}

		oracleType = oracletypes.OracleType(tmpType)
	}

	feed := &storkPriceFeed{
		storkFetcher: storkFetcher,
		providerName: cfg.ProviderName,
		ticker:       cfg.Ticker,
		interval:     pullInterval,
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

func (f *storkPriceFeed) AssetPair() *oracletypes.AssetPair {
	return f.storkFetcher.AssetPair(f.ticker)
}

func (f *storkPriceFeed) PullPrice(_ context.Context) (
	priceData *PriceData,
	err error,
) {
	pair := f.storkFetcher.AssetPair(f.ticker)
	if pair == nil || len(pair.SignedPrices) == 0 {
		return nil, nil
	}

	return &PriceData{
		Ticker:       Ticker(f.ticker),
		ProviderName: f.ProviderName(),
		Symbol:       f.Symbol(),
		AssetPair:    pair,
		Timestamp:    time.Now(),
		OracleType:   f.OracleType(),
	}, nil
}

// ConvertDataToAssetPair converts data get from websocket to list of asset pairs
func ConvertDataToAssetPair(data Data, assetId string, refTimestamp uint64) (result oracletypes.AssetPair) {
	var signedPricesOfAssetPair []*oracletypes.SignedPriceOfAssetPair
	for i := range data.SignedPrices {
		newSignedPriceAssetPair := ConvertSignedPrice(data.SignedPrices[i])
		if ConvertTimestampToSecond(refTimestamp) != newSignedPriceAssetPair.Timestamp {
			log.Warningf("timestamp mismatch: %d != %d", ConvertTimestampToSecond(refTimestamp), newSignedPriceAssetPair.Timestamp)
			continue
		}
		signedPricesOfAssetPair = append(signedPricesOfAssetPair, &newSignedPriceAssetPair)
	}
	result.SignedPrices = signedPricesOfAssetPair
	result.AssetId = assetId

	return result
}

// ConvertSignedPrice converts signed price to SignedPriceOfAssetPair of Stork
func ConvertSignedPrice(signeds SignedPrice) oracletypes.SignedPriceOfAssetPair {
	var signedPriceOfAssetPair oracletypes.SignedPriceOfAssetPair

	signature := CombineSignatureToString(signeds.TimestampedSignature.Signature)

	signedPriceOfAssetPair.Signature = common.Hex2Bytes(signature)
	signedPriceOfAssetPair.PublisherKey = signeds.PublisherKey
	signedPriceOfAssetPair.Timestamp = ConvertTimestampToSecond(signeds.TimestampedSignature.Timestamp)
	signedPriceOfAssetPair.Price = signeds.Price

	return signedPriceOfAssetPair
}

func ConvertTimestampToSecond(timestamp uint64) uint64 {
	switch {
	// nanosecond
	case timestamp > 1e18:
		return timestamp / uint64(1_000_000_000)
	// microsecond
	case timestamp > 1e15:
		return timestamp / uint64(1_000_000)
	// millisecond
	case timestamp > 1e12:
		return timestamp / uint64(1_000)
	// second
	default:
		return timestamp
	}
}

// CombineSignatureToString combines a signature to a string
func CombineSignatureToString(signature Signature) (result string) {
	prunedR := strings.TrimPrefix(signature.R, "0x")
	prunedS := strings.TrimPrefix(signature.S, "0x")
	prunedV := strings.TrimPrefix(signature.V, "0x")

	return prunedR + prunedS + prunedV
}
