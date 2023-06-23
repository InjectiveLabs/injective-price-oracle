# injective-price-oracle

Оракул Injective з динамічними потоками цін. Дозволяє будь-кому розпочати власний процес подання попередньо затверджених цін до модуля оракула в ланцюжку Injective.

## Початок роботи

По-перше, переконайтеся, що ваш Go env правильно налаштовано, завантажте дистрибутив з https://go.dev/dl/. Мінімально необхідна версія - 1.17.

Клонуйте цей репозиторій:

```bash
git clone git@github.com:InjectiveLabs/injective-price-oracle-ext.git
cd injective-price-oracle-ext
make install
```

Після того, як бінарник зібрано, він має бути доступний для використання у CLI:

```bash
$ injective-price-oracle version

Version dev (77a1017)
Compiled at 20220203-2207 using Go go1.17.3 (arm64)
```

## Оточення

Перед запуском служби `injective-price-oracle` переконайтеся, що ви скопіювали файл шаблону [.env](.env.example) заповнили його правильними значеннями, особливо для ключів Cosmos. Ви можете використовувати останній `injectived` реліз для керування директорією ключів. Ніколи не рекомендується використовувати приватний ключ як відкритий текст у `ORACLE_COSMOS_PK`, окрім випадків тестування.

Більше інформації про приватні ключі: [Посібник з управління приватними ключами (Oracle)](https://injective.notion.site/A-Guide-to-Private-Key-Management-Oracle-e07fddc2fe7043b5803a97c118dccdcf)

```bash
cp .env.example .env
```

## Запуск з динамічними потоками через бінарний файл

Цей приклад завантажує всі динамічні потоки з каталогу [examples/](examples/)! Переконайтеся, що ви вказали правильний шлях до теки TOML.

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

## Запуск з динамічними потоками через docker-compose
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
2. Запускайте та отримуйте звіти
```
docker-compose up -d injective-price-oracle
docker logs injective-price-oracle
```

## Додавання нових потоків

Існує два способи додавання нових потоків.

### Динамічні потоки (TOML)

Найкращим способом буде створення **динамічних каналів** за допомогою [DOT-синтаксису](https://en.wikipedia.org/wiki/DOT_(graph_description_language)), використовуючи інновації Chainlink у цій галузі (див. [Job Pipelines](https://docs.chain.link/docs/jobs/task-types/pipelines/)).

Погляньте на цей найпростіший приклад:

```toml
provider = "binance_v3"
ticker = "INJ/USDT"
pullInterval = "1m"
observationSource = """
   ticker [type=http method=GET url="https://api.binance.com/api/v3/ticker/price?symbol=INJUSDT"];
   parsePrice [type="jsonparse" path="price"]
   multiplyDecimals [type="multiply" times=1000000]

   ticker -> parsePrice -> multiplyDecimals
"""
```

Гарно, чи не так? Файл `observationSource` подано у синтаксисі DOT, а решта файлу є конфігурацією TOML. Помістіть ці конфіги під будь-якими іменами у спеціальний каталог і запустіть оракул, посилаючись на цей каталог за допомогою `--dynamic-feeds <dir>`.

Дивіться повну документацію щодо підтримуваних [Задач](https://docs.chain.link/docs/tasks/), якою ви можете скористатися.

Список підтримуваних задач конвеєра:

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

За потреби може бути додано більше.

Список конфігураційних полів:

* `provider` - ім'я (або slug) використовуваного провайдера, що використовується для цілей логування.
* `ticker` - назва тикера в ланцюжку Injective. Використовується для завантаження потоків для увімкнених тикерів.
* `pullInterval` специфікація тривалості інтервалу у синтаксисі тривалості зі стилізацією під Go. Не може бути від'ємним або меншим за "1s". Допустимі одиниці часу: "ns", "us" (or "µs"), "ms", "s", "m", "h".
* `observationSource` - специфікація конвеєра у синтаксисі DOT

Примітки щодо змін:

* Задача `http` була змінена з посилання Chainlink, щоб пропустити опцію `allowUnrestrictedNetworkAccess` оскільки в цьому контексті TOML довіряють. Додано можливість вказувати додаткові HTTP-заголовки, оскільки деякі API для отримання цін вимагають авторизації - `headerMap`. Використання: `headerMap="{\\"x-api-key\\": \\"foobar\\"}"`

#### Перевірка динамічних потоків

Під час розробки іноді виникає потреба оцінити, чи коректний його TOML-файл і чи специфікація пайплайну дає правильний результат. Щоб не запускати весь потік E2E ланцюжком, існує проста команда без стану - `probe`!

Probe виконує наступні дії:

* Завантажує TOML і розбирає пайплайн
* Створює динамічний ціновий потік, ніби він оркестрований в оракулі
* Спробує отримати ціну за один раз, використовуючи пайплайн
* Виводить відповідь або помилку

Приклад:

```
$ injective-price-oracle probe examples/dynamic_binance.toml

INFO[0000] PullPrice (pipeline run) done in 530.560708ms  dynamic=true provider=binance_v3 svc=oracle ticker=INJ/USDT
INFO[0000] Answer: 4948000
```

### Native Go код

Так, ви також можете просто форкнути цей репо і додати власні нативні реалізації цінових потоків. Приклад для Binance наведено в [feed_binance.go](/oracle/feed_binance.go). Будь-який складний потік може бути доданий, якщо його реалізація відповідає цьому інтерфейсу Go:

```go
type PricePuller interface {
	Provider() FeedProvider
	ProviderName() string
	Symbol() string
	Interval() time.Duration

	// Метод PullPrice повинен бути реалізований для того, щоб отримати ціну
	// із зовнішнього джерела, що обробляється методом PricePuller.
	PullPrice(ctx context.Context) (price decimal.Decimal, err error)
}
```
