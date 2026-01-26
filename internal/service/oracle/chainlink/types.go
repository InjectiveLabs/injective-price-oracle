package chainlink

import (
	"context"

	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"
)

type Fetcher interface {
	Start(ctx context.Context) error
	ChainlinkReport(feedID string) *oracletypes.ChainlinkReport
}

type Config struct {
	WsURL     string
	APIKey    string
	APISecret string
	FeedIDs   []string
}
