package sqlx

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// DB wraps *sqlx.DB with OpenTelemetry instrumentation.
// It provides instrumented versions of all sqlx-specific methods
// like Get, Select, NamedExec, and NamedQuery.
type DB struct {
	*sqlx.DB
	cfg *config
}

// Open opens a database connection with OpenTelemetry instrumentation.
// It returns a *DB that wraps *sqlx.DB with automatic tracing and metrics.
//
// Example:
//
//	db, err := sentinelsqlx.Open("postgres", dsn,
//	    sentinelsqlx.WithDBSystem("postgresql"),
//	    sentinelsqlx.WithDBName("mydb"),
//	)
func Open(driverName, dsn string, opts ...Option) (*DB, error) {
	cfg := newConfig(opts...)

	db, err := sqlx.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}

	return &DB{DB: db, cfg: cfg}, nil
}

// Connect opens and verifies a database connection.
// It is equivalent to Open followed by Ping.
//
// Example:
//
//	db, err := sentinelsqlx.Connect(ctx, "postgres", dsn,
//	    sentinelsqlx.WithDBSystem("postgresql"),
//	)
func Connect(ctx context.Context, driverName, dsn string, opts ...Option) (*DB, error) {
	cfg := newConfig(opts...)

	db, err := sqlx.ConnectContext(ctx, driverName, dsn)
	if err != nil {
		return nil, err
	}

	return &DB{DB: db, cfg: cfg}, nil
}

// NewDB wraps an existing *sql.DB with sqlx and instrumentation.
//
// Example:
//
//	sqlDB, _ := sql.Open("postgres", dsn)
//	db := sentinelsqlx.NewDB(sqlDB, "postgres",
//	    sentinelsqlx.WithDBSystem("postgresql"),
//	)
func NewDB(db *sql.DB, driverName string, opts ...Option) *DB {
	cfg := newConfig(opts...)
	return &DB{
		DB:  sqlx.NewDb(db, driverName),
		cfg: cfg,
	}
}

// MustConnect is like Connect but panics on error.
func MustConnect(ctx context.Context, driverName, dsn string, opts ...Option) *DB {
	db, err := Connect(ctx, driverName, dsn, opts...)
	if err != nil {
		panic(err)
	}
	return db
}

// MustOpen is like Open but panics on error.
func MustOpen(driverName, dsn string, opts ...Option) *DB {
	db, err := Open(driverName, dsn, opts...)
	if err != nil {
		panic(err)
	}
	return db
}

// GetContext executes a query that is expected to return at most one row
// and scans the result into dest.
func (db *DB) GetContext(
	ctx context.Context,
	dest interface{},
	query string,
	args ...interface{},
) error {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := db.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.Get", query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(db.cfg.queryAttributes(query)...),
	)
	defer span.End()

	err := db.DB.GetContext(ctx, dest, query, args...)

	db.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		db.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}

// SelectContext executes a query and scans all results into dest.
func (db *DB) SelectContext(
	ctx context.Context,
	dest interface{},
	query string,
	args ...interface{},
) error {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := db.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.Select", query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(db.cfg.queryAttributes(query)...),
	)
	defer span.End()

	err := db.DB.SelectContext(ctx, dest, query, args...)

	db.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		db.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}

// NamedExecContext executes a named query.
func (db *DB) NamedExecContext(
	ctx context.Context,
	query string,
	arg interface{},
) (sql.Result, error) {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := db.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.NamedExec", query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(db.cfg.queryAttributes(query)...),
	)
	defer span.End()

	result, err := db.DB.NamedExecContext(ctx, query, arg)

	db.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		db.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return result, err
}

// NamedQueryContext executes a named query and returns rows.
func (db *DB) NamedQueryContext(
	ctx context.Context,
	query string,
	arg interface{},
) (*sqlx.Rows, error) {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := db.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.NamedQuery", query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(db.cfg.queryAttributes(query)...),
	)
	defer span.End()

	rows, err := db.DB.NamedQueryContext(ctx, query, arg)

	db.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		db.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return rows, err
}

// QueryxContext executes a query and returns sqlx.Rows.
func (db *DB) QueryxContext(
	ctx context.Context,
	query string,
	args ...interface{},
) (*sqlx.Rows, error) {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := db.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.Queryx", query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(db.cfg.queryAttributes(query)...),
	)
	defer span.End()

	rows, err := db.DB.QueryxContext(ctx, query, args...)

	db.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		db.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return rows, err
}

// QueryRowxContext executes a query and returns a single sqlx.Row.
func (db *DB) QueryRowxContext(ctx context.Context, query string, args ...interface{}) *sqlx.Row {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := db.cfg.Tracer.Start(ctx, sqlxSpanName("sqlx.QueryRowx", query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(db.cfg.queryAttributes(query)...),
	)
	defer span.End()

	row := db.DB.QueryRowxContext(ctx, query, args...)

	// Record metrics (we can't know if there's an error until Scan is called)
	db.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		db.cfg.baseAttributes(),
		nil,
	)

	return row
}

