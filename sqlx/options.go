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
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(cfg *config) {
		cfg.TracerProvider = tp
	}
}

// WithMeterProvider sets a custom meter provider.
func WithMeterProvider(mp metric.MeterProvider) Option {
	return func(cfg *config) {
		cfg.MeterProvider = mp
	}
}

// WithDBSystem sets the database system identifier.
func WithDBSystem(system string) Option {
	return func(cfg *config) {
		cfg.DBSystem = system
	}
}

// WithDBName sets the database name.
func WithDBName(name string) Option {
	return func(cfg *config) {
		cfg.DBName = name
	}
}

// WithInstanceName sets the database instance identifier.
func WithInstanceName(name string) Option {
	return func(cfg *config) {
		cfg.InstanceName = name
	}
}

// WithQuerySanitizer sets a custom query sanitizer function.
func WithQuerySanitizer(fn func(string) string) Option {
	return func(cfg *config) {
		cfg.QuerySanitizer = fn
	}
}

// WithDisableQuery disables recording of SQL queries in spans.
func WithDisableQuery() Option {
	return func(cfg *config) {
		cfg.DisableQuery = true
	}
}
