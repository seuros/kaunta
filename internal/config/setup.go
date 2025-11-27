package config

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/lib/pq"
	"github.com/spf13/viper"

	"github.com/seuros/kaunta/internal/models"
)

// SetupStatus contains information about the setup state
type SetupStatus struct {
	NeedsSetup        bool   // Whether setup wizard needs to run
	HasDatabaseConfig bool   // Whether database configuration exists
	HasUsers          bool   // Whether there are any users in the database
	Reason            string // Human-readable reason for needing setup
}

// CheckSetupStatus determines if the setup wizard needs to run
func CheckSetupStatus() (*SetupStatus, error) {
	status := &SetupStatus{}

	// Load configuration
	cfg, err := Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Check if we have install lock
	v := newBaseViper()
	_ = v.ReadInConfig()
	if v.IsSet("security.install_lock") && v.GetBool("security.install_lock") {
		// Installation is locked, skip setup
		status.NeedsSetup = false
		return status, nil
	}

	// Check database configuration
	if cfg.DatabaseURL == "" {
		status.NeedsSetup = true
		status.HasDatabaseConfig = false
		status.Reason = "No database configured"
		return status, nil
	}

	status.HasDatabaseConfig = true

	// Try to connect to the database
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		status.NeedsSetup = true
		status.Reason = "Database connection failed"
		return status, nil
	}
	defer func() { _ = db.Close() }()

	// Test the connection
	if err := db.Ping(); err != nil {
		status.NeedsSetup = true
		status.Reason = "Cannot reach database"
		return status, nil
	}

	// Check if users table exists and has any users
	hasUsers, err := models.HasAnyUsers(context.Background(), db)
	if err != nil {
		// Table might not exist yet, treat as no users
		status.HasUsers = false
		status.NeedsSetup = true
		status.Reason = "Database not initialized"
		return status, nil
	}

	status.HasUsers = hasUsers
	if !hasUsers {
		status.NeedsSetup = true
		status.Reason = "No users found"
		return status, nil
	}

	// All checks passed
	status.NeedsSetup = false
	return status, nil
}

// SaveConfig saves the configuration to a TOML file
func SaveConfig(cfg *Config) error {
	// Determine config path
	configPath := getConfigPath()

	// Create directory if it doesn't exist
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create viper instance
	v := viper.New()
	v.SetConfigType("toml")

	// Save database URL directly (not nested keys)
	if cfg.DatabaseURL != "" {
		v.Set("database_url", cfg.DatabaseURL)
	}

	// Set server configuration
	v.Set("server.host", "0.0.0.0")
	if cfg.Port != "" {
		v.Set("server.port", cfg.Port)
	}

	// Set other configuration
	if cfg.DataDir != "" {
		v.Set("data_dir", cfg.DataDir)
	}

	// Set trusted origins
	if len(cfg.TrustedOrigins) > 0 {
		v.Set("trusted_origins", strings.Join(cfg.TrustedOrigins, ","))
	}

	v.Set("secure_cookies", cfg.SecureCookies)

	// Set install lock
	v.Set("security.install_lock", true)

	// Write config file
	if err := v.WriteConfigAs(configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Set restrictive permissions (config contains database password)
	if err := os.Chmod(configPath, 0600); err != nil {
		return fmt.Errorf("failed to set config permissions: %w", err)
	}

	return nil
}

// getConfigPath returns the path to the configuration file
func getConfigPath() string {
	// Check current directory first
	if _, err := os.Stat("kaunta.toml"); err == nil {
		return "kaunta.toml"
	}

	// Use XDG config directory
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		if home, err := os.UserHomeDir(); err == nil {
			configHome = filepath.Join(home, ".config")
		}
	}

	if configHome != "" {
		return filepath.Join(configHome, "kaunta", "kaunta.toml")
	}

	// Fallback to current directory
	return "kaunta.toml"
}

// DatabaseConfig represents database connection parameters
type DatabaseConfig struct {
	Type     string
	Host     string
	Port     int
	Name     string
	User     string
	Password string
	SSLMode  string
}

// ParseDatabaseURL parses a PostgreSQL connection URL
func ParseDatabaseURL(url string) DatabaseConfig {
	cfg := DatabaseConfig{
		Type:    "postgres",
		Host:    "localhost",
		Port:    5432,
		SSLMode: "disable",
	}

	// Basic parsing of postgres://user:pass@host:port/dbname?sslmode=disable
	if strings.HasPrefix(url, "postgres://") || strings.HasPrefix(url, "postgresql://") {
		url = strings.TrimPrefix(url, "postgres://")
		url = strings.TrimPrefix(url, "postgresql://")

		// Parse user:pass@host:port/dbname?params
		parts := strings.Split(url, "@")
		if len(parts) == 2 {
			// Parse user:pass
			userPass := parts[0]
			if idx := strings.Index(userPass, ":"); idx > -1 {
				cfg.User = userPass[:idx]
				cfg.Password = userPass[idx+1:]
			} else {
				cfg.User = userPass
			}

			// Parse host:port/dbname?params
			remainder := parts[1]

			// Split by query params
			if idx := strings.Index(remainder, "?"); idx > -1 {
				params := remainder[idx+1:]
				remainder = remainder[:idx]

				// Parse params
				for _, param := range strings.Split(params, "&") {
					kv := strings.Split(param, "=")
					if len(kv) == 2 && kv[0] == "sslmode" {
						cfg.SSLMode = kv[1]
					}
				}
			}

			// Parse host:port/dbname
			if idx := strings.Index(remainder, "/"); idx > -1 {
				hostPort := remainder[:idx]
				cfg.Name = remainder[idx+1:]

				// Parse host:port
				if idx := strings.LastIndex(hostPort, ":"); idx > -1 {
					cfg.Host = hostPort[:idx]
					if port := hostPort[idx+1:]; port != "" {
						_, _ = fmt.Sscanf(port, "%d", &cfg.Port)
					}
				} else {
					cfg.Host = hostPort
				}
			}
		}
	}

	return cfg
}

// BuildDatabaseURL constructs a PostgreSQL connection URL from configuration
func BuildDatabaseURL(cfg DatabaseConfig) string {
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == 0 {
		cfg.Port = 5432
	}
	if cfg.SSLMode == "" {
		cfg.SSLMode = "disable"
	}

	// Build the connection string
	url := fmt.Sprintf("postgres://%s", cfg.User)
	if cfg.Password != "" {
		url += fmt.Sprintf(":%s", cfg.Password)
	}
	url += fmt.Sprintf("@%s:%d/%s?sslmode=%s", cfg.Host, cfg.Port, cfg.Name, cfg.SSLMode)

	return url
}
