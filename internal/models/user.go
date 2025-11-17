package models

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
)

// User represents a user in the system
type User struct {
	UserID       uuid.UUID
	Username     string
	PasswordHash string
	Name         *string
	CreatedAt    string
	UpdatedAt    *string
}

// HasAnyUsers checks if there are any users in the database
func HasAnyUsers(ctx context.Context, db *sql.DB) (bool, error) {
	var exists bool

	// First check if the users table exists
	tableQuery := `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = 'users'
		)
	`

	err := db.QueryRowContext(ctx, tableQuery).Scan(&exists)
	if err != nil {
		return false, err
	}

	if !exists {
		// Table doesn't exist, so no users
		return false, nil
	}

	// Check if any users exist in the table
	userQuery := "SELECT EXISTS(SELECT 1 FROM users LIMIT 1)"
	err = db.QueryRowContext(ctx, userQuery).Scan(&exists)
	if err != nil {
		// Table exists but query failed, could be a permission issue
		// Treat as no users for setup purposes
		return false, nil
	}

	return exists, nil
}

// CreateUser creates a new user in the database
func CreateUser(ctx context.Context, db *sql.DB, username, password, name string) (*User, error) {
	userID := uuid.New()

	query := `
		INSERT INTO users (user_id, username, password_hash, name)
		VALUES ($1, $2, hash_password($3), NULLIF($4, ''))
		RETURNING user_id, username, name, created_at
	`

	user := &User{}
	err := db.QueryRowContext(ctx, query, userID, username, password, name).Scan(
		&user.UserID,
		&user.Username,
		&user.Name,
		&user.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	return user, nil
}

// ValidateUser checks if username and password are valid
func ValidateUser(ctx context.Context, db *sql.DB, username, password string) (*User, error) {
	query := `
		SELECT user_id, username, name, created_at
		FROM users
		WHERE username = $1 AND password_hash = hash_password($2)
	`

	user := &User{}
	err := db.QueryRowContext(ctx, query, username, password).Scan(
		&user.UserID,
		&user.Username,
		&user.Name,
		&user.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // User not found or invalid password
	}
	if err != nil {
		return nil, err
	}

	return user, nil
}
