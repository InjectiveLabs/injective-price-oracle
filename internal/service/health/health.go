package health

import (
	"context"

	"github.com/InjectiveLabs/metrics"
	log "github.com/InjectiveLabs/suplog"

	injectivehealthapi "github.com/InjectiveLabs/injective-price-oracle/api/gen/health"
)

type Service struct {
	logger  log.Logger
	svcTags metrics.Tags
}

func NewHealthService(logger log.Logger, svcTags metrics.Tags) *Service {
	return &Service{
		logger:  logger,
		svcTags: svcTags,
	}
}

// GetStatus Get the latest block.
func (s *Service) GetStatus(_ context.Context) (res *injectivehealthapi.HealthStatusResponse, err error) {
	defer metrics.ReportFuncCallAndTimingWithErr(s.svcTags)(&err)

	return &injectivehealthapi.HealthStatusResponse{
		Errmsg: nil,
		Data:   &injectivehealthapi.HealthStatus{},
		S:      "ok",
		Status: "ok",
	}, nil
}
