package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config holds application configuration
type Config struct {
	DatabaseURL    string
	Port           string
	DataDir        string
	SecureCookies  bool
	TrustedOrigins []string
}

// Load loads configuration from multiple sources with priority:
// 1. Command flags (set via viper.Set)
// 2. Config file (~/.kaunta/config.toml or ./kaunta.toml)
// 3. Environment variables
func Load() (*Config, error) {
	v := newBaseViper()
	_ = v.ReadInConfig()
	return buildConfig(v, "", "", ""), nil
}

// LoadWithOverrides loads config and applies flag overrides
func LoadWithOverrides(databaseURL, port, dataDir string) (*Config, error) {
	v := newBaseViper()
	_ = v.ReadInConfig()
	return buildConfig(v, databaseURL, port, dataDir), nil
}

func newBaseViper() *viper.Viper {
	v := viper.New()
	v.SetConfigName("kaunta")
	v.SetConfigType("toml")
	v.AddConfigPath(".")
	if home, err := os.UserHomeDir(); err == nil {
		v.AddConfigPath(filepath.Join(home, ".kaunta"))
	}
	return v
}

func buildConfig(v *viper.Viper, overrideDatabaseURL, overridePort, overrideDataDir string) *Config {
	cfg := &Config{
		Port:           "3000",
		DataDir:        "./data",
		SecureCookies:  true, // Default to secure (safe for production/HTTPS proxies)
		TrustedOrigins: []string{"localhost"},
	}

	// Apply config file values
	if v.IsSet("database_url") {
		cfg.DatabaseURL = v.GetString("database_url")
	}
	if v.IsSet("port") {
		cfg.Port = v.GetString("port")
	}
	if v.IsSet("data_dir") {
		cfg.DataDir = v.GetString("data_dir")
	}
	if v.IsSet("trusted_origins") {
		cfg.TrustedOrigins = parseTrustedOrigins(v.GetString("trusted_origins"))
	}
	if v.IsSet("secure_cookies") {
		cfg.SecureCookies = v.GetBool("secure_cookies")
	}

	// Environment fallback (only if not configured)
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	}
	if !v.IsSet("port") {
		if envPort := os.Getenv("PORT"); envPort != "" {
			cfg.Port = envPort
		}
	}
	if !v.IsSet("data_dir") {
		if envDataDir := os.Getenv("DATA_DIR"); envDataDir != "" {
			cfg.DataDir = envDataDir
		}
	}
	if !v.IsSet("trusted_origins") {
		if envOrigins := os.Getenv("TRUSTED_ORIGINS"); envOrigins != "" {
			cfg.TrustedOrigins = parseTrustedOrigins(envOrigins)
		}
	}
	if !v.IsSet("secure_cookies") {
		if envSecure := os.Getenv("SECURE_COOKIES"); envSecure != "" {
			cfg.SecureCookies = envSecure == "true"
		}
		// Otherwise keep default (true)
	}

	// Apply overrides (flags) last
	if overrideDatabaseURL != "" {
		cfg.DatabaseURL = overrideDatabaseURL
	}
	if overridePort != "" {
		cfg.Port = overridePort
	}
	if overrideDataDir != "" {
		cfg.DataDir = overrideDataDir
	}

	return cfg
}

// parseTrustedOrigins parses a comma-separated string into a slice of trimmed, lowercased origins
func parseTrustedOrigins(originsStr string) []string {
	if originsStr == "" {
		return []string{}
	}

	parts := strings.Split(originsStr, ",")
	origins := make([]string, 0, len(parts))

	for _, part := range parts {
		origin := strings.TrimSpace(part)
		origin = strings.ToLower(origin)
		// Strip protocol if provided
		origin = strings.TrimPrefix(origin, "http://")
		origin = strings.TrimPrefix(origin, "https://")
		origin = strings.TrimSuffix(origin, "/")

		if origin != "" {
			origins = append(origins, origin)
		}
	}

	return origins
}
