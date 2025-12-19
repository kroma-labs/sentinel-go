package sqlx

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Tx wraps *sqlx.Tx with OpenTelemetry instrumentation.
type Tx struct {
	*sqlx.Tx
	cfg *config
}

// GetContext executes a query that returns at most one row and scans into dest.
func (tx *Tx) GetContext(
	ctx context.Context,
	dest interface{},
	query string,
	args ...interface{},
) error {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := tx.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.Tx.Get", query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(tx.cfg.queryAttributes(query)...),
	)
	defer span.End()

	err := tx.Tx.GetContext(ctx, dest, query, args...)

	tx.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		tx.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}

// SelectContext executes a query and scans all results into dest.
func (tx *Tx) SelectContext(
	ctx context.Context,
	dest interface{},
	query string,
	args ...interface{},
) error {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := tx.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.Tx.Select", query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(tx.cfg.queryAttributes(query)...),
	)
	defer span.End()

	err := tx.Tx.SelectContext(ctx, dest, query, args...)

	tx.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		tx.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}

// NamedExecContext executes a named query within the transaction.
func (tx *Tx) NamedExecContext(
	ctx context.Context,
	query string,
	arg interface{},
) (sql.Result, error) {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := tx.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.Tx.NamedExec", query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(tx.cfg.queryAttributes(query)...),
	)
	defer span.End()

	result, err := tx.Tx.NamedExecContext(ctx, query, arg)

	tx.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		tx.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return result, err
}

// NamedQuery executes a named query within the transaction.
func (tx *Tx) NamedQuery(query string, arg interface{}) (*sqlx.Rows, error) {
	start := time.Now()
	ctx := context.Background()
	operation := extractOperation(query)

	ctx, span := tx.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.Tx.NamedQuery", query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(tx.cfg.queryAttributes(query)...),
	)
	defer span.End()

	rows, err := tx.Tx.NamedQuery(query, arg)

	tx.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		tx.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return rows, err
}

// QueryxContext executes a query and returns sqlx.Rows.
func (tx *Tx) QueryxContext(
	ctx context.Context,
	query string,
	args ...interface{},
) (*sqlx.Rows, error) {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := tx.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.Tx.Queryx", query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(tx.cfg.queryAttributes(query)...),
	)
	defer span.End()

	rows, err := tx.Tx.QueryxContext(ctx, query, args...)

	tx.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		tx.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return rows, err
}

// QueryRowxContext executes a query and returns a single sqlx.Row.
func (tx *Tx) QueryRowxContext(ctx context.Context, query string, args ...interface{}) *sqlx.Row {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := tx.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.Tx.QueryRowx", query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(tx.cfg.queryAttributes(query)...),
	)
	defer span.End()

	row := tx.Tx.QueryRowxContext(ctx, query, args...)

	tx.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		tx.cfg.baseAttributes(),
		nil,
	)

	return row
}

