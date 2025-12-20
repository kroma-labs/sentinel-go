package config

const (
	// Database configuration
	DefaultDSN         = "postgres://user:password@localhost:5588/example_db?sslmode=disable"
	DefaultDBSystem    = "postgresql"
	DefaultDBName      = "example_db"
	DefaultInstance    = "primary"
	DefaultMaxOpen     = 10
	DefaultMaxIdle     = 5
	DefaultMaxLifetime = 3600 // 1 hour in seconds
	DefaultMaxIdleTime = 900  // 15 minutes in seconds

	// Server configuration
	MetricsPort = ":2112"

	// OpenTelemetry configuration
	OTLPEndpoint   = "localhost:4317"
	ServiceName    = "sentinel-sqlx-example"
	ServiceVersion = "0.1.0"

	// Operation intervals
	OperationInterval = 5 // seconds
)
