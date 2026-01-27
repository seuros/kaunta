package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// SelfWebsiteID is the hardcoded UUID for the self-tracking website (dogfooding)
// Using nil UUID makes it deterministic and easily identifiable across all installations
const SelfWebsiteID = "00000000-0000-0000-0000-000000000000"

// Config holds application configuration
type Config struct {
	DatabaseURL    string
	Port           string
	DataDir        string
	SecureCookies  bool
	TrustedOrigins []string
	InstallLock    bool // Whether installation is locked (setup completed)
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
	v.AddConfigPath(".")

	// Use XDG Base Directory specification
	// Manual implementation to support testing (xdg library caches at init)
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		if home, err := os.UserHomeDir(); err == nil {
			configHome = filepath.Join(home, ".config")
		}
	}
	if configHome != "" {
		v.AddConfigPath(filepath.Join(configHome, "kaunta"))
	}

	return v
}

func buildConfig(v *viper.Viper, overrideDatabaseURL, overridePort, overrideDataDir string) *Config {
	cfg := &Config{
		Port:           "3000",
		DataDir:        "./data",
		SecureCookies:  true, // Default to secure (safe for production/HTTPS proxies)
		TrustedOrigins: []string{"localhost"},
		InstallLock:    false,
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
	if v.IsSet("security.install_lock") {
		cfg.InstallLock = v.GetBool("security.install_lock")
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
		origin, err := SanitizeTrustedDomain(part)
		if err != nil {
			continue
		}
		origins = append(origins, origin)
	}

	return origins
}
