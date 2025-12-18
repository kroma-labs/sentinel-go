package sqlx

import (
	"context"
	"database/sql"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// metrics holds the metric instruments for database operations.
type metrics struct {
	// Query latency histogram
	queryDuration metric.Float64Histogram

	// Connection pool gauges
	openConnections metric.Int64ObservableGauge
	idleConnections metric.Int64ObservableGauge
	maxConnections  metric.Int64ObservableGauge
	usedConnections metric.Int64ObservableGauge
	waitCount       metric.Int64ObservableCounter
	waitDuration    metric.Float64ObservableCounter
}

// newMetrics creates and registers metric instruments.
func newMetrics(meter metric.Meter) (*metrics, error) {
	m := &metrics{}
	var err error

	m.queryDuration, err = meter.Float64Histogram(
		"db.client.operation.duration",
		metric.WithDescription("Duration of database client operations in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.001, 0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 10,
		),
	)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// recordQueryDuration records the duration of a query operation.
func (m *metrics) recordQueryDuration(
	ctx context.Context,
	duration time.Duration,
	operation string,
	attrs []attribute.KeyValue,
	err error,
) {
	if m == nil || m.queryDuration == nil {
		return
	}

	allAttrs := make([]attribute.KeyValue, 0, len(attrs)+2)
	allAttrs = append(allAttrs, attrs...)

	if operation != "" {
		allAttrs = append(allAttrs, attribute.String("db.operation", operation))
	}

	status := "ok"
	if err != nil {
		status = "error"
	}
	allAttrs = append(allAttrs, attribute.String("status", status))

	m.queryDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(allAttrs...))
}

// registerPoolMetrics registers connection pool metrics with callbacks.
func (m *metrics) registerPoolMetrics(
	meter metric.Meter,
	db *sql.DB,
	attrs []attribute.KeyValue,
) error {
	var err error

	m.openConnections, err = meter.Int64ObservableGauge(
		"db.client.connections.open",
		metric.WithDescription("Number of open connections in the pool"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return err
	}

	m.idleConnections, err = meter.Int64ObservableGauge(
		"db.client.connections.idle",
		metric.WithDescription("Number of idle connections in the pool"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return err
	}

	m.maxConnections, err = meter.Int64ObservableGauge(
		"db.client.connections.max",
		metric.WithDescription("Maximum number of connections allowed in the pool"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return err
	}

	m.usedConnections, err = meter.Int64ObservableGauge(
		"db.client.connections.used",
		metric.WithDescription("Number of connections currently in use"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return err
	}

	m.waitCount, err = meter.Int64ObservableCounter(
		"db.client.connections.wait_count",
		metric.WithDescription("Total number of times waited for a connection"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		return err
	}

	m.waitDuration, err = meter.Float64ObservableCounter(
		"db.client.connections.wait_duration",
		metric.WithDescription("Total time waited for connections in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	_, err = meter.RegisterCallback(
		func(_ context.Context, o metric.Observer) error {
			stats := db.Stats()

			o.ObserveInt64(m.openConnections, int64(stats.OpenConnections),
				metric.WithAttributes(attrs...))
			o.ObserveInt64(m.idleConnections, int64(stats.Idle),
				metric.WithAttributes(attrs...))
			o.ObserveInt64(m.maxConnections, int64(stats.MaxOpenConnections),
				metric.WithAttributes(attrs...))
			o.ObserveInt64(m.usedConnections, int64(stats.InUse),
				metric.WithAttributes(attrs...))
			o.ObserveInt64(m.waitCount, stats.WaitCount,
				metric.WithAttributes(attrs...))
			o.ObserveFloat64(m.waitDuration, stats.WaitDuration.Seconds(),
				metric.WithAttributes(attrs...))

			return nil
		},
		m.openConnections,
		m.idleConnections,
		m.maxConnections,
		m.usedConnections,
		m.waitCount,
		m.waitDuration,
	)

	return err
}

// RecordPoolMetrics registers connection pool metrics for a sqlx database.
//
// Example:
//
//	db, _ := sentinelsqlx.Open("postgres", dsn,
//	    sentinelsqlx.WithDBSystem("postgresql"),
//	    sentinelsqlx.WithDBName("mydb"),
//	)
//
//	err := sentinelsqlx.RecordPoolMetrics(db, otel.GetMeterProvider().Meter("myapp"))
func RecordPoolMetrics(db *DB, meter metric.Meter, attrs ...attribute.KeyValue) error {
	m := &metrics{}

	// Use config attributes from the wrapped DB
	if db.cfg != nil {
		baseAttrs := db.cfg.baseAttributes()
		attrs = append(baseAttrs, attrs...)
	}

	return m.registerPoolMetrics(meter, db.DB.DB, attrs)
}
