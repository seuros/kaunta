package database

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"

	"github.com/seuros/kaunta/internal/logging"
)

var DB *sql.DB

// Connect connects to database using DATABASE_URL environment variable
func Connect() error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL environment variable not set")
	}
	return ConnectWithURL(databaseURL)
}

// ConnectWithURL connects to database using provided URL
func ConnectWithURL(databaseURL string) error {
	if databaseURL == "" {
		return fmt.Errorf("database URL cannot be empty")
	}

	var err error
	DB, err = sql.Open("postgres", databaseURL)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err = DB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	logging.L().Info("database connected")
	return nil
}

func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
