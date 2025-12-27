package database

import (
	"database/sql"
	"fmt"
	"os"
	"runtime"

	_ "github.com/lib/pq"
	"go.uber.org/zap"

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

	// Configure PostgreSQL 18 async I/O for better performance
	if err = configureAsyncIO(DB); err != nil {
		logging.L().Warn("async I/O configuration skipped", zap.Error(err))
	}

	logging.L().Info("database connected")
	return nil
}

// configureAsyncIO enables PostgreSQL 18 async I/O on supported platforms.
// On Linux with io_uring support, this can provide up to 3x faster sequential scans.
// Falls back gracefully on macOS/BSD/Windows.
func configureAsyncIO(db *sql.DB) error {
	// Only attempt io_uring on Linux (requires kernel 5.1+)
	if runtime.GOOS != "linux" {
		return nil
	}

	// Try to enable io_uring for async I/O
	_, err := db.Exec("SET io_method = 'io_uring'")
	if err != nil {
		// Fallback to worker threads if io_uring unavailable
		_, err = db.Exec("SET io_method = 'worker'")
		if err != nil {
			// Keep sync mode - not a critical error
			return nil
		}
		logging.L().Debug("async I/O using worker threads")
	} else {
		logging.L().Debug("async I/O using io_uring")
	}

	// Increase I/O concurrency for partitioned tables (errors ignored - non-critical)
	_, _ = db.Exec("SET effective_io_concurrency = 16")
	_, _ = db.Exec("SET maintenance_io_concurrency = 16")

	return nil
}

func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
