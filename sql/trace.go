package sql

import (
	"regexp"
	"strings"
)

// Regex patterns for query sanitization.
var (
	// stringLiteralRegex matches single-quoted strings, handling escaped quotes.
	// Example matches: 'hello', 'it\'s', 'foo''bar'
	stringLiteralRegex = regexp.MustCompile(`'(?:[^'\\]|\\.)*'`)

	// numericLiteralRegex matches numeric literals (integers and floats).
	// Example matches: 123, 45.67, 0.5
	numericLiteralRegex = regexp.MustCompile(`\b\d+\.?\d*\b`)

	// hexLiteralRegex matches hex literals.
	// Example matches: 0xDEADBEEF, 0xFF, 0x1a2b
	hexLiteralRegex = regexp.MustCompile(`0[xX][0-9a-fA-F]+`)
)

// spanName returns a span name from a SQL query.
// Returns the SQL operation (SELECT, INSERT, etc.) or "SQL" for empty/unknown queries.
// This is used for OpenTelemetry span names which must not be empty.
//
// Example:
//
//	spanName("SELECT * FROM users") // returns "SELECT"
//	spanName("")                    // returns "SQL"
func spanName(query string) string {
	op := extractOperation(query)
	if op != "" {
		return op
	}
	return "SQL"
}

// extractOperation extracts the SQL operation (first word) from a query.
// Returns uppercase operation name or empty string if query is empty.
// This is used for the db.operation span attribute.
//
// Example:
//
//	extractOperation("SELECT * FROM users") // returns "SELECT"
//	extractOperation("insert into users")   // returns "INSERT"
//	extractOperation("")                    // returns ""
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

// DefaultQuerySanitizer is a basic query sanitizer that replaces
// literal values with placeholders to prevent sensitive data from
// appearing in traces.
//
// What it sanitizes:
//   - String literals: 'john' → '?'
//   - Numeric literals: 123, 45.67 → ?
//   - Hex literals: 0xDEADBEEF → ?
//
// Example:
//
//	DefaultQuerySanitizer("SELECT * FROM users WHERE id = 123")
//	// returns "SELECT * FROM users WHERE id = ?"
//
//	DefaultQuerySanitizer("SELECT * FROM users WHERE name = 'john'")
//	// returns "SELECT * FROM users WHERE name = '?'"
//
// Note: This is a simple regex-based implementation. For production use
// with complex queries, consider using a proper SQL parser.
func DefaultQuerySanitizer(query string) string {
	// Replace string literals (single quotes, handling escaped quotes)
	query = stringLiteralRegex.ReplaceAllString(query, "'?'")

	// Replace numeric literals (integers and floats)
	query = numericLiteralRegex.ReplaceAllString(query, "?")

	// Replace hex literals (0x...)
	query = hexLiteralRegex.ReplaceAllString(query, "?")

	return query
}
