package types

import (
	"context"
	"strings"
	"time"

	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"
	"github.com/shopspring/decimal"
)

// binance, kraken. coin
type Fetcher interface {
	Start(ctx context.Context) error
}

// PriceData is the common interface for all price data types
type PriceData interface {
	GetTicker() string
	GetProviderName() string
	GetSymbol() string
	GetPrice() decimal.Decimal
	GetTimestamp() time.Time
	GetOracleType() oracletypes.OracleType
}

// PricePuller is the interface for pulling prices from external sources
type PricePuller interface {
	Provider() FeedProvider
	ProviderName() string
	Symbol() string
	Interval() time.Duration
	PullPrice(ctx context.Context) (priceData PriceData, err error)
	OracleType() oracletypes.OracleType
}

// FeedProvider represents the type of price feed provider
type FeedProvider string

func (f FeedProvider) String() string {
	return string(f)
}

const (
	FeedProviderStork     FeedProvider = "stork"
	FeedProviderChainlink FeedProvider = "chainlink"
)

// Ticker represents a trading pair (e.g., "BTC/USDT")
type Ticker string

func (t Ticker) Base() string {
	return strings.Split(string(t), "/")[0]
}

func (t Ticker) Quote() string {
	return strings.Split(string(t), "/")[1]
}

func (t Ticker) String() string {
	return string(t)
}

// FeedConfig represents a generic feed configuration
type FeedConfig struct {
	ProviderName      string `toml:"provider"`
	FeedID            string `toml:"feedId"`
	Ticker            string `toml:"ticker"`
	PullInterval      string `toml:"pullInterval"`
	ObservationSource string `toml:"observationSource"`
	OracleType        string `toml:"oracleType"`
}

type WsConfig struct {
	WebsocketUrl    string
	WebsocketHeader string
	Message         string
}
