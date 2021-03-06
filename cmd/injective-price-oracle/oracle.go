package main

import (
	"context"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	exchangetypes "github.com/InjectiveLabs/sdk-go/chain/exchange/types"
	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"
	cli "github.com/jawher/mow.cli"
	"github.com/pkg/errors"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/xlab/closer"
	log "github.com/xlab/suplog"

	chainclient "github.com/InjectiveLabs/sdk-go/chain/client"

	"github.com/InjectiveLabs/injective-price-oracle/oracle"
)

// oracleCmd action runs the service
//
// $ injective-price-oracle start
func oracleCmd(cmd *cli.Cmd) {
	var (
		// Cosmos params
		cosmosChainID   *string
		cosmosGRPC      *string
		tendermintRPC   *string
		cosmosGasPrices *string

		// Cosmos Key Management
		cosmosKeyringDir     *string
		cosmosKeyringAppName *string
		cosmosKeyringBackend *string

		cosmosKeyFrom       *string
		cosmosKeyPassphrase *string
		cosmosPrivKey       *string
		cosmosUseLedger     *bool

		// External Feeds params
		dynamicFeedsDir *string
		binanceBaseURL  *string

		// Metrics
		statsdPrefix   *string
		statsdAddr     *string
		statsdStuckDur *string
		statsdMocking  *string
		statsdDisabled *string
	)

	initCosmosOptions(
		cmd,
		&cosmosChainID,
		&cosmosGRPC,
		&tendermintRPC,
		&cosmosGasPrices,
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
		&dynamicFeedsDir,
	)

	initStatsdOptions(
		cmd,
		&statsdPrefix,
		&statsdAddr,
		&statsdStuckDur,
		&statsdMocking,
		&statsdDisabled,
	)

	cmd.Action = func() {
		// ensure a clean exit
		defer closer.Close()

		startMetricsGathering(
			statsdPrefix,
			statsdAddr,
			statsdStuckDur,
			statsdMocking,
			statsdDisabled,
		)

		if *cosmosUseLedger {
			log.Fatalln("cannot really use Ledger for oracle service loop, since signatures msut be realtime")
		}

		senderAddress, cosmosKeyring, err := initCosmosKeyring(
			cosmosKeyringDir,
			cosmosKeyringAppName,
			cosmosKeyringBackend,
			cosmosKeyFrom,
			cosmosKeyPassphrase,
			cosmosPrivKey,
			cosmosUseLedger,
		)
		if err != nil {
			log.WithError(err).Fatalln("failed to init Cosmos keyring")
		}

		log.Infoln("using Cosmos Sender", senderAddress.String())

		clientCtx, err := chainclient.NewClientContext(*cosmosChainID, senderAddress.String(), cosmosKeyring)
		if err != nil {
			log.WithError(err).Fatalln("failed to initialize cosmos client context")
		}
		clientCtx = clientCtx.WithNodeURI(*tendermintRPC)
		tmRPC, err := rpchttp.New(*tendermintRPC, "/websocket")
		if err != nil {
			log.WithError(err).Fatalln("failed to connect to tendermint RPC")
		}
		clientCtx = clientCtx.WithClient(tmRPC)

		cosmosClient, err := chainclient.NewCosmosClient(clientCtx, *cosmosGRPC, chainclient.OptionGasPrices(*cosmosGasPrices))
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"endpoint": *cosmosGRPC,
			}).Fatalln("failed to connect to daemon, is injectived running?")
		}
		closer.Bind(func() {
			cosmosClient.Close()
		})

		log.Infoln("waiting for GRPC services")
		time.Sleep(1 * time.Second)

		daemonWaitCtx, cancelWait := context.WithTimeout(context.Background(), time.Minute)
		daemonConn := cosmosClient.QueryClient()
		waitForService(daemonWaitCtx, daemonConn)
		cancelWait()

		feedProviderConfigs := map[oracle.FeedProvider]interface{}{
			oracle.FeedProviderBinance: &oracle.BinanceEndpointConfig{
				BaseURL: *binanceBaseURL,
			},
		}

		dynamicFeedConfigs := make([]*oracle.DynamicFeedConfig, 0, 10)
		if len(*dynamicFeedsDir) > 0 {
			err := filepath.WalkDir(*dynamicFeedsDir, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				} else if d.IsDir() {
					return nil
				} else if filepath.Ext(path) != ".toml" {
					return nil
				}

				cfgBody, err := ioutil.ReadFile(path)
				if err != nil {
					err = errors.Wrapf(err, "failed to read dynamic feed config")
					return err
				}

				feedCfg, err := oracle.ParseDynamicFeedConfig(cfgBody)
				if err != nil {
					log.WithError(err).WithFields(log.Fields{
						"filename": d.Name(),
					}).Errorln("failed to parse dynamic feed config")
					return nil
				}

				dynamicFeedConfigs = append(dynamicFeedConfigs, feedCfg)

				return nil
			})

			if err != nil {
				err = errors.Wrapf(err, "dynamic feeds dir is specified, but failed to read from it: %s", *dynamicFeedsDir)
				log.WithError(err).Fatalln("failed to load dynamic feeds")
				return
			}

			log.Infof("found %d dynamic feed configs", len(dynamicFeedConfigs))
		}

		svc, err := oracle.NewService(
			cosmosClient,
			exchangetypes.NewQueryClient(daemonConn),
			oracletypes.NewQueryClient(daemonConn),
			feedProviderConfigs,
			dynamicFeedConfigs,
		)
		if err != nil {
			log.Fatalln(err)
		}

		closer.Bind(func() {
			svc.Close()
		})

		go func() {
			if err := svc.Start(); err != nil {
				log.Errorln(err)

				// signal there that the app failed
				os.Exit(1)
			}
		}()

		closer.Hold()
	}
}
