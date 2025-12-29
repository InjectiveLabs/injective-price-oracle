package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	"github.com/InjectiveLabs/sdk-go/client/common"
	log "github.com/InjectiveLabs/suplog"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	cosmtypes "github.com/cosmos/cosmos-sdk/types"
	cli "github.com/jawher/mow.cli"
	"github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"
	streams "github.com/smartcontractkit/data-streams-sdk/go"
	"github.com/xlab/closer"

	svcoracle "github.com/InjectiveLabs/injective-price-oracle/internal/service/oracle"
	"github.com/InjectiveLabs/injective-price-oracle/internal/service/oracle/chainlink"
	"github.com/InjectiveLabs/injective-price-oracle/internal/service/oracle/stork"
	"github.com/InjectiveLabs/injective-price-oracle/internal/service/oracle/types"
	"github.com/InjectiveLabs/injective-price-oracle/internal/service/oracle/utils"
)

type CosmosConfig struct {
	tendermintRPC    string
	cosmosGRPC       string
	cosmosStreamGRPC string
	cosmosGasPrices  string
	cosmosGasAdjust  float64
}

// oracleCmd action runs the service
//
// $ injective-price-oracle start
func oracleCmd(cmd *cli.Cmd) {
	var (
		// Cosmos params
		cosmosOverrideNetwork bool
		cosmosChainID         string
		cosmosGRPCs           []string
		cosmosStreamGRPCs     []string
		tendermintRPCs        []string
		cosmosGasPrices       string
		cosmosGasAdjust       float64
		networkNode           string

		// Cosmos Key Management
		cosmosKeyringDir     *string
		cosmosKeyringAppName *string
		cosmosKeyringBackend *string

		cosmosKeyFrom       *string
		cosmosKeyPassphrase *string
		cosmosPrivKey       *string
		cosmosUseLedger     *bool

		// External Feeds params
		feedsDir       *string
		binanceBaseURL *string

		// Metrics
		statsdPrefix   *string
		statsdAddr     *string
		statsdAgent    *string
		statsdStuckDur *string
		statsdMocking  *string
		statsdDisabled *string

		// Stork Oracle websocket params
		websocketUrl              *string
		websocketHeader           *string
		websocketSubscribeMessage *string

		// Chainlink Data Streams params
		chainlinkWsURL     *string
		chainlinkAPIKey    *string
		chainlinkAPISecret *string
	)

	initCosmosOptions(
		cmd,
		&cosmosOverrideNetwork,
		&cosmosChainID,
		&cosmosGRPCs,
		&cosmosStreamGRPCs,
		&tendermintRPCs,
		&cosmosGasPrices,
		&cosmosGasAdjust,
		&networkNode,
	)

	initCosmosKeyOptions(
		cmd,
		&cosmosKeyringDir,
		&cosmosKeyringAppName,
		&cosmosKeyringBackend,
		&cosmosKeyFrom,
		&cosmosKeyPassphrase,
		&cosmosPrivKey,
		&cosmosUseLedger,
	)

	initExternalFeedsOptions(
		cmd,
		&binanceBaseURL,
		&feedsDir,
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

	initStorkOracleWebSocketOptions(
		cmd,
		&websocketUrl,
		&websocketHeader,
		&websocketSubscribeMessage,
	)

	initChainlinkDataStreamsOptions(
		cmd,
		&chainlinkWsURL,
		&chainlinkAPIKey,
		&chainlinkAPISecret,
	)

	cmd.Action = func() {
		ctx := context.Background()
		// ensure a clean exit
		defer closer.Close()

		startMetricsGathering(
			statsdPrefix,
			statsdAddr,
			statsdAgent,
			statsdStuckDur,
			statsdMocking,
			statsdDisabled,
		)

		if *cosmosUseLedger {
			log.Fatalln("cannot really use Ledger for oracle service loop, since signatures msut be realtime")
		}

		networkNodeSplit := strings.Split(networkNode, ",")
		networkStr, node := networkNodeSplit[0], networkNodeSplit[1]
		network := common.LoadNetwork(networkStr, node)

		senderAddress, cosmosKeyring, err := chainclient.InitCosmosKeyring(
			*cosmosKeyringDir,
			*cosmosKeyringAppName,
			*cosmosKeyringBackend,
			*cosmosKeyFrom,
			*cosmosKeyPassphrase,
			*cosmosPrivKey,
			*cosmosUseLedger,
		)
		if err != nil {
			log.WithError(err).Fatalln("failed to init Cosmos keyring")
		}

		log.Infoln("using Injective Sender", senderAddress.String())
		cosmosClients := make([]chainclient.ChainClient, 0)

		if cosmosOverrideNetwork {
			for i := 0; i < len(tendermintRPCs); i++ {
				cosmosClient, err := NewCosmosClient(ctx, senderAddress, cosmosKeyring, network, &CosmosConfig{
					tendermintRPC:    tendermintRPCs[i],
					cosmosGRPC:       cosmosGRPCs[i],
					cosmosStreamGRPC: cosmosStreamGRPCs[i],
					cosmosGasPrices:  cosmosGasPrices,
					cosmosGasAdjust:  cosmosGasAdjust,
				})
				if err != nil {
					log.WithError(err).Warningln("failed to initialize cosmos client")
					continue
				}

				cosmosClients = append(cosmosClients, cosmosClient)
			}
		} else {
			cosmosClient, err := NewCosmosClient(ctx, senderAddress, cosmosKeyring, network, &CosmosConfig{
				cosmosGasPrices: cosmosGasPrices,
				cosmosGasAdjust: cosmosGasAdjust,
			})
			if err != nil {
				log.WithError(err).Fatalln("failed to initialize cosmos client")
			}

			cosmosClients = append(cosmosClients, cosmosClient)
		}

		if len(cosmosClients) == 0 {
			log.Fatalln("no cosmos clients initialized")
		}

		var storkEnabled bool
		storkMap := make(map[string]struct{})
		chainlinkMap := make(map[string]struct{})

		var chainlinkEnabled bool

		feedConfigs := make(map[string]*types.FeedConfig)

		if len(*feedsDir) > 0 {
			err := filepath.WalkDir(*feedsDir, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				} else if d.IsDir() {
					return nil
				} else if filepath.Ext(path) != ".toml" {
					return nil
				}

				cfgBody, err := os.ReadFile(path)
				if err != nil {
					err = errors.Wrapf(err, "failed to read feed config")
					return err
				}

				// First try to determine provider type by parsing as generic FeedConfig
				var genericCfg types.FeedConfig
				if err := toml.Unmarshal(cfgBody, &genericCfg); err != nil {
					log.WithError(err).WithFields(log.Fields{
						"filename": d.Name(),
					}).Errorln("failed to parse feed config")
					return nil
				}

				if genericCfg.ProviderName == types.FeedProviderStork.String() {
					storkEnabled = true
					feedCfg, err := stork.ParseStorkFeedConfig(cfgBody)
					if err != nil {
						log.WithError(err).WithFields(log.Fields{
							"filename": d.Name(),
						}).Errorln("failed to parse stork feed config")
						return nil
					}
					storkMap[feedCfg.Ticker] = struct{}{}
					feedConfigs[filepath.Base(path)] = feedCfg
				} else if genericCfg.ProviderName == types.FeedProviderChainlink.String() {
					chainlinkEnabled = true
					// Parse Chainlink specific config to extract feed IDs
					feedCfg, err := chainlink.ParseChainlinkFeedConfig(cfgBody)
					if err != nil {
						log.WithError(err).WithFields(log.Fields{
							"filename": d.Name(),
						}).Errorln("failed to parse stork feed config")
						return nil
					}
					chainlinkMap[feedCfg.FeedID] = struct{}{}
					feedConfigs[filepath.Base(path)] = feedCfg
				} else {
					// Unsupported provider
					log.WithFields(log.Fields{
						"filename": d.Name(),
						"provider": genericCfg.ProviderName,
					}).Warningln("unsupported feed provider, skipping")
				}

				return nil
			})

			if err != nil {
				err = errors.Wrapf(err, "feeds dir is specified, but failed to read from it: %s", *feedsDir)
				log.WithError(err).Fatalln("failed to load dynamic feeds")
				return
			}

			log.Infof("found %d dynamic feed configs", len(feedConfigs))
		}

		var storkFetcher stork.StorkFetcher

		if storkEnabled {
			var storkTickers []string
			for ticker := range storkMap {
				storkTickers = append(storkTickers, ticker)
			}

			storkFetcher = stork.NewFetcher(*websocketSubscribeMessage, storkTickers)
		}

		var chainlinkFetcher chainlink.ChainLinkFetcher

		if chainlinkEnabled {
			var feeds []string
			for feedID := range chainlinkMap {
				feeds = append(feeds, feedID)
			}

			// Set up the SDK client configuration
			cfg := streams.Config{
				ApiKey:    *chainlinkAPIKey,
				ApiSecret: *chainlinkAPISecret,
				WsURL:     *chainlinkWsURL,
				Logger:    streams.LogPrintf,
			}

			log.Infoln("creating Chainlink Data Streams client")
			log.Infoln("Chainlink Data Streams WS URL:", cfg.WsURL)
			log.Infoln("Chainlink Data Streams Feeds:", feeds)
			log.Infoln("Chainlink Data Streams API Key:", cfg.ApiKey)
			log.Infoln("Chainlink Data Streams API Secret:", cfg.ApiSecret)

			client, err := streams.New(cfg)
			if err != nil {
				log.WithError(err).Fatalln("failed to create Chainlink Data Streams client")
				return
			}

			fetcher, err := chainlink.NewFetcher(client, feeds)
			if err != nil {
				log.WithError(err).Fatalln("failed to create Chainlink fetcher")
			}
			chainlinkFetcher = fetcher
		}

		svc, err := svcoracle.NewService(
			ctx,
			cosmosClients,
			feedConfigs,
			storkFetcher,
			chainlinkFetcher,
		)
		if err != nil {
			log.Fatalln(err)
		}

		closer.Bind(func() {
			svc.Close()
		})

		go func() {
			if storkFetcher == nil {
				return // no stork feeds
			}
			connectIn := 0 * time.Second
			for {
				select {
				case <-ctx.Done():
					return
				case <-time.After(connectIn):
				}

				connectIn = 5 * time.Second
				conn, err := utils.ConnectWebSocket(ctx, *websocketUrl, *websocketHeader, svcoracle.MaxRetriesReConnectWebSocket)
				if err != nil {
					log.WithError(err).Errorln("failed to connect to WebSocket")
					continue
				}

				err = storkFetcher.Start(ctx, conn)
				if err != nil {
					log.WithError(err).Errorln("stork fetcher failed")
				}
			}
		}()

		go func() {
			if chainlinkFetcher == nil {
				return // no chainlink feeds
			}

			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				err := chainlinkFetcher.Start(ctx)
				if err != nil {
					log.WithError(err).Errorln("chainlink fetcher failed, retrying in 5 seconds")
					time.Sleep(5 * time.Second)
					continue
				}
			}
		}()

		go func() {
			if err := svc.Start(ctx); err != nil {
				log.Errorln(err)

				// signal there that the app failed
				os.Exit(1)
			}
		}()

		closer.Hold()
	}
}

