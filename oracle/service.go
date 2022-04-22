package oracle

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	cosmtypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	log "github.com/xlab/suplog"

	chainclient "github.com/InjectiveLabs/sdk-go/chain/client"
	exchangetypes "github.com/InjectiveLabs/sdk-go/chain/exchange/types"
	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"

	"github.com/InjectiveLabs/metrics"
)

type Service interface {
	Start() error
	Close()
}

type PricePuller interface {
	Provider() FeedProvider
	ProviderName() string
	Symbol() string
	Interval() time.Duration

	// PullPrice method must be implemented in order to get a price
	// from external source, handled by PricePuller.
	PullPrice(ctx context.Context) (price decimal.Decimal, err error)
}

type MultiPricePuller interface {
	PricePuller

	AddSymbol(symbol string)
	Symbols() []string

	// PullPrices is a method that allows to pull multiple prices in a single batch.
	PullPrices(ctx context.Context) (prices map[string]decimal.Decimal, err error)
}

type oracleSvc struct {
	pricePullers        map[string]PricePuller
	supportedPriceFeeds map[string]PriceFeedConfig
	feedProviderConfigs map[FeedProvider]interface{}
	cosmosClient        chainclient.CosmosClient
	exchangeQueryClient exchangetypes.QueryClient
	oracleQueryClient   oracletypes.QueryClient

	logger  log.Logger
	svcTags metrics.Tags
}

const (
	maxRespTime           = 15 * time.Second
	maxRespHeadersTime    = 15 * time.Second
	maxRespBytes          = 10 * 1024 * 1024
	maxTxStatusRetries    = 3
	maxRetriesPerInterval = 3
)

var (
	zeroPrice = decimal.Decimal{}
)

type FeedProvider string

const (
	FeedProviderDynamic FeedProvider = "_"
	FeedProviderBinance FeedProvider = "binance"

	// TODO: add your native implementations here
)

type PriceFeedConfig struct {
	Symbol        string
	FeedProvider  FeedProvider
	PullInterval  time.Duration
	DynamicConfig *DynamicFeedConfig
}

// getEnabledFeeds returns a mapping between ticker and price feeder config, this will query
// the chain to fetch only those configs where current Cosmos sender is authorized as a relayer.
func (s *oracleSvc) getEnabledFeeds() map[string]PriceFeedConfig {
	metrics.ReportFuncCall(s.svcTags)
	doneFn := metrics.ReportFuncTiming(s.svcTags)
	defer doneFn()

	ctx, cancelFn := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancelFn()

	feeds := make(map[string]PriceFeedConfig)

	sender := s.cosmosClient.FromAddress().String()
	res, err := s.oracleQueryClient.PriceFeedPriceStates(ctx, &oracletypes.QueryPriceFeedPriceStatesRequest{})
	if err != nil {
		metrics.ReportFuncError(s.svcTags)
		return nil
	}

	for _, priceFeedState := range res.PriceStates {
		var found bool
		for _, relayer := range priceFeedState.Relayers {
			if strings.ToLower(relayer) == strings.ToLower(sender) {
				found = true
			}
		}

		if !found {
			continue
		}

		ticker := fmt.Sprintf("%s/%s", priceFeedState.Base, priceFeedState.Quote)
		if feed, ok := s.supportedPriceFeeds[ticker]; ok {
			feeds[ticker] = feed
		} else {
			s.logger.WithFields(log.Fields{
				"sender": strings.ToLower(sender),
			}).Warningf("current sender is authorized in %s feed, but no corresponding feed config loaded", ticker)
		}
	}

	s.logger.Infof("got %d enabled price feeds", len(feeds))

	return feeds
}

