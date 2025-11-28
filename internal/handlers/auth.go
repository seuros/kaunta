package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/logging"
	"github.com/seuros/kaunta/internal/middleware"
	"go.uber.org/zap"
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

type userRecord struct {
	UserID       uuid.UUID
	Username     string
	Name         sql.NullString
	PasswordHash string
}

var (
	fetchUserByUsername    = fetchUserFromDB
	verifyPasswordHashFunc = verifyPasswordInDB
	insertSessionFunc      = insertSessionInDB
	sessionTokenGenerator  = generateSessionToken
	deleteSessionFunc      = deleteSessionInDB
	fetchUserDetailsFunc   = fetchUserDetailsFromDB
)

// secureCookiesEnabled determines if cookies should use Secure flag and SameSite=None
// The config is loaded by CLI and set as env var, so we read from there
func secureCookiesEnabled() bool {
	env := os.Getenv("SECURE_COOKIES")
	if env == "" {
		return true // Default to secure (safer for production)
	}
	return env == "true"
}

// HandleLogin authenticates user and creates session
func HandleLogin(c fiber.Ctx) error {
	var req LoginRequest
	if err := c.Bind().Body(&req); err != nil {
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

	user, err := fetchUserByUsername(req.Username)
	if errors.Is(err, sql.ErrNoRows) {
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
	passwordValid, err := verifyPasswordHashFunc(req.Password, user.PasswordHash)
	if err != nil || !passwordValid {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid username or password",
		})
	}

	// Generate session token
	token, tokenHash, err := sessionTokenGenerator()
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

	if err := insertSessionFunc(sessionID, user.UserID, tokenHash, expiresAt, userAgent, ipAddress); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create session",
		})
	}

	// Set session cookie
	secure := secureCookiesEnabled()
	sameSite := "Lax"
	if secure {
		sameSite = "None" // Required for cross-domain CNAME setups
	}

	c.Cookie(&fiber.Cookie{
		Name:     "kaunta_session",
		Value:    token,
		Expires:  expiresAt,
		HTTPOnly: true,
		Secure:   secure,
		SameSite: sameSite,
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
			UserID:   user.UserID,
			Username: user.Username,
		},
	}

	if user.Name.Valid {
		nameStr := user.Name.String
		response.User.Name = &nameStr
	}

	return c.JSON(response)
}

// HandleLogout invalidates the current session
func HandleLogout(c fiber.Ctx) error {
	// Get user from context
	user := middleware.GetUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Not authenticated",
		})
	}

	// Delete CSRF token (GoFiber v3 best practice)
	handler := csrf.HandlerFromContext(c)
	if handler != nil {
		if err := handler.DeleteToken(c); err != nil {
			// Log but don't fail logout
			logging.L().Warn("failed to delete CSRF token", zap.Error(err))
		}
	}

	// Delete session from database
	if err := deleteSessionFunc(user.SessionID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to logout",
		})
	}

	// Clear session cookie
	secure := secureCookiesEnabled()
	sameSite := "Lax"
	if secure {
		sameSite = "None"
	}

	c.Cookie(&fiber.Cookie{
		Name:     "kaunta_session",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		Path:     "/",
	})

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Logout successful",
	})
}

// HandleMe returns current user info
func HandleMe(c fiber.Ctx) error {
	user := middleware.GetUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Not authenticated",
		})
	}

	// Get full user details
	name, createdAt, err := fetchUserDetailsFunc(user.UserID)
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

func fetchUserFromDB(username string) (*userRecord, error) {
	query := `
		SELECT user_id, username, name, password_hash
		FROM users
		WHERE username = $1
	`

	var record userRecord
	err := database.DB.QueryRow(query, username).Scan(
		&record.UserID,
		&record.Username,
		&record.Name,
		&record.PasswordHash,
	)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func verifyPasswordInDB(password, passwordHash string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func insertSessionInDB(sessionID uuid.UUID, userID uuid.UUID, tokenHash string, expiresAt time.Time, userAgent, ipAddress string) error {
	insertQuery := `
		INSERT INTO user_sessions (session_id, user_id, token_hash, expires_at, user_agent, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	// Handle empty IP address (e.g., from Docker networking)
	var ipParam interface{} = ipAddress
	if ipAddress == "" {
		ipParam = nil
	}

	_, err := database.DB.Exec(insertQuery, sessionID, userID, tokenHash, expiresAt, userAgent, ipParam)
	return err
}

func deleteSessionInDB(sessionID uuid.UUID) error {
	query := `DELETE FROM user_sessions WHERE session_id = $1`
	_, err := database.DB.Exec(query, sessionID)
	return err
}

func fetchUserDetailsFromDB(userID uuid.UUID) (sql.NullString, time.Time, error) {
	var name sql.NullString
	var createdAt time.Time

	query := `SELECT name, created_at FROM users WHERE user_id = $1`
	err := database.DB.QueryRow(query, userID).Scan(&name, &createdAt)
	if err != nil {
		return sql.NullString{}, time.Time{}, err
	}
	return name, createdAt, nil
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
