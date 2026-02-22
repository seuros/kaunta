package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/render"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

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
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req LoginRequest
	if err := render.DecodeJSON(r.Body, &req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]any{"error": "Invalid request body"})
		return
	}

	// Validate input
	if req.Username == "" || req.Password == "" {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, map[string]any{"error": "Username and password are required"})
		return
	}

	user, err := fetchUserByUsername(req.Username)
	if errors.Is(err, sql.ErrNoRows) {
		render.Status(r, http.StatusUnauthorized)
		render.JSON(w, r, map[string]any{"error": "Invalid username or password"})
		return
	}
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, map[string]any{"error": "Authentication error"})
		return
	}

	// Verify password using PostgreSQL function
	passwordValid, err := verifyPasswordHashFunc(req.Password, user.PasswordHash)
	if err != nil || !passwordValid {
		render.Status(r, http.StatusUnauthorized)
		render.JSON(w, r, map[string]any{"error": "Invalid username or password"})
		return
	}

	// Generate session token
	token, tokenHash, err := sessionTokenGenerator()
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, map[string]any{"error": "Failed to create session"})
		return
	}

	// Create session in database
	sessionID := uuid.New()
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days

	// Get user agent and IP
	userAgent := r.Header.Get("User-Agent")
	if len(userAgent) > 500 {
		userAgent = userAgent[:500]
	}
	ipAddress := clientIP(r)

	if err := insertSessionFunc(sessionID, user.UserID, tokenHash, expiresAt, userAgent, ipAddress); err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, map[string]any{"error": "Failed to create session"})
		return
	}

	// Set session cookie
	secure := secureCookiesEnabled()
	sameSite := "Lax"
	if secure {
		sameSite = "None" // Required for cross-domain CNAME setups
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "kaunta_session",
		Value:    token,
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   secure,
		SameSite: parseSameSite(sameSite),
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

	render.JSON(w, r, response)
}

// HandleMe returns current user info
func HandleMe(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		render.Status(r, http.StatusUnauthorized)
		render.JSON(w, r, map[string]any{"error": "Not authenticated"})
		return
	}

	// Get full user details
	name, createdAt, err := fetchUserDetailsFunc(user.UserID)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, map[string]any{"error": "Failed to get user info"})
		return
	}

	result := map[string]any{
		"user_id":    user.UserID,
		"username":   user.Username,
		"created_at": createdAt,
	}

	if name.Valid {
		result["name"] = name.String
	}

	render.JSON(w, r, result)
}

func parseSameSite(mode string) http.SameSite {
	switch strings.ToLower(mode) {
	case "none":
		return http.SameSiteNoneMode
	case "strict":
		return http.SameSiteStrictMode
	default:
		return http.SameSiteLaxMode
	}
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
	var ipParam any = ipAddress
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
