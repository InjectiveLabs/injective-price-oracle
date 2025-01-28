package oracle

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"cosmossdk.io/math"
	"github.com/InjectiveLabs/metrics"
	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"
	log "github.com/InjectiveLabs/suplog"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

const (
	messageTypeInvalid            messageType = "invalid_message"
	messageTypeOraclePrices       messageType = "oracle_prices"
	messageTypeSubscribe          messageType = "subscribe"
	MaxStorkTimestampIntervalNano             = 500_000_000 // 5000ms
)

var ErrInvalidMessage = errors.New("received invalid message")

type StorkFetcher interface {
	Start(ctx context.Context, conn *websocket.Conn) error
	AssetPair(ticker string) *oracletypes.AssetPair
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
func NewStorkFetcher(storkMessage string, storkTickers []string) *storkFetcher {
	feed := &storkFetcher{
		message:     storkMessage,
		tickers:     storkTickers,
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

func (f *storkFetcher) Start(_ context.Context, conn *websocket.Conn) error {
	f.conn = conn

	defer f.reset()

	err := f.subscribe()
	if err != nil {
		return err
	}

	return f.startReadingMessages()
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
	f.mu.Lock()
	defer f.mu.Unlock()

	f.conn.Close()
	f.latestPairs = make(map[string]*oracletypes.AssetPair)
}

func (f *storkFetcher) startReadingMessages() error {
	for {
		var err error
		var messageRead []byte
		now := time.Now()
		// Read message from the WebSocket
		_, messageRead, err = f.conn.ReadMessage()
		if err != nil {
			// Report the error and return
			metrics.CustomReport(func(s metrics.Statter, tagSpec []string) {
				s.Count("feed_provider.stork.unable_read_message.size", 1, tagSpec, 1)
			}, f.svcTags)
			f.logger.Warningln("error reading message:", err)
			return err
		}
		f.logger.Debugln("time taken to read message:", time.Since(now))

		f.logger.Debugln("received message:", string(messageRead))

		// Process the received message
		var msgResp messageResponse
		if err = json.Unmarshal(messageRead, &msgResp); err != nil {
			f.logger.Warningln("error unmarshalling feed message:", err)
			continue
		}

		switch msgResp.Type {
		case messageTypeInvalid.String():
			// Report the invalid message and return
			metrics.ReportFuncError(f.svcTags)
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
				asset := data[assetId]
				timestamp, err := getTimestampInRange(asset)
				if err != nil {
					metrics.CustomReport(func(s metrics.Statter, tagSpec []string) {
						s.Count("feed_provider.stork.max_diff_threshold", 1, tagSpec, 1)
					}, f.svcTags)
					f.logger.Warningln("error finding timestamp:", err)
					continue
				}

				pair := ConvertDataToAssetPair(asset, assetId, timestamp)
				newPairs[assetId] = &pair
			}

			// Safely update the latestPairs with a write lock
			f.mu.Lock()
			for key, value := range newPairs {
				var v = value
				f.latestPairs[key] = v
			}
			f.mu.Unlock()

		default:
			metrics.ReportFuncError(f.svcTags)
			f.logger.Warningln("received unknown message type:", msgResp.Type)
		}
	}
}

func getTimestampInRange(asset Data) (uint64, error) {
	var newestTimestamp uint64
	oldestTimestamp := ^uint64(0)

	for _, signedPrice := range asset.SignedPrices {
		if signedPrice.TimestampedSignature.Timestamp > newestTimestamp {
			newestTimestamp = signedPrice.TimestampedSignature.Timestamp
		}
		if signedPrice.TimestampedSignature.Timestamp < oldestTimestamp {
			oldestTimestamp = signedPrice.TimestampedSignature.Timestamp
		}
	}

	if newestTimestamp == 0 {
		return 0, fmt.Errorf("asset '%s' has no price timestamps", asset.AssetID)
	}

	if newestTimestamp-oldestTimestamp > MaxStorkTimestampIntervalNano {
		return 0, fmt.Errorf("assset '%s' price timestamps between %d and %d exceed threshold %d", asset.AssetID, oldestTimestamp, newestTimestamp, MaxStorkTimestampIntervalNano)
	}

	return newestTimestamp, nil
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
