package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckSetupStatus_NoConfig(t *testing.T) {
	// Save and clear DATABASE_URL
	origDB := os.Getenv("DATABASE_URL")
	_ = os.Unsetenv("DATABASE_URL")
	defer func() {
		if origDB != "" {
			_ = os.Setenv("DATABASE_URL", origDB)
		}
	}()

	// Create temp dir for config
	tempDir := t.TempDir()
	origHome := os.Getenv("XDG_CONFIG_HOME")
	_ = os.Setenv("XDG_CONFIG_HOME", tempDir)
	defer func() {
		if origHome != "" {
			_ = os.Setenv("XDG_CONFIG_HOME", origHome)
		} else {
			_ = os.Unsetenv("XDG_CONFIG_HOME")
		}
	}()

	// Test with no configuration
	status, err := CheckSetupStatus()
	require.NoError(t, err)
	assert.True(t, status.NeedsSetup)
	assert.False(t, status.HasDatabaseConfig)
	assert.False(t, status.HasUsers)
	assert.Equal(t, "No database configured", status.Reason)
}

func TestCheckSetupStatus_WithInstallLock(t *testing.T) {
	// Create temp config file with install lock
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "kaunta.toml")

	// Write config with install lock
	configContent := `
[security]
install_lock = true

[database]
host = "localhost"
port = 5432
name = "kaunta"
user = "postgres"
`
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	// Change to temp directory
	origDir, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer func() { _ = os.Chdir(origDir) }()

	// Test - should not need setup when install lock is true
	status, err := CheckSetupStatus()
	require.NoError(t, err)
	assert.False(t, status.NeedsSetup)
}

func TestParseDatabaseURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected DatabaseConfig
	}{
		{
			name: "full postgres URL",
			url:  "postgres://user:pass@localhost:5432/dbname?sslmode=disable",
			expected: DatabaseConfig{
				Type:     "postgres",
				Host:     "localhost",
				Port:     5432,
				Name:     "dbname",
				User:     "user",
				Password: "pass",
				SSLMode:  "disable",
			},
		},
		{
			name: "postgresql prefix",
			url:  "postgresql://user@localhost/dbname",
			expected: DatabaseConfig{
				Type:     "postgres",
				Host:     "localhost",
				Port:     5432,
				Name:     "dbname",
				User:     "user",
				Password: "",
				SSLMode:  "disable",
			},
		},
		{
			name: "no password",
			url:  "postgres://user@host:1234/db?sslmode=require",
			expected: DatabaseConfig{
				Type:     "postgres",
				Host:     "host",
				Port:     1234,
				Name:     "db",
				User:     "user",
				Password: "",
				SSLMode:  "require",
			},
		},
		{
			name: "invalid URL",
			url:  "not-a-url",
			expected: DatabaseConfig{
				Type:    "postgres",
				Host:    "localhost",
				Port:    5432,
				SSLMode: "disable",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseDatabaseURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildDatabaseURL(t *testing.T) {
	tests := []struct {
		name     string
		config   DatabaseConfig
		expected string
	}{
		{
			name: "full config",
			config: DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Name:     "testdb",
				User:     "testuser",
				Password: "testpass",
				SSLMode:  "disable",
			},
			expected: "postgres://testuser:testpass@localhost:5432/testdb?sslmode=disable",
		},
		{
			name: "no password",
			config: DatabaseConfig{
				Host:    "db.example.com",
				Port:    5433,
				Name:    "myapp",
				User:    "appuser",
				SSLMode: "require",
			},
			expected: "postgres://appuser@db.example.com:5433/myapp?sslmode=require",
		},
		{
			name: "defaults",
			config: DatabaseConfig{
				Name: "kaunta",
				User: "postgres",
			},
			expected: "postgres://postgres@localhost:5432/kaunta?sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildDatabaseURL(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSaveConfig(t *testing.T) {
	// Create temp directory for config
	tempDir := t.TempDir()

	// Set XDG_CONFIG_HOME to temp directory so config is saved there
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	_ = os.Setenv("XDG_CONFIG_HOME", tempDir)
	defer func() { _ = os.Setenv("XDG_CONFIG_HOME", origXDG) }()

	// Create config to save
	cfg := &Config{
		DatabaseURL:    "postgres://user:pass@localhost:5432/kaunta?sslmode=disable",
		Port:           "3000",
		DataDir:        "./data",
		SecureCookies:  true,
		TrustedOrigins: []string{"localhost", "example.com"},
		InstallLock:    true,
	}

	// Save config
	err := SaveConfig(cfg)
	require.NoError(t, err)

	// Check file exists (in XDG config structure)
	configPath := filepath.Join(tempDir, "kaunta", "kaunta.toml")
	assert.FileExists(t, configPath)

	// Read and verify content
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)

	// Check for key elements
	contentStr := string(content)
	assert.Contains(t, contentStr, "database_url = 'postgres://user:pass@localhost:5432/kaunta?sslmode=disable'")
	assert.Contains(t, contentStr, "[security]")
	assert.Contains(t, contentStr, "install_lock = true")
	assert.Contains(t, contentStr, "[server]")
	assert.Contains(t, contentStr, "port = '3000'")
}
