package oracle

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/InjectiveLabs/metrics"
	exchangetypes "github.com/InjectiveLabs/sdk-go/chain/exchange/types/v2"
	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"
	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	log "github.com/InjectiveLabs/suplog"
	cosmtypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"

	"github.com/InjectiveLabs/injective-price-oracle/internal/service/oracle/chainlink"
	"github.com/InjectiveLabs/injective-price-oracle/internal/service/oracle/stork"
	"github.com/InjectiveLabs/injective-price-oracle/internal/service/oracle/types"
)

type Service interface {
	Start(ctx context.Context) error
	Close()
}

type oracleSvc struct {
	pricePullers        map[string]types.PricePuller
	supportedPriceFeeds map[string]PriceFeedConfig
	cosmosClients       []chainclient.ChainClient
	exchangeQueryClient exchangetypes.QueryClient
	oracleQueryClient   oracletypes.QueryClient

	logger  log.Logger
	svcTags metrics.Tags
}

const (
	maxRespTime                  = 15 * time.Second
	maxRetriesPerInterval        = 3
	MaxRetriesReConnectWebSocket = 5
)

type PriceFeedConfig struct {
	Symbol        string
	FeedProvider  types.FeedProvider
	PullInterval  time.Duration
	DynamicConfig *types.FeedConfig
}

func NewService(
	_ context.Context,
	cosmosClients []chainclient.ChainClient,
	feedConfigs map[string]*types.FeedConfig,
	storkFetcher stork.Fetcher,
	chainlinkFetcher chainlink.Fetcher,
) (Service, error) {
	svc := &oracleSvc{
		cosmosClients: cosmosClients,
		logger:        log.WithField("svc", "oracle"),
		svcTags: metrics.Tags{
			"svc": "price_oracle",
		},
	}

	svc.pricePullers = map[string]types.PricePuller{}
	for _, feedCfg := range feedConfigs {
		switch feedCfg.ProviderName {
		case types.FeedProviderStork.String():
			ticker := feedCfg.Ticker
			pricePuller, err := stork.NewStorkPriceFeed(storkFetcher, feedCfg)
			if err != nil {
				err = errors.Wrapf(err, "failed to init stork price feed for ticker %s", ticker)
				return nil, err
			}
			svc.pricePullers[ticker] = pricePuller
		case types.FeedProviderChainlink.String():
			ticker := feedCfg.Ticker
			pricePuller, err := chainlink.NewChainlinkPriceFeed(chainlinkFetcher, feedCfg)
			if err != nil {
				err = errors.Wrapf(err, "failed to init chainlink price feed for ticker %s", ticker)
				return nil, err
			}
			svc.pricePullers[ticker] = pricePuller
		default:
			// Unsupported provider
			svc.logger.WithField("provider", feedCfg.ProviderName).Warningln("unsupported feed provider, skipping")
		}
	}

	svc.logger.Infof("initialized %d price pullers", len(svc.pricePullers))
	return svc, nil
}

func (s *oracleSvc) Start(ctx context.Context) (err error) {
	defer s.panicRecover(&err)

	if len(s.pricePullers) > 0 {
		s.logger.Infoln("starting pullers for", len(s.pricePullers), "feeds")

		dataC := make(chan types.PriceData, len(s.pricePullers))

		for ticker, pricePuller := range s.pricePullers {
			switch pricePuller.Provider() {
			case types.FeedProviderStork, types.FeedProviderChainlink:
				go s.processSetPriceFeed(ticker, pricePuller, dataC)
			default:
				s.logger.WithField("provider", pricePuller.Provider()).Warningln("unsupported price feed provider")
			}
		}

		s.commitSetPrices(ctx, dataC)
	}

	return
}

func (s *oracleSvc) processSetPriceFeed(ctx context.Context, ticker string, pricePuller types.PricePuller, dataC chan<- types.PriceData) {
	feedLogger := s.logger.WithFields(log.Fields{
		"ticker":   ticker,
		"provider": pricePuller.ProviderName(),
	})

	symbol := pricePuller.Symbol()

	t := time.NewTimer(5 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			feedLogger.Infoln("context cancelled, stopping price feed")
			return
		case <-t.C:
			var result *PriceData
			var err error

			for i := 0; i < maxRetriesPerInterval; i++ {
				requestCtx, cancelFn := context.WithTimeout(ctx, maxRespTime)
				result, err = pricePuller.PullPrice(requestCtx)
				cancelFn()

				if err == nil {
					break
				}

				time.Sleep(100 * time.Millisecond)
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

			if result != nil {
				dataC <- result
			}

			t.Reset(pricePuller.Interval())
		}
	}
}

const (
	commitPriceBatchTimeLimit = 5 * time.Second
	chainMaxTimeLimit         = 3 * time.Second
	commitPriceBatchSizeLimit = 100
	maxRetries                = 3
)

var pullIntervalChain = 500 * time.Millisecond

