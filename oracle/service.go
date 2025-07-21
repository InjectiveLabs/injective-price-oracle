package oracle

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"cosmossdk.io/math"

	log "github.com/InjectiveLabs/suplog"
	cosmtypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"

	"github.com/InjectiveLabs/metrics"
	exchangetypes "github.com/InjectiveLabs/sdk-go/chain/exchange/types/v2"
	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"
	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
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
	PullPrice(ctx context.Context) (priceData *PriceData, err error)
	OracleType() oracletypes.OracleType
}

type FeedConfig struct {
	ProviderName      string `toml:"provider"`
	Ticker            string `toml:"ticker"`
	PullInterval      string `toml:"pullInterval"`
	ObservationSource string `toml:"observationSource"`
	OracleType        string `toml:"oracleType"`
}

type oracleSvc struct {
	pricePullers        map[string]PricePuller
	supportedPriceFeeds map[string]PriceFeedConfig
	cosmosClients       []chainclient.ChainClientV2
	exchangeQueryClient exchangetypes.QueryClient
	oracleQueryClient   oracletypes.QueryClient
	config              *StorkConfig

	logger  log.Logger
	svcTags metrics.Tags
}

const (
	maxRespTime                  = 15 * time.Second
	maxRespHeadersTime           = 15 * time.Second
	maxRespBytes                 = 10 * 1024 * 1024
	maxTxStatusRetries           = 3
	maxRetriesPerInterval        = 3
	MaxRetriesReConnectWebSocket = 5
)

var (
	zeroPrice = decimal.Decimal{}
)

type FeedProvider string

func (f FeedProvider) String() string {
	return string(f)
}

const (
	FeedProviderDynamic FeedProvider = "_"
	FeedProviderBinance FeedProvider = "binance"
	FeedProviderStork   FeedProvider = "stork"

	// TODO: add your native implementations here
)

type PriceFeedConfig struct {
	Symbol        string
	FeedProvider  FeedProvider
	PullInterval  time.Duration
	DynamicConfig *FeedConfig
}

// getEnabledFeeds returns a mapping between ticker and price feeder config, this will query
// the chain to fetch only those configs where current Cosmos sender is authorized as a relayer.
func (s *oracleSvc) getEnabledFeeds(cosmosClient chainclient.ChainClient) map[string]PriceFeedConfig {
	metrics.ReportFuncCall(s.svcTags)
	doneFn := metrics.ReportFuncTiming(s.svcTags)
	defer doneFn()

	ctx, cancelFn := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancelFn()

	feeds := make(map[string]PriceFeedConfig)

	sender := strings.ToLower(cosmosClient.FromAddress().String())
	res, err := s.oracleQueryClient.PriceFeedPriceStates(ctx, &oracletypes.QueryPriceFeedPriceStatesRequest{})
	if err != nil {
		metrics.ReportFuncError(s.svcTags)
		return nil
	}

	for _, priceFeedState := range res.PriceStates {
		var found bool
		for _, relayer := range priceFeedState.Relayers {
			if strings.ToLower(relayer) == sender {
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
				"sender": sender,
			}).Warningf("current sender is authorized in %s feed, but no corresponding feed config loaded", ticker)
		}
	}

	s.logger.Infof("got %d enabled price feeds", len(feeds))

	return feeds
}

func NewService(
	_ context.Context,
	cosmosClients []chainclient.ChainClientV2,
	feedConfigs map[string]*FeedConfig,
	storkFetcher StorkFetcher,
) (Service, error) {
	svc := &oracleSvc{
		cosmosClients: cosmosClients,
		logger:        log.WithField("svc", "oracle"),
		svcTags: metrics.Tags{
			"svc": "price_oracle",
		},
	}

	// supportedPriceFeeds is a mapping between price ticker and its pricefeed config
	svc.supportedPriceFeeds = map[string]PriceFeedConfig{}
	for _, feedCfg := range feedConfigs {
		svc.supportedPriceFeeds[feedCfg.Ticker] = PriceFeedConfig{
			FeedProvider:  FeedProviderDynamic,
			DynamicConfig: feedCfg,
		}
	}

	svc.pricePullers = map[string]PricePuller{}
	for _, feedCfg := range feedConfigs {
		switch feedCfg.ProviderName {
		case FeedProviderStork.String():
			ticker := feedCfg.Ticker
			pricePuller, err := NewStorkPriceFeed(storkFetcher, feedCfg)
			if err != nil {
				err = errors.Wrapf(err, "failed to init stork price feed for ticker %s", ticker)
				return nil, err
			}
			svc.pricePullers[ticker] = pricePuller
		default: // TODO this should be replaced with correct providers
			ticker := feedCfg.Ticker
			pricePuller, err := NewDynamicPriceFeed(feedCfg)
			if err != nil {
				err = errors.Wrapf(err, "failed to init dynamic price feed for ticker %s", ticker)
				return nil, err
			}
			svc.pricePullers[ticker] = pricePuller
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
			case FeedProviderBinance, FeedProviderStork, FeedProviderDynamic:
				go s.processSetPriceFeed(ticker, pricePuller, dataC)
			default:
				s.logger.WithField("provider", pricePuller.Provider()).Warningln("unsupported price feed provider")
			}
		}

		s.commitSetPrices(dataC)
	}

	return
}

