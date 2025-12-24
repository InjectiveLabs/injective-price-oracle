package chainlink

import (
	"context"
	"sync"
	"time"

	"github.com/InjectiveLabs/metrics"
	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"
	log "github.com/InjectiveLabs/suplog"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	streams "github.com/smartcontractkit/data-streams-sdk/go"
	"github.com/smartcontractkit/data-streams-sdk/go/feed"
	"github.com/smartcontractkit/data-streams-sdk/go/report"
	v3 "github.com/smartcontractkit/data-streams-sdk/go/report/v3"
)

type ChainLinkFetcher interface {
	Start(ctx context.Context) error
	ChainlinkReport(feedID string) *oracletypes.ChainlinkReport
}

type chainlinkFetcher struct {
	client       streams.Client
	stream       streams.Stream
	latestPrices map[string]*oracletypes.ChainlinkReport
	feedIDs      []string
	mu           sync.RWMutex

	logger  log.Logger
	svcTags metrics.Tags
}

// NewFetcher returns a new Fetcher instance.
func NewFetcher(client streams.Client, feedIds []string) (*chainlinkFetcher, error) {
	fetcher := &chainlinkFetcher{
		latestPrices: make(map[string]*oracletypes.ChainlinkReport),
		logger: log.WithFields(log.Fields{
			"svc":      "oracle",
			"dynamic":  true,
			"provider": "chainlinkFetcher",
		}),
		client:  client,
		feedIDs: feedIds,

		svcTags: metrics.Tags{
			"provider": "chainlinkFetcher",
		},
	}

	return fetcher, nil
}

func (f *chainlinkFetcher) logPrintf(format string, args ...interface{}) {
	f.logger.Infof(format, args...)
}

func (f *chainlinkFetcher) ChainlinkReport(feedID string) *oracletypes.ChainlinkReport {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.latestPrices[feedID]
}

func (f *chainlinkFetcher) Start(ctx context.Context) error {
	if len(f.feedIDs) == 0 {
		return errors.New("no feed IDs to subscribe to")
	}

	// Parse the feed IDs
	var ids []feed.ID
	for _, feedIDStr := range f.feedIDs {
		var fid feed.ID
		if err := fid.FromString(feedIDStr); err != nil {
			return errors.Wrapf(err, "invalid stream ID %s", feedIDStr)
		}

		ids = append(ids, fid)
	}

	f.logger.Infof("subscribing to %d Chainlink feed IDs: %v", len(ids), f.feedIDs)

	// Subscribe to the feeds
	stream, err := f.client.Stream(ctx, ids)
	if err != nil {
		return errors.Wrap(err, "failed to subscribe to Chainlink streams")
	}

	f.stream = stream
	f.logger.Infoln("successfully subscribed to Chainlink Data Streams")

	return f.startReadingReports(ctx)
}

func (f *chainlinkFetcher) startReadingReports(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			f.logger.Infoln("context cancelled, stopping Chainlink fetcher")
			return ctx.Err()
		default:
		}

		reportResponse, err := f.stream.Read(ctx)
		if err != nil {
			metrics.CustomReport(func(s metrics.Statter, tagSpec []string) {
				s.Count("feed_provider.chainlink.read_error.count", 1, tagSpec, 1)
			}, f.svcTags)
			f.logger.WithError(err).Warningln("error reading from Chainlink stream")
			continue
		}

		now := time.Now()

		// TODO this needs to be decoded per feedID config???
		decodedReport, decodeErr := report.Decode[v3.Data](reportResponse.FullReport)
		if decodeErr != nil {
			f.logger.WithError(decodeErr).Warningln("failed to decode Chainlink report")
			metrics.CustomReport(func(s metrics.Statter, tagSpec []string) {
				s.Count("feed_provider.chainlink.decode_error.count", 1, tagSpec, 1)
			}, f.svcTags)
			continue
		}

		// Calculate latency
		ts := time.Unix(int64(decodedReport.Data.ObservationsTimestamp), 0)
		latency := now.Sub(ts) / time.Millisecond

		metrics.CustomReport(func(s metrics.Statter, tagSpec []string) {
			s.Timing("feed_provider.chainlink.price_receive.latency", latency, tagSpec, 1)
			s.Count("feed_provider.chainlink.price_receive.count", 1, tagSpec, 1)
		}, f.svcTags)

		// Log the decoded report
		f.logger.WithFields(log.Fields{
			"feedID":                reportResponse.FeedID.String(),
			"observationsTimestamp": decodedReport.Data.ObservationsTimestamp,
			"benchmarkPrice":        decodedReport.Data.BenchmarkPrice.String(),
			"bid":                   decodedReport.Data.Bid.String(),
			"ask":                   decodedReport.Data.Ask.String(),
			"validFromTimestamp":    decodedReport.Data.ValidFromTimestamp,
			"expiresAt":             decodedReport.Data.ExpiresAt,
		}).Infoln("received Chainlink report")

		// Create complete PriceData
		priceData := &oracletypes.ChainlinkReport{
			FeedId:                common.Hex2Bytes(reportResponse.FeedID.String()),
			FullReport:            reportResponse.FullReport,
			ValidFromTimestamp:    reportResponse.ValidFromTimestamp,
			ObservationsTimestamp: reportResponse.ObservationsTimestamp,
		}

		// Update the latest prices
		f.mu.Lock()
		f.latestPrices[reportResponse.FeedID.String()] = priceData
		f.mu.Unlock()

		metrics.CustomReport(func(s metrics.Statter, tagSpec []string) {
			s.Count("feed_provider.chainlink.latest_pairs_update.count", 1, tagSpec, 1)
		}, f.svcTags)
	}
}

func (f *chainlinkFetcher) Close() error {
	if f.stream != nil {
		return f.stream.Close()
	}

	f.mu.Lock()
	f.latestPrices = make(map[string]*oracletypes.ChainlinkReport)
	f.mu.Unlock()
	f.logger.Infoln("Chainlink fetcher closed")

	return nil
}
