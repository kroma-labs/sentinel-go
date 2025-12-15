// Package sql provides an instrumented database/sql driver wrapper
// with automatic OpenTelemetry tracing and metrics.
//
// Usage:
//
//	import sentinelsql "github.com/kroma-labs/sentinel-go/sql"
//
//	db, err := sentinelsql.Open("postgres", dsn,
//	    sentinelsql.WithDBSystem("postgresql"),
//	    sentinelsql.WithDBName("myapp"),
//	)
//	// db is *sql.DB - fully compatible with stdlib
//	rows, _ := db.QueryContext(ctx, "SELECT * FROM users")
package sql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync"
)

// Compile-time interface checks.
var (
	_ driver.Driver        = (*otelDriver)(nil)
	_ driver.DriverContext = (*otelDriver)(nil)
	_ driver.Connector     = (*otelConnector)(nil)
	_ driver.Connector     = (*dsnConnector)(nil)
)

// Driver registration state.
// Go's sql.Register is process-wide and panics on duplicate names.
// We use a registry to track wrapped drivers and reuse them when possible.
var (
	registryMu sync.RWMutex
	registry   = make(map[string]*otelDriver)
)

// Open wraps the specified driver and opens a database connection.
// It returns a standard *sql.DB that is fully compatible with database/sql.
// All operations will be automatically traced and metered.
//
// The driver is registered once per (driverName, options) combination.
// Subsequent calls with the same driver name and options reuse the registration.
//
// Example:
//
//	db, err := sentinelsql.Open("postgres",
//	    "postgres://user:pass@localhost/mydb?sslmode=disable",
//	    sentinelsql.WithDBSystem("postgresql"),
//	    sentinelsql.WithDBName("mydb"),
//	)
func Open(driverName, dsn string, opts ...Option) (*sql.DB, error) {
	// Create config to generate a deterministic key
	cfg := newConfig(opts...)

	// Generate a unique but deterministic driver name based on config
	wrappedName := fmt.Sprintf("otel:%s:%s:%s", driverName, cfg.DBSystem, cfg.DBName)

	// Check if already registered
	registryMu.RLock()
	_, exists := registry[wrappedName]
	registryMu.RUnlock()

	if !exists {
		// Get the original driver
		db, err := sql.Open(driverName, dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open database: %w", err)
		}
		originalDriver := db.Driver()
		db.Close()

		// Create and register the wrapped driver
		wrapped := &otelDriver{
			driver: originalDriver,
			cfg:    cfg,
		}

		registryMu.Lock()
		// Double-check after acquiring write lock
		if _, exists := registry[wrappedName]; !exists {
			registry[wrappedName] = wrapped
			sql.Register(wrappedName, wrapped)
		}
		registryMu.Unlock()
	}

	// Open using the wrapped driver
	return sql.Open(wrappedName, dsn)
}

// WrapDriver wraps a driver.Driver with OpenTelemetry instrumentation.
// Use this when you need more control over driver registration.
//
// Example:
//
//	wrapped := sentinelsql.WrapDriver(myDriver,
//	    sentinelsql.WithDBSystem("postgresql"),
//	)
//	sql.Register("my-otel-driver", wrapped)
func WrapDriver(d driver.Driver, opts ...Option) driver.Driver {
	cfg := newConfig(opts...)
	return &otelDriver{
		driver: d,
		cfg:    cfg,
	}
}

// Register registers a wrapped driver with the given name.
// This is useful when you want to control the driver name explicitly.
//
// Example:
//
//	sentinelsql.Register("otel-postgres", pgDriver,
//	    sentinelsql.WithDBSystem("postgresql"),
//	)
//	db, _ := sql.Open("otel-postgres", dsn)
func Register(name string, d driver.Driver, opts ...Option) {
	wrapped := WrapDriver(d, opts...)
	sql.Register(name, wrapped)
}

// otelDriver wraps a driver.Driver with OpenTelemetry instrumentation.
type otelDriver struct {
	driver driver.Driver
	cfg    *config
}

// Open implements driver.Driver.
func (d *otelDriver) Open(name string) (driver.Conn, error) {
	conn, err := d.driver.Open(name)
	if err != nil {
		return nil, err
	}
	return newOtelConn(conn, d.cfg), nil
}

// OpenConnector implements driver.DriverContext.
func (d *otelDriver) OpenConnector(name string) (driver.Connector, error) {
	if dc, ok := d.driver.(driver.DriverContext); ok {
		connector, err := dc.OpenConnector(name)
		if err != nil {
			return nil, err
		}
		return &otelConnector{
			connector: connector,
			driver:    d,
			cfg:       d.cfg,
		}, nil
	}
	// Fallback for drivers that don't implement DriverContext
	return &dsnConnector{
		dsn:    name,
		driver: d,
	}, nil
}

// otelConnector wraps a driver.Connector with instrumentation.
type otelConnector struct {
	connector driver.Connector
	driver    *otelDriver
	cfg       *config
}

// Connect implements driver.Connector.
func (c *otelConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return newOtelConn(conn, c.cfg), nil
}

// Driver implements driver.Connector.
func (c *otelConnector) Driver() driver.Driver {
	return c.driver
}

// dsnConnector is a fallback connector for drivers that don't implement DriverContext.
type dsnConnector struct {
	dsn    string
	driver *otelDriver
}

// Connect implements driver.Connector.
func (c *dsnConnector) Connect(_ context.Context) (driver.Conn, error) {
	conn, err := c.driver.driver.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return newOtelConn(conn, c.driver.cfg), nil
}

// Driver implements driver.Connector.
func (c *dsnConnector) Driver() driver.Driver {
	return c.driver
}
