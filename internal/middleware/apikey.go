package middleware

import (
	"database/sql"
	"strings"

	"github.com/gofiber/fiber/v3"

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

// APIKeyAuth middleware validates API keys for the ingest endpoints
func APIKeyAuth(c fiber.Ctx) error {
	// Extract key from Authorization header (Bearer token)
	key := extractAPIKey(c)
	if key == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing API key",
		})
	}

	// Validate key prefix
	if !strings.HasPrefix(key, "kaunta_live_") {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid API key format",
		})
	}

	// Hash and lookup
	keyHash := models.HashAPIKey(key)
	apiKey, err := apiKeyValidator(keyHash)

	if err == sql.ErrNoRows {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid API key",
		})
	}

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Authentication error",
		})
	}

	// Check if key is valid (not revoked, not expired)
	if !apiKey.IsValid() {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "API key revoked or expired",
		})
	}

	// Check scope
	if !apiKey.HasScope("ingest") {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "API key does not have ingest permission",
		})
	}

	// Update last_used_at asynchronously (don't block request)
	go models.UpdateAPIKeyLastUsed(apiKey.KeyID)

	// Store API key context in Fiber locals
	c.Locals("api_key", apiKey)

	return c.Next()
}

// extractAPIKey extracts the API key from request headers
// Supports: Authorization: Bearer <key> or X-API-Key: <key>
func extractAPIKey(c fiber.Ctx) string {
	// Try Authorization header first (preferred)
	authHeader := c.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	// Fallback to X-API-Key header
	if apiKey := c.Get("X-API-Key"); apiKey != "" {
		return apiKey
	}

	return ""
}

// validateAPIKeyFromDB validates an API key hash against the database
func validateAPIKeyFromDB(keyHash string) (*models.APIKey, error) {
	return models.GetAPIKeyByHash(keyHash)
}

// GetAPIKey retrieves the authenticated API key from context
func GetAPIKey(c fiber.Ctx) *models.APIKey {
	if apiKey, ok := c.Locals("api_key").(*models.APIKey); ok {
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
