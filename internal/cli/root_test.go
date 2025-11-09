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

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newFiberApp(path string, handler fiber.Handler) *fiber.App {
	app := fiber.New()
	app.Get(path, handler)
	return app
}

func performRequest(t *testing.T, app *fiber.App, target string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	return resp
}

func TestHandleIndexReturnsHTML(t *testing.T) {
	app := newFiberApp("/", handleIndex([]byte("<h1>Hello</h1>")))
	resp := performRequest(t, app, "/")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, "<h1>Hello</h1>", string(body))
	assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
}

func TestHandleHealthPayload(t *testing.T) {
	app := newFiberApp("/health", handleHealth)
	resp := performRequest(t, app, "/health")

	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "healthy", payload["status"])
	assert.Equal(t, "kaunta", payload["service"])
	assert.Equal(t, false, payload["react"])
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

	app := newFiberApp("/up", handleUp)
	resp := performRequest(t, app, "/up")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHandleUpReturnsServiceUnavailableWhenPingFails(t *testing.T) {
	stubPingDatabase(t, func() error {
		return errors.New("boom")
	})

	app := newFiberApp("/up", handleUp)
	resp := performRequest(t, app, "/up")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Contains(t, string(body), "database unavailable")
}

func TestHandleVersionReturnsCurrentVersion(t *testing.T) {
	originalVersion := Version
	Version = "1.2.3"
	t.Cleanup(func() {
		Version = originalVersion
	})

	app := newFiberApp("/api/version", handleVersion)
	resp := performRequest(t, app, "/api/version")

	var payload map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	assert.Equal(t, "1.2.3", payload["version"])
}

func TestHandleTrackerScriptSetsCachingAndSecurityHeaders(t *testing.T) {
	script := []byte("console.log('hello');")
	app := newFiberApp("/k.js", handleTrackerScript(script))
	resp := performRequest(t, app, "/k.js")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	hash := sha256.Sum256(script)
	expectedETag := `"` + hex.EncodeToString(hash[:8]) + `"`

	assert.Equal(t, string(script), string(body))
	assert.Equal(t, "application/javascript; charset=utf-8", resp.Header.Get("Content-Type"))
	assert.Equal(t, expectedETag, resp.Header.Get("ETag"))
	assert.Equal(t, "public, max-age=3600, immutable", resp.Header.Get("Cache-Control"))
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "*", resp.Header.Get("Timing-Allow-Origin"))
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
