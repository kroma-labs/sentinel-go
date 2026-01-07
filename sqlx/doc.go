// Package sqlx provides an instrumented wrapper around jmoiron/sqlx
// with automatic OpenTelemetry tracing and metrics.
//
// # Features
//
//   - OpenTelemetry tracing with span per query
//   - Prometheus-compatible metrics for query latency
//   - Full sqlx API support (Get, Select, NamedExec, etc.)
//   - Struct scanning and named parameter binding
//   - Transaction support with instrumentation
//
// # Quick Start
//
// Open a database connection with instrumentation:
//
//	import sentinelsqlx "github.com/kroma-labs/sentinel-go/sqlx"
//
//	db, err := sentinelsqlx.Open("postgres", dsn,
//	    sentinelsqlx.WithDBSystem("postgresql"),
//	    sentinelsqlx.WithDBName("mydb"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Close()
//
// # Struct Scanning
//
// Use Get and Select for automatic struct scanning:
//
//	type User struct {
//	    ID    int    `db:"id"`
//	    Name  string `db:"name"`
//	    Email string `db:"email"`
//	}
//
//	// Single row
//	var user User
//	err := db.GetContext(ctx, &user, "SELECT * FROM users WHERE id = $1", 1)
//
//	// Multiple rows
//	var users []User
//	err := db.SelectContext(ctx, &users, "SELECT * FROM users WHERE active = true")
//
// # Named Parameters
//
// Use named queries with structs or maps:
//
//	user := User{Name: "John", Email: "john@example.com"}
//	result, err := db.NamedExecContext(ctx,
//	    "INSERT INTO users (name, email) VALUES (:name, :email)",
//	    user,
//	)
//
// # Transactions
//
// Instrumented transactions with automatic tracing:
//
//	tx, err := db.BeginTxx(ctx, nil)
//	if err != nil {
//	    return err
//	}
//	defer tx.Rollback()
//
//	_, err = tx.ExecContext(ctx, "UPDATE accounts SET balance = balance - $1", amount)
//	if err != nil {
//	    return err
//	}
//
//	return tx.Commit()
//
// # Configuration Options
//
// Common options for customization:
//
//	db, _ := sentinelsqlx.Open("postgres", dsn,
//	    sentinelsqlx.WithDBSystem("postgresql"),    // Required: database type
//	    sentinelsqlx.WithDBName("users_db"),        // Database name
//	    sentinelsqlx.WithInstanceName("replica"),   // Connection identifier
//	    sentinelsqlx.WithTracerProvider(tp),        // Custom tracer provider
//	    sentinelsqlx.WithMeterProvider(mp),         // Custom meter provider
//	)
//
// # Observability
//
// The wrapper automatically emits:
//
// Traces:
//   - Span per query: sqlx.Get, sqlx.Select, sqlx.NamedExec, etc.
//   - Attributes: db.system, db.name, db.statement, db.operation
//
// Metrics:
//   - db.client.query.duration (histogram by operation)
package sqlx
