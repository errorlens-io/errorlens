package ingest

// Wire-protocol identifiers.
//
// The standard open-source error SDKs send and read these exact header and
// parameter names, so they cannot be renamed without breaking every client.
// They are kept here, in one place, and never shown in the product UI.
const (
	hdrAuth        = "X-Sentry-Auth"
	hdrError       = "X-Sentry-Error"
	hdrRateLimits  = "X-Sentry-Rate-Limits"
	hdrTrace       = "sentry-trace"
	authScheme     = "sentry "
	paramPublicKey = "sentry_key"
)
