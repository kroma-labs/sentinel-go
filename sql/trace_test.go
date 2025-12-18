package sql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpanName(t *testing.T) {
	type args struct {
		query string
	}

	tests := []struct {
		name     string
		args     args
		wantName string
	}{
		{
			name:     "given SELECT query, then returns SELECT",
			args:     args{query: "SELECT * FROM users WHERE id = 1"},
			wantName: "SELECT",
		},
		{
			name:     "given INSERT query, then returns INSERT",
			args:     args{query: "INSERT INTO users (name) VALUES ('test')"},
			wantName: "INSERT",
		},
		{
			name:     "given UPDATE query, then returns UPDATE",
			args:     args{query: "UPDATE users SET name = 'test' WHERE id = 1"},
			wantName: "UPDATE",
		},
		{
			name:     "given DELETE query, then returns DELETE",
			args:     args{query: "DELETE FROM users WHERE id = 1"},
			wantName: "DELETE",
		},
		{
			name:     "given empty query, then returns SQL default",
			args:     args{query: ""},
			wantName: "SQL",
		},
		{
			name:     "given whitespace only, then returns SQL default",
			args:     args{query: "   "},
			wantName: "SQL",
		},
		{
			name:     "given query with leading whitespace, then returns operation",
			args:     args{query: "   SELECT * FROM users"},
			wantName: "SELECT",
		},
		{
			name:     "given lowercase query, then returns uppercase operation",
			args:     args{query: "select * from users"},
			wantName: "SELECT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := spanName(tt.args.query)
			assert.Equal(t, tt.wantName, got)
		})
	}
}

func TestExtractOperation(t *testing.T) {
	type args struct {
		query string
	}

	tests := []struct {
		name          string
		args          args
		wantOperation string
	}{
		{
			name:          "given SELECT statement, then returns SELECT",
			args:          args{query: "SELECT id FROM users"},
			wantOperation: "SELECT",
		},
		{
			name:          "given INSERT statement, then returns INSERT",
			args:          args{query: "INSERT INTO users (id) VALUES (1)"},
			wantOperation: "INSERT",
		},
		{
			name:          "given UPDATE statement, then returns UPDATE",
			args:          args{query: "UPDATE users SET name = 'test'"},
			wantOperation: "UPDATE",
		},
		{
			name:          "given DELETE statement, then returns DELETE",
			args:          args{query: "DELETE FROM users"},
			wantOperation: "DELETE",
		},
		{
			name:          "given CREATE statement, then returns CREATE",
			args:          args{query: "CREATE TABLE users (id INT)"},
			wantOperation: "CREATE",
		},
		{
			name:          "given DROP statement, then returns DROP",
			args:          args{query: "DROP TABLE users"},
			wantOperation: "DROP",
		},
		{
			name:          "given empty string, then returns empty string",
			args:          args{query: ""},
			wantOperation: "",
		},
		{
			name:          "given single word command, then returns that word uppercased",
			args:          args{query: "COMMIT"},
			wantOperation: "COMMIT",
		},
		{
			name:          "given query with newline after operation, then returns operation",
			args:          args{query: "SELECT\n* FROM users"},
			wantOperation: "SELECT",
		},
		{
			name:          "given query with tab after operation, then returns operation",
			args:          args{query: "SELECT\t* FROM users"},
			wantOperation: "SELECT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractOperation(tt.args.query)
			assert.Equal(t, tt.wantOperation, got)
		})
	}
}

func TestDefaultQuerySanitizer(t *testing.T) {
	type args struct {
		query string
	}

	tests := []struct {
		name      string
		args      args
		wantQuery string
	}{
		{
			name:      "given query with string literal, then replaces with placeholder",
			args:      args{query: "SELECT * FROM users WHERE name = 'john'"},
			wantQuery: "SELECT * FROM users WHERE name = '?'",
		},
		{
			name:      "given query with numeric literal, then replaces with placeholder",
			args:      args{query: "SELECT * FROM users WHERE id = 123"},
			wantQuery: "SELECT * FROM users WHERE id = ?",
		},
		{
			name:      "given query with multiple literals, then replaces all",
			args:      args{query: "SELECT * FROM users WHERE id = 1 AND name = 'test'"},
			wantQuery: "SELECT * FROM users WHERE id = ? AND name = '?'",
		},
		{
			name:      "given query with escaped quote, then handles correctly",
			args:      args{query: "SELECT * FROM users WHERE name = 'it\\'s'"},
			wantQuery: "SELECT * FROM users WHERE name = '?'",
		},
		{
			name:      "given query with hex literal, then replaces with placeholder",
			args:      args{query: "SELECT * FROM users WHERE id = 0xDEADBEEF"},
			wantQuery: "SELECT * FROM users WHERE id = ?",
		},
		{
			name:      "given query with float literal, then replaces with placeholder",
			args:      args{query: "SELECT * FROM products WHERE price = 19.99"},
			wantQuery: "SELECT * FROM products WHERE price = ?",
		},
		{
			name:      "given empty query, then returns empty",
			args:      args{query: ""},
			wantQuery: "",
		},
		{
			name:      "given query without literals, then returns unchanged",
			args:      args{query: "SELECT * FROM users"},
			wantQuery: "SELECT * FROM users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultQuerySanitizer(tt.args.query)
			assert.Equal(t, tt.wantQuery, got)
		})
	}
}

