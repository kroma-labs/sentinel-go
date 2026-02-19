package config

const (
	// Server configuration
	Addr        = ":8080"
	MetricsPort = ":2113"

	// OpenTelemetry configuration
	OTLPEndpoint   = "localhost:4319"
	ServiceName    = "sentinel-httpserver-example"
	ServiceVersion = "0.1.0"
	ConfigVersion  = "v1.0.0"
)
