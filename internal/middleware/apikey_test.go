package middleware

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"

	"github.com/seuros/kaunta/internal/models"
)

func stubAPIKeyValidator(t *testing.T, stub func(keyHash string) (*models.APIKey, error)) {
	t.Helper()
	original := apiKeyValidator
	SetAPIKeyValidator(stub)
	t.Cleanup(func() {
		apiKeyValidator = original
	})
}

func newAPIKeyTestApp(handler fiber.Handler) *fiber.App {
	app := fiber.New()
	app.Use(APIKeyAuth)
	app.Post("/", handler)
	return app
}

func TestAPIKeyAuthMissingKey(t *testing.T) {
	app := newAPIKeyTestApp(func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)
	assert.Contains(t, string(body), "Missing API key")
}

func TestAPIKeyAuthInvalidFormat(t *testing.T) {
	app := newAPIKeyTestApp(func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer invalid_key_format")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)
	assert.Contains(t, string(body), "Invalid API key format")
}

func TestAPIKeyAuthNotFound(t *testing.T) {
	stubAPIKeyValidator(t, func(keyHash string) (*models.APIKey, error) {
		return nil, sql.ErrNoRows
	})

	app := newAPIKeyTestApp(func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer kaunta_live_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)
	assert.Contains(t, string(body), "Invalid API key")
}

func TestAPIKeyAuthRevoked(t *testing.T) {
	revokedAt := time.Now().Add(-1 * time.Hour)
	stubAPIKeyValidator(t, func(keyHash string) (*models.APIKey, error) {
		return &models.APIKey{
			KeyID:              uuid.New(),
			WebsiteID:          uuid.New(),
			RevokedAt:          &revokedAt,
			Scopes:             []string{"ingest"},
			RateLimitPerMinute: 1000,
		}, nil
	})

	app := newAPIKeyTestApp(func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer kaunta_live_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)
	assert.Contains(t, string(body), "revoked or expired")
}

func TestAPIKeyAuthExpired(t *testing.T) {
	expiredAt := time.Now().Add(-1 * time.Hour)
	stubAPIKeyValidator(t, func(keyHash string) (*models.APIKey, error) {
		return &models.APIKey{
			KeyID:              uuid.New(),
			WebsiteID:          uuid.New(),
			ExpiresAt:          &expiredAt,
			Scopes:             []string{"ingest"},
			RateLimitPerMinute: 1000,
		}, nil
	})

	app := newAPIKeyTestApp(func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer kaunta_live_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestAPIKeyAuthNoIngestScope(t *testing.T) {
	stubAPIKeyValidator(t, func(keyHash string) (*models.APIKey, error) {
		return &models.APIKey{
			KeyID:              uuid.New(),
			WebsiteID:          uuid.New(),
			Scopes:             []string{"read"}, // No ingest scope
			RateLimitPerMinute: 1000,
		}, nil
	})

	app := newAPIKeyTestApp(func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer kaunta_live_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)

	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)
	assert.Contains(t, string(body), "ingest permission")
}

func TestAPIKeyAuthSuccess(t *testing.T) {
	expectedKey := &models.APIKey{
		KeyID:              uuid.New(),
		WebsiteID:          uuid.New(),
		Scopes:             []string{"ingest"},
		RateLimitPerMinute: 1000,
	}

	stubAPIKeyValidator(t, func(keyHash string) (*models.APIKey, error) {
		return expectedKey, nil
	})

	var capturedKey *models.APIKey

	app := newAPIKeyTestApp(func(c fiber.Ctx) error {
		capturedKey = GetAPIKey(c)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer kaunta_live_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	require.NotNil(t, capturedKey)
	assert.Equal(t, expectedKey.KeyID, capturedKey.KeyID)
	assert.Equal(t, expectedKey.WebsiteID, capturedKey.WebsiteID)
}

func TestAPIKeyAuthXAPIKeyHeader(t *testing.T) {
	expectedKey := &models.APIKey{
		KeyID:              uuid.New(),
		WebsiteID:          uuid.New(),
		Scopes:             []string{"ingest"},
		RateLimitPerMinute: 1000,
	}

	stubAPIKeyValidator(t, func(keyHash string) (*models.APIKey, error) {
		return expectedKey, nil
	})

	app := newAPIKeyTestApp(func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-API-Key", "kaunta_live_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestGetAPIKeyWithoutContext(t *testing.T) {
	app := fiber.New()
	ctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(ctx)
	assert.Nil(t, GetAPIKey(ctx))
}

func TestAPIKeyIsValid(t *testing.T) {
	tests := []struct {
		name     string
		key      *models.APIKey
		expected bool
	}{
		{
			name: "valid key",
			key: &models.APIKey{
				KeyID: uuid.New(),
			},
			expected: true,
		},
		{
			name: "revoked key",
			key: &models.APIKey{
				KeyID:     uuid.New(),
				RevokedAt: ptrTime(time.Now()),
			},
			expected: false,
		},
		{
			name: "expired key",
			key: &models.APIKey{
				KeyID:     uuid.New(),
				ExpiresAt: ptrTime(time.Now().Add(-1 * time.Hour)),
			},
			expected: false,
		},
		{
			name: "future expiry",
			key: &models.APIKey{
				KeyID:     uuid.New(),
				ExpiresAt: ptrTime(time.Now().Add(1 * time.Hour)),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.key.IsValid())
		})
	}
}

func TestAPIKeyHasScope(t *testing.T) {
	key := &models.APIKey{
		Scopes: []string{"ingest", "read"},
	}

	assert.True(t, key.HasScope("ingest"))
	assert.True(t, key.HasScope("read"))
	assert.False(t, key.HasScope("admin"))
	assert.False(t, key.HasScope("write"))
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
