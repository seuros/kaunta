package handlers

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/google/uuid"

	"github.com/seuros/kaunta/internal/logging"
	"github.com/seuros/kaunta/internal/middleware"
	"go.uber.org/zap"
)

// DatastarLoginRequest represents the login signals from Datastar
type DatastarLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// HandleLoginSSE handles login via Datastar SSE
// GET /api/auth/login-ds?datastar={signals}
func HandleLoginSSE(c fiber.Ctx) error {
	// Extract all context values BEFORE entering stream (fiber context invalid inside stream callback)
	signalsJSON := c.Query("datastar")
	userAgent := c.Get("User-Agent")
	if len(userAgent) > 500 {
		userAgent = userAgent[:500]
	}
	ipAddress := c.IP()

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	// Parse and validate request BEFORE streaming
	var req DatastarLoginRequest
	var parseErr string

	if signalsJSON == "" {
		parseErr = "Invalid request"
	} else if err := json.Unmarshal([]byte(signalsJSON), &req); err != nil {
		parseErr = "Invalid request format"
	} else if req.Username == "" || req.Password == "" {
		parseErr = "Username and password are required"
	}

	// Authenticate user BEFORE streaming
	var authErr string
	var token string
	var expiresAt time.Time

	if parseErr == "" {
		// Fetch user from database
		user, err := fetchUserByUsername(req.Username)
		if errors.Is(err, sql.ErrNoRows) {
			authErr = "Invalid username or password"
		} else if err != nil {
			authErr = "Authentication error"
		} else {
			// Verify password
			passwordValid, verifyErr := verifyPasswordHashFunc(req.Password, user.PasswordHash)
			if verifyErr != nil || !passwordValid {
				authErr = "Invalid username or password"
			} else {
				// Generate session token
				var tokenHash string
				token, tokenHash, err = sessionTokenGenerator()
				if err != nil {
					authErr = "Failed to create session"
				} else {
					// Create session in database
					sessionID := uuid.New()
					expiresAt = time.Now().Add(7 * 24 * time.Hour) // 7 days

					if err := insertSessionFunc(sessionID, user.UserID, tokenHash, expiresAt, userAgent, ipAddress); err != nil {
						authErr = "Failed to create session"
					}
				}
			}
		}
	}

	// Set cookie on success BEFORE starting stream (headers must be set first)
	if parseErr == "" && authErr == "" {
		secure := secureCookiesEnabled()
		sameSite := "Lax"
		if secure {
			sameSite = "None"
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
	}

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)

		// Send appropriate response based on pre-computed results
		if parseErr != "" {
			_ = sse.PatchSignals(map[string]any{
				"error":   parseErr,
				"loading": false,
			})
			return
		}

		if authErr != "" {
			_ = sse.PatchSignals(map[string]any{
				"error":   authErr,
				"loading": false,
			})
			return
		}

		// Success - clear state and redirect via script execution
		_ = sse.PatchSignals(map[string]any{
			"error":   "",
			"loading": false,
		})
		_ = sse.ExecuteScript("window.location.href = '/dashboard'")
	})
}

// HandleLogoutSSE handles logout via Datastar SSE
// POST /api/auth/logout-ds
func HandleLogoutSSE(c fiber.Ctx) error {
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
			logging.L().Warn("failed to delete CSRF token", zap.Error(err))
		}
	}

	// Delete session from database
	logoutErr := ""
	if err := deleteSessionFunc(user.SessionID); err != nil {
		logoutErr = "Failed to logout"
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

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)

		if logoutErr != "" {
			_ = sse.PatchSignals(map[string]any{
				"error": logoutErr,
			})
			return
		}

		// Clear localStorage and redirect to login
		_ = sse.ExecuteScript("localStorage.removeItem('kaunta_website'); localStorage.removeItem('kaunta_dateRange'); window.location.href = '/login'")
	})
}