func composeStorkOracleMsgs(cosmosClient chainclient.ChainClient, priceBatch []types.PriceData) (result []cosmtypes.Msg) {
	if len(priceBatch) == 0 {
		return nil
	}

	assetPairs := make([]*oracletypes.AssetPair, 0)
	for _, pData := range priceBatch {
		if pData.GetOracleType() != oracletypes.OracleType_Stork {
			continue
		}

		if chainlinkData, ok := pData.(*stork.StorkPriceData); ok {
			assetPair := chainlinkData.AssetPair
			assetPairs = append(assetPairs, assetPair)
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

func composeChainlinkOracleMsgs(cosmosClient chainclient.ChainClient, priceBatch []types.PriceData) (result []cosmtypes.Msg) {
	if len(priceBatch) == 0 {
		return nil
	}

	reports := make([]*oracletypes.ChainlinkReport, 0)
	for _, pData := range priceBatch {
		if pData.GetOracleType() != oracletypes.OracleType_Chainlink {
			continue
		}

		if chainlinkData, ok := pData.(*chainlink.ChainlinkPriceData); ok {
			report := chainlinkData.ChainlinkReport
			reports = append(reports, report)
		}
	}

	if len(reports) > 0 {
		msg := &oracletypes.MsgRelayChainlinkPrices{
			Sender:  cosmosClient.FromAddress().String(),
			Reports: reports,
		}

		log.Debugf("chainlink reports: %d reports", len(reports))
		result = append(result, msg)
	}

	return result
}

func composeMsgs(cosmoClient chainclient.ChainClient, priceBatch []types.PriceData) (result []cosmtypes.Msg) {
	result = append(result, composeStorkOracleMsgs(cosmoClient, priceBatch)...)
	result = append(result, composeChainlinkOracleMsgs(cosmoClient, priceBatch)...)
	return result
}

func (s *oracleSvc) commitSetPrices(ctx context.Context, dataC <-chan types.PriceData) {
	metrics.ReportFuncCall(s.svcTags)
	doneFn := metrics.ReportFuncTiming(s.svcTags)
	defer doneFn()

	expirationTimer := time.NewTimer(commitPriceBatchTimeLimit)
	defer expirationTimer.Stop()

	pricesBatch := make(map[string]*PriceData)
	pricesMeta := make(map[string]int)

	resetBatch := func() (map[string]types.PriceData, map[string]int) {
		expirationTimer.Reset(commitPriceBatchTimeLimit)

		prev := pricesBatch
		prevMeta := pricesMeta
		pricesBatch = make(map[string]types.PriceData)
		pricesMeta = make(map[string]int)
		return prev, prevMeta
	}

	submitBatch := func(currentBatch map[string]types.PriceData, currentMeta map[string]int, timeout bool) {
		if len(currentBatch) == 0 {
			return
		}

		batchLog := s.logger.WithFields(log.Fields{
			"batch_size": len(currentBatch),
			"timeout":    timeout,
		})

		var priceBatch []types.PriceData
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

			if success := s.broadcastToClient(ctx, cosmosClient, msgs, currentMeta, pullIntervalChain, maxRetries, batchLog); success {
				return
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.Infoln("context cancelled, stopping commitSetPrices")
			prevBatch, prevMeta := resetBatch()
			submitBatch(prevBatch, prevMeta, false)
			return
		case priceData, ok := <-dataC:
			if !ok {
				s.logger.Infoln("stopping committing prices")
				prevBatch, prevMeta := resetBatch()
				submitBatch(prevBatch, prevMeta, false)
				return
			}

			// Validate based on oracle type
			if priceData.GetOracleType() == oracletypes.OracleType_Stork {
				if storkData, ok := priceData.(*stork.StorkPriceData); ok {
					if storkData.AssetPair == nil {
						s.logger.WithFields(log.Fields{
							"ticker":   priceData.GetTicker(),
							"provider": priceData.GetProviderName(),
						}).Debugln("got nil asset pair for stork oracle, skipping")
						continue
					}
				}
			} else if priceData.GetOracleType() == oracletypes.OracleType_Chainlink {
				if chainlinkData, ok := priceData.(*chainlink.ChainlinkPriceData); ok {
					if chainlinkData.ChainlinkReport.FeedId == nil || chainlinkData.ChainlinkReport == nil {
						s.logger.WithFields(log.Fields{
							"ticker":   priceData.GetTicker(),
							"provider": priceData.GetProviderName(),
						}).Debugln("got invalid chainlink report data, skipping")
						continue
					}
				}
			} else {
				// For other oracle types, validate price
				if priceData.GetPrice().IsZero() || priceData.GetPrice().IsNegative() {
					s.logger.WithFields(log.Fields{
						"ticker":   priceData.GetTicker(),
						"provider": priceData.GetProviderName(),
					}).Debugln("got negative or zero price, skipping")
					continue
				}
			}

			pricesMeta[priceData.GetOracleType().String()]++
			pricesBatch[priceData.GetOracleType().String()+":"+priceData.GetSymbol()] = priceData

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
	ctx context.Context,
	cosmosClient chainclient.ChainClient,
	msgs []cosmtypes.Msg,
	currentMeta map[string]int,
	pullIntervalChain time.Duration,
	maxRetries uint32,
	batchLog log.Logger,
) bool {
	ts := time.Now()
	requestCtx, cancelFn := context.WithTimeout(ctx, chainMaxTimeLimit)
	defer cancelFn()

	txResp, err := cosmosClient.SyncBroadcastMsgWithContext(requestCtx, &pullIntervalChain, maxRetries, msgs...)
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

		diff := time.Since(ts)

		batchLog.WithFields(log.Fields{
			"cosmosClient": cosmosClient.ClientContext().From,
			"height":       txResp.TxResponse.Height,
			"hash":         txResp.TxResponse.TxHash,
			"duration":     diff,
		}).Infoln("sent Tx successfully in ", diff)

		metrics.Timer("price_oracle.execution_time", diff, s.svcTags)
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