func NewCosmosClient(ctx context.Context, senderAddress cosmtypes.AccAddress, cosmosKeyring keyring.Keyring, network common.Network, cosmosConfig *CosmosConfig) (chainclient.ChainClient, error) {
	if cosmosConfig != nil {
		if cosmosConfig.tendermintRPC != "" {
			network.TmEndpoint = cosmosConfig.tendermintRPC
		}
		if cosmosConfig.cosmosGRPC != "" {
			network.ChainGrpcEndpoint = cosmosConfig.cosmosGRPC
		}
		if cosmosConfig.cosmosStreamGRPC != "" {
			network.ChainStreamGrpcEndpoint = cosmosConfig.cosmosStreamGRPC
		}
	}

	clientCtx, err := chainclient.NewClientContext(network.ChainId, senderAddress.String(), cosmosKeyring)
	if err != nil {
		return nil, err
	}

	tmRPC, err := rpchttp.New(network.TmEndpoint)
	if err != nil {
		return nil, err
	}

	clientCtx = clientCtx.WithNodeURI(network.TmEndpoint).WithClient(tmRPC)
	txFactory := chainclient.NewTxFactory(clientCtx)
	txFactory = txFactory.WithGasAdjustment(cosmosConfig.cosmosGasAdjust)
	txFactory = txFactory.WithGasPrices(cosmosConfig.cosmosGasPrices)

	cosmosClient, err := chainclient.NewChainClient(clientCtx, network, common.OptionTxFactory(&txFactory))
	if err != nil {
		return nil, err
	}
	closer.Bind(func() {
		cosmosClient.Close()
	})

	log.Infoln("waiting for GRPC services")

	daemonWaitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()

	daemonConn := cosmosClient.QueryClient()
	if err := waitForService(daemonWaitCtx, daemonConn); err != nil {
		return nil, fmt.Errorf("failed to wait for cosmos client connection: %w", err)
	}

	return cosmosClient, err
}
