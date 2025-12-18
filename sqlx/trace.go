package sqlx

import (
	"regexp"
	"strings"

	"go.opentelemetry.io/otel/attribute"
)

// Regex patterns for query sanitization - pre-compiled for performance.
var (
	// stringLiteralRegex matches single-quoted strings, handling escaped quotes.
	stringLiteralRegex = regexp.MustCompile(`'(?:[^'\\]|\\.)*'`)

	// numericLiteralRegex matches numeric literals (integers and floats).
	numericLiteralRegex = regexp.MustCompile(`\b\d+\.?\d*\b`)

	// hexLiteralRegex matches hex literals.
	hexLiteralRegex = regexp.MustCompile(`0[xX][0-9a-fA-F]+`)
)

// spanName returns a span name from a SQL query.
// Returns the SQL operation (SELECT, INSERT, etc.) or "SQL" for empty/unknown queries.
func spanName(query string) string {
	op := extractOperation(query)
	if op != "" {
		return op
	}
	return "SQL"
}

// extractOperation extracts the SQL operation (first word) from a query.
// Returns uppercase operation name or empty string if query is empty.
//
// This uses a simple string-based approach (not regex) for performance:
// - Trims whitespace
// - Finds the first space/tab/newline
// - Returns the uppercase first word
func extractOperation(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}

	// Find the first word (the SQL command)
	spaceIdx := strings.IndexAny(query, " \t\n\r")
	if spaceIdx == -1 {
		return strings.ToUpper(query)
	}

	return strings.ToUpper(query[:spaceIdx])
}

// sqlxSpanName generates a span name for sqlx-specific operations.
func sqlxSpanName(method, query string) string {
	op := extractOperation(query)
	if op == "" {
		return method
	}
	return method + ": " + op
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

	op := extractOperation(query)
	if op != "" {
		attrs = append(attrs, attribute.String("db.operation", op))
	}

	return attrs
}

// DefaultQuerySanitizer is a basic query sanitizer that replaces
// literal values with placeholders to prevent sensitive data from
// appearing in traces.
//
// What it sanitizes:
//   - String literals: 'john' → '?'
//   - Numeric literals: 123, 45.67 → ?
//   - Hex literals: 0xDEADBEEF → ?
func DefaultQuerySanitizer(query string) string {
	// Replace string literals (single quotes, handling escaped quotes)
	query = stringLiteralRegex.ReplaceAllString(query, "'?'")

	// Replace numeric literals (integers and floats)
	query = numericLiteralRegex.ReplaceAllString(query, "?")

	// Replace hex literals (0x...)
	query = hexLiteralRegex.ReplaceAllString(query, "?")

	return query
}