func (s *oracleSvc) processSetPriceFeed(ticker string, pricePuller PricePuller, dataC chan<- *PriceData) {
	feedLogger := s.logger.WithFields(log.Fields{
		"ticker":   ticker,
		"provider": pricePuller.ProviderName(),
	})

	symbol := pricePuller.Symbol()

	t := time.NewTimer(5 * time.Second)
	for {
		select {
		case <-t.C:
			ctx, cancelFn := context.WithTimeout(context.Background(), maxRespTime)
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
					metrics.ReportFuncCallAndTimingWithErr(s.svcTags)(&err)
					feedLogger.WithFields(log.Fields{
						"symbol":  symbol,
						"retries": maxRetriesPerInterval,
					}).WithError(err).Errorln("failed to fetch price")

					t.Reset(pricePuller.Interval())
					continue
				}
			}

			if result != nil {
				dataC <- result
			}

			t.Reset(pricePuller.Interval())
		}
	}
}

const (
	commitPriceBatchTimeLimit = 5 * time.Second
	commitPriceBatchSizeLimit = 100
	maxRetries                = 6
	chainMaxTimeLimit         = 5 * time.Second
)

var pullIntervalChain = 500 * time.Millisecond

func composePriceFeedMsgs(cosmosClient chainclient.ChainClientV2, priceBatch []*PriceData) (results []cosmtypes.Msg) {
	msg := &oracletypes.MsgRelayPriceFeedPrice{
		Sender: cosmosClient.FromAddress().String(),
	}

	for _, priceData := range priceBatch {
		if priceData.OracleType != oracletypes.OracleType_PriceFeed {
			continue
		}

		msg.Base = append(msg.Base, priceData.Ticker.Base())
		msg.Quote = append(msg.Quote, priceData.Ticker.Quote())
		msg.Price = append(msg.Price, math.LegacyMustNewDecFromStr(priceData.Price.String()))
	}

	if len(msg.Base) > 0 {
		return []cosmtypes.Msg{msg}
	}

	return nil
}

func composeProviderFeedMsgs(cosmosClient chainclient.ChainClientV2, priceBatch []*PriceData) (result []cosmtypes.Msg) {
	if len(priceBatch) == 0 {
		return nil
	}

	providerToMsg := make(map[string]*oracletypes.MsgRelayProviderPrices)
	for _, priceData := range priceBatch {
		if priceData.OracleType != oracletypes.OracleType_Provider {
			continue
		}

		provider := strings.ToLower(priceData.ProviderName)
		msg, exist := providerToMsg[provider]
		if !exist {
			msg = &oracletypes.MsgRelayProviderPrices{
				Sender:   cosmosClient.FromAddress().String(),
				Provider: priceData.ProviderName,
			}
			providerToMsg[provider] = msg
		}

		msg.Symbols = append(msg.Symbols, priceData.Symbol)
		msg.Prices = append(msg.Prices, math.LegacyMustNewDecFromStr(priceData.Price.String()))
	}

	for _, msg := range providerToMsg {
		result = append(result, msg)
	}
	return result
}

func composeStorkOracleMsgs(cosmosClient chainclient.ChainClientV2, priceBatch []*PriceData) (result []cosmtypes.Msg) {
	if len(priceBatch) == 0 {
		return nil
	}

	assetPairs := make([]*oracletypes.AssetPair, 0)
	for _, pData := range priceBatch {
		var priceData = pData
		if priceData.OracleType != oracletypes.OracleType_Stork {
			continue
		}

		if priceData.AssetPair != nil {
			assetPairs = append(assetPairs, priceData.AssetPair)
		}
	}

	if len(assetPairs) > 0 {
		msg := &oracletypes.MsgRelayStorkPrices{
			Sender:     cosmosClient.FromAddress().String(),
			AssetPairs: assetPairs,
		}

		log.Debugf("assetPairs: %v", assetPairs)
		result = append(result, msg)
	}

	return result
}

func composeMsgs(cosmoClient chainclient.ChainClientV2, priceBatch []*PriceData) (result []cosmtypes.Msg) {
	result = append(result, composePriceFeedMsgs(cosmoClient, priceBatch)...)
	result = append(result, composeProviderFeedMsgs(cosmoClient, priceBatch)...)
	result = append(result, composeStorkOracleMsgs(cosmoClient, priceBatch)...)
	return result
}

