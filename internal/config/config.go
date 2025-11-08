package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds application configuration
type Config struct {
	DatabaseURL string
	Port        string
	DataDir     string
}

// Load loads configuration from multiple sources with priority:
// 1. Command flags (set via viper.Set)
// 2. Config file (~/.kaunta/config.toml or ./kaunta.toml)
// 3. Environment variables
func Load() (*Config, error) {
	v := viper.New()

	// Set config file name and type
	v.SetConfigName("kaunta")
	v.SetConfigType("toml")

	// Add config search paths
	// 1. Current directory
	v.AddConfigPath(".")

	// 2. User home directory (~/.kaunta/)
	if home, err := os.UserHomeDir(); err == nil {
		v.AddConfigPath(filepath.Join(home, ".kaunta"))
	}

	// Set default values
	v.SetDefault("port", "3000")
	v.SetDefault("data_dir", "./data")

	// Bind environment variables
	v.SetEnvPrefix("") // No prefix, allow DATABASE_URL directly
	v.AutomaticEnv()

	// Map environment variable names to config keys
	_ = v.BindEnv("database_url", "DATABASE_URL")
	_ = v.BindEnv("port", "PORT")
	_ = v.BindEnv("data_dir", "DATA_DIR")

	// Read config file if it exists (don't error if not found)
	_ = v.ReadInConfig()

	return &Config{
		DatabaseURL: v.GetString("database_url"),
		Port:        v.GetString("port"),
		DataDir:     v.GetString("data_dir"),
	}, nil
}

// LoadWithOverrides loads config and applies flag overrides
func LoadWithOverrides(databaseURL, port, dataDir string) (*Config, error) {
	v := viper.New()

	// Set config file name and type
	v.SetConfigName("kaunta")
	v.SetConfigType("toml")

	// Add config search paths
	v.AddConfigPath(".")
	if home, err := os.UserHomeDir(); err == nil {
		v.AddConfigPath(filepath.Join(home, ".kaunta"))
	}

	// Set default values
	v.SetDefault("port", "3000")
	v.SetDefault("data_dir", "./data")

	// Bind environment variables
	v.SetEnvPrefix("")
	v.AutomaticEnv()
	_ = v.BindEnv("database_url", "DATABASE_URL")
	_ = v.BindEnv("port", "PORT")
	_ = v.BindEnv("data_dir", "DATA_DIR")

	// Read config file
	_ = v.ReadInConfig()

	// Apply flag overrides (highest priority)
	if databaseURL != "" {
		v.Set("database_url", databaseURL)
	}
	if port != "" {
		v.Set("port", port)
	}
	if dataDir != "" {
		v.Set("data_dir", dataDir)
	}

	return &Config{
		DatabaseURL: v.GetString("database_url"),
		Port:        v.GetString("port"),
		DataDir:     v.GetString("data_dir"),
	}, nil
}
