package oracle

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/InjectiveLabs/metrics"
	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	log "github.com/xlab/suplog"
)

type StorkFetcher interface {
}

type StorkConfig struct {
	WebsocketUrl    string
	WebsocketHeader string
	Message         string
}

type storkFetcher struct {
	conn        *websocket.Conn
	latestPairs map[string]*oracletypes.AssetPair
	tickers     []string
	message     string
	mu          sync.RWMutex

	logger  log.Logger
	svcTags metrics.Tags
}

// NewStorkFetcher returns a new StorkFetcher instance.
func NewStorkFetcher(conn *websocket.Conn, storkConfig *StorkConfig) (*storkFetcher, error) {
	feed := &storkFetcher{
		conn:        conn,
		message:     storkConfig.Message,
		latestPairs: make(map[string]*oracletypes.AssetPair),
		logger: log.WithFields(log.Fields{
			"svc":      "oracle",
			"dynamic":  true,
			"provider": "storkFetcher",
		}),

		svcTags: metrics.Tags{
			"provider": "storkFetcher",
		},
	}

	return feed, nil
}

func (f *storkFetcher) AssetPair(ticker string) *oracletypes.AssetPair {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.latestPairs[ticker]
}

func (f *storkFetcher) Start() error {
	err := f.subscribe()
	if err != nil {
		return err
	}

	go f.startReadingMessages()
	return nil
}

func (f *storkFetcher) AddTicker(ticker string) {
	f.tickers = append(f.tickers, ticker)
}

// subscribe sends the initial subscription message to the WebSocket server.
func (f *storkFetcher) subscribe() error {
	if len(f.tickers) == 0 {
		f.logger.Errorf("no tickers to subscribe to")
		return errors.New("no tickers to subscribe to")
	}

	err := f.conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(f.message, strings.Join(f.tickers, "\",\""))))
	if err != nil {
		f.logger.Warningln("error writing subscription message:", err)
		return err
	}

	return nil
}

func (f *storkFetcher) startReadingMessages() {
	// Create a ticker that ticks every 10 seconds
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var err error
			var messageRead []byte

			// Read message from the WebSocket
			_, messageRead, err = f.conn.ReadMessage()
			if err != nil {
				f.logger.Warningln("error reading message:", err)
				continue
			}

			// Process the received message
			var msgResp messageResponse
			if err = json.Unmarshal(messageRead, &msgResp); err != nil {
				f.logger.Warningln("error unmarshalling feed message:", err)
				continue
			}

			// Extract asset pairs from the message
			assetIds := make([]string, 0)
			for key := range msgResp.Data {
				assetIds = append(assetIds, key)
			}

			// Update the cached asset pairs
			newPairs := make(map[string]*oracletypes.AssetPair, len(assetIds))
			for _, assetId := range assetIds {
				pair := ConvertDataToAssetPair(msgResp.Data[assetId], assetId)
				newPairs[assetId] = &pair
			}

			// Safely update the latestPairs with a write lock
			f.mu.Lock()
			for key, value := range newPairs {
				f.latestPairs[key] = value
			}
			f.mu.Unlock()
		}
	}
}
