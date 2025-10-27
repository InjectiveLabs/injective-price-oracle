package design

import (
	. "goa.design/goa/v3/dsl"
)

// APIKeyAuth defines our security scheme
var APIKeyAuth = APIKeySecurity("api_key", func() {
	Scope("api:read", "Read-only access")
})
