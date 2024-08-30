package oracle

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"cosmossdk.io/math"
	"github.com/InjectiveLabs/metrics"
	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	log "github.com/xlab/suplog"
)

const (
	messageTypeInvalid      messageType = "invalid_message"
	messageTypeOraclePrices messageType = "oracle_prices"
	messageTypeSubscribe    messageType = "subscribe"
)

var ErrInvalidMessage = errors.New("received invalid message")

type StorkFetcher interface {
	Start() error
	Reconnect(conn *websocket.Conn) error
	AssetPair(ticker string) *oracletypes.AssetPair
	AddTicker(ticker string)
}

type messageType string

func (m messageType) String() string {
	return string(m)
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
func NewStorkFetcher(conn *websocket.Conn, storkMessage string) *storkFetcher {
	feed := &storkFetcher{
		conn:        conn,
		message:     storkMessage,
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

	return feed
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

	return f.startReadingMessages()
}

func (f *storkFetcher) Reconnect(conn *websocket.Conn) error {
	f.conn = conn
	return f.Start()
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

	f.logger.Debugln("subscribing to tickers:", f.tickers)
	f.logger.Debugln(fmt.Sprintf(f.message, strings.Join(f.tickers, "\",\"")))
	err := f.conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(f.message, strings.Join(f.tickers, "\",\""))))
	if err != nil {
		f.logger.Warningln("error writing subscription message:", err)
		return err
	}

	return nil
}

func (f *storkFetcher) reset() {
	f.conn.Close()
	f.latestPairs = make(map[string]*oracletypes.AssetPair)
}

func (f *storkFetcher) startReadingMessages() error {
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
				f.reset()
				return err
			}

			f.logger.Debugln("received message:", string(messageRead))

			// Process the received message
			var msgResp messageResponse
			if err = json.Unmarshal(messageRead, &msgResp); err != nil {
				f.logger.Warningln("error unmarshalling feed message:", err)
				continue
			}

			switch msgResp.Type {
			case messageTypeInvalid.String():
				f.reset()
				return ErrInvalidMessage
			case messageTypeSubscribe.String():
				f.logger.Infof("subscribed to tickers: %s", strings.Join(f.tickers, ","))
			case messageTypeOraclePrices.String():
				var data oracleData
				if err = json.Unmarshal(msgResp.Data, &data); err != nil {
					f.logger.Warningln("error unmarshalling oracle data:", err)
					continue
				}

				// Extract asset pairs from the message
				assetIds := make([]string, 0)
				for key := range data {
					assetIds = append(assetIds, key)
				}

				// Update the cached asset pairs
				newPairs := make(map[string]*oracletypes.AssetPair, len(assetIds))
				for _, assetId := range assetIds {
					pair := ConvertDataToAssetPair(data[assetId], assetId)
					newPairs[assetId] = &pair
				}

				// Safely update the latestPairs with a write lock
				f.mu.Lock()
				for key, value := range newPairs {
					f.latestPairs[key] = value
				}
				f.mu.Unlock()

			default:
				f.logger.Warningln("received unknown message type:", msgResp.Type)
			}
		}
	}
}

type messageResponse struct {
	Type    string          `json:"type"`
	TraceID string          `json:"trace_id"`
	Data    json.RawMessage `json:"data"`
}

type oracleData map[string]Data

type Data struct {
	Timestamp     int64         `json:"timestamp"`
	AssetID       string        `json:"asset_id"`
	SignatureType string        `json:"signature_type"`
	Trigger       string        `json:"trigger"`
	Price         string        `json:"price"`
	SignedPrices  []SignedPrice `json:"signed_prices"`
}

type SignedPrice struct {
	PublisherKey         string               `json:"publisher_key"`
	ExternalAssetID      string               `json:"external_asset_id"`
	SignatureType        string               `json:"signature_type"`
	Price                math.LegacyDec       `json:"price"`
	TimestampedSignature TimestampedSignature `json:"timestamped_signature"`
}

type TimestampedSignature struct {
	Signature Signature `json:"signature"`
	Timestamp uint64    `json:"timestamp"`
	MsgHash   string    `json:"msg_hash"`
}

type Signature struct {
	R string `json:"r"`
	S string `json:"s"`
	V string `json:"v"`
}