// ExecContext executes a query without returning rows.
func (tx *Tx) ExecContext(
	ctx context.Context,
	query string,
	args ...interface{},
) (sql.Result, error) {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := tx.cfg.Tracer.Start(ctx, spanName(query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(tx.cfg.queryAttributes(query)...),
	)
	defer span.End()

	result, err := tx.Tx.ExecContext(ctx, query, args...)

	tx.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		tx.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return result, err
}

// QueryContext executes a query and returns rows.
func (tx *Tx) QueryContext(
	ctx context.Context,
	query string,
	args ...interface{},
) (*sql.Rows, error) {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := tx.cfg.Tracer.Start(ctx, spanName(query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(tx.cfg.queryAttributes(query)...),
	)
	defer span.End()

	rows, err := tx.Tx.QueryContext(ctx, query, args...)

	tx.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		tx.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return rows, err
}

// QueryRowContext executes a query and returns a single row.
func (tx *Tx) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := tx.cfg.Tracer.Start(ctx, spanName(query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(tx.cfg.queryAttributes(query)...),
	)
	defer span.End()

	row := tx.Tx.QueryRowContext(ctx, query, args...)

	tx.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		tx.cfg.baseAttributes(),
		nil,
	)

	return row
}

// PrepareNamedContext prepares a named statement within the transaction.
func (tx *Tx) PrepareNamedContext(ctx context.Context, query string) (*NamedStmt, error) {
	start := time.Now()

	ctx, span := tx.cfg.Tracer.Start(ctx, "sqlx.Tx.PrepareNamed",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(tx.cfg.queryAttributes(query)...),
	)
	defer span.End()

	stmt, err := tx.Tx.PrepareNamedContext(ctx, query)

	tx.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		"PREPARE",
		tx.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	return &NamedStmt{NamedStmt: stmt, cfg: tx.cfg, query: query}, nil
}

// PrepareNamed prepares a named statement within the transaction.
func (tx *Tx) PrepareNamed(query string) (*NamedStmt, error) {
	return tx.PrepareNamedContext(context.Background(), query)
}

// PreparexContext prepares a statement within the transaction.
func (tx *Tx) PreparexContext(ctx context.Context, query string) (*Stmt, error) {
	start := time.Now()

	ctx, span := tx.cfg.Tracer.Start(ctx, "sqlx.Tx.Preparex",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(tx.cfg.queryAttributes(query)...),
	)
	defer span.End()

	stmt, err := tx.Tx.PreparexContext(ctx, query)

	tx.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		"PREPARE",
		tx.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	return &Stmt{Stmt: stmt, cfg: tx.cfg, query: query}, nil
}

// Preparex prepares a statement within the transaction.
func (tx *Tx) Preparex(query string) (*Stmt, error) {
	return tx.PreparexContext(context.Background(), query)
}

// StmtxContext returns a version of the prepared statement bound to this transaction.
func (tx *Tx) StmtxContext(ctx context.Context, stmt *Stmt) *Stmt {
	return &Stmt{
		Stmt:  tx.Tx.StmtxContext(ctx, stmt.Stmt),
		cfg:   tx.cfg,
		query: stmt.query,
	}
}

// Stmtx returns a version of the prepared statement bound to this transaction.
func (tx *Tx) Stmtx(stmt *Stmt) *Stmt {
	return tx.StmtxContext(context.Background(), stmt)
}

// NamedStmtContext returns a version of the named statement bound to this transaction.
func (tx *Tx) NamedStmtContext(ctx context.Context, stmt *NamedStmt) *NamedStmt {
	return &NamedStmt{
		NamedStmt: tx.Tx.NamedStmtContext(ctx, stmt.NamedStmt),
		cfg:       tx.cfg,
		query:     stmt.query,
	}
}

// NamedStmt returns a version of the named statement bound to this transaction.
func (tx *Tx) NamedStmt(stmt *NamedStmt) *NamedStmt {
	return tx.NamedStmtContext(context.Background(), stmt)
}

// Commit commits the transaction.
func (tx *Tx) Commit() error {
	start := time.Now()
	ctx := context.Background()

	ctx, span := tx.cfg.Tracer.Start(ctx, "COMMIT",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(tx.cfg.baseAttributes()...),
	)
	defer span.End()

	err := tx.Tx.Commit()

	tx.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		"COMMIT",
		tx.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}

// Rollback aborts the transaction.
func (tx *Tx) Rollback() error {
	start := time.Now()
	ctx := context.Background()

	ctx, span := tx.cfg.Tracer.Start(ctx, "ROLLBACK",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(tx.cfg.baseAttributes()...),
	)
	defer span.End()

	err := tx.Tx.Rollback()

	tx.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		"ROLLBACK",
		tx.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}

// Rebind transforms a query from QUESTION to the DB driver's bindvar type.
func (tx *Tx) Rebind(query string) string {
	return tx.Tx.Rebind(query)
}

// BindNamed binds a named query to a map or struct.
func (tx *Tx) BindNamed(query string, arg interface{}) (string, []interface{}, error) {
	return tx.Tx.BindNamed(query, arg)
}

// DriverName returns the driver name.
func (tx *Tx) DriverName() string {
	return tx.Tx.DriverName()
}

// Unsafe returns a version of Tx that silently ignores missing destination fields.
func (tx *Tx) Unsafe() *Tx {
	return &Tx{
		Tx:  tx.Tx.Unsafe(),
		cfg: tx.cfg,
	}
}