func (s *oracleSvc) commitSetPrices(dataC <-chan *PriceData) {
	metrics.ReportFuncCall(s.svcTags)
	doneFn := metrics.ReportFuncTiming(s.svcTags)
	defer doneFn()

	expirationTimer := time.NewTimer(commitPriceBatchTimeLimit)
	pricesBatch := make(map[string]*PriceData)
	pricesMeta := make(map[string]int)

	resetBatch := func() (map[string]*PriceData, map[string]int) {
		expirationTimer.Reset(commitPriceBatchTimeLimit)

		prev := pricesBatch
		prevMeta := pricesMeta
		pricesBatch = make(map[string]*PriceData)
		pricesMeta = make(map[string]int)
		return prev, prevMeta
	}

	submitBatch := func(currentBatch map[string]*PriceData, currentMeta map[string]int, timeout bool) {
		if len(currentBatch) == 0 {
			return
		}

		batchLog := s.logger.WithFields(log.Fields{
			"batch_size": len(currentBatch),
			"timeout":    timeout,
		})

		var priceBatch []*PriceData
		for _, msg := range currentBatch {
			priceBatch = append(priceBatch, msg)
		}

		// Iterate over all cosmos clients and try to send the batch
		// if one of the clients is successful, we return
		// otherwise, we continue to the next client
		for _, cosmosClient := range s.cosmosClients {
			msgs := composeMsgs(cosmosClient, priceBatch)
			if len(msgs) == 0 {
				batchLog.WithField("client", cosmosClient.ClientContext().From).
					Debugf("pipeline composed no messages for this client")
				return
			}

			if success := s.broadcastToClient(cosmosClient, msgs, currentMeta, pullIntervalChain, maxRetries, batchLog); success {
				return
			}
		}
	}

	for {
		select {
		case priceData, ok := <-dataC:
			if !ok {
				s.logger.Infoln("stopping committing prices")
				prevBatch, prevMeta := resetBatch()
				submitBatch(prevBatch, prevMeta, false)
				return
			}
			if priceData.OracleType == oracletypes.OracleType_Stork {
				if priceData.AssetPair == nil {
					s.logger.WithFields(log.Fields{
						"ticker":   priceData.Ticker,
						"provider": priceData.ProviderName,
					}).Debugln("got nil asset pair for stork oracle, skipping")
					continue
				}
			} else {
				if priceData.Price.IsZero() || priceData.Price.IsNegative() {
					s.logger.WithFields(log.Fields{
						"ticker":   priceData.Ticker,
						"provider": priceData.ProviderName,
					}).Debugln("got negative or zero price, skipping")
					continue
				}
			}
			pricesMeta[priceData.OracleType.String()]++
			pricesBatch[priceData.OracleType.String()+":"+priceData.Symbol] = priceData

			if len(pricesBatch) >= commitPriceBatchSizeLimit {
				prevBatch, prevMeta := resetBatch()
				submitBatch(prevBatch, prevMeta, false)
			}
		case <-expirationTimer.C:
			prevBatch, prevMeta := resetBatch()
			submitBatch(prevBatch, prevMeta, true)
		}
	}
}

func (s *oracleSvc) broadcastToClient(
	cosmosClient chainclient.ChainClientV2,
	msgs []cosmtypes.Msg,
	currentMeta map[string]int,
	pullIntervalChain time.Duration,
	maxRetries uint32,
	batchLog log.Logger,
) bool {
	ts := time.Now()
	ctx, cancelFn := context.WithTimeout(context.Background(), chainMaxTimeLimit)
	defer cancelFn()

	txResp, err := cosmosClient.SyncBroadcastMsg(ctx, &pullIntervalChain, maxRetries, msgs...)
	if err != nil {
		metrics.ReportFuncError(s.svcTags)
		batchLog.WithError(err).WithField("client", cosmosClient.ClientContext().From).
			Errorln("failed to SyncBroadcastMsg")
		return false
	}

	if txResp.TxResponse != nil {
		if txResp.TxResponse.Code != 0 {
			metrics.ReportFuncError(s.svcTags)
			batchLog.WithFields(log.Fields{
				"cosmosClient": cosmosClient.ClientContext().From,
				"hash":         txResp.TxResponse.TxHash,
				"err_code":     txResp.TxResponse.Code,
			}).Errorf("set price Tx error: %s", txResp.String())
			return false
		}

		for oracleType, count := range currentMeta {
			metrics.CustomReport(func(s metrics.Statter, tagSpec []string) {
				s.Count(fmt.Sprintf("price_oracle.%s.submitted.price.size", strings.ToLower(oracleType)), int64(count), tagSpec, 1)
			}, s.svcTags)
		}

		batchLog.WithFields(log.Fields{
			"cosmosClient": cosmosClient.ClientContext().From,
			"height":       txResp.TxResponse.Height,
			"hash":         txResp.TxResponse.TxHash,
			"duration":     time.Since(ts),
		}).Infoln("sent Tx successfully")
		return true
	}

	return false
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
	// graceful shutdown if needed
}
