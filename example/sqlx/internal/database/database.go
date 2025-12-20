package database

import (
	"context"
	"log"
	"time"

	"github.com/kroma-labs/sentinel-go/example/sqlx/internal/config"
	sentinelsqlx "github.com/kroma-labs/sentinel-go/sqlx"
	_ "github.com/lib/pq" // Register postgres driver

	"go.opentelemetry.io/otel"
)

// DB wraps the sqlx database connection with Sentinel instrumentation
type DB struct {
	*sentinelsqlx.DB
}

// New creates a new database connection with Sentinel SQLX instrumentation
func New(ctx context.Context) (*DB, error) {
	db, err := sentinelsqlx.Open("postgres", config.DefaultDSN,
		sentinelsqlx.WithDBSystem(config.DefaultDBSystem),
		sentinelsqlx.WithDBName(config.DefaultDBName),
		sentinelsqlx.WithInstanceName(config.DefaultInstance),
	)
	if err != nil {
		return nil, err
	}

	// Configure connection pool
	db.SetMaxOpenConns(config.DefaultMaxOpen)
	db.SetMaxIdleConns(config.DefaultMaxIdle)
	db.SetConnMaxLifetime(time.Duration(config.DefaultMaxLifetime) * time.Second)
	db.SetConnMaxIdleTime(time.Duration(config.DefaultMaxIdleTime) * time.Second)

	// Register Connection Pool Metrics
	// Attributes (db.system, db.name) are automatically detected from the driver!
	err = sentinelsqlx.RecordPoolMetrics(db, otel.GetMeterProvider().Meter("example-app"))
	if err != nil {
		log.Printf("Failed to register pool metrics: %v", err)
	}

	return &DB{DB: db}, nil
}
