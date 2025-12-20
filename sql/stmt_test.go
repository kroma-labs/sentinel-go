package sql

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/kroma-labs/sentinel-go/sql/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewOtelStmt(t *testing.T) {
	t.Run("given stmt, config and query, then creates wrapped statement", func(t *testing.T) {
		mockStmt := mocks.NewDriverStmt(t)
		cfg := newConfig(WithDBSystem("postgresql"))
		query := "SELECT * FROM users"

		otelStmt := newOtelStmt(mockStmt, cfg, query)

		require.NotNil(t, otelStmt)
		assert.Equal(t, mockStmt, otelStmt.stmt)
		assert.Equal(t, cfg, otelStmt.cfg)
		assert.Equal(t, query, otelStmt.query)
	})
}

func TestOtelStmt_Close(t *testing.T) {
	t.Run("given stmt, then closes underlying stmt", func(t *testing.T) {
		mockStmt := mocks.NewDriverStmt(t)
		mockStmt.EXPECT().Close().Return(nil)

		cfg := newConfig()
		otelStmt := newOtelStmt(mockStmt, cfg, "SELECT 1")

		err := otelStmt.Close()

		assert.NoError(t, err)
	})
}

func TestOtelStmt_NumInput(t *testing.T) {
	t.Run("given stmt, then returns numInput from underlying stmt", func(t *testing.T) {
		mockStmt := mocks.NewDriverStmt(t)
		mockStmt.EXPECT().NumInput().Return(2)

		cfg := newConfig()
		otelStmt := newOtelStmt(mockStmt, cfg, "SELECT 1")

		got := otelStmt.NumInput()

		assert.Equal(t, 2, got)
	})
}

func TestOtelStmt_ExecContext(t *testing.T) {
	type args struct {
		query    string
		stmtArgs []driver.NamedValue
	}

	tests := []struct {
		name    string
		args    args
		mockFn  func(*mocks.DriverStmt, *mocks.DriverResult)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "given successful exec, then returns result",
			args: args{
				query:    "INSERT INTO users (name) VALUES (?)",
				stmtArgs: []driver.NamedValue{{Value: "test"}},
			},
			mockFn: func(stmt *mocks.DriverStmt, result *mocks.DriverResult) {
				stmt.EXPECT().
					ExecContext(mock.Anything, mock.Anything).
					Return(result, nil)
			},
			wantErr: assert.NoError,
		},
		{
			name: "given exec error, then returns error",
			args: args{
				query:    "INSERT INTO users (name) VALUES (?)",
				stmtArgs: []driver.NamedValue{{Value: "test"}},
			},
			mockFn: func(stmt *mocks.DriverStmt, _ *mocks.DriverResult) {
				stmt.EXPECT().
					ExecContext(mock.Anything, mock.Anything).
					Return(nil, assert.AnError)
			},
			wantErr: assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStmt := mocks.NewDriverStmt(t)
			mockResult := mocks.NewDriverResult(t)
			tt.mockFn(mockStmt, mockResult)

			cfg := newConfig(WithDBSystem("postgresql"))
			otelStmt := newOtelStmt(mockStmt, cfg, tt.args.query)

			ctx := context.Background()
			result, err := otelStmt.ExecContext(ctx, tt.args.stmtArgs)

			tt.wantErr(t, err)
			if err == nil {
				assert.NotNil(t, result)
			}
		})
	}
}

func TestOtelStmt_QueryContext(t *testing.T) {
	type args struct {
		query    string
		stmtArgs []driver.NamedValue
	}

	tests := []struct {
		name    string
		args    args
		mockFn  func(*mocks.DriverStmt, *mocks.DriverRows)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "given successful query, then returns rows",
			args: args{
				query:    "SELECT * FROM users WHERE id = ?",
				stmtArgs: []driver.NamedValue{{Value: 1}},
			},
			mockFn: func(stmt *mocks.DriverStmt, rows *mocks.DriverRows) {
				stmt.EXPECT().
					QueryContext(mock.Anything, mock.Anything).
					Return(rows, nil)
			},
			wantErr: assert.NoError,
		},
		{
			name: "given query error, then returns error",
			args: args{
				query:    "SELECT * FROM users WHERE id = ?",
				stmtArgs: []driver.NamedValue{{Value: 1}},
			},
			mockFn: func(stmt *mocks.DriverStmt, _ *mocks.DriverRows) {
				stmt.EXPECT().
					QueryContext(mock.Anything, mock.Anything).
					Return(nil, assert.AnError)
			},
			wantErr: assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStmt := mocks.NewDriverStmt(t)
			mockRows := mocks.NewDriverRows(t)
			tt.mockFn(mockStmt, mockRows)

			cfg := newConfig(WithDBSystem("postgresql"))
			otelStmt := newOtelStmt(mockStmt, cfg, tt.args.query)

			ctx := context.Background()
			rows, err := otelStmt.QueryContext(ctx, tt.args.stmtArgs)

			tt.wantErr(t, err)
			if err == nil {
				assert.NotNil(t, rows)
			}
		})
	}
}

func TestNamedValueToValue(t *testing.T) {
	type args struct {
		input []driver.NamedValue
	}

	tests := []struct {
		name string
		args args
		want []driver.Value
	}{
		{
			name: "given empty slice, then returns empty slice",
			args: args{input: []driver.NamedValue{}},
			want: []driver.Value{},
		},
		{
			name: "given named values, then returns values",
			args: args{
				input: []driver.NamedValue{
					{Ordinal: 1, Value: "test"},
					{Ordinal: 2, Value: 123},
					{Ordinal: 3, Value: true},
				},
			},
			want: []driver.Value{"test", 123, true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := namedValueToValue(tt.args.input)

			assert.Equal(t, tt.want, got)
		})
	}
}
