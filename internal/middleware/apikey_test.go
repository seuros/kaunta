package middleware

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func executeAPIKeyMiddleware(t *testing.T, handler http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	if handler == nil {
		handler = func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}
	}

	recorder := httptest.NewRecorder()
	APIKeyAuth(http.HandlerFunc(handler)).ServeHTTP(recorder, req)
	return recorder
}

func TestAPIKeyAuthMissingKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	resp := executeAPIKeyMiddleware(t, nil, req)

	assert.Equal(t, http.StatusUnauthorized, resp.Code)

	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)
	assert.Contains(t, string(body), "Missing API key")
}

func TestAPIKeyAuthInvalidFormat(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer invalid_key_format")

	resp := executeAPIKeyMiddleware(t, nil, req)

	assert.Equal(t, http.StatusUnauthorized, resp.Code)

	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)
	assert.Contains(t, string(body), "Invalid API key format")
}

func TestAPIKeyAuthNotFound(t *testing.T) {
	stubAPIKeyValidator(t, func(keyHash string) (*models.APIKey, error) {
		return nil, sql.ErrNoRows
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer kaunta_live_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	resp := executeAPIKeyMiddleware(t, nil, req)

	assert.Equal(t, http.StatusUnauthorized, resp.Code)

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

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer kaunta_live_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	resp := executeAPIKeyMiddleware(t, nil, req)

	assert.Equal(t, http.StatusUnauthorized, resp.Code)

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

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer kaunta_live_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	resp := executeAPIKeyMiddleware(t, nil, req)

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
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

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer kaunta_live_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	resp := executeAPIKeyMiddleware(t, nil, req)

	assert.Equal(t, http.StatusForbidden, resp.Code)

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

	handler := func(w http.ResponseWriter, r *http.Request) {
		capturedKey = GetAPIKey(r)
		w.WriteHeader(http.StatusOK)
	}

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer kaunta_live_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	resp := executeAPIKeyMiddleware(t, handler, req)

	assert.Equal(t, http.StatusOK, resp.Code)
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

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-API-Key", "kaunta_live_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	resp := executeAPIKeyMiddleware(t, nil, req)

	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestGetAPIKeyWithoutContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	assert.Nil(t, GetAPIKey(req))
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
