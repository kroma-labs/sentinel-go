package sqlx

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTx_GetContext(t *testing.T) {
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
				mock.ExpectBegin()
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
				mock.ExpectBegin()
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

			tx, err := db.BeginTxx(context.Background(), nil)
			require.NoError(t, err)

			var got user
			err = tx.GetContext(context.Background(), &got, tt.args.query, tt.args.id)

			tt.wantErr(t, err)
			assert.Equal(t, tt.want, got)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestTx_SelectContext(t *testing.T) {
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
				mock.ExpectBegin()
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

			tx, err := db.BeginTxx(context.Background(), nil)
			require.NoError(t, err)

			var got []user
			err = tx.SelectContext(context.Background(), &got, tt.args.query)

			tt.wantErr(t, err)
			assert.Equal(t, tt.want, got)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestTx_ExecContext(t *testing.T) {
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
				mock.ExpectBegin()
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
				mock.ExpectBegin()
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

			tx, err := db.BeginTxx(context.Background(), nil)
			require.NoError(t, err)

			result, err := tx.ExecContext(context.Background(), tt.args.query, tt.args.name)

			tt.wantErr(t, err)
			if result != nil {
				got, _ := result.RowsAffected()
				assert.Equal(t, tt.wantRowsAffected, got)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestTx_QueryContext(t *testing.T) {
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
				mock.ExpectBegin()
				rows := sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(2)
				mock.ExpectQuery("SELECT id FROM users").WillReturnRows(rows)
			},
			wantErr: assert.NoError,
		},
		{
			name: "given query that fails, then returns error",
			args: args{query: "SELECT id FROM nonexistent"},
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectQuery("SELECT id FROM nonexistent").
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

			tx, err := db.BeginTxx(context.Background(), nil)
			require.NoError(t, err)

			rows, err := tx.QueryContext(context.Background(), tt.args.query)

			tt.wantErr(t, err)
			if rows != nil {
				rows.Close()
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestTx_Commit(t *testing.T) {
	tests := []struct {
		name    string
		mockFn  func(sqlmock.Sqlmock)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "given successful commit, then returns nil",
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectCommit()
			},
			wantErr: assert.NoError,
		},
		{
			name: "given commit fails, then returns error",
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectCommit().WillReturnError(sql.ErrConnDone)
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
			require.NoError(t, err)

			err = tx.Commit()

			tt.wantErr(t, err)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestTx_Rollback(t *testing.T) {
	tests := []struct {
		name    string
		mockFn  func(sqlmock.Sqlmock)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "given successful rollback, then returns nil",
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectRollback()
			},
			wantErr: assert.NoError,
		},
		{
			name: "given rollback fails, then returns error",
			mockFn: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectRollback().WillReturnError(sql.ErrConnDone)
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
			require.NoError(t, err)

			err = tx.Rollback()

			tt.wantErr(t, err)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestTx_Rebind(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	mock.ExpectBegin()

	db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))
	tx, err := db.BeginTxx(context.Background(), nil)
	require.NoError(t, err)

	got := tx.Rebind("SELECT * FROM users WHERE id = ?")
	assert.Contains(t, got, "$")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTx_DriverName(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	mock.ExpectBegin()

	db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))
	tx, err := db.BeginTxx(context.Background(), nil)
	require.NoError(t, err)

	got := tx.DriverName()
	assert.Equal(t, "postgres", got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTx_Unsafe(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	mock.ExpectBegin()

	db := NewDB(mockDB, "postgres", WithDBSystem("postgresql"))
	tx, err := db.BeginTxx(context.Background(), nil)
	require.NoError(t, err)

	unsafeTx := tx.Unsafe()
	require.NotNil(t, unsafeTx)
	assert.NotNil(t, unsafeTx.Tx)
	assert.Equal(t, tx.cfg, unsafeTx.cfg)
	require.NoError(t, mock.ExpectationsWereMet())
}