func NewService(
	cosmosClient chainclient.CosmosClient,
	exchangeQueryClient exchangetypes.QueryClient,
	oracleQueryClient oracletypes.QueryClient,
	feedProviderConfigs map[FeedProvider]interface{},
	dynamicFeedConfigs []*DynamicFeedConfig,
) (Service, error) {
	svc := &oracleSvc{
		cosmosClient:        cosmosClient,
		exchangeQueryClient: exchangeQueryClient,
		oracleQueryClient:   oracleQueryClient,

		feedProviderConfigs: feedProviderConfigs,
		logger:              log.WithField("svc", "oracle"),
		svcTags: metrics.Tags{
			"svc": "price_oracle",
		},
	}

	// supportedPriceFeeds is a mapping between price ticker and
	// price feed config for a specific symbol.
	svc.supportedPriceFeeds = map[string]PriceFeedConfig{
		// Example hardcoded config
		//
		// "BNB/USDT":   {Symbol: "BNBUSDT", FeedProvider: FeedProviderBinance, PullInterval: 1 * time.Minute},
	}

	for _, feedCfg := range dynamicFeedConfigs {
		svc.supportedPriceFeeds[feedCfg.Ticker] = PriceFeedConfig{
			FeedProvider:  FeedProviderDynamic,
			DynamicConfig: feedCfg,
		}
	}

	enabledFeeds := svc.getEnabledFeeds()
	svc.pricePullers = make(map[string]PricePuller, len(enabledFeeds))

	for ticker, feedCfg := range enabledFeeds {
		tickerLog := svc.logger.WithField("ticker", ticker)

		switch feedCfg.FeedProvider {
		case FeedProviderBinance:
			tickerLog.WithField("ticker", ticker).Errorln("binance native implementation is not enabled")
			continue

			// Example usage of initializing native implementation with passing a config:
			//
			// svc.pricePullers[ticker] = NewBinancePriceFeed(
			// 	feedCfg.Symbol,
			// 	feedCfg.PullInterval,
			// 	svc.feedProviderConfigs[FeedProviderBinance].(*BinanceEndpointConfig),
			// )

		case FeedProviderDynamic:
			pricePuller, err := NewDynamicPriceFeed(feedCfg.DynamicConfig)
			if err != nil {
				err = errors.Wrapf(err, "failed to init dynamic price feed for ticker %s", ticker)
				return nil, err
			}
			svc.pricePullers[ticker] = pricePuller

		default:
			tickerLog.WithField("ticker", ticker).Warningf("ticker has no supported feed provider: %s", feedCfg.FeedProvider)
			continue
		}
	}

	svc.logger.Infof("initialized %d price pullers", len(svc.pricePullers))

	return svc, nil
}

func (s *oracleSvc) Start() (err error) {
	defer s.panicRecover(&err)

	if len(s.pricePullers) > 0 {
		s.logger.Infoln("starting pullers for", len(s.pricePullers), "feeds")

		dataC := make(chan *PriceData, len(s.pricePullers))

		for ticker, pricePuller := range s.pricePullers {
			switch pricePuller.Provider() {
			case FeedProviderBinance,
				FeedProviderDynamic:

				go s.processSetPriceFeed(ticker, pricePuller.ProviderName(), pricePuller, dataC)

			default:
				s.logger.WithField("provider", pricePuller.Provider()).Warningln("unsupported price feed provider")
			}
		}

		s.commitSetPrices(dataC)
	}

	return
}

func (s *oracleSvc) processSetPriceFeed(ticker, providerName string, pricePuller PricePuller, dataC chan<- *PriceData) {
	feedLogger := s.logger.WithFields(log.Fields{
		"ticker":   ticker,
		"provider": pricePuller.ProviderName(),
	})

	ctx := context.Background()
	symbol := pricePuller.Symbol()

	t := time.NewTimer(5 * time.Second)
	for {
		select {
		case <-t.C:
			ctx, cancelFn := context.WithTimeout(ctx, maxRespTime)
			defer cancelFn()

			result, err := pricePuller.PullPrice(ctx)
			if err != nil {
				metrics.ReportFuncError(s.svcTags)
				feedLogger.WithError(err).Warningln("retrying PullPrice after error")

				for i := 0; i < maxRetriesPerInterval; i++ {
					if result, err = pricePuller.PullPrice(ctx); err != nil {
						time.Sleep(time.Second)
						continue
					}
					break
				}

				if err != nil {
					metrics.ReportFuncError(s.svcTags)
					feedLogger.WithFields(log.Fields{
						"symbol":  symbol,
						"retries": maxRetriesPerInterval,
					}).WithError(err).Errorln("failed to fetch price")

					t.Reset(pricePuller.Interval())
					continue
				}
			}

			dataC <- &PriceData{
				Ticker:    Ticker(ticker),
				Symbol:    symbol,
				Timestamp: time.Now().UTC(),
				Price:     result,
			}

			t.Reset(pricePuller.Interval())
		}
	}
}

