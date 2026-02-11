package handlers

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/seuros/kaunta/internal/httpx"
	"github.com/seuros/kaunta/internal/middleware"
)

// DatastarLoginRequest represents the login signals from Datastar
type DatastarLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// HandleLoginSSE handles login via Datastar SSE
// GET /api/auth/login-ds?datastar={signals}
func HandleLoginSSE(w http.ResponseWriter, r *http.Request) {
	signalsJSON := r.URL.Query().Get("datastar")
	userAgent := r.Header.Get("User-Agent")
	if len(userAgent) > 500 {
		userAgent = userAgent[:500]
	}
	ipAddress := httpx.ClientIP(r)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.Error(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

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
		sameSite := http.SameSiteLaxMode
		if secure {
			sameSite = http.SameSiteNoneMode
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "kaunta_session",
			Value:    token,
			Expires:  expiresAt,
			HttpOnly: true,
			Secure:   secure,
			SameSite: sameSite,
			Path:     "/",
		})
	}

	writer := bufio.NewWriter(w)
	sse := NewDatastarSSE(writer)

	if parseErr != "" {
		_ = sse.PatchSignals(map[string]any{
			"error":   parseErr,
			"loading": false,
		})
		_ = writer.Flush()
		flusher.Flush()
		return
	}

	if authErr != "" {
		_ = sse.PatchSignals(map[string]any{
			"error":   authErr,
			"loading": false,
		})
		_ = writer.Flush()
		flusher.Flush()
		return
	}

	_ = sse.PatchSignals(map[string]any{
		"error":   "",
		"loading": false,
	})
	_ = sse.ExecuteScript("window.location.href = '/dashboard'")
	_ = writer.Flush()
	flusher.Flush()
}

// HandleLogoutSSE handles logout via Datastar SSE
// POST /api/auth/logout-ds
func HandleLogoutSSE(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		httpx.Error(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	// Delete session from database
	logoutErr := ""
	if err := deleteSessionFunc(user.SessionID); err != nil {
		logoutErr = "Failed to logout"
	}

	secure := secureCookiesEnabled()
	sameSite := http.SameSiteLaxMode
	if secure {
		sameSite = http.SameSiteNoneMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "kaunta_session",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		Path:     "/",
	})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.Error(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	writer := bufio.NewWriter(w)
	sse := NewDatastarSSE(writer)

	if logoutErr != "" {
		_ = sse.PatchSignals(map[string]any{
			"error": logoutErr,
		})
		_ = writer.Flush()
		flusher.Flush()
		return
	}

	_ = sse.ExecuteScript("localStorage.removeItem('kaunta_website')")
	_ = sse.ExecuteScript("localStorage.removeItem('kaunta_dateRange')")
	_ = sse.ExecuteScript("window.location.href = '/login'")
	_ = writer.Flush()
	flusher.Flush()
}
