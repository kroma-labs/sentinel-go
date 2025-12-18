package sql

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	// scope is the instrumentation scope name for OpenTelemetry.
	// This identifies the library in traces and metrics.
	scope = "github.com/kroma-labs/sentinel-go/sql"
)

// config holds the configuration for instrumentation.
type config struct {
	// TracerProvider is the tracer provider to use.
	// If not set, uses the global provider via otel.GetTracerProvider().
	// When no global provider is configured, a no-op tracer is used (safe, but no traces).
	TracerProvider trace.TracerProvider

	// MeterProvider is the meter provider to use.
	// If not set, uses the global provider via otel.GetMeterProvider().
	// When no global provider is configured, a no-op meter is used (safe, but no metrics).
	MeterProvider metric.MeterProvider

	// Tracer is the tracer instance created from TracerProvider.
	Tracer trace.Tracer

	// Meter is the meter instance created from MeterProvider.
	Meter metric.Meter

	// Metrics holds the metric instruments.
	Metrics *metrics

	// DBSystem identifies the database management system (DBMS) product.
	// Examples: "postgresql", "mysql", "sqlite", "mssql", "oracle"
	// See: https://opentelemetry.io/docs/specs/semconv/database/database-spans/
	DBSystem string

	// DBName is the name of the database being accessed.
	// Examples: "users_db", "orders", "analytics"
	// This helps distinguish between multiple databases on the same server.
	DBName string

	// InstanceName identifies a specific database connection instance.
	// Use this to distinguish between multiple connections to the same database,
	// such as primary/replica setups or read/write splits.
	//
	// Examples: "primary", "replica", "read", "write", "shard-1"
	//
	// This is added as the "db.instance" attribute on all spans, making it
	// easy to filter traces by connection type in your observability tool.
	InstanceName string

	// QuerySanitizer sanitizes SQL queries before adding to spans.
	// If nil, queries are included as-is (may expose sensitive data).
	//
	// Example using DefaultQuerySanitizer:
	//   Input:  "SELECT * FROM users WHERE id = 123 AND name = 'john'"
	//   Output: "SELECT * FROM users WHERE id = ? AND name = '?'"
	QuerySanitizer func(query string) string

	// DisableQuery disables recording of SQL queries in spans.
	// Use this for security if queries may contain sensitive data
	// and you cannot use a sanitizer.
	DisableQuery bool
}

// newConfig creates a new config with defaults and applies options.
func newConfig(opts ...Option) *config {
	cfg := &config{
		TracerProvider: otel.GetTracerProvider(),
		MeterProvider:  otel.GetMeterProvider(),
	}

	for _, opt := range opts {
		opt(cfg)
	}

	// Initialize tracer and meter after options are applied.
	// If no provider is configured globally, these will be no-op implementations
	// that safely do nothing - no errors, just no telemetry data collected.
	cfg.Tracer = cfg.TracerProvider.Tracer(scope)
	cfg.Meter = cfg.MeterProvider.Meter(scope)

	// Initialize metrics (ignore errors, will just be nil if fails)
	cfg.Metrics, _ = newMetrics(cfg.Meter)

	return cfg
}

// Option configures the instrumentation.
type Option func(*config)

// WithTracerProvider sets a custom tracer provider.
// If not called, the global provider from otel.GetTracerProvider() is used.
//
// Example:
//
//	tp := sdktrace.NewTracerProvider(...)
//	db, _ := sentinelsql.Open("postgres", dsn,
//	    sentinelsql.WithTracerProvider(tp),
//	)
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(cfg *config) {
		cfg.TracerProvider = tp
	}
}

// WithMeterProvider sets a custom meter provider.
// If not called, the global provider from otel.GetMeterProvider() is used.
//
// Example:
//
//	mp := sdkmetric.NewMeterProvider(...)
//	db, _ := sentinelsql.Open("postgres", dsn,
//	    sentinelsql.WithMeterProvider(mp),
//	)
func WithMeterProvider(mp metric.MeterProvider) Option {
	return func(cfg *config) {
		cfg.MeterProvider = mp
	}
}