const (
	commitPriceBatchTimeLimit = 5 * time.Second
	commitPriceBatchSizeLimit = 100
)

func (s *oracleSvc) commitSetPrices(dataC <-chan *PriceData) {
	expirationTimer := time.NewTimer(commitPriceBatchTimeLimit)
	pricesBatch := make([]*PriceData, 0, commitPriceBatchSizeLimit)

	resetBatch := func() []*PriceData {
		expirationTimer.Reset(commitPriceBatchTimeLimit)

		prev := pricesBatch
		pricesBatch = make([]*PriceData, 0, commitPriceBatchSizeLimit)
		return prev
	}

	submitBatch := func(currentBatch []*PriceData, timeout bool) {
		if len(currentBatch) == 0 {
			return
		}

		batchLog := s.logger.WithFields(log.Fields{
			"batch_size": len(currentBatch),
			"timeout":    timeout,
		})

		msg := &oracletypes.MsgRelayPriceFeedPrice{
			Sender: s.cosmosClient.FromAddress().String(),
			Base:   make([]string, 0, len(currentBatch)),
			Quote:  make([]string, 0, len(currentBatch)),
			Price:  make([]cosmtypes.Dec, 0, len(currentBatch)),
		}

		for _, priceData := range currentBatch {
			msg.Base = append(msg.Base, priceData.Ticker.Base())
			msg.Quote = append(msg.Quote, priceData.Ticker.Quote())
			msg.Price = append(msg.Price, cosmtypes.MustNewDecFromStr(priceData.Price.String()))
		}

		ts := time.Now()
		txResp, err := s.cosmosClient.SyncBroadcastMsg(msg)
		if err != nil {
			metrics.ReportFuncError(s.svcTags)
			batchLog.WithError(err).Errorln("failed to SyncBroadcastMsg")
			return
		}

		if txResp.Code != 0 {
			batchLog.WithFields(log.Fields{
				"hash":     txResp.TxHash,
				"err_code": txResp.Code,
			}).Errorf("set price Tx error: %s", txResp.String())

			return
		}

		batchLog.WithField("hash", txResp.TxHash).Infoln("sent Tx in", time.Since(ts))
	}

	for {
		select {
		case priceData, ok := <-dataC:
			if !ok {
				s.logger.Infoln("stopping committing prices")
				prevBatch := resetBatch()
				submitBatch(prevBatch, false)
				return
			}

			if priceData.Price.IsZero() || priceData.Price.IsNegative() {
				s.logger.WithFields(log.Fields{
					"ticker":   priceData.Ticker,
					"provider": priceData.ProviderName,
				}).Errorln("got negative or zero price, skipping")
				continue
			}

			pricesBatch = append(pricesBatch, priceData)

			if len(pricesBatch) >= commitPriceBatchSizeLimit {
				prevBatch := resetBatch()
				submitBatch(prevBatch, false)

			}
		case <-expirationTimer.C:
			prevBatch := resetBatch()
			submitBatch(prevBatch, true)
		}
	}
}

func (s *oracleSvc) panicRecover(err *error) {
	if r := recover(); r != nil {
		*err = errors.Errorf("%v", r)

		if e, ok := r.(error); ok {
			s.logger.WithError(e).Errorln("service main loop panicked with an error")
			s.logger.Debugln(string(debug.Stack()))
		} else {
			s.logger.Errorln(r)
		}
	}
}

func (s *oracleSvc) Close() {
	// TODO: graceful shutdown if needed
}
