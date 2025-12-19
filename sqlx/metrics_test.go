package sqlx

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetrics_RecordQueryDuration(t *testing.T) {
	type args struct {
		operation string
		err       error
	}

	tests := []struct {
		name string
		args args
	}{
		{
			name: "given successful query, then records with ok status",
			args: args{
				operation: "SELECT",
				err:       nil,
			},
		},
		{
			name: "given failed query, then records with error status",
			args: args{
				operation: "SELECT",
				err:       assert.AnError,
			},
		},
		{
			name: "given empty operation, then records without operation attribute",
			args: args{
				operation: "",
				err:       nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newConfig(WithDBSystem("postgresql"))
			require.NotNil(t, cfg.Metrics)

			// Should not panic
			cfg.Metrics.recordQueryDuration(
				context.Background(),
				100,
				tt.args.operation,
				cfg.baseAttributes(),
				tt.args.err,
			)
		})
	}
}

func TestMetrics_NilMetrics(t *testing.T) {
	t.Helper()
	// Test that nil metrics don't cause panics
	var m *metrics
	m.recordQueryDuration(context.Background(), 100, "SELECT", nil, nil)
}

func TestDB_QueryContext(t *testing.T) {
	type args struct {
		query string
	}

	tests := []struct {
		name    string
		args    args
		mockFn  func(sqlmock.Sqlmock)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "given valid query, then returns rows",
			args: args{query: "SELECT id FROM users"},
			mockFn: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(2)
				mock.ExpectQuery("SELECT id FROM users").WillReturnRows(rows)
			},
			wantErr: assert.NoError,
		},
		{
			name: "given query that fails, then returns error",
			args: args{query: "SELECT id FROM nonexistent"},
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT id FROM nonexistent").
					WillReturnError(assert.AnError)
			},
			wantErr: assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer mockDB.Close()

			db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))
			tt.mockFn(mock)

			rows, err := db.QueryContext(context.Background(), tt.args.query)

			tt.wantErr(t, err)
			if rows != nil {
				rows.Close()
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDB_QueryRowContext(t *testing.T) {
	type args struct {
		query string
	}

	tests := []struct {
		name   string
		args   args
		mockFn func(sqlmock.Sqlmock)
		want   int
	}{
		{
			name: "given valid query, then returns row",
			args: args{query: "SELECT id FROM users WHERE id = 1"},
			mockFn: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id"}).AddRow(1)
				mock.ExpectQuery("SELECT id FROM users WHERE id = 1").WillReturnRows(rows)
			},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer mockDB.Close()

			db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))
			tt.mockFn(mock)

			row := db.QueryRowContext(context.Background(), tt.args.query)
			require.NotNil(t, row)

			var got int
			err = row.Scan(&got)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDB_QueryxContext(t *testing.T) {
	type args struct {
		query string
	}

	tests := []struct {
		name    string
		args    args
		mockFn  func(sqlmock.Sqlmock)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "given valid query, then returns sqlx rows",
			args: args{query: "SELECT id, name FROM users"},
			mockFn: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name"}).
					AddRow(1, "John").
					AddRow(2, "Jane")
				mock.ExpectQuery("SELECT id, name FROM users").WillReturnRows(rows)
			},
			wantErr: assert.NoError,
		},
		{
			name: "given query that fails, then returns error",
			args: args{query: "SELECT id FROM nonexistent"},
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT id FROM nonexistent").
					WillReturnError(assert.AnError)
			},
			wantErr: assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer mockDB.Close()

			db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))
			tt.mockFn(mock)

			rows, err := db.QueryxContext(context.Background(), tt.args.query)

			tt.wantErr(t, err)
			if rows != nil {
				rows.Close()
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDB_QueryRowxContext(t *testing.T) {
	type args struct {
		query string
	}

	tests := []struct {
		name   string
		args   args
		mockFn func(sqlmock.Sqlmock)
	}{
		{
			name: "given valid query, then returns sqlx row",
			args: args{query: "SELECT id, name FROM users WHERE id = 1"},
			mockFn: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "John")
				mock.ExpectQuery("SELECT id, name FROM users WHERE id = 1").WillReturnRows(rows)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer mockDB.Close()

			db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))
			tt.mockFn(mock)

			row := db.QueryRowxContext(context.Background(), tt.args.query)
			require.NotNil(t, row)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDB_Beginx(t *testing.T) {
	tests := []struct {
		name    string
		mockFn  func(sqlmock.Sqlmock)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "given successful begin, then returns Tx",
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
			},
			wantErr: assert.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer mockDB.Close()

			db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))
			tt.mockFn(mock)

			tx, err := db.Beginx()

			tt.wantErr(t, err)
			if err == nil {
				require.NotNil(t, tx)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDB_BindNamed(t *testing.T) {
	type args struct {
		query string
		arg   interface{}
	}

	type user struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}

	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "given struct with named params, then binds correctly",
			args: args{
				query: "SELECT * FROM users WHERE id = :id AND name = :name",
				arg:   user{ID: 1, Name: "John"},
			},
			wantErr: assert.NoError,
		},
		{
			name: "given map with named params, then binds correctly",
			args: args{
				query: "SELECT * FROM users WHERE id = :id",
				arg:   map[string]interface{}{"id": 1},
			},
			wantErr: assert.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB, _, err := sqlmock.New()
			require.NoError(t, err)
			defer mockDB.Close()

			db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))

			query, args, err := db.BindNamed(tt.args.query, tt.args.arg)

			tt.wantErr(t, err)
			if err == nil {
				assert.NotEmpty(t, query)
				assert.NotEmpty(t, args)
			}
		})
	}
}

func TestDB_MapperFunc(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))

	// Should not panic
	db.MapperFunc(func(s string) string {
		return s
	})
}

func TestDB_Connect(t *testing.T) {
	type args struct {
		driverName string
		dsn        string
		opts       []Option
	}

	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "given invalid driver, then returns error",
			args: args{
				driverName: "nonexistent_driver",
				dsn:        "some_dsn",
				opts:       nil,
			},
			wantErr: assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := Connect(
				context.Background(),
				tt.args.driverName,
				tt.args.dsn,
				tt.args.opts...)

			tt.wantErr(t, err)
			if err != nil {
				require.Nil(t, db)
			}
		})
	}
}

func TestDB_MustOpen_Panic(t *testing.T) {
	assert.Panics(t, func() {
		MustOpen("nonexistent_driver", "some_dsn")
	})
}

func TestDB_MustConnect_Panic(t *testing.T) {
	assert.Panics(t, func() {
		MustConnect(context.Background(), "nonexistent_driver", "some_dsn")
	})
}

func TestDB_MustBegin_Panic(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	mock.ExpectBegin().WillReturnError(assert.AnError)

	db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))

	assert.Panics(t, func() {
		db.MustBegin()
	})
}

func TestDB_MustBeginTx_Success(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	mock.ExpectBegin()

	db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))

	assert.NotPanics(t, func() {
		tx := db.MustBeginTx(context.Background(), nil)
		require.NotNil(t, tx)
	})

	require.NoError(t, mock.ExpectationsWereMet())
}
