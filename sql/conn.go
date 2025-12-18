package sql

import (
	"context"
	"database/sql/driver"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Compile-time interface checks.
var (
	_ driver.Conn               = (*otelConn)(nil)
	_ driver.ConnPrepareContext = (*otelConn)(nil)
	_ driver.ConnBeginTx        = (*otelConn)(nil)
	_ driver.ExecerContext      = (*otelConn)(nil)
	_ driver.QueryerContext     = (*otelConn)(nil)
	_ driver.Pinger             = (*otelConn)(nil)
	_ driver.SessionResetter    = (*otelConn)(nil)
	_ driver.Validator          = (*otelConn)(nil)
)

// otelConn wraps a driver.Conn with OpenTelemetry instrumentation.
type otelConn struct {
	conn driver.Conn
	cfg  *config
}

// newOtelConn creates a new instrumented connection.
func newOtelConn(conn driver.Conn, cfg *config) *otelConn {
	return &otelConn{
		conn: conn,
		cfg:  cfg,
	}
}

// Prepare implements driver.Conn.
func (c *otelConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	return newOtelStmt(stmt, c.cfg, query), nil
}

// Close implements driver.Conn.
func (c *otelConn) Close() error {
	return c.conn.Close()
}

// Begin implements driver.Conn.
// Deprecated: Use BeginTx instead. This exists for driver.Conn interface compatibility.
func (c *otelConn) Begin() (driver.Tx, error) {
	tx, err := c.conn.Begin() //nolint:staticcheck // Required for driver.Conn interface
	if err != nil {
		return nil, err
	}
	return newOtelTx(tx, c.cfg), nil
}

// PrepareContext implements driver.ConnPrepareContext.
func (c *otelConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	var stmt driver.Stmt
	var err error

	if preparer, ok := c.conn.(driver.ConnPrepareContext); ok {
		stmt, err = preparer.PrepareContext(ctx, query)
	} else {
		stmt, err = c.conn.Prepare(query)
	}

	if err != nil {
		return nil, err
	}
	return newOtelStmt(stmt, c.cfg, query), nil
}

// BeginTx implements driver.ConnBeginTx.
func (c *otelConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	start := time.Now()
	ctx, span := c.cfg.Tracer.Start(ctx, "BEGIN",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(c.cfg.baseAttributes()...),
	)
	defer span.End()

	var tx driver.Tx
	var err error

	if beginner, ok := c.conn.(driver.ConnBeginTx); ok {
		tx, err = beginner.BeginTx(ctx, opts)
	} else {
		tx, err = c.conn.Begin() //nolint:staticcheck // Fallback for older drivers
	}

	// Record metrics
	c.cfg.Metrics.recordQueryDuration(ctx, time.Since(start), "BEGIN", c.cfg.baseAttributes(), err)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	return newOtelTx(tx, c.cfg), nil
}

// ExecContext implements driver.ExecerContext.
func (c *otelConn) ExecContext(
	ctx context.Context,
	query string,
	args []driver.NamedValue,
) (driver.Result, error) {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := c.cfg.Tracer.Start(ctx, spanName(query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(c.cfg.queryAttributes(query)...),
	)
	defer span.End()

	if execer, ok := c.conn.(driver.ExecerContext); ok {
		result, err := execer.ExecContext(ctx, query, args)

		// Record metrics
		c.cfg.Metrics.recordQueryDuration(
			ctx,
			time.Since(start),
			operation,
			c.cfg.baseAttributes(),
			err,
		)

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		return result, nil
	}

	// Fallback: prepare and execute
	return nil, driver.ErrSkip
}

// QueryContext implements driver.QueryerContext.
func (c *otelConn) QueryContext(
	ctx context.Context,
	query string,
	args []driver.NamedValue,
) (driver.Rows, error) {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := c.cfg.Tracer.Start(ctx, spanName(query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(c.cfg.queryAttributes(query)...),
	)
	defer span.End()

	if queryer, ok := c.conn.(driver.QueryerContext); ok {
		rows, err := queryer.QueryContext(ctx, query, args)

		// Record metrics
		c.cfg.Metrics.recordQueryDuration(
			ctx,
			time.Since(start),
			operation,
			c.cfg.baseAttributes(),
			err,
		)

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		return rows, nil
	}

	// Fallback: let database/sql handle it
	return nil, driver.ErrSkip
}

// Ping implements driver.Pinger.
func (c *otelConn) Ping(ctx context.Context) error {
	start := time.Now()
	ctx, span := c.cfg.Tracer.Start(ctx, "PING",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(c.cfg.baseAttributes()...),
	)
	defer span.End()

	var err error
	if pinger, ok := c.conn.(driver.Pinger); ok {
		err = pinger.Ping(ctx)
	}

	// Record metrics
	c.cfg.Metrics.recordQueryDuration(ctx, time.Since(start), "PING", c.cfg.baseAttributes(), err)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

// ResetSession implements driver.SessionResetter.
func (c *otelConn) ResetSession(ctx context.Context) error {
	if resetter, ok := c.conn.(driver.SessionResetter); ok {
		return resetter.ResetSession(ctx)
	}
	return nil
}

// IsValid implements driver.Validator.
func (c *otelConn) IsValid() bool {
	if validator, ok := c.conn.(driver.Validator); ok {
		return validator.IsValid()
	}
	return true
}

// baseAttributes returns the base attributes for all spans and metrics.
func (cfg *config) baseAttributes() []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 3)
	if cfg.DBSystem != "" {
		attrs = append(attrs, attribute.String("db.system", cfg.DBSystem))
	}
	if cfg.DBName != "" {
		attrs = append(attrs, attribute.String("db.name", cfg.DBName))
	}
	if cfg.InstanceName != "" {
		attrs = append(attrs, attribute.String("db.instance", cfg.InstanceName))
	}
	return attrs
}

// queryAttributes returns attributes for query spans.
func (cfg *config) queryAttributes(query string) []attribute.KeyValue {
	attrs := cfg.baseAttributes()

	if !cfg.DisableQuery && query != "" {
		sanitized := query
		if cfg.QuerySanitizer != nil {
			sanitized = cfg.QuerySanitizer(query)
		}
		attrs = append(attrs, attribute.String("db.statement", sanitized))
	}

	// Extract operation from query
	op := extractOperation(query)
	if op != "" {
		attrs = append(attrs, attribute.String("db.operation", op))
	}

	return attrs
}