func TestBaseAttributes(t *testing.T) {
	type args struct {
		cfg *config
	}

	tests := []struct {
		name         string
		args         args
		wantCount    int
		wantContains map[string]string
	}{
		{
			name: "given config with all fields, then returns all attributes",
			args: args{
				cfg: &config{
					DBSystem:     "postgresql",
					DBName:       "testdb",
					InstanceName: "primary",
				},
			},
			wantCount: 3,
			wantContains: map[string]string{
				"db.system":   "postgresql",
				"db.name":     "testdb",
				"db.instance": "primary",
			},
		},
		{
			name:         "given empty config, then returns empty slice",
			args:         args{cfg: &config{}},
			wantCount:    0,
			wantContains: map[string]string{},
		},
		{
			name: "given config with only DBSystem, then returns one attribute",
			args: args{
				cfg: &config{DBSystem: "mysql"},
			},
			wantCount: 1,
			wantContains: map[string]string{
				"db.system": "mysql",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := tt.args.cfg.baseAttributes()
			assert.Len(t, attrs, tt.wantCount)

			attrMap := make(map[string]string)
			for _, attr := range attrs {
				attrMap[string(attr.Key)] = attr.Value.AsString()
			}

			for key, wantValue := range tt.wantContains {
				assert.Equal(t, wantValue, attrMap[key], "attribute %s", key)
			}
		})
	}
}

func TestQueryAttributes(t *testing.T) {
	type args struct {
		cfg   *config
		query string
	}

	tests := []struct {
		name         string
		args         args
		wantContains map[string]string
		wantMissing  []string
	}{
		{
			name: "given config with DB info, then includes statement and operation",
			args: args{
				cfg:   &config{DBSystem: "postgresql", DBName: "testdb"},
				query: "SELECT * FROM users",
			},
			wantContains: map[string]string{
				"db.system":    "postgresql",
				"db.name":      "testdb",
				"db.statement": "SELECT * FROM users",
				"db.operation": "SELECT",
			},
		},
		{
			name: "given config with sanitizer, then sanitizes query",
			args: args{
				cfg:   &config{DBSystem: "postgresql", QuerySanitizer: DefaultQuerySanitizer},
				query: "SELECT * FROM users WHERE id = 123",
			},
			wantContains: map[string]string{
				"db.statement": "SELECT * FROM users WHERE id = ?",
				"db.operation": "SELECT",
			},
		},
		{
			name: "given config with DisableQuery, then omits statement",
			args: args{
				cfg:   &config{DBSystem: "postgresql", DisableQuery: true},
				query: "SELECT * FROM users",
			},
			wantContains: map[string]string{
				"db.operation": "SELECT",
			},
			wantMissing: []string{"db.statement"},
		},
		{
			name: "given empty query, then operation is empty",
			args: args{
				cfg:   &config{DBSystem: "postgresql"},
				query: "",
			},
			wantContains: map[string]string{
				"db.system": "postgresql",
			},
			wantMissing: []string{"db.statement", "db.operation"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := tt.args.cfg.queryAttributes(tt.args.query)

			attrMap := make(map[string]string)
			for _, attr := range attrs {
				attrMap[string(attr.Key)] = attr.Value.AsString()
			}

			for key, wantValue := range tt.wantContains {
				assert.Equal(t, wantValue, attrMap[key], "attribute %s", key)
			}

			for _, key := range tt.wantMissing {
				_, exists := attrMap[key]
				assert.False(t, exists, "attribute %s should be missing", key)
			}
		})
	}
}

func TestNewConfig(t *testing.T) {
	type args struct {
		opts []Option
	}

	tests := []struct {
		name       string
		args       args
		wantAssert func(*config) bool
	}{
		{
			name: "given no options, then uses defaults",
			args: args{opts: nil},
			wantAssert: func(cfg *config) bool {
				return cfg.TracerProvider != nil && cfg.MeterProvider != nil
			},
		},
		{
			name: "given WithDBSystem, then sets DBSystem",
			args: args{opts: []Option{WithDBSystem("postgresql")}},
			wantAssert: func(cfg *config) bool {
				return cfg.DBSystem == "postgresql"
			},
		},
		{
			name: "given WithDBName, then sets DBName",
			args: args{opts: []Option{WithDBName("mydb")}},
			wantAssert: func(cfg *config) bool {
				return cfg.DBName == "mydb"
			},
		},
		{
			name: "given WithInstanceName, then sets InstanceName",
			args: args{opts: []Option{WithInstanceName("primary")}},
			wantAssert: func(cfg *config) bool {
				return cfg.InstanceName == "primary"
			},
		},
		{
			name: "given WithDisableQuery, then sets DisableQuery",
			args: args{opts: []Option{WithDisableQuery()}},
			wantAssert: func(cfg *config) bool {
				return cfg.DisableQuery == true
			},
		},
		{
			name: "given WithQuerySanitizer, then sets sanitizer",
			args: args{opts: []Option{WithQuerySanitizer(DefaultQuerySanitizer)}},
			wantAssert: func(cfg *config) bool {
				return cfg.QuerySanitizer != nil
			},
		},
		{
			name: "given multiple options, then applies all",
			args: args{
				opts: []Option{
					WithDBSystem("postgresql"),
					WithDBName("users"),
					WithInstanceName("replica"),
				},
			},
			wantAssert: func(cfg *config) bool {
				return cfg.DBSystem == "postgresql" &&
					cfg.DBName == "users" &&
					cfg.InstanceName == "replica"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newConfig(tt.args.opts...)
			require.NotNil(t, cfg)
			assert.True(t, tt.wantAssert(cfg))
		})
	}
}
