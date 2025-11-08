package middleware

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/seuros/kaunta/internal/database"
)

// UserContext holds the authenticated user information
type UserContext struct {
	UserID    uuid.UUID
	Username  string
	SessionID uuid.UUID
}

// Auth middleware validates session tokens and loads user context
func Auth(c *fiber.Ctx) error {
	// Extract token from cookie
	token := c.Cookies("kaunta_session")
	if token == "" {
		// Also check Authorization header for API clients
		authHeader := c.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}

	if token == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized - no session token provided",
		})
	}

	// Validate session using PostgreSQL function
	var userCtx UserContext
	query := `SELECT user_id, username, session_id FROM validate_session($1)`

	err := database.DB.QueryRow(query, hashToken(token)).Scan(
		&userCtx.UserID,
		&userCtx.Username,
		&userCtx.SessionID,
	)

	if err == sql.ErrNoRows {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized - invalid or expired session",
		})
	}

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Authentication error",
		})
	}

	// Store user context in Fiber locals
	c.Locals("user", &userCtx)

	return c.Next()
}

// GetUser retrieves the authenticated user from context
func GetUser(c *fiber.Ctx) *UserContext {
	if user, ok := c.Locals("user").(*UserContext); ok {
		return user
	}
	return nil
}

// hashToken creates SHA256 hash of token for database lookup
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
