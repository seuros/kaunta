package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/seuros/kaunta/internal/config"
	"github.com/seuros/kaunta/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleHealthPayload(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp := httptest.NewRecorder()
	handleHealth(resp, req)

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Equal(t, "healthy", payload["status"])
	assert.Equal(t, "kaunta", payload["service"])
}

func stubPingDatabase(t *testing.T, fn func() error) {
	t.Helper()
	original := pingDatabase
	pingDatabase = fn
	t.Cleanup(func() {
		pingDatabase = original
	})
}

func TestHandleUpReturnsOKWhenDatabaseHealthy(t *testing.T) {
	stubPingDatabase(t, func() error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/up", nil)
	resp := httptest.NewRecorder()
	upHandler(resp, req)
	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestHandleUpReturnsServiceUnavailableWhenPingFails(t *testing.T) {
	stubPingDatabase(t, func() error {
		return errors.New("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/up", nil)
	resp := httptest.NewRecorder()
	upHandler(resp, req)

	assert.Equal(t, http.StatusServiceUnavailable, resp.Code)
}

func TestHandleVersionReturnsCurrentVersion(t *testing.T) {
	originalVersion := Version
	Version = "1.2.3"
	t.Cleanup(func() {
		Version = originalVersion
	})

	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	resp := httptest.NewRecorder()
	handleVersion(resp, req)

	var payload map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	assert.Equal(t, "1.2.3", payload["version"])
}

func TestHandleTrackerScriptSetsCachingAndSecurityHeaders(t *testing.T) {
	script := []byte("console.log('hello');")
	req := httptest.NewRequest(http.MethodGet, "/k.js", nil)
	resp := httptest.NewRecorder()
	handleTrackerScript(script).ServeHTTP(resp, req)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	hash := sha256.Sum256(script)
	expectedETag := `"` + hex.EncodeToString(hash[:8]) + `"`

	assert.Equal(t, string(script), string(body))
	assert.Equal(t, "application/javascript; charset=utf-8", resp.Header().Get("Content-Type"))
	assert.Equal(t, expectedETag, resp.Header().Get("ETag"))
	assert.Equal(t, "public, max-age=3600, immutable", resp.Header().Get("Cache-Control"))
	assert.Equal(t, "*", resp.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "*", resp.Header().Get("Timing-Allow-Origin"))
}

func TestGetEnvReturnsOverrides(t *testing.T) {
	t.Setenv("CLI_TEST_KEY", "present")
	assert.Equal(t, "present", getEnv("CLI_TEST_KEY", "fallback"))

	t.Setenv("CLI_EMPTY_KEY", "")
	assert.Equal(t, "fallback", getEnv("CLI_EMPTY_KEY", "fallback"))
}

func TestLoginPageHTMLContainsFormAndScript(t *testing.T) {
	html := loginPageHTML()
	assert.Contains(t, html, `<form id="loginForm">`)
	assert.Contains(t, html, "fetch('/api/auth/login'")
	assert.Contains(t, html, "window.location.href = '/dashboard'")
}

func TestLoginPageHTMLReadsCSRFFromCookie(t *testing.T) {
	html := loginPageHTML()
	// Should read token from cookie
	assert.Contains(t, html, "getCsrfToken()")
	assert.Contains(t, html, "kaunta_csrf=")
	// Should send token in header
	assert.Contains(t, html, "'X-CSRF-Token': csrfToken")
	// Should NOT fetch token from API
	assert.NotContains(t, html, "fetch('/api/auth/csrf')")
	assert.NotContains(t, html, "fetchCSRFToken()")
}

func TestSecureCookiesEnabledUsesConfigWhenPresent(t *testing.T) {
	cfg := &config.Config{SecureCookies: true}
	assert.True(t, secureCookiesEnabled(cfg))

	cfg.SecureCookies = false
	assert.False(t, secureCookiesEnabled(cfg))
}

func TestSecureCookiesEnabledFallsBackToEnv(t *testing.T) {
	t.Setenv("SECURE_COOKIES", "true")
	assert.True(t, secureCookiesEnabled(nil))

	t.Setenv("SECURE_COOKIES", "false")
	assert.False(t, secureCookiesEnabled(nil))
}

func TestSyncTrustedOriginsUpsertsDomains(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = mockDB.Close() })

	origDB := database.DB
	database.DB = mockDB
	t.Cleanup(func() { database.DB = origDB })

	domains := []string{"example.com", "app.test"}

	for _, domain := range domains {
		mock.ExpectExec("INSERT INTO trusted_origin").
			WithArgs(domain).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}

	syncTrustedOrigins(domains)

	require.NoError(t, mock.ExpectationsWereMet())
}
