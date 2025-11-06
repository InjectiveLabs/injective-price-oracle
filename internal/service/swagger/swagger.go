package swagger

import (
	swaggerAPI "github.com/InjectiveLabs/injective-price-oracle/api/gen/swagger"

	log "github.com/InjectiveLabs/suplog"
)

type SwaggerService = swaggerAPI.Service

// swaggerService example implementation.
// The example methods log the requests and return zero values.
type swaggerService struct {
	logger log.Logger
}

// NewSwaggerService returns the swagger service implementation.
func NewSwaggerService() SwaggerService {
	return &swaggerService{log.WithField("module", "swagger")}
}
