package database

import (
	"context"
	"log"
)

// CreateTable creates the users table if it doesn't exist
func (db *DB) CreateTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(100),
			email VARCHAR(100)
		)
	`
	_, err := db.ExecContext(ctx, query)
	return err
}

// InsertUsers inserts sample users into the database
func (db *DB) InsertUsers(ctx context.Context) error {
	users := []struct {
		Name  string
		Email string
	}{
		{"Alice", "alice@example.com"},
		{"Bob", "bob@example.com"},
		{"Charlie", "charlie@example.com"},
	}

	for _, user := range users {
		_, err := db.ExecContext(
			ctx,
			"INSERT INTO users (name, email) VALUES ($1, $2) ON CONFLICT DO NOTHING",
			user.Name,
			user.Email,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// QueryUsers queries and logs users from the database
func (db *DB) QueryUsers(ctx context.Context) error {
	rows, err := db.QueryContext(ctx, "SELECT id, name, email FROM users LIMIT 10")
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id int
		var name, email string
		if err := rows.Scan(&id, &name, &email); err != nil {
			return err
		}
		count++
	}
	log.Printf("📖 Queried %d users", count)
	return rows.Err()
}
