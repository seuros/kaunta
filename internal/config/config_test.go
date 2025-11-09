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
	configDir := filepath.Join(home, ".kaunta")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "kaunta.toml"), []byte(contents), 0o644))
}

func TestLoadDefaultsWhenNoConfigSources(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	unsetEnv(t, "DATABASE_URL")
	unsetEnv(t, "PORT")
	unsetEnv(t, "DATA_DIR")

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "", cfg.DatabaseURL)
	assert.Equal(t, "3000", cfg.Port)
	assert.Equal(t, "./data", cfg.DataDir)
}

func TestLoadUsesEnvironmentVariables(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DATABASE_URL", "postgres://env-user:env-pass@localhost:5432/envdb")
	t.Setenv("PORT", "4321")
	t.Setenv("DATA_DIR", "/tmp/env-data")

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "postgres://env-user:env-pass@localhost:5432/envdb", cfg.DatabaseURL)
	assert.Equal(t, "4321", cfg.Port)
	assert.Equal(t, "/tmp/env-data", cfg.DataDir)
}

func TestLoadWithOverridesPriority(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestConfig(t, home, `
database_url = "postgres://config"
port = "4000"
data_dir = "./config-data"
`)

	t.Setenv("DATABASE_URL", "postgres://env")
	t.Setenv("PORT", "5000")
	unsetEnv(t, "DATA_DIR")

	cfg, err := LoadWithOverrides("postgres://flag", "", "")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "postgres://flag", cfg.DatabaseURL)
	assert.Equal(t, "5000", cfg.Port)
	assert.Equal(t, "./config-data", cfg.DataDir)

	cfg, err = LoadWithOverrides("", "", "/override-data")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "postgres://env", cfg.DatabaseURL)
	assert.Equal(t, "5000", cfg.Port)
	assert.Equal(t, "/override-data", cfg.DataDir)
}
