package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	original, existed := os.LookupEnv(key)
	if existed {
		t.Cleanup(func() {
			_ = os.Setenv(key, original)
		})
	} else {
		t.Cleanup(func() {
			_ = os.Unsetenv(key)
		})
	}
	_ = os.Unsetenv(key)
}

func writeTestConfig(t *testing.T, home string, contents string) {
	t.Helper()
	// Use XDG config path
	configDir := filepath.Join(home, ".config", "kaunta")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "kaunta.toml"), []byte(contents), 0o644))
}

func TestLoadDefaultsWhenNoConfigSources(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpHome, ".config"))
	unsetEnv(t, "DATABASE_URL")
	unsetEnv(t, "PORT")
	unsetEnv(t, "DATA_DIR")
	unsetEnv(t, "SECURE_COOKIES")

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "", cfg.DatabaseURL)
	assert.Equal(t, "3000", cfg.Port)
	assert.Equal(t, "./data", cfg.DataDir)
	assert.True(t, cfg.SecureCookies) // Default to secure cookies for production safety
}

func TestLoadUsesEnvironmentVariables(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpHome, ".config"))
	t.Setenv("DATABASE_URL", "postgres://env-user:env-pass@localhost:5432/envdb")
	t.Setenv("PORT", "4321")
	t.Setenv("DATA_DIR", "/tmp/env-data")
	t.Setenv("SECURE_COOKIES", "true")

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "postgres://env-user:env-pass@localhost:5432/envdb", cfg.DatabaseURL)
	assert.Equal(t, "4321", cfg.Port)
	assert.Equal(t, "/tmp/env-data", cfg.DataDir)
	assert.True(t, cfg.SecureCookies)
}

func TestLoadWithOverridesPriority(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	writeTestConfig(t, home, `
database_url = "postgres://config"
port = "4000"
data_dir = "./config-data"
secure_cookies = true
`)

	t.Setenv("DATABASE_URL", "postgres://env")
	t.Setenv("PORT", "5000")
	unsetEnv(t, "DATA_DIR")
	t.Setenv("SECURE_COOKIES", "false")

	cfg, err := LoadWithOverrides("postgres://flag", "", "")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "postgres://flag", cfg.DatabaseURL)
	assert.Equal(t, "4000", cfg.Port)
	assert.Equal(t, "./config-data", cfg.DataDir)
	assert.True(t, cfg.SecureCookies)

	cfg, err = LoadWithOverrides("", "", "/override-data")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "postgres://config", cfg.DatabaseURL)
	assert.Equal(t, "4000", cfg.Port)
	assert.Equal(t, "/override-data", cfg.DataDir)
	assert.True(t, cfg.SecureCookies)
}

func TestLoadFallsBackToEnvWhenConfigMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	writeTestConfig(t, home, `
data_dir = "./config-data"
`)

	t.Setenv("DATABASE_URL", "postgres://env")
	t.Setenv("PORT", "5000")
	t.Setenv("SECURE_COOKIES", "true")
	t.Setenv("TRUSTED_ORIGINS", "example.com,foo.test")

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "postgres://env", cfg.DatabaseURL)
	assert.Equal(t, "5000", cfg.Port)
	assert.Equal(t, "./config-data", cfg.DataDir)
	assert.True(t, cfg.SecureCookies)
	assert.Equal(t, []string{"example.com", "foo.test"}, cfg.TrustedOrigins)
}

func TestParseTrustedOrigins(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single domain without scheme",
			input:    "example.com",
			expected: []string{"example.com"},
		},
		{
			name:     "preserves https scheme",
			input:    "https://example.com",
			expected: []string{"https://example.com"},
		},
		{
			name:     "preserves http scheme",
			input:    "http://example.com",
			expected: []string{"http://example.com"},
		},
		{
			name:     "multiple origins with mixed schemes",
			input:    "https://secure.example.com, http://insecure.test, plain.domain",
			expected: []string{"https://secure.example.com", "http://insecure.test", "plain.domain"},
		},
		{
			name:     "strips trailing slashes",
			input:    "https://example.com/",
			expected: []string{"https://example.com"},
		},
		{
			name:     "lowercases origins",
			input:    "HTTPS://Example.COM",
			expected: []string{"https://example.com"},
		},
		{
			name:     "trims whitespace",
			input:    "  https://example.com  ,  http://test.com  ",
			expected: []string{"https://example.com", "http://test.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTrustedOrigins(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
