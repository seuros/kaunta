// Package test provides testing utilities for Kaunta
package test

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/peterldowns/pgtestdb"
	"github.com/peterldowns/pgtestdb/migrators/golangmigrator"
)

// TestDB holds database connection for tests
type TestDB struct {
	DB *sql.DB
}

// NewTestDB creates a fresh test database with migrations applied
func NewTestDB(t *testing.T) *TestDB {
	t.Helper()

	// Find the project root by looking for migrations directory
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	var migrationsPath string

	// Walk up directories to find migrations
	currentPath := wd
	for {
		testPath := filepath.Join(currentPath, "internal", "database", "migrations")
		if _, err := os.Stat(testPath); err == nil {
			migrationsPath = testPath
			break
		}
		parent := filepath.Dir(currentPath)
		if parent == currentPath {
			t.Fatalf("could not find migrations directory")
		}
		currentPath = parent
	}

	// Get database URL from environment or construct default
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}

	// Parse DATABASE_URL to extract connection parameters
	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("failed to parse DATABASE_URL: %v", err)
	}

	host := parsedURL.Hostname()
	port := parsedURL.Port()
	if port == "" {
		port = "5432"
	}

	user := parsedURL.User.Username()
	password, _ := parsedURL.User.Password()

	database := strings.TrimPrefix(parsedURL.Path, "/")
	if database == "" {
		database = "postgres"
	}

	options := parsedURL.RawQuery

	// Create isolated test database using template cloning
	// This is much faster than running migrations for each test (~20ms per test)
	// Use the same user for both admin operations and test database connections
	// to avoid needing to create a separate pgtdbuser in CI
	testRole := &pgtestdb.Role{
		Username:     user,
		Password:     password,
		Capabilities: "NOSUPERUSER NOCREATEDB NOCREATEROLE",
	}

	db := pgtestdb.New(t, pgtestdb.Config{
		DriverName: "pgx",
		Host:       host,
		Port:       port,
		User:       user,
		Password:   password,
		Database:   database,
		Options:    options,
		TestRole:   testRole,
	}, golangmigrator.New(migrationsPath))

	return &TestDB{
		DB: db,
	}
}

// Close closes the database connection
func (tdb *TestDB) Close() error {
	if tdb.DB != nil {
		return tdb.DB.Close()
	}
	return nil
}

// Exec executes a raw SQL query for test setup/teardown
func (tdb *TestDB) Exec(ctx context.Context, query string, args ...any) error {
	_, err := tdb.DB.ExecContext(ctx, query, args...)
	return err
}

// QueryRow executes a query returning a single row
func (tdb *TestDB) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return tdb.DB.QueryRowContext(ctx, query, args...)
}

// Query executes a query returning multiple rows
func (tdb *TestDB) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return tdb.DB.QueryContext(ctx, query, args...)
}
