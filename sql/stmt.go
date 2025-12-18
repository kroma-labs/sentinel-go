package sql

import (
	"context"
	"database/sql/driver"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Compile-time interface checks.
var (
	_ driver.Stmt             = (*otelStmt)(nil)
	_ driver.StmtExecContext  = (*otelStmt)(nil)
	_ driver.StmtQueryContext = (*otelStmt)(nil)
)

// otelStmt wraps a driver.Stmt with OpenTelemetry instrumentation.
type otelStmt struct {
	stmt  driver.Stmt
	cfg   *config
	query string
}

// newOtelStmt creates a new instrumented statement.
func newOtelStmt(stmt driver.Stmt, cfg *config, query string) *otelStmt {
	return &otelStmt{
		stmt:  stmt,
		cfg:   cfg,
		query: query,
	}
}

// Close implements driver.Stmt.
func (s *otelStmt) Close() error {
	return s.stmt.Close()
}

// NumInput implements driver.Stmt.
func (s *otelStmt) NumInput() int {
	return s.stmt.NumInput()
}

// Exec implements driver.Stmt.
// Deprecated: Use ExecContext instead. This exists for driver.Stmt interface compatibility.
func (s *otelStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.stmt.Exec(args) //nolint:staticcheck // Required for driver.Stmt interface
}

// Query implements driver.Stmt.
// Deprecated: Use QueryContext instead. This exists for driver.Stmt interface compatibility.
func (s *otelStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.stmt.Query(args) //nolint:staticcheck // Required for driver.Stmt interface
}

// ExecContext implements driver.StmtExecContext.
func (s *otelStmt) ExecContext(
	ctx context.Context,
	args []driver.NamedValue,
) (driver.Result, error) {
	ctx, span := s.cfg.Tracer.Start(ctx, spanName(s.query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(s.cfg.queryAttributes(s.query)...),
	)
	defer span.End()

	var result driver.Result
	var err error

	if execer, ok := s.stmt.(driver.StmtExecContext); ok {
		result, err = execer.ExecContext(ctx, args)
	} else {
		// Fallback to non-context version
		values := namedValueToValue(args)
		result, err = s.stmt.Exec(values) //nolint:staticcheck // Fallback for older drivers
	}

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	return result, nil
}

// QueryContext implements driver.StmtQueryContext.
func (s *otelStmt) QueryContext(
	ctx context.Context,
	args []driver.NamedValue,
) (driver.Rows, error) {
	ctx, span := s.cfg.Tracer.Start(ctx, spanName(s.query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(s.cfg.queryAttributes(s.query)...),
	)
	defer span.End()

	var rows driver.Rows
	var err error

	if queryer, ok := s.stmt.(driver.StmtQueryContext); ok {
		rows, err = queryer.QueryContext(ctx, args)
	} else {
		// Fallback to non-context version
		values := namedValueToValue(args)
		rows, err = s.stmt.Query(values) //nolint:staticcheck // Fallback for older drivers
	}

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	return rows, nil
}

// namedValueToValue converts NamedValue slice to Value slice.
func namedValueToValue(named []driver.NamedValue) []driver.Value {
	values := make([]driver.Value, len(named))
	for i, nv := range named {
		values[i] = nv.Value
	}
	return values
}
