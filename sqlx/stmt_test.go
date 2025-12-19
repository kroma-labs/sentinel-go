package sqlx

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStmt_GetContext(t *testing.T) {
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
			name: "given valid prepared query, then scans into dest",
			args: args{
				query: "SELECT id, name FROM users WHERE id = ?",
				id:    1,
			},
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectPrepare("SELECT id, name FROM users WHERE id = ?")
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
				mock.ExpectPrepare("SELECT id, name FROM users WHERE id = ?")
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

			stmt, err := db.PreparexContext(context.Background(), tt.args.query)
			require.NoError(t, err)

			var got user
			err = stmt.GetContext(context.Background(), &got, tt.args.id)

			tt.wantErr(t, err)
			assert.Equal(t, tt.want, got)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStmt_SelectContext(t *testing.T) {
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
			name: "given valid prepared query, then scans all rows",
			args: args{query: "SELECT id, name FROM users"},
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectPrepare("SELECT id, name FROM users")
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer mockDB.Close()

			db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))
			tt.mockFn(mock)

			stmt, err := db.PreparexContext(context.Background(), tt.args.query)
			require.NoError(t, err)

			var got []user
			err = stmt.SelectContext(context.Background(), &got)

			tt.wantErr(t, err)
			assert.Equal(t, tt.want, got)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStmt_ExecContext(t *testing.T) {
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
			name: "given valid prepared INSERT, then returns result",
			args: args{
				query: "INSERT INTO users (name) VALUES (?)",
				name:  "John",
			},
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectPrepare("INSERT INTO users")
				mock.ExpectExec("INSERT INTO users").
					WithArgs("John").
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			wantErr:          assert.NoError,
			wantRowsAffected: 1,
		},
		{
			name: "given exec that fails, then returns error",
			args: args{
				query: "INSERT INTO users (name) VALUES (?)",
				name:  "John",
			},
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectPrepare("INSERT INTO users")
				mock.ExpectExec("INSERT INTO users").
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

			stmt, err := db.PreparexContext(context.Background(), tt.args.query)
			require.NoError(t, err)

			result, err := stmt.ExecContext(context.Background(), tt.args.name)

			tt.wantErr(t, err)
			if result != nil {
				got, _ := result.RowsAffected()
				assert.Equal(t, tt.wantRowsAffected, got)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStmt_QueryContext(t *testing.T) {
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
			name: "given valid prepared query, then returns rows",
			args: args{query: "SELECT id FROM users"},
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectPrepare("SELECT id FROM users")
				rows := sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(2)
				mock.ExpectQuery("SELECT id FROM users").WillReturnRows(rows)
			},
			wantErr: assert.NoError,
		},
		{
			name: "given query that fails, then returns error",
			args: args{query: "SELECT id FROM users"},
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectPrepare("SELECT id FROM users")
				mock.ExpectQuery("SELECT id FROM users").
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

			stmt, err := db.PreparexContext(context.Background(), tt.args.query)
			require.NoError(t, err)

			rows, err := stmt.QueryContext(context.Background())

			tt.wantErr(t, err)
			if rows != nil {
				rows.Close()
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStmt_QueryRowContext(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	mock.ExpectPrepare("SELECT id FROM users WHERE id = ?")
	rows := sqlmock.NewRows([]string{"id"}).AddRow(1)
	mock.ExpectQuery("SELECT id FROM users WHERE id = ?").
		WithArgs(1).
		WillReturnRows(rows)

	db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))

	stmt, err := db.PreparexContext(context.Background(), "SELECT id FROM users WHERE id = ?")
	require.NoError(t, err)

	row := stmt.QueryRowContext(context.Background(), 1)
	require.NotNil(t, row)

	var got int
	err = row.Scan(&got)
	require.NoError(t, err)
	assert.Equal(t, 1, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStmt_QueryxContext(t *testing.T) {
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
			name: "given valid prepared query, then returns sqlx rows",
			args: args{query: "SELECT id, name FROM users"},
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectPrepare("SELECT id, name FROM users")
				rows := sqlmock.NewRows([]string{"id", "name"}).
					AddRow(1, "John").
					AddRow(2, "Jane")
				mock.ExpectQuery("SELECT id, name FROM users").WillReturnRows(rows)
			},
			wantErr: assert.NoError,
		},
		{
			name: "given query that fails, then returns error",
			args: args{query: "SELECT id FROM users"},
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectPrepare("SELECT id FROM users")
				mock.ExpectQuery("SELECT id FROM users").
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

			stmt, err := db.PreparexContext(context.Background(), tt.args.query)
			require.NoError(t, err)

			rows, err := stmt.QueryxContext(context.Background())

			tt.wantErr(t, err)
			if rows != nil {
				rows.Close()
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestStmt_QueryRowxContext(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	mock.ExpectPrepare("SELECT id, name FROM users WHERE id = ?")
	rows := sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "John")
	mock.ExpectQuery("SELECT id, name FROM users WHERE id = ?").
		WithArgs(1).
		WillReturnRows(rows)

	db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))

	stmt, err := db.PreparexContext(context.Background(), "SELECT id, name FROM users WHERE id = ?")
	require.NoError(t, err)

	row := stmt.QueryRowxContext(context.Background(), 1)
	require.NotNil(t, row)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStmt_Unsafe(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	mock.ExpectPrepare("SELECT id FROM users")

	db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))

	stmt, err := db.PreparexContext(context.Background(), "SELECT id FROM users")
	require.NoError(t, err)

	unsafeStmt := stmt.Unsafe()
	require.NotNil(t, unsafeStmt)
	assert.Equal(t, stmt.cfg, unsafeStmt.cfg)
	assert.Equal(t, stmt.query, unsafeStmt.query)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDB_PreparexContext(t *testing.T) {
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
			name: "given valid query, then returns prepared stmt",
			args: args{query: "SELECT id FROM users WHERE id = ?"},
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectPrepare("SELECT id FROM users WHERE id = ?")
			},
			wantErr: assert.NoError,
		},
		{
			name: "given prepare fails, then returns error",
			args: args{query: "INVALID SQL"},
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectPrepare("INVALID SQL").WillReturnError(assert.AnError)
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

			stmt, err := db.PreparexContext(context.Background(), tt.args.query)

			tt.wantErr(t, err)
			if err == nil {
				require.NotNil(t, stmt)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDB_Preparex(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	mock.ExpectPrepare("SELECT id FROM users")

	db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))

	stmt, err := db.Preparex("SELECT id FROM users")
	require.NoError(t, err)
	require.NotNil(t, stmt)
	require.NoError(t, mock.ExpectationsWereMet())
}