// BeginTxx starts an instrumented transaction.
func (db *DB) BeginTxx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	start := time.Now()

	ctx, span := db.cfg.Tracer.Start(ctx, "BEGIN",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(db.cfg.baseAttributes()...),
	)
	defer span.End()

	tx, err := db.DB.BeginTxx(ctx, opts)

	db.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		"BEGIN",
		db.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	return &Tx{Tx: tx, cfg: db.cfg}, nil
}

// Beginx starts an instrumented transaction with default options.
func (db *DB) Beginx() (*Tx, error) {
	return db.BeginTxx(context.Background(), nil)
}

// MustBeginTx starts a transaction and panics on error.
func (db *DB) MustBeginTx(ctx context.Context, opts *sql.TxOptions) *Tx {
	tx, err := db.BeginTxx(ctx, opts)
	if err != nil {
		panic(err)
	}
	return tx
}

// MustBegin starts a transaction and panics on error.
func (db *DB) MustBegin() *Tx {
	return db.MustBeginTx(context.Background(), nil)
}

// PrepareNamedContext prepares an instrumented named statement.
func (db *DB) PrepareNamedContext(ctx context.Context, query string) (*NamedStmt, error) {
	start := time.Now()

	ctx, span := db.cfg.Tracer.Start(ctx, "sqlx.PrepareNamed",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(db.cfg.queryAttributes(query)...),
	)
	defer span.End()

	stmt, err := db.DB.PrepareNamedContext(ctx, query)

	db.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		"PREPARE",
		db.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	return &NamedStmt{NamedStmt: stmt, cfg: db.cfg, query: query}, nil
}

// PrepareNamed prepares a named statement without context.
func (db *DB) PrepareNamed(query string) (*NamedStmt, error) {
	return db.PrepareNamedContext(context.Background(), query)
}

// PreparexContext prepares an instrumented statement.
func (db *DB) PreparexContext(ctx context.Context, query string) (*Stmt, error) {
	start := time.Now()

	ctx, span := db.cfg.Tracer.Start(ctx, "sqlx.Preparex",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(db.cfg.queryAttributes(query)...),
	)
	defer span.End()

	stmt, err := db.DB.PreparexContext(ctx, query)

	db.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		"PREPARE",
		db.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	return &Stmt{Stmt: stmt, cfg: db.cfg, query: query}, nil
}

// Preparex prepares a statement without context.
func (db *DB) Preparex(query string) (*Stmt, error) {
	return db.PreparexContext(context.Background(), query)
}

// Rebind transforms a query from QUESTION to the DB driver's bindvar type.
func (db *DB) Rebind(query string) string {
	return db.DB.Rebind(query)
}

// BindNamed binds a named query to a map or struct.
func (db *DB) BindNamed(query string, arg interface{}) (string, []interface{}, error) {
	return db.DB.BindNamed(query, arg)
}

// DriverName returns the driver name.
func (db *DB) DriverName() string {
	return db.DB.DriverName()
}

// MapperFunc sets a custom field name mapper.
func (db *DB) MapperFunc(mf func(string) string) {
	db.DB.MapperFunc(mf)
}

// Unsafe returns a version of DB that silently ignores missing destination fields.
func (db *DB) Unsafe() *DB {
	return &DB{
		DB:  db.DB.Unsafe(),
		cfg: db.cfg,
	}
}

// PingContext verifies the database connection.
func (db *DB) PingContext(ctx context.Context) error {
	start := time.Now()

	ctx, span := db.cfg.Tracer.Start(ctx, "PING",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(db.cfg.baseAttributes()...),
	)
	defer span.End()

	err := db.DB.PingContext(ctx)

	db.cfg.Metrics.recordQueryDuration(ctx, time.Since(start), "PING", db.cfg.baseAttributes(), err)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}

// ExecContext executes a query without returning rows.
func (db *DB) ExecContext(
	ctx context.Context,
	query string,
	args ...interface{},
) (sql.Result, error) {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := db.cfg.Tracer.Start(ctx, spanName(query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(db.cfg.queryAttributes(query)...),
	)
	defer span.End()

	result, err := db.DB.ExecContext(ctx, query, args...)

	db.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		db.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return result, err
}

// QueryContext executes a query and returns rows.
func (db *DB) QueryContext(
	ctx context.Context,
	query string,
	args ...interface{},
) (*sql.Rows, error) {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := db.cfg.Tracer.Start(ctx, spanName(query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(db.cfg.queryAttributes(query)...),
	)
	defer span.End()

	rows, err := db.DB.QueryContext(ctx, query, args...)

	db.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		db.cfg.baseAttributes(),
		err,
	)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return rows, err
}

// QueryRowContext executes a query and returns a single row.
func (db *DB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	start := time.Now()
	operation := extractOperation(query)

	ctx, span := db.cfg.Tracer.Start(ctx, spanName(query),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(db.cfg.queryAttributes(query)...),
	)
	defer span.End()

	row := db.DB.QueryRowContext(ctx, query, args...)

	db.cfg.Metrics.recordQueryDuration(
		ctx,
		time.Since(start),
		operation,
		db.cfg.baseAttributes(),
		nil,
	)

	return row
}
