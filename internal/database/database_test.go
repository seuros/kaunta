package database

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnect_MissingDatabaseURL(t *testing.T) {
	// Save original DATABASE_URL
	originalURL := os.Getenv("DATABASE_URL")
	defer func() {
		if originalURL != "" {
			_ = os.Setenv("DATABASE_URL", originalURL)
		} else {
			_ = os.Unsetenv("DATABASE_URL")
		}
	}()

	// Unset DATABASE_URL
	_ = os.Unsetenv("DATABASE_URL")

	// Attempt to connect
	err := Connect()

	// Should return error
	require.Error(t, err, "Connect should fail when DATABASE_URL is not set")
	assert.Contains(t, err.Error(), "DATABASE_URL environment variable not set", "Error message should mention DATABASE_URL")
}

func TestConnect_InvalidDatabaseURL(t *testing.T) {
	// Save original DATABASE_URL
	originalURL := os.Getenv("DATABASE_URL")
	defer func() {
		if originalURL != "" {
			_ = os.Setenv("DATABASE_URL", originalURL)
		} else {
			_ = os.Unsetenv("DATABASE_URL")
		}
	}()

	// Set invalid DATABASE_URL
	_ = os.Setenv("DATABASE_URL", "invalid://not-a-database")

	// Attempt to connect
	err := Connect()

	// Should return error (connection failure expected)
	require.Error(t, err, "Connect should fail with invalid DATABASE_URL")
}

func TestClose_NilDB(t *testing.T) {
	// Save original DB
	originalDB := DB
	defer func() {
		DB = originalDB
	}()

	// Set DB to nil
	DB = nil

	// Should not panic or error
	err := Close()
	assert.NoError(t, err, "Close should not error when DB is nil")
}

func TestDatabaseURL_Formats(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		shouldError bool
	}{
		{
			name:        "Empty URL",
			url:         "",
			shouldError: true,
		},
		{
			name:        "PostgreSQL URL",
			url:         "postgres://user:pass@localhost:5432/dbname",
			shouldError: false, // sql.Open won't error, but Ping will fail
		},
		{
			name:        "PostgreSQL with SSL",
			url:         "postgres://user:pass@localhost:5432/dbname?sslmode=require",
			shouldError: false, // sql.Open won't error, but Ping will fail
		},
		{
			name:        "Invalid scheme",
			url:         "mysql://user:pass@localhost:3306/dbname",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original DATABASE_URL
			originalURL := os.Getenv("DATABASE_URL")
			defer func() {
				if originalURL != "" {
					_ = os.Setenv("DATABASE_URL", originalURL)
				} else {
					_ = os.Unsetenv("DATABASE_URL")
				}
			}()

			_ = os.Setenv("DATABASE_URL", tt.url)

			err := Connect()

			if tt.url == "" {
				// Empty URL should error immediately
				require.Error(t, err)
				assert.Contains(t, err.Error(), "DATABASE_URL environment variable not set")
			} else {
				// Non-empty URLs will fail at Ping stage (expected since no real DB)
				// We're just testing that the URL parsing doesn't panic
				assert.NotNil(t, err, "Should error due to no actual database connection")
			}
		})
	}
}

func TestDB_GlobalVariable(t *testing.T) {
	// Test that DB global variable exists and can be set
	originalDB := DB

	// Should be able to set to nil
	DB = nil
	assert.Nil(t, DB, "DB should be nil")

	// Restore
	DB = originalDB
}

func TestConnect_ErrorMessages(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		expectedError string
	}{
		{
			name:          "Missing URL",
			url:           "",
			expectedError: "DATABASE_URL environment variable not set",
		},
		{
			name:          "Bad host",
			url:           "postgres://user:pass@nonexistent-host-12345:5432/db",
			expectedError: "failed to ping database",
		},
		{
			name:          "Invalid port",
			url:           "postgres://user:pass@localhost:99999/db",
			expectedError: "", // Will fail at different stage
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original DATABASE_URL
			originalURL := os.Getenv("DATABASE_URL")
			defer func() {
				if originalURL != "" {
					_ = os.Setenv("DATABASE_URL", originalURL)
				} else {
					_ = os.Unsetenv("DATABASE_URL")
				}
			}()

			_ = os.Setenv("DATABASE_URL", tt.url)

			err := Connect()

			require.Error(t, err, "Should return error")

			if tt.expectedError != "" {
				assert.Contains(t, err.Error(), tt.expectedError, "Error message should contain expected text")
			}
		})
	}
}

// Test database connection state management
func TestDatabaseConnectionState(t *testing.T) {
	// This test verifies that the DB variable can be properly managed
	// without requiring an actual database connection

	// Save original state
	originalDB := DB
	defer func() {
		DB = originalDB
	}()

	// Test that DB can be nil
	DB = nil
	assert.Nil(t, DB, "DB should be settable to nil")

	// Test Close with nil DB
	err := Close()
	assert.NoError(t, err, "Close should handle nil DB gracefully")

	// Restore DB for other tests
	DB = originalDB
}

// Benchmark database operations (no actual DB needed)
func BenchmarkConnect(b *testing.B) {
	// Save original DATABASE_URL
	originalURL := os.Getenv("DATABASE_URL")
	defer func() {
		if originalURL != "" {
			_ = os.Setenv("DATABASE_URL", originalURL)
		} else {
			_ = os.Unsetenv("DATABASE_URL")
		}
	}()

	// Set a dummy URL (will fail but we're measuring the attempt)
	_ = os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")

	b.ResetTimer()
	for b.Loop() {
		_ = Connect()
		_ = Close()
	}
}
