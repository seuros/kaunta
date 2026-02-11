package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/seuros/kaunta/internal/httpx"
	"github.com/seuros/kaunta/internal/models"
)

// APIKeyContext holds the authenticated API key information
type APIKeyContext struct {
	KeyID              string
	WebsiteID          string
	Name               *string
	Scopes             []string
	RateLimitPerMinute int
	WebsiteRateLimit   int
}

// apiKeyValidator is the function used to validate API keys (can be mocked in tests)
var apiKeyValidator = validateAPIKeyFromDB

type apiContextKey string

const apiKeyContextKey apiContextKey = "api_key"

// APIKeyAuth middleware validates API keys for the ingest endpoints.
func APIKeyAuth(next http.Handler) http.Handler {
	return apiKeyAuthWithScope(next, "ingest")
}

// APIKeyAuthAny validates API key without checking scope (handler checks scope).
func APIKeyAuthAny(next http.Handler) http.Handler {
	return apiKeyAuthWithScope(next, "")
}

// apiKeyAuthWithScope validates API key, optionally checking for specific scope.
func apiKeyAuthWithScope(next http.Handler, requiredScope string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := extractAPIKey(r)
		if key == "" {
			httpx.Error(w, http.StatusUnauthorized, "Missing API key")
			return
		}

		if !strings.HasPrefix(key, "kaunta_live_") {
			httpx.Error(w, http.StatusUnauthorized, "Invalid API key format")
			return
		}

		keyHash := models.HashAPIKey(key)
		apiKey, err := apiKeyValidator(keyHash)

		if err == sql.ErrNoRows {
			httpx.Error(w, http.StatusUnauthorized, "Invalid API key")
			return
		}

		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Authentication error")
			return
		}

		if !apiKey.IsValid() {
			httpx.Error(w, http.StatusUnauthorized, "API key revoked or expired")
			return
		}

		if requiredScope != "" && !apiKey.HasScope(requiredScope) {
			httpx.Error(w, http.StatusForbidden, "API key does not have "+requiredScope+" permission")
			return
		}

		go models.UpdateAPIKeyLastUsed(apiKey.KeyID)

		ctx := context.WithValue(r.Context(), apiKeyContextKey, apiKey)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// extractAPIKey extracts the API key from request headers
// Supports: Authorization: Bearer <key> or X-API-Key: <key>
func extractAPIKey(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		return apiKey
	}

	return ""
}

// validateAPIKeyFromDB validates an API key hash against the database
func validateAPIKeyFromDB(keyHash string) (*models.APIKey, error) {
	return models.GetAPIKeyByHash(keyHash)
}

// GetAPIKey retrieves the authenticated API key from context
func GetAPIKey(r *http.Request) *models.APIKey {
	if apiKey, ok := r.Context().Value(apiKeyContextKey).(*models.APIKey); ok {
		return apiKey
	}
	return nil
}

// SetAPIKeyValidator allows tests to inject a mock validator
func SetAPIKeyValidator(validator func(string) (*models.APIKey, error)) {
	apiKeyValidator = validator
}

// ResetAPIKeyValidator resets the validator to the default implementation
func ResetAPIKeyValidator() {
	apiKeyValidator = validateAPIKeyFromDB
}
