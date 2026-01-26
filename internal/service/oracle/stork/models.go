package stork

import (
	"time"

	"github.com/shopspring/decimal"

	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"

	"github.com/InjectiveLabs/injective-price-oracle/internal/service/oracle/types"
)

const FeedProviderStork types.FeedProvider = "stork"

// PriceData stores price data for Stork Oracle
type StorkPriceData struct {
	Ticker       string
	ProviderName string
	Symbol       string
	Price        decimal.Decimal
	AssetPair    *oracletypes.AssetPair
	Timestamp    time.Time
	OracleType   oracletypes.OracleType
}

// Interface implementation methods
func (s *StorkPriceData) GetTicker() string                     { return s.Ticker }
func (s *StorkPriceData) GetProviderName() string               { return s.ProviderName }
func (s *StorkPriceData) GetSymbol() string                     { return s.Symbol }
func (s *StorkPriceData) GetPrice() decimal.Decimal             { return s.Price }
func (s *StorkPriceData) GetTimestamp() time.Time               { return s.Timestamp }
func (s *StorkPriceData) GetOracleType() oracletypes.OracleType { return s.OracleType }
