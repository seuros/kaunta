package middleware

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/seuros/kaunta/internal/database"
)

// UserContext holds the authenticated user information
type UserContext struct {
	UserID    uuid.UUID
	Username  string
	SessionID uuid.UUID
}

var sessionValidator = validateSessionFromDB

// Auth middleware validates session tokens and loads user context
func Auth(c fiber.Ctx) error {
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
	userCtx, err := sessionValidator(HashToken(token))

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
	c.Locals("user", userCtx)

	return c.Next()
}

// AuthWithRedirect middleware validates session tokens and redirects to /login for dashboard routes
func AuthWithRedirect(c fiber.Ctx) error {
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
		return c.Redirect().To("/login")
	}

	// Validate session using PostgreSQL function
	userCtx, err := sessionValidator(HashToken(token))

	if err == sql.ErrNoRows {
		return c.Redirect().To("/login")
	}

	if err != nil {
		return c.Redirect().To("/login")
	}

	// Store user context in Fiber locals
	c.Locals("user", userCtx)

	return c.Next()
}

// GetUser retrieves the authenticated user from context
func GetUser(c fiber.Ctx) *UserContext {
	if user, ok := c.Locals("user").(*UserContext); ok {
		return user
	}
	return nil
}

// HashToken creates SHA256 hash of token for database lookup
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func validateSessionFromDB(tokenHash string) (*UserContext, error) {
	var userCtx UserContext
	query := `SELECT user_id, username, session_id FROM validate_session($1)`

	err := database.DB.QueryRow(query, tokenHash).Scan(
		&userCtx.UserID,
		&userCtx.Username,
		&userCtx.SessionID,
	)
	if err != nil {
		return nil, err
	}
	return &userCtx, nil
}
