package oracle

import (
	"context"
	"fmt"
	"github.com/InjectiveLabs/metrics"
	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"
	"github.com/gorilla/websocket"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	log "github.com/xlab/suplog"
	"net/http"
	"net/url"
	"time"
	"encoding/base64"
)

var _ PricePuller = &storkPriceFeed{}

type StorkFeedConfig struct {
	ProviderName string `toml:"provider"`
	Ticker       string `toml:"ticker"`
	PullInterval string `toml:"pullInterval"`
	Url          string `toml:"url"`
	Header       string `toml:"header"`
	Data         string `toml:"data"`
	OracleType   string `toml:"oracleType"`
}

type StorkConfig struct {
}

type storkPriceFeed struct {
	ticker       string
	providerName string
	interval     time.Duration
	url          string
	header       string
	data         string

	runNonce int32

	logger  log.Logger
	svcTags metrics.Tags

	oracleType oracletypes.OracleType
}

func ParseStorkFeedConfig(body []byte) (*StorkFeedConfig, error) {
	var config StorkFeedConfig
	if err := toml.Unmarshal(body, &config); err != nil {
		err = errors.Wrap(err, "failed to unmarshal TOML config")
		return nil, err
	}

	return &config, nil
}

// NewStorkPriceFeed returns price puller
func NewStorkPriceFeed(cfg *StorkFeedConfig) (PricePuller, error) {
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
		oracleType = oracletypes.OracleType_PriceFeed
	} else {
		tmpType, exist := oracletypes.OracleType_value[cfg.OracleType]
		if !exist {
			return nil, fmt.Errorf("oracle type does not exist: %s", cfg.OracleType)
		}

		oracleType = oracletypes.OracleType(tmpType)
	}

	feed := &storkPriceFeed{
		ticker:       cfg.Ticker,
		providerName: cfg.ProviderName,
		interval:     pullInterval,
		url:          cfg.Url,
		header:       cfg.Header,
		data:         cfg.Data,
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

func (f *storkPriceFeed) PullAssetPair(ctx context.Context) (assetPair oracletypes.AssetPair, err error) {
	metrics.ReportFuncCall(f.svcTags)
	doneFn := metrics.ReportFuncTiming(f.svcTags)
	defer doneFn()

	// Parse the URL
	u, err := url.Parse(f.url)
	if err != nil {
		log.Fatal("Error parsing URL:", err)
	}
	header := http.Header{}
	header.Add("Authorization", "Basic " + base64.StdEncoding.EncodeToString([]byte(f.header)))


	dialer := websocket.DefaultDialer
	dialer.EnableCompression = true

	// Connect to the WebSocket server
	conn, resp, err := dialer.Dial(u.String(), header)
	if err != nil {
		if resp != nil {
            log.Printf("Handshake failed with status: %d\n", resp.StatusCode)
            for k, v := range resp.Header {
                log.Printf("%s: %v\n", k, v)
            }
        }
        log.Fatal("Error connecting to WebSocket:", err)
	}
	defer conn.Close()

	log.Println("Connected to WebSocket server")

	return oracletypes.AssetPair{}, nil
}

func (f *storkPriceFeed) PullPrice(ctx context.Context) (
	price decimal.Decimal,
	err error,
) {
	return zeroPrice, nil
}
