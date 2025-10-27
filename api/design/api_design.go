package design

import (
	. "goa.design/goa/v3/dsl"
	cors "goa.design/plugins/v3/cors/dsl"
	_ "goa.design/plugins/v3/docs"
)

var _ = Service("Injective Price Oracle API", func() {
	Description("Injective-Price-Oracle services API doc")

	HTTP(func() {
		Path("/api/price-oracle/v1")
	})

	cors.Origin("*", func() {
		cors.Methods("POST")
		cors.Headers("Content-Type")
	})

	Error("invalid_arg", ErrorResult, "Invalid argument")
	Error("not_found", ErrorResult, "Not found")
	Error("internal", ErrorResult, "Internal Server Error")
	Error("unauthorized", ErrorResult, "Unauthorized")

	Method("probe", func() {
		Security(APIKeyAuth)
		Description("Validate TOML file")
		Payload(func() {
			APIKey("api_key", "key", String, "API key for authentication")
			Field(1, "content", Bytes, "TOML file contents")
			Required("content")
		})

		Result(ProbeResponse)

		HTTP(func() {
			Header("key:X-Api-Key")
			POST("/probe")
			MultipartRequest()
			Response(StatusOK)
			Response("invalid_arg", StatusBadRequest)
			Response("internal", StatusInternalServerError)
			Response("unauthorized", StatusUnauthorized)

		})
	})
})

var ProbeResponse = Type("ProbeResponse", func() {
	Field(1, "result", String, func() {
		Description("Result of the probe")
	})

	Required(
		"result",
	)
})
