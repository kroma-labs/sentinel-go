// Package sql provides an instrumented database/sql driver wrapper
// with automatic OpenTelemetry tracing and metrics.
//
// # Features
//
//   - OpenTelemetry tracing with span per query
//   - Prometheus-compatible metrics for query latency
//   - Automatic query operation extraction (SELECT, INSERT, etc.)
//   - Query sanitization for secure logging
//   - Full compatibility with database/sql interface
//
// # Quick Start
//
// Open a database connection with instrumentation:
//
//	import sentinelsql "github.com/kroma-labs/sentinel-go/sql"
//
//	db, err := sentinelsql.Open("postgres", dsn,
//	    sentinelsql.WithDBSystem("postgresql"),
//	    sentinelsql.WithDBName("myapp"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Close()
//
//	// Use like standard *sql.DB
//	rows, err := db.QueryContext(ctx, "SELECT * FROM users")
//
// # Driver Registration
//
// For more control, register a wrapped driver:
//
//	driver := sentinelsql.WrapDriver(pq.Driver{},
//	    sentinelsql.WithDBSystem("postgresql"),
//	)
//	sql.Register("postgres-instrumented", driver)
//
//	db, _ := sql.Open("postgres-instrumented", dsn)
//
// # Configuration Options
//
// Common options for customization:
//
//	db, _ := sentinelsql.Open("postgres", dsn,
//	    sentinelsql.WithDBSystem("postgresql"),     // Required: database type
//	    sentinelsql.WithDBName("users_db"),         // Database name
//	    sentinelsql.WithInstanceName("primary"),    // Connection identifier
//	    sentinelsql.WithQuerySanitizer(sanitizer),  // Mask sensitive values
//	    sentinelsql.WithDisableQuery(true),         // Omit queries from spans
//	)
//
// # Query Sanitization
//
// Use DefaultQuerySanitizer to mask sensitive values:
//
//	// Input:  "SELECT * FROM users WHERE id = 123"
//	// Output: "SELECT * FROM users WHERE id = ?"
//
//	db, _ := sentinelsql.Open("postgres", dsn,
//	    sentinelsql.WithQuerySanitizer(sentinelsql.DefaultQuerySanitizer),
//	)
//
// # Observability
//
// The wrapper automatically emits:
//
// Traces:
//   - Span per query with operation name
//   - Attributes: db.system, db.name, db.statement, db.operation
//
// Metrics:
//   - db.client.query.duration (histogram by operation)
package sql
