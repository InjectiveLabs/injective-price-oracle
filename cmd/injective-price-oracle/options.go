package main

import cli "github.com/jawher/mow.cli"

// initGlobalOptions defines some global CLI options, that are useful for most parts of the app.
// Before adding option to there, consider moving it into the actual Cmd.
func initGlobalOptions(
	envName **string,
	appLogLevel **string,
	svcWaitTimeout **string,
) {
	*envName = app.String(cli.StringOpt{
		Name:   "e env",
		Desc:   "The environment name this app runs in. Used for metrics and error reporting.",
		EnvVar: "ORACLE_ENV",
		Value:  "local",
	})

	*appLogLevel = app.String(cli.StringOpt{
		Name:   "l log-level",
		Desc:   "Available levels: error, warn, info, debug.",
		EnvVar: "ORACLE_LOG_LEVEL",
		Value:  "info",
	})

	*svcWaitTimeout = app.String(cli.StringOpt{
		Name:   "svc-wait-timeout",
		Desc:   "Standard wait timeout for external services (e.g. Cosmos daemon GRPC connection)",
		EnvVar: "ORACLE_SERVICE_WAIT_TIMEOUT",
		Value:  "1m",
	})
}

func initCosmosOptions(
	cmd *cli.Cmd,
	cosmosOverrideNetwork *bool,
	cosmosChainID *string,
	cosmosGRPCs *[]string,
	cosmosStreamGRPCs *[]string,
	tendermintRPCs *[]string,
	cosmosGasPrices *string,
	cosmosGasAdjust *float64,
	networkNode *string,
) {
	cmd.BoolPtr(cosmosOverrideNetwork, cli.BoolOpt{
		Name:   "cosmos-override-network",
		Desc:   "Override the Cosmos network and node configuration.",
		EnvVar: "ORACLE_COSMOS_OVERRIDE_NETWORK",
		Value:  false,
	})
	cmd.StringPtr(cosmosChainID, cli.StringOpt{
		Name:   "cosmos-chain-id",
		Desc:   "Specify Chain ID of the Cosmos network.",
		EnvVar: "ORACLE_COSMOS_CHAIN_ID",
		Value:  "injective-1",
	})

	cmd.StringsPtr(cosmosGRPCs, cli.StringsOpt{
		Name:   "cosmos-grpc",
		Desc:   "Cosmos GRPC querying endpoints",
		EnvVar: "ORACLE_COSMOS_GRPC",
		Value:  []string{"tcp://localhost:9900"},
	})

	cmd.StringsPtr(cosmosStreamGRPCs, cli.StringsOpt{
		Name:   "cosmos-stream-grpc",
		Desc:   "Cosmos Stream GRPC querying endpoints",
		EnvVar: "ORACLE_COSMOS_STREAM_GRPC",
		Value:  []string{"tcp://localhost:9999"},
	})

	cmd.StringsPtr(tendermintRPCs, cli.StringsOpt{
		Name:   "tendermint-rpc",
		Desc:   "Tendermint RPC endpoints",
		EnvVar: "ORACLE_TENDERMINT_RPC",
		Value:  []string{"http://localhost:26657"},
	})

	cmd.StringPtr(cosmosGasPrices, cli.StringOpt{
		Name:   "cosmos-gas-prices",
		Desc:   "Specify Cosmos chain transaction fees as sdk.Coins gas prices",
		EnvVar: "ORACLE_COSMOS_GAS_PRICES",
		Value:  "", // example: 500000000inj
	})

	cmd.Float64Ptr(cosmosGasAdjust, cli.Float64Opt{
		Name:   "cosmos-gas-adjust",
		Desc:   "Specify Cosmos chain transaction fees gas adjustment factor",
		EnvVar: "ORACLE_COSMOS_GAS_ADJUST",
		Value:  1.5,
	})

	cmd.StringPtr(networkNode, cli.StringOpt{
		Name:   "cosmos-network-node",
		Desc:   "Specify network and node (e.g mainnet,lb)",
		EnvVar: "ORACLE_NETWORK_NODE",
		Value:  "mainnet,lb",
	})
}

