package stork

import (
	"context"

	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"
	"github.com/gorilla/websocket"
)

type Fetcher interface {
	Start(ctx context.Context, conn *websocket.Conn) error
	AssetPair(ticker string) *oracletypes.AssetPair
}
