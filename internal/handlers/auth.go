package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/middleware"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	User    *struct {
		UserID   uuid.UUID `json:"user_id"`
		Username string    `json:"username"`
		Name     *string   `json:"name,omitempty"`
	} `json:"user,omitempty"`
}

// HandleLogin authenticates user and creates session
func HandleLogin(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate input
	if req.Username == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Username and password are required",
		})
	}

	// Get user and verify password using PostgreSQL function
	var userID uuid.UUID
	var username string
	var name sql.NullString
	var passwordHash string

	query := `
		SELECT user_id, username, name, password_hash
		FROM users
		WHERE username = $1
	`

	err := database.DB.QueryRow(query, req.Username).Scan(&userID, &username, &name, &passwordHash)
	if err == sql.ErrNoRows {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid username or password",
		})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Authentication error",
		})
	}

	// Verify password using PostgreSQL function
	var passwordValid bool
	err = database.DB.QueryRow("SELECT verify_password($1, $2)", req.Password, passwordHash).Scan(&passwordValid)
	if err != nil || !passwordValid {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid username or password",
		})
	}

	// Generate session token
	token, tokenHash, err := generateSessionToken()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create session",
		})
	}

	// Create session in database
	sessionID := uuid.New()
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days

	// Get user agent and IP
	userAgent := c.Get("User-Agent")
	if len(userAgent) > 500 {
		userAgent = userAgent[:500]
	}
	ipAddress := c.IP()

	insertQuery := `
		INSERT INTO user_sessions (session_id, user_id, token_hash, expires_at, user_agent, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err = database.DB.Exec(insertQuery, sessionID, userID, tokenHash, expiresAt, userAgent, ipAddress)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create session",
		})
	}

	// Set session cookie
	c.Cookie(&fiber.Cookie{
		Name:     "kaunta_session",
		Value:    token,
		Expires:  expiresAt,
		HTTPOnly: true,
		Secure:   c.Protocol() == "https",
		SameSite: "Lax",
		Path:     "/",
	})

	// Return success response
	response := LoginResponse{
		Success: true,
		Message: "Login successful",
		User: &struct {
			UserID   uuid.UUID `json:"user_id"`
			Username string    `json:"username"`
			Name     *string   `json:"name,omitempty"`
		}{
			UserID:   userID,
			Username: username,
		},
	}

	if name.Valid {
		nameStr := name.String
		response.User.Name = &nameStr
	}

	return c.JSON(response)
}

// HandleLogout invalidates the current session
func HandleLogout(c *fiber.Ctx) error {
	// Get user from context
	user := middleware.GetUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Not authenticated",
		})
	}

	// Delete session from database
	query := `DELETE FROM user_sessions WHERE session_id = $1`
	_, err := database.DB.Exec(query, user.SessionID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to logout",
		})
	}

	// Clear session cookie
	c.Cookie(&fiber.Cookie{
		Name:     "kaunta_session",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
		Secure:   c.Protocol() == "https",
		SameSite: "Lax",
		Path:     "/",
	})

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Logout successful",
	})
}

// HandleMe returns current user info
func HandleMe(c *fiber.Ctx) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Not authenticated",
		})
	}

	// Get full user details
	var name sql.NullString
	var createdAt time.Time

	query := `SELECT name, created_at FROM users WHERE user_id = $1`
	err := database.DB.QueryRow(query, user.UserID).Scan(&name, &createdAt)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get user info",
		})
	}

	result := fiber.Map{
		"user_id":    user.UserID,
		"username":   user.Username,
		"created_at": createdAt,
	}

	if name.Valid {
		result["name"] = name.String
	}

	return c.JSON(result)
}

// generateSessionToken creates a random session token and its hash
func generateSessionToken() (token string, hash string, err error) {
	// Generate 32 random bytes
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", err
	}

	// Encode as hex string
	token = hex.EncodeToString(bytes)

	// Create SHA256 hash for database storage
	hashBytes := sha256.Sum256([]byte(token))
	hash = hex.EncodeToString(hashBytes[:])

	return token, hash, nil
}
