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
		Host("mainnet", func() {
			URI("https://k8s.mainnet.price-oracle.injective.network")
		})
		Host("testnet", func() {
			URI("https://k8s.mainnet.staging.price-oracle.injective.network")
		})
	})
})

var _ = Service("Swagger", func() {
	Description("The API Swagger specification")

	HTTP(func() {
		Path("/swagger")
	})

	Files("/index.html", "./swagger/index.html", func() {
		Description("Provide Swagger-UI html document")
	})

	Files("/{*path}", "./swagger/", func() {
		Description("Serve static content.")
	})
})
