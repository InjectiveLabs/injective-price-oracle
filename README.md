# injective-price-oracle

Injective's Oracle with dynamic price feeds. Allows anyone to start their own pre-approved price submission process to the oracle module on the Injective Chain.

## Getting Started

First, make sure your Go env is correctly configured, download a distribution from https://go.dev/dl/ the minimum required version is 1.17.

Clone this repo:

```bash
git clone git@github.com:InjectiveLabs/injective-price-oracle-ext.git
cd injective-price-oracle-ext
make install
```

After binary has been built, it should be available for usage in CLI:

```bash
$ injective-price-oracle version

Version dev (77a1017)
Compiled at 20220203-2207 using Go go1.17.3 (arm64)
```

## Environment

Before starting the `injective-price-oracle` service, make sure you copy a template [.env](.env.example) file and fill the correct values, especially for the Cosmos keys. You can use latest `injectived` release to manage the keyring dir. It's never recommended to use private key as plaintext in `ORACLE_COSMOS_PK`, except for testing.

More info on private keys: [A Guide to Private Key Management (Oracle)](https://injective.notion.site/A-Guide-to-Private-Key-Management-Oracle-e07fddc2fe7043b5803a97c118dccdcf)

```bash
cp .env.example .env
```

## Running with dynamic feeds via binary

This is an example that loads all dynamic feeds from [examples/](examples/) dir! Make sure to specify correct path to your TOML dir.

```bash
$ injective-price-oracle start --dynamic-feeds examples


INFO[0000] using Cosmos Sender inj128jwakuw3wrq6ye7m4p64wrzc5rfl8tvwzc6s8
INFO[0000] waiting for GRPC services
INFO[0001] found 1 dynamic feed configs
INFO[0001] got 1 enabled price feeds                     svc=oracle
INFO[0001] initialized 1 price pullers                   svc=oracle
INFO[0001] starting pullers for 1 feeds                  svc=oracle
INFO[0007] PullPrice (pipeline run) done in 1.0506275s   dynamic=true provider=binance_v3 svc=oracle ticker=INJ/USDT
INFO[0014] sent Tx in 2.72982375s                        batch_size=1 hash=1D7D02BDBAEC200BD585E90215459E93C760A1317EFF9D83B822FA4F34AD6A03 svc=oracle timeout=true
INFO[0067] PullPrice (pipeline run) done in 314.4035ms   dynamic=true provider=binance_v3 svc=oracle ticker=INJ/USDT
INFO[0073] sent Tx in 1.706471708s                       batch_size=1 hash=6E3A6C8F7706DB0B0355C5691A628A56CD5A87BB14877D2F0D151178FCF2784A svc=oracle timeout=true
INFO[0128] PullPrice (pipeline run) done in 310.32875ms  dynamic=true provider=binance_v3 svc=oracle ticker=INJ/USDT
INFO[0133] sent Tx in 1.776902583s                       batch_size=1 hash=29D615079A891F25E5ADE167E78D478F8AA99CEEFED7DB47B3F5E71BFEDEB582 svc=oracle timeout=true
```

## Running with dynamic feeds via docker-compose
1. Docker-compose file
```
version: '3.8'
networks:
  injective:
    name: injective
services:
  injective-price-oracle:
    container_name: injective-price-oracle
    image: public.ecr.aws/l9h3g6c6/injective-price-oracle:prod
    build: ../../../injective-price-oracle/
    command: injective-price-oracle start --dynamic-feeds /root/oracle-feeds
    logging:
      driver: journald
    environment:
      # log config
      ORACLE_ENV: prod
      ORACLE_LOG_LEVEL: info
      # chain config
      ORACLE_SERVICE_WAIT_TIMEOUT: "1m"
      ORACLE_COSMOS_CHAIN_ID: injective-1
      ORACLE_COSMOS_GRPC: tcp://sentry0.injective.network:9900
      ORACLE_TENDERMINT_RPC: http://sentry0.injective.network:26657
      ORACLE_COSMOS_GAS_PRICES: 500000000inj
      ORACLE_DYNAMIC_FEEDS_DIR:
      # keyring config
      ORACLE_COSMOS_KEYRING: file
      ORACLE_COSMOS_KEYRING_DIR: /root/keyring-oracle
      ORACLE_COSMOS_KEYRING_APP: injectived
      ORACLE_COSMOS_FROM: oracle-user
      ORACLE_COSMOS_FROM_PASSPHRASE: 12345678
      ORACLE_COSMOS_PK:
      ORACLE_COSMOS_USE_LEDGER: "false"
      # You can pass variables from env here into specific integrations,
      # make sure to suport that in the source code.
      # ORACLE_BINANCE_URL=
      # statsd config
      ORACLE_STATSD_PREFIX: "inj-oracle"
      ORACLE_STATSD_ADDR: host.docker.internal:8125
      ORACLE_STATSD_STUCK_DUR: 5m
      ORACLE_STATSD_MOCKING: "false"
      ORACLE_STATSD_DISABLED: "false"
    networks:
      - injective
    volumes:
      - ~/keyring-oracle:/root/keyring-oracle
      - ~/docker-volume/oracle-feeds:/root/oracle-feeds
```
2. Start and get logs
```
docker-compose up -d injective-price-oracle
docker logs injective-price-oracle
```

## Adding new feeds

There are two ways to add new feeds.

### Dynamic feeds (TOML)

Most preferred way is to create **dynamic feeds** using [DOT Syntax](https://en.wikipedia.org/wiki/DOT_(graph_description_language)), using the Chainlink innovation in this area (see [Job Pipelines](https://docs.chain.link/docs/jobs/task-types/pipelines/)).

Check out this most simple example:

```toml
provider = "binance_v3"
ticker = "INJ/USDT"
pullInterval = "1m"
observationSource = """
   ticker [type=http method=GET url="https://api.binance.com/api/v3/ticker/price?symbol=INJUSDT"];
   parsePrice [type="jsonparse" path="price"]
   multiplyDecimals [type="multiply" times=1]

   ticker -> parsePrice -> multiplyDecimals
"""
```

Beautiful, isn't it? The `observationSource` provided in DOT Syntax, while the rest of the file is a TOML config. Place these configs under any names into a special dir and start the oracle referencing the dir with `--dynamic-feeds <dir>`.

See the full documentation on the supported [Tasks](https://docs.chain.link/docs/tasks/) that you can use.

List of supported pipeline tasks:

* `http` - [docs](https://docs.chain.link/docs/jobs/task-types/http/)🔗
* `mean` - [docs](https://docs.chain.link/docs/jobs/task-types/mean/)🔗
* `median` - [docs](https://docs.chain.link/docs/jobs/task-types/median/)🔗
* `mode` - [docs](https://docs.chain.link/docs/jobs/task-types/mode/)🔗
* `sum` - [docs](https://docs.chain.link/docs/jobs/task-types/sum/)🔗
* `multiply` - [docs](https://docs.chain.link/docs/jobs/task-types/multiply/)🔗
* `divide` - [docs](https://docs.chain.link/docs/jobs/task-types/divide/)🔗
* `jsonparse` - [docs](https://docs.chain.link/docs/jobs/task-types/jsonparse/)🔗
* `any` - [docs](https://docs.chain.link/docs/jobs/task-types/any/)🔗
* `ethabiencode` - [docs](https://docs.chain.link/docs/jobs/task-types/eth-abi-encode/)
* `ethabiencode2` - [docs](https://github.com/smartcontractkit/chainlink/blob/develop/docs/CHANGELOG.md#enhanced-abi-encoding-support)🔗
* `ethabidecode` - [docs](https://docs.chain.link/docs/jobs/task-types/eth-abi-decode/)🔗
* `ethabidecodelog` - [docs](https://docs.chain.link/docs/jobs/task-types/eth-abi-decode-log/)🔗
* `merge` - [docs](https://github.com/smartcontractkit/chainlink/blob/develop/docs/CHANGELOG.md#merge-task-type)🔗
* `lowercase`
* `uppercase`

More can be added if needed.

List of config fields:

* `provider` - name (or slug) of the used provider, used for logging purposes, ⚠️ needs to be unique across all feed providers.
* `ticker` - name of the ticker on the Injective Chain. Used for loading feeds for enabled tickers.
* `pullInterval` time duration spec in Go-flavoured duration syntax. Cannot be negative or less than "1s". Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
* `observationSource` - pipeline spec in DOT Syntax

Notes on changes:

* `http` task has been changed from the Chainlink's reference, to skip `allowUnrestrictedNetworkAccess` option, since TOMLs are trusted in this context. Added ability to specify additional HTTP headers, since some price fetching APIs require authorization – `headerMap`. Usage: `headerMap="{\\"x-api-key\\": \\"foobar\\"}"`

#### Probing dynamic feeds

During development sometimes one needs to evaluate if his TOML file is correct and the pipeline specification yields a correct result. To avoid running the whole E2E flow with chain, there is a simple stateless command - `probe`!

Probe does the following:

* Loads TOML and parses the pipleine
* Created a dynamic price feed, as if it was orchestrated in the oracle
* Tries to pull the price once, using the pipeline
* Prints the anwer or an error

Example:

```
$ injective-price-oracle probe examples/dynamic_binance.toml

INFO[0000] PullPrice (pipeline run) done in 530.560708ms  dynamic=true provider=binance_v3 svc=oracle ticker=INJ/USDT
INFO[0000] Answer: 4948000
```

### Native Go code

Yes, you can also simply fork this repo and add own native implementations of the price feeds. There is a Binance example provided in [feed_binance.go](/oracle/feed_binance.go). Any complex feed can be added as long as the implementation follows this Go interface:

```go
type PricePuller interface {
	Provider() FeedProvider
	ProviderName() string
	Symbol() string
	Interval() time.Duration

	// PullPrice method must be implemented in order to get a price
	// from external source, handled by PricePuller.
	PullPrice(ctx context.Context) (price decimal.Decimal, err error)
}
```
