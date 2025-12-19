package sqlx

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Stmt wraps *sqlx.Stmt with OpenTelemetry instrumentation.
type Stmt struct {
	*sqlx.Stmt
	cfg   *config
	query string
}

// GetContext executes the prepared statement for a single row.
func (s *Stmt) GetContext(ctx context.Context, dest interface{}, args ...interface{}) error {
	start := time.Now()
	operation := extractOperation(s.query)

	ctx, span := s.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.Stmt.Get", s.query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(s.cfg.queryAttributes(s.query)...),
	)
	defer span.End()

	err := s.Stmt.GetContext(ctx, dest, args...)

	s.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		s.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}

// SelectContext executes the prepared statement and scans results into dest.
func (s *Stmt) SelectContext(ctx context.Context, dest interface{}, args ...interface{}) error {
	start := time.Now()
	operation := extractOperation(s.query)

	ctx, span := s.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.Stmt.Select", s.query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(s.cfg.queryAttributes(s.query)...),
	)
	defer span.End()

	err := s.Stmt.SelectContext(ctx, dest, args...)

	s.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		s.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}

// ExecContext executes the prepared statement.
func (s *Stmt) ExecContext(ctx context.Context, args ...interface{}) (sql.Result, error) {
	start := time.Now()
	operation := extractOperation(s.query)

	ctx, span := s.cfg.Tracer.Start(ctx, spanName(s.query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(s.cfg.queryAttributes(s.query)...),
	)
	defer span.End()

	result, err := s.Stmt.ExecContext(ctx, args...)

	s.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		s.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return result, err
}

// QueryContext executes the prepared statement and returns rows.
func (s *Stmt) QueryContext(ctx context.Context, args ...interface{}) (*sql.Rows, error) {
	start := time.Now()
	operation := extractOperation(s.query)

	ctx, span := s.cfg.Tracer.Start(ctx, spanName(s.query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(s.cfg.queryAttributes(s.query)...),
	)
	defer span.End()

	rows, err := s.Stmt.QueryContext(ctx, args...)

	s.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		s.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return rows, err
}

// QueryRowContext executes the prepared statement and returns a single row.
func (s *Stmt) QueryRowContext(ctx context.Context, args ...interface{}) *sql.Row {
	start := time.Now()
	operation := extractOperation(s.query)

	ctx, span := s.cfg.Tracer.Start(ctx, spanName(s.query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(s.cfg.queryAttributes(s.query)...),
	)
	defer span.End()

	row := s.Stmt.QueryRowContext(ctx, args...)

	s.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		s.cfg.baseAttributes(),
		nil,
	)

	return row
}

// QueryxContext executes the prepared statement and returns sqlx.Rows.
func (s *Stmt) QueryxContext(ctx context.Context, args ...interface{}) (*sqlx.Rows, error) {
	start := time.Now()
	operation := extractOperation(s.query)

	ctx, span := s.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.Stmt.Queryx", s.query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(s.cfg.queryAttributes(s.query)...),
	)
	defer span.End()

	rows, err := s.Stmt.QueryxContext(ctx, args...)

	s.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		s.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return rows, err
}

// QueryRowxContext executes the prepared statement and returns sqlx.Row.
func (s *Stmt) QueryRowxContext(ctx context.Context, args ...interface{}) *sqlx.Row {
	start := time.Now()
	operation := extractOperation(s.query)

	ctx, span := s.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.Stmt.QueryRowx", s.query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(s.cfg.queryAttributes(s.query)...),
	)
	defer span.End()

	row := s.Stmt.QueryRowxContext(ctx, args...)

	s.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		s.cfg.baseAttributes(),
		nil,
	)

	return row
}

// Unsafe returns a version of Stmt that silently ignores missing destination fields.
func (s *Stmt) Unsafe() *Stmt {
	return &Stmt{
		Stmt:  s.Stmt.Unsafe(),
		cfg:   s.cfg,
		query: s.query,
	}
}

// NamedStmt wraps *sqlx.NamedStmt with OpenTelemetry instrumentation.
type NamedStmt struct {
	*sqlx.NamedStmt
	cfg   *config
	query string
}