// WithDBSystem sets the database system identifier (DBMS product).
// This is added as the "db.system" attribute on all spans.
//
// Common values:
//   - "postgresql" - PostgreSQL
//   - "mysql" - MySQL
//   - "sqlite" - SQLite
//   - "mssql" - Microsoft SQL Server
//   - "oracle" - Oracle Database
//
// Example:
//
//	db, _ := sentinelsql.Open("postgres", dsn,
//	    sentinelsql.WithDBSystem("postgresql"),
//	)
func WithDBSystem(system string) Option {
	return func(cfg *config) {
		cfg.DBSystem = system
	}
}

// WithDBName sets the database name being accessed.
// This is added as the "db.name" attribute on all spans.
//
// Example:
//
//	// Connecting to "users_db" database
//	db, _ := sentinelsql.Open("postgres", dsn,
//	    sentinelsql.WithDBName("users_db"),
//	)
func WithDBName(name string) Option {
	return func(cfg *config) {
		cfg.DBName = name
	}
}

// WithInstanceName sets an identifier for this specific database connection.
// This is added as the "db.instance" attribute on all spans.
//
// Use this to distinguish between multiple connections to the SAME database,
// such as:
//   - Primary/replica setups: "primary", "replica-1", "replica-2"
//   - Read/write splits: "read", "write"
//   - Sharded databases: "shard-0", "shard-1"
//   - Connection pools: "pool-a", "pool-b"
//
// This makes it easy to filter and analyze traces by connection type.
//
// Example - Primary/Replica setup:
//
//	// Primary connection for writes
//	writerDB, _ := sentinelsql.Open("postgres", primaryDSN,
//	    sentinelsql.WithDBSystem("postgresql"),
//	    sentinelsql.WithDBName("myapp"),
//	    sentinelsql.WithInstanceName("primary"),
//	)
//
//	// Replica connection for reads
//	readerDB, _ := sentinelsql.Open("postgres", replicaDSN,
//	    sentinelsql.WithDBSystem("postgresql"),
//	    sentinelsql.WithDBName("myapp"),
//	    sentinelsql.WithInstanceName("replica"),
//	)
//
// In your traces, you'll see:
//
//	Span: SELECT FROM users
//	├── db.system: postgresql
//	├── db.name: myapp
//	└── db.instance: replica  ← Easy to identify!
func WithInstanceName(name string) Option {
	return func(cfg *config) {
		cfg.InstanceName = name
	}
}

// WithQuerySanitizer sets a custom query sanitizer function.
// The sanitizer receives the raw SQL query and should return a sanitized version
// with sensitive data (like literals) replaced with placeholders.
//
// Use DefaultQuerySanitizer for a basic implementation that replaces
// string literals, numbers, and hex values with "?" placeholders.
//
// Example:
//
//	db, _ := sentinelsql.Open("postgres", dsn,
//	    sentinelsql.WithQuerySanitizer(sentinelsql.DefaultQuerySanitizer),
//	)
//	// Query: "SELECT * FROM users WHERE id = 123"
//	// Recorded as: "SELECT * FROM users WHERE id = ?"
func WithQuerySanitizer(fn func(string) string) Option {
	return func(cfg *config) {
		cfg.QuerySanitizer = fn
	}
}

// WithDisableQuery disables recording of SQL queries in spans entirely.
// Use this when queries may contain sensitive data and you cannot use a sanitizer.
//
// When enabled, the "db.statement" attribute will not be added to spans,
// but "db.operation" (SELECT, INSERT, etc.) will still be recorded.
//
// Example:
//
//	db, _ := sentinelsql.Open("postgres", dsn,
//	    sentinelsql.WithDisableQuery(),
//	)
func WithDisableQuery() Option {
	return func(cfg *config) {
		cfg.DisableQuery = true
	}
}
