package database

import (
	"context"
	"log"
)

// User represents a user in the database
type User struct {
	ID    int    `db:"id"`
	Name  string `db:"name"`
	Email string `db:"email"`
}

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

// InsertUsers inserts sample users into the database using sqlx
func (db *DB) InsertUsers(ctx context.Context) error {
	users := []User{
		{Name: "Alice", Email: "alice@example.com"},
		{Name: "Bob", Email: "bob@example.com"},
		{Name: "Charlie", Email: "charlie@example.com"},
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

// QueryUsers queries users using sqlx's SelectContext (scans into slice)
func (db *DB) QueryUsers(ctx context.Context) error {
	var users []User
	err := db.SelectContext(ctx, &users, "SELECT id, name, email FROM users LIMIT 10")
	if err != nil {
		return err
	}
	log.Printf("📖 Queried %d users via SelectContext", len(users))
	return nil
}

// GetUser queries a single user using sqlx's GetContext
func (db *DB) GetUser(ctx context.Context, name string) (*User, error) {
	var user User
	err := db.GetContext(ctx, &user, "SELECT id, name, email FROM users WHERE name = $1", name)
	if err != nil {
		return nil, err
	}
	log.Printf("📖 Got user via GetContext: %s (%s)", user.Name, user.Email)
	return &user, nil
}

// InsertWithTransaction demonstrates transaction usage with sqlx
func (db *DB) InsertWithTransaction(ctx context.Context) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	// Ensure rollback on error
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Insert a user within transaction
	_, err = tx.ExecContext(ctx,
		"INSERT INTO users (name, email) VALUES ($1, $2) ON CONFLICT DO NOTHING",
		"Transaction User",
		"tx@example.com",
	)
	if err != nil {
		return err
	}

	// Query within transaction using GetContext
	var user User
	err = tx.GetContext(
		ctx,
		&user,
		"SELECT id, name, email FROM users WHERE email = $1",
		"tx@example.com",
	)
	if err != nil {
		return err
	}
	log.Printf("📖 Transaction query result: %s (%s)", user.Name, user.Email)

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		return err
	}
	log.Printf("✅ Transaction committed successfully")
	return nil
}