// GetContext executes the named statement for a single row.
func (ns *NamedStmt) GetContext(ctx context.Context, dest interface{}, arg interface{}) error {
	start := time.Now()
	operation := extractOperation(ns.query)

	ctx, span := ns.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.NamedStmt.Get", ns.query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(ns.cfg.queryAttributes(ns.query)...),
	)
	defer span.End()

	err := ns.NamedStmt.GetContext(ctx, dest, arg)

	ns.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		ns.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}

// SelectContext executes the named statement and scans results into dest.
func (ns *NamedStmt) SelectContext(ctx context.Context, dest interface{}, arg interface{}) error {
	start := time.Now()
	operation := extractOperation(ns.query)

	ctx, span := ns.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.NamedStmt.Select", ns.query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(ns.cfg.queryAttributes(ns.query)...),
	)
	defer span.End()

	err := ns.NamedStmt.SelectContext(ctx, dest, arg)

	ns.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		ns.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}

// ExecContext executes the named statement.
func (ns *NamedStmt) ExecContext(ctx context.Context, arg interface{}) (sql.Result, error) {
	start := time.Now()
	operation := extractOperation(ns.query)

	ctx, span := ns.cfg.Tracer.Start(ctx, spanName(ns.query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(ns.cfg.queryAttributes(ns.query)...),
	)
	defer span.End()

	result, err := ns.NamedStmt.ExecContext(ctx, arg)

	ns.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		ns.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return result, err
}

// QueryContext executes the named statement and returns rows.
func (ns *NamedStmt) QueryContext(ctx context.Context, arg interface{}) (*sql.Rows, error) {
	start := time.Now()
	operation := extractOperation(ns.query)

	ctx, span := ns.cfg.Tracer.Start(ctx, spanName(ns.query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(ns.cfg.queryAttributes(ns.query)...),
	)
	defer span.End()

	rows, err := ns.NamedStmt.QueryContext(ctx, arg)

	ns.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		ns.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return rows, err
}

// QueryRowContext executes the named statement and returns a single row.
func (ns *NamedStmt) QueryRowContext(ctx context.Context, arg interface{}) *sqlx.Row {
	start := time.Now()
	operation := extractOperation(ns.query)

	ctx, span := ns.cfg.Tracer.Start(ctx, spanName(ns.query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(ns.cfg.queryAttributes(ns.query)...),
	)
	defer span.End()

	row := ns.NamedStmt.QueryRowContext(ctx, arg)

	ns.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		ns.cfg.baseAttributes(),
		nil,
	)

	return row
}

// QueryxContext executes the named statement and returns sqlx.Rows.
func (ns *NamedStmt) QueryxContext(ctx context.Context, arg interface{}) (*sqlx.Rows, error) {
	start := time.Now()
	operation := extractOperation(ns.query)

	ctx, span := ns.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.NamedStmt.Queryx", ns.query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(ns.cfg.queryAttributes(ns.query)...),
	)
	defer span.End()

	rows, err := ns.NamedStmt.QueryxContext(ctx, arg)

	ns.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		ns.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return rows, err
}

// QueryRowxContext executes the named statement and returns sqlx.Row.
func (ns *NamedStmt) QueryRowxContext(ctx context.Context, arg interface{}) *sqlx.Row {
	start := time.Now()
	operation := extractOperation(ns.query)

	ctx, span := ns.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.NamedStmt.QueryRowx", ns.query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(ns.cfg.queryAttributes(ns.query)...),
	)
	defer span.End()

	row := ns.NamedStmt.QueryRowxContext(ctx, arg)

	ns.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		ns.cfg.baseAttributes(),
		nil,
	)

	return row
}

// MustExecContext executes the named statement and panics on error.
func (ns *NamedStmt) MustExecContext(ctx context.Context, arg interface{}) sql.Result {
	result, err := ns.ExecContext(ctx, arg)
	if err != nil {
		panic(err)
	}
	return result
}

// Unsafe returns a version of NamedStmt that silently ignores missing fields.
func (ns *NamedStmt) Unsafe() *NamedStmt {
	return &NamedStmt{
		NamedStmt: ns.NamedStmt.Unsafe(),
		cfg:       ns.cfg,
		query:     ns.query,
	}
}

// Close closes the named statement.
func (ns *NamedStmt) Close() error {
	return ns.NamedStmt.Close()
}
