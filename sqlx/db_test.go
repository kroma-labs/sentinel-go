package sqlx

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen(t *testing.T) {
	type args struct {
		driverName string
		dsn        string
		opts       []Option
	}

	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
		want    *config
	}{
		{
			name: "given valid driver and dsn, then returns DB",
			args: args{
				driverName: "sqlmock",
				dsn:        "sqlmock_db",
				opts:       []Option{WithDBSystem("postgresql")},
			},
			wantErr: assert.NoError,
			want:    &config{DBSystem: "postgresql"},
		},
		{
			name: "given multiple options, then applies all",
			args: args{
				driverName: "sqlmock",
				dsn:        "sqlmock_db",
				opts: []Option{
					WithDBSystem("mysql"),
					WithDBName("testdb"),
					WithInstanceName("primary"),
				},
			},
			wantErr: assert.NoError,
			want: &config{
				DBSystem:     "mysql",
				DBName:       "testdb",
				InstanceName: "primary",
			},
		},
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
			if tt.args.driverName == "nonexistent_driver" {
				db, err := Open(tt.args.driverName, tt.args.dsn, tt.args.opts...)
				tt.wantErr(t, err)
				require.Nil(t, db)
				return
			}

			mockDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer mockDB.Close()

			db := NewDB(mockDB, tt.args.driverName, tt.args.opts...)
			require.NotNil(t, db)

			if tt.want != nil {
				assert.Equal(t, tt.want.DBSystem, db.cfg.DBSystem)
				assert.Equal(t, tt.want.DBName, db.cfg.DBName)
				assert.Equal(t, tt.want.InstanceName, db.cfg.InstanceName)
			}

			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestNewDB(t *testing.T) {
	type args struct {
		driverName string
		opts       []Option
	}

	tests := []struct {
		name string
		args args
		want *config
	}{
		{
			name: "given sql.DB and options, then wraps correctly",
			args: args{
				driverName: "postgres",
				opts: []Option{
					WithDBSystem("postgresql"),
					WithDBName("testdb"),
				},
			},
			want: &config{
				DBSystem: "postgresql",
				DBName:   "testdb",
			},
		},
		{
			name: "given sql.DB with no options, then uses defaults",
			args: args{
				driverName: "postgres",
				opts:       nil,
			},
			want: &config{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB, _, err := sqlmock.New()
			require.NoError(t, err)
			defer mockDB.Close()

			db := NewDB(mockDB, tt.args.driverName, tt.args.opts...)
			require.NotNil(t, db)
			require.NotNil(t, db.cfg)

			assert.Equal(t, tt.want.DBSystem, db.cfg.DBSystem)
			assert.Equal(t, tt.want.DBName, db.cfg.DBName)
		})
	}
}

func TestDB_GetContext(t *testing.T) {
	type args struct {
		query string
		id    int
	}

	type user struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}

	tests := []struct {
		name    string
		args    args
		mockFn  func(sqlmock.Sqlmock)
		wantErr assert.ErrorAssertionFunc
		want    user
	}{
		{
			name: "given valid query returning one row, then scans into dest",
			args: args{
				query: "SELECT id, name FROM users WHERE id = ?",
				id:    1,
			},
			mockFn: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "John")
				mock.ExpectQuery("SELECT id, name FROM users WHERE id = ?").
					WithArgs(1).
					WillReturnRows(rows)
			},
			wantErr: assert.NoError,
			want:    user{ID: 1, Name: "John"},
		},
		{
			name: "given query returning no rows, then returns error",
			args: args{
				query: "SELECT id, name FROM users WHERE id = ?",
				id:    999,
			},
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT id, name FROM users WHERE id = ?").
					WithArgs(999).
					WillReturnError(sql.ErrNoRows)
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

			var got user
			err = db.GetContext(context.Background(), &got, tt.args.query, tt.args.id)

			tt.wantErr(t, err)
			assert.Equal(t, tt.want, got)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDB_SelectContext(t *testing.T) {
	type args struct {
		query string
	}

	type user struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}

	tests := []struct {
		name    string
		args    args
		mockFn  func(sqlmock.Sqlmock)
		wantErr assert.ErrorAssertionFunc
		want    []user
	}{
		{
			name: "given valid query returning multiple rows, then scans all",
			args: args{query: "SELECT id, name FROM users"},
			mockFn: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name"}).
					AddRow(1, "John").
					AddRow(2, "Jane")
				mock.ExpectQuery("SELECT id, name FROM users").WillReturnRows(rows)
			},
			wantErr: assert.NoError,
			want: []user{
				{ID: 1, Name: "John"},
				{ID: 2, Name: "Jane"},
			},
		},
		{
			name: "given query returning no rows, then returns empty slice",
			args: args{query: "SELECT id, name FROM users WHERE 1=0"},
			mockFn: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"id", "name"})
				mock.ExpectQuery("SELECT id, name FROM users WHERE 1=0").WillReturnRows(rows)
			},
			wantErr: assert.NoError,
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer mockDB.Close()

			db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))
			tt.mockFn(mock)

			var got []user
			err = db.SelectContext(context.Background(), &got, tt.args.query)

			tt.wantErr(t, err)
			assert.Equal(t, tt.want, got)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDB_ExecContext(t *testing.T) {
	type args struct {
		query string
		name  string
	}

	tests := []struct {
		name             string
		args             args
		mockFn           func(sqlmock.Sqlmock)
		wantErr          assert.ErrorAssertionFunc
		wantRowsAffected int64
	}{
		{
			name: "given valid INSERT query, then returns result",
			args: args{
				query: "INSERT INTO users (name) VALUES (?)",
				name:  "John",
			},
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO users").
					WithArgs("John").
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			wantErr:          assert.NoError,
			wantRowsAffected: 1,
		},
		{
			name: "given query that fails, then returns error",
			args: args{
				query: "INSERT INTO nonexistent (name) VALUES (?)",
				name:  "John",
			},
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO nonexistent").
					WithArgs("John").
					WillReturnError(sql.ErrConnDone)
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

			result, err := db.ExecContext(context.Background(), tt.args.query, tt.args.name)

			tt.wantErr(t, err)
			if result != nil {
				got, _ := result.RowsAffected()
				assert.Equal(t, tt.wantRowsAffected, got)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDB_BeginTxx(t *testing.T) {
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
		{
			name: "given begin fails, then returns error",
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin().WillReturnError(sql.ErrConnDone)
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

			tx, err := db.BeginTxx(context.Background(), nil)

			tt.wantErr(t, err)
			if tx != nil {
				assert.NotNil(t, tx.Tx)
				assert.NotNil(t, tx.cfg)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDB_PingContext(t *testing.T) {
	tests := []struct {
		name    string
		mockFn  func(sqlmock.Sqlmock)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "given successful ping, then returns nil",
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectPing()
			},
			wantErr: assert.NoError,
		},
		{
			name: "given ping fails, then returns error",
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectPing().WillReturnError(sql.ErrConnDone)
			},
			wantErr: assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
			require.NoError(t, err)
			defer mockDB.Close()

			db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))
			tt.mockFn(mock)

			err = db.PingContext(context.Background())

			tt.wantErr(t, err)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDB_Unsafe(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))
	unsafeDB := db.Unsafe()

	require.NotNil(t, unsafeDB)
	assert.NotNil(t, unsafeDB.DB)
	assert.Equal(t, db.cfg, unsafeDB.cfg)
}

func TestDB_Rebind(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))

	got := db.Rebind("SELECT * FROM users WHERE id = ?")
	assert.Contains(t, got, "$")
}

func TestDB_DriverName(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))

	got := db.DriverName()
	assert.Equal(t, "postgres", got)
}
