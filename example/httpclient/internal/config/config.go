package config

const (
	// Server configuration
	MetricsPort = ":2112"

	// OpenTelemetry configuration
	OTLPEndpoint   = "localhost:4317"
	ServiceName    = "sentinel-httpclient-example"
	ServiceVersion = "0.1.0"

	// Operation intervals
	OperationInterval = 5 // seconds

	// Mock API configuration (using httpbin for testing)
	MockAPIBaseURL = "http://localhost:8080"
)
