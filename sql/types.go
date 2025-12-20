package sql

import (
	"context"
	"database/sql/driver"
)

// DriverConn represents a database connection for testing.
type DriverConn interface {
	driver.Conn
	driver.ConnPrepareContext
	driver.ConnBeginTx
	driver.ExecerContext
	driver.QueryerContext
	driver.Pinger
}

// DriverTx represents a database transaction for testing.
type DriverTx interface {
	Commit() error
	Rollback() error
}

// DriverStmt represents a prepared statement for testing.
type DriverStmt interface {
	driver.Stmt
	driver.StmtExecContext
	driver.StmtQueryContext
}

// DriverResult represents the result of an Exec query.
type DriverResult interface {
	LastInsertId() (int64, error)
	RowsAffected() (int64, error)
}

// DriverRows represents rows returned from a query.
type DriverRows interface {
	Columns() []string
	Close() error
	Next(dest []driver.Value) error
}

// DriverConnector represents a driver connector for testing.
type DriverConnector interface {
	Connect(ctx context.Context) (driver.Conn, error)
	Driver() driver.Driver
}
