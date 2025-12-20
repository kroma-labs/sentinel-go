// Package sqlx provides an instrumented sqlx wrapper
// with automatic OpenTelemetry tracing and metrics.
//
// This package wraps github.com/jmoiron/sqlx to provide automatic
// instrumentation for sqlx-specific methods like Get, Select, NamedExec,
// and NamedQuery, in addition to the standard database/sql operations.
//
// Usage:
//
//	import sentinelsqlx "github.com/kroma-labs/sentinel-go/sqlx"
//
//	db, err := sentinelsqlx.Open("postgres", dsn,
//	    sentinelsqlx.WithDBSystem("postgresql"),
//	    sentinelsqlx.WithDBName("myapp"),
//	)
//	// db is *sentinelsqlx.DB - wraps *sqlx.DB with instrumentation
//
//	var user User
//	err = db.GetContext(ctx, &user, "SELECT * FROM users WHERE id = $1", 1)
//
//	var users []User
//	err = db.SelectContext(ctx, &users, "SELECT * FROM users")
package sqlx

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	// scope is the instrumentation scope name for OpenTelemetry.
	scope = "github.com/kroma-labs/sentinel-go/sqlx"
)

// config holds the configuration for instrumentation.
type config struct {
	// TracerProvider is the tracer provider to use.
	TracerProvider trace.TracerProvider

	// MeterProvider is the meter provider to use.
	MeterProvider metric.MeterProvider

	// Tracer is the tracer instance.
	Tracer trace.Tracer

	// Meter is the meter instance.
	Meter metric.Meter

	// Metrics holds the metric instruments.
	Metrics *metrics

	// DBSystem identifies the database management system.
	DBSystem string

	// DBName is the name of the database.
	DBName string

	// InstanceName identifies a specific database instance.
	InstanceName string

	// QuerySanitizer sanitizes SQL queries before adding to spans.
	QuerySanitizer func(query string) string

	// DisableQuery disables recording of SQL queries in spans.
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

	cfg.Tracer = cfg.TracerProvider.Tracer(scope)
	cfg.Meter = cfg.MeterProvider.Meter(scope)
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
//	db, _ := sentinelsqlx.Open("postgres", dsn,
//	    sentinelsqlx.WithTracerProvider(tp),
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
//	db, _ := sentinelsqlx.Open("postgres", dsn,
//	    sentinelsqlx.WithMeterProvider(mp),
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
//	db, _ := sentinelsqlx.Open("postgres", dsn,
//	    sentinelsqlx.WithDBSystem("postgresql"),
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
//	db, _ := sentinelsqlx.Open("postgres", dsn,
//	    sentinelsqlx.WithDBName("users_db"),
//	)
func WithDBName(name string) Option {
	return func(cfg *config) {
		cfg.DBName = name
	}
}

// WithInstanceName sets an identifier for this specific database connection.
// This is added as the "db.instance" attribute on all spans.
//
// Use this to distinguish between multiple connections to the SAME database:
//   - Primary/replica setups: "primary", "replica-1"
//   - Read/write splits: "read", "write"
//   - Sharded databases: "shard-0", "shard-1"
//
// Example:
//
//	// Primary for writes
//	writerDB, _ := sentinelsqlx.Open("postgres", primaryDSN,
//	    sentinelsqlx.WithInstanceName("primary"),
//	)
//
//	// Replica for reads
//	readerDB, _ := sentinelsqlx.Open("postgres", replicaDSN,
//	    sentinelsqlx.WithInstanceName("replica"),
//	)
func WithInstanceName(name string) Option {
	return func(cfg *config) {
		cfg.InstanceName = name
	}
}

// WithQuerySanitizer sets a custom query sanitizer function.
// The sanitizer receives the raw SQL query and should return a sanitized version
// with sensitive data (like literals) replaced with placeholders.
//
// Use DefaultQuerySanitizer for a basic implementation:
//
//	db, _ := sentinelsqlx.Open("postgres", dsn,
//	    sentinelsqlx.WithQuerySanitizer(sentinelsqlx.DefaultQuerySanitizer),
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
// The "db.statement" attribute will not be added to spans,
// but "db.operation" (SELECT, INSERT, etc.) will still be recorded.
//
// Example:
//
//	db, _ := sentinelsqlx.Open("postgres", dsn,
//	    sentinelsqlx.WithDisableQuery(),
//	)
func WithDisableQuery() Option {
	return func(cfg *config) {
		cfg.DisableQuery = true
	}
}
