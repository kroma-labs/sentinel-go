package database

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/kroma-labs/sentinel-go/example/sql/internal/config"
	sentinelsql "github.com/kroma-labs/sentinel-go/sql"
	_ "github.com/lib/pq" // Register postgres driver

	"go.opentelemetry.io/otel"
)

// DB wraps the database connection
type DB struct {
	*sql.DB
}

// New creates a new database connection with Sentinel instrumentation
func New(ctx context.Context) (*DB, error) {
	db, err := sentinelsql.Open("postgres", config.DefaultDSN,
		sentinelsql.WithDBSystem(config.DefaultDBSystem),
		sentinelsql.WithDBName(config.DefaultDBName),
		sentinelsql.WithInstanceName(config.DefaultInstance),
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
	err = sentinelsql.RecordPoolMetrics(db, otel.GetMeterProvider().Meter("example-app"))
	if err != nil {
		log.Printf("Failed to register pool metrics: %v", err)
	}

	return &DB{DB: db}, nil
}
