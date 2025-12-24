package chainlink

import (
	"time"

	"github.com/shopspring/decimal"

	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"

	"github.com/InjectiveLabs/injective-price-oracle/internal/service/oracle/types"
)

const FeedProviderChainlink types.FeedProvider = "chainlink"

// PriceData stores price data for Chainlink Oracle
type ChainlinkPriceData struct {
	Ticker          string
	ProviderName    string
	Symbol          string
	Price           decimal.Decimal
	ChainlinkReport *oracletypes.ChainlinkReport
	Timestamp       time.Time
	OracleType      oracletypes.OracleType
}

// Interface implementation methods
func (c *ChainlinkPriceData) GetTicker() string                     { return c.Ticker }
func (c *ChainlinkPriceData) GetProviderName() string               { return c.ProviderName }
func (c *ChainlinkPriceData) GetSymbol() string                     { return c.Symbol }
func (c *ChainlinkPriceData) GetPrice() decimal.Decimal             { return c.Price }
func (c *ChainlinkPriceData) GetTimestamp() time.Time               { return c.Timestamp }
func (c *ChainlinkPriceData) GetOracleType() oracletypes.OracleType { return c.OracleType }
