//go:generate goa gen github.com/InjectiveLabs/injective-price-oracle/api/design -o ../
package design

import (
	. "goa.design/goa/v3/dsl"
	_ "goa.design/plugins/v3/docs"
)

// API describes the global properties of the API server.
var _ = API("PriceOracle", func() {
	Title("PriceOracle Service")
	Description("HTTP server for the Price Oracle API")
	Server("PriceOracle", func() {
		Host("0.0.0.0", func() {
			URI("http://0.0.0.0:9924")
			URI("grpc://0.0.0.0:9881")
		})
	})
})
