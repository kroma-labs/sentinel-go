package sql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

func TestNewConfig_Options(t *testing.T) {
	tests := []struct {
		name       string
		opts       []Option
		wantAssert func(*testing.T, *config)
	}{
		{
			name: "given no options, then uses global providers",
			opts: nil,
			wantAssert: func(t *testing.T, cfg *config) {
				assert.Equal(t, otel.GetTracerProvider(), cfg.TracerProvider)
				assert.Equal(t, otel.GetMeterProvider(), cfg.MeterProvider)
				assert.NotNil(t, cfg.Tracer)
				assert.NotNil(t, cfg.Meter)
			},
		},
		{
			name: "given WithDBSystem, then sets DBSystem",
			opts: []Option{WithDBSystem("postgresql")},
			wantAssert: func(t *testing.T, cfg *config) {
				assert.Equal(t, "postgresql", cfg.DBSystem)
			},
		},
		{
			name: "given WithDBName, then sets DBName",
			opts: []Option{WithDBName("mydb")},
			wantAssert: func(t *testing.T, cfg *config) {
				assert.Equal(t, "mydb", cfg.DBName)
			},
		},
		{
			name: "given WithInstanceName, then sets InstanceName",
			opts: []Option{WithInstanceName("primary")},
			wantAssert: func(t *testing.T, cfg *config) {
				assert.Equal(t, "primary", cfg.InstanceName)
			},
		},
		{
			name: "given multiple options, then applies all",
			opts: []Option{
				WithDBSystem("mysql"),
				WithDBName("users"),
				WithInstanceName("replica"),
			},
			wantAssert: func(t *testing.T, cfg *config) {
				assert.Equal(t, "mysql", cfg.DBSystem)
				assert.Equal(t, "users", cfg.DBName)
				assert.Equal(t, "replica", cfg.InstanceName)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newConfig(tt.opts...)
			require.NotNil(t, cfg)
			tt.wantAssert(t, cfg)
		})
	}
}

func TestWithQuerySanitizer(t *testing.T) {
	tests := []struct {
		name      string
		sanitizer func(string) string
		input     string
		want      string
	}{
		{
			name:      "given custom sanitizer, then applies to query",
			sanitizer: func(_ string) string { return "SANITIZED" },
			input:     "SELECT * FROM users WHERE id = 123",
			want:      "SANITIZED",
		},
		{
			name:      "given DefaultQuerySanitizer, then replaces literals",
			sanitizer: DefaultQuerySanitizer,
			input:     "SELECT * FROM users WHERE id = 123",
			want:      "SELECT * FROM users WHERE id = ?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newConfig(WithQuerySanitizer(tt.sanitizer))
			require.NotNil(t, cfg.QuerySanitizer)

			got := cfg.QuerySanitizer(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWithDisableQuery(t *testing.T) {
	t.Run("given WithDisableQuery, then DisableQuery is true", func(t *testing.T) {
		cfg := newConfig(WithDisableQuery())
		assert.True(t, cfg.DisableQuery)
	})

	t.Run("given no WithDisableQuery, then DisableQuery is false", func(t *testing.T) {
		cfg := newConfig()
		assert.False(t, cfg.DisableQuery)
	})
}

func TestConfigBaseAttributes(t *testing.T) {
	tests := []struct {
		name      string
		opts      []Option
		wantCount int
		wantAttrs map[string]string
	}{
		{
			name: "given all fields set, then returns all attributes",
			opts: []Option{
				WithDBSystem("postgresql"),
				WithDBName("mydb"),
				WithInstanceName("primary"),
			},
			wantCount: 3,
			wantAttrs: map[string]string{
				"db.system":   "postgresql",
				"db.name":     "mydb",
				"db.instance": "primary",
			},
		},
		{
			name:      "given only DBSystem, then returns one attribute",
			opts:      []Option{WithDBSystem("mysql")},
			wantCount: 1,
			wantAttrs: map[string]string{"db.system": "mysql"},
		},
		{
			name:      "given no options, then returns empty attributes",
			opts:      nil,
			wantCount: 0,
			wantAttrs: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newConfig(tt.opts...)
			attrs := cfg.baseAttributes()

			assert.Len(t, attrs, tt.wantCount)
			for _, attr := range attrs {
				key := string(attr.Key)
				if expected, ok := tt.wantAttrs[key]; ok {
					assert.Equal(t, expected, attr.Value.AsString())
				}
			}
		})
	}
}

func TestConfigQueryAttributes(t *testing.T) {
	tests := []struct {
		name          string
		opts          []Option
		query         string
		wantStatement bool
		wantOperation string
	}{
		{
			name:          "given query without sanitizer, then includes raw query",
			opts:          []Option{WithDBSystem("postgresql")},
			query:         "SELECT * FROM users WHERE id = 123",
			wantStatement: true,
			wantOperation: "SELECT",
		},
		{
			name: "given query with sanitizer, then includes sanitized query",
			opts: []Option{
				WithDBSystem("postgresql"),
				WithQuerySanitizer(DefaultQuerySanitizer),
			},
			query:         "SELECT * FROM users WHERE id = 123",
			wantStatement: true,
			wantOperation: "SELECT",
		},
		{
			name:          "given DisableQuery, then excludes statement",
			opts:          []Option{WithDBSystem("postgresql"), WithDisableQuery()},
			query:         "SELECT * FROM users",
			wantStatement: false,
			wantOperation: "SELECT",
		},
		{
			name:          "given INSERT query, then extracts INSERT operation",
			opts:          []Option{},
			query:         "INSERT INTO users (name) VALUES ('test')",
			wantStatement: true,
			wantOperation: "INSERT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newConfig(tt.opts...)
			attrs := cfg.queryAttributes(tt.query)

			hasStatement := false
			hasOperation := false
			for _, attr := range attrs {
				key := string(attr.Key)
				if key == "db.statement" {
					hasStatement = true
				}
				if key == "db.operation" {
					hasOperation = true
					assert.Equal(t, tt.wantOperation, attr.Value.AsString())
				}
			}

			assert.Equal(t, tt.wantStatement, hasStatement)
			if tt.wantOperation != "" {
				assert.True(t, hasOperation)
			}
		})
	}
}
