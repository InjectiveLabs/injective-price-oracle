package design

import (
	. "goa.design/goa/v3/dsl"
	cors "goa.design/plugins/v3/cors/dsl"
	_ "goa.design/plugins/v3/docs"
)

var _ = Service("health", func() {
	Description("HealthAPI allows to check if backend data is up-to-date and reliable or not.")

	Error("internal", ErrorResult, "Internal Server Error")

	HTTP(func() {
		Path("/api/health/v1")
	})
	GRPC(func() {
		Package("injective.price.oracle.api.v1")
	})
	// Sets CORS response headers for requests with Origin header matching the string "localhost"
	cors.Origin("*", func() {
		cors.Methods("GET")
		cors.Headers("Content-Type")
	})

	Method("GetStatus", func() {
		Description("Get current backend health status")

		Result(HealthStatusResponse)

		HTTP(func() {
			GET("/status")

			Response(StatusOK)
			Response("internal", StatusInternalServerError)
		})

		GRPC(func() {
			Response(CodeOK)
			Response("internal", CodeInternal)
		})
	})

})

var HealthStatusResponse = Type("HealthStatusResponse", func() {
	Reference(BaseHealthResponse)
	Field(1, "s")
	Field(2, "errmsg")
	Field(3, "data", HealthStatus)
	Field(4, "status")

	Required("s", "status")
})

var HealthStatus = Type("HealthStatus", func() {
	Description("Status defines the structure for health information")
})

var BaseHealthResponse = Type("BaseHealthResponse", func() {
	Description("Base response of Health API.")

	Field(1, "s", String, func() {
		Description("Status of the response.")
		Example("error")
		Enum(
			"ok",
			"error",
			"no_data",
		)
	})

	Field(2, "errmsg", String, func() {
		Description("Error message.")
		Example("Something has failed")
	})
})