func initCosmosKeyOptions(
	cmd *cli.Cmd,
	cosmosKeyringDir **string,
	cosmosKeyringAppName **string,
	cosmosKeyringBackend **string,
	cosmosKeyFrom **string,
	cosmosKeyPassphrase **string,
	cosmosPrivKey **string,
	cosmosUseLedger **bool,
) {
	*cosmosKeyringBackend = cmd.String(cli.StringOpt{
		Name:   "cosmos-keyring",
		Desc:   "Specify Cosmos keyring backend (os|file|kwallet|pass|test)",
		EnvVar: "ORACLE_COSMOS_KEYRING",
		Value:  "file",
	})

	*cosmosKeyringDir = cmd.String(cli.StringOpt{
		Name:   "cosmos-keyring-dir",
		Desc:   "Specify Cosmos keyring dir, if using file keyring.",
		EnvVar: "ORACLE_COSMOS_KEYRING_DIR",
		Value:  "",
	})

	*cosmosKeyringAppName = cmd.String(cli.StringOpt{
		Name:   "cosmos-keyring-app",
		Desc:   "Specify Cosmos keyring app name.",
		EnvVar: "ORACLE_COSMOS_KEYRING_APP",
		Value:  "injectived",
	})

	*cosmosKeyFrom = cmd.String(cli.StringOpt{
		Name:   "cosmos-from",
		Desc:   "Specify the Cosmos validator key name or address. If specified, must exist in keyring, ledger or match the privkey.",
		EnvVar: "ORACLE_COSMOS_FROM",
	})

	*cosmosKeyPassphrase = cmd.String(cli.StringOpt{
		Name:   "cosmos-from-passphrase",
		Desc:   "Specify keyring passphrase, otherwise Stdin will be used.",
		EnvVar: "ORACLE_COSMOS_FROM_PASSPHRASE",
	})

	*cosmosPrivKey = cmd.String(cli.StringOpt{
		Name:   "cosmos-pk",
		Desc:   "Provide a raw Cosmos account private key of the validator in hex. USE FOR TESTING ONLY!",
		EnvVar: "ORACLE_COSMOS_PK",
	})

	*cosmosUseLedger = cmd.Bool(cli.BoolOpt{
		Name:   "cosmos-use-ledger",
		Desc:   "Use the Cosmos app on hardware ledger to sign transactions.",
		EnvVar: "ORACLE_COSMOS_USE_LEDGER",
		Value:  false,
	})
}

func initExternalFeedsOptions(
	cmd *cli.Cmd,
	binanceBaseURL **string,
	feedsDir **string,
) {
	*binanceBaseURL = cmd.String(cli.StringOpt{
		Name:   "binance-url",
		Desc:   "Binance API Base URL",
		EnvVar: "ORACLE_BINANCE_URL",
	})

	*feedsDir = cmd.String(cli.StringOpt{
		Name:   "feeds-dir",
		Desc:   "Path to feeds configuration files in TOML format",
		EnvVar: "ORACLE_FEEDS_DIR",
	})
}

// initStatsdOptions sets options for StatsD metrics.
func initStatsdOptions(
	cmd *cli.Cmd,
	statsdPrefix **string,
	statsdAddr **string,
	statsdAgent **string,
	statsdStuckDur **string,
	statsdMocking **string,
	statsdDisabled **string,
) {
	*statsdPrefix = cmd.String(cli.StringOpt{
		Name:   "statsd-prefix",
		Desc:   "Specify StatsD compatible metrics prefix.",
		EnvVar: "ORACLE_STATSD_PREFIX",
		Value:  "oracle",
	})

	*statsdAddr = cmd.String(cli.StringOpt{
		Name:   "statsd-addr",
		Desc:   "UDP address of a StatsD compatible metrics aggregator.",
		EnvVar: "ORACLE_STATSD_ADDR",
		Value:  "localhost:8125",
	})

	*statsdAgent = cmd.String(cli.StringOpt{
		Name:   "statsd-agent",
		Desc:   "Specify the agent name for StatsD metrics.",
		EnvVar: "ORACLE_STATSD_AGENT",
		Value:  "datadog",
	})

	*statsdStuckDur = cmd.String(cli.StringOpt{
		Name:   "statsd-stuck-func",
		Desc:   "Sets a duration to consider a function to be stuck (e.g. in deadlock).",
		EnvVar: "ORACLE_STATSD_STUCK_DUR",
		Value:  "5m",
	})

	*statsdMocking = cmd.String(cli.StringOpt{
		Name:   "statsd-mocking",
		Desc:   "If enabled replaces statsd client with a mock one that simply logs values.",
		EnvVar: "ORACLE_STATSD_MOCKING",
		Value:  "false",
	})

	*statsdDisabled = cmd.String(cli.StringOpt{
		Name:   "statsd-disabled",
		Desc:   "Force disabling statsd reporting completely.",
		EnvVar: "ORACLE_STATSD_DISABLED",
		Value:  "true",
	})
}

func initStorkOracleWebSocket(
	cmd *cli.Cmd,
	websocketUrl **string,
	websocketHeader **string,
	websocketSubscribeMessage **string,
) {
	*websocketUrl = cmd.String(cli.StringOpt{
		Name:   "websocket-url",
		Desc:   "Stork websocket URL",
		EnvVar: "STORK_WEBSOCKET_URL",
	})
	*websocketHeader = cmd.String(cli.StringOpt{
		Name:   "websocket-header",
		Desc:   "Stork websocket header",
		EnvVar: "STORK_WEBSOCKET_HEADER",
	})
	*websocketSubscribeMessage = cmd.String(cli.StringOpt{
		Name:   "websocket-subscribe-message",
		Desc:   "Stork websocket subscribe message",
		EnvVar: "STORK_WEBSOCKET_SUBSCRIBE_MESSAGE",
	})
}
