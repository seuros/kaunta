package middleware

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/httpx"
)

// UserContext holds the authenticated user information
type UserContext struct {
	UserID    uuid.UUID
	Username  string
	SessionID uuid.UUID
}

var sessionValidator = validateSessionFromDB

type contextKey string

const userContextKey contextKey = "user"

// Auth middleware validates session tokens and loads user context.
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractSessionToken(r)
		if token == "" {
			httpx.Error(w, http.StatusUnauthorized, "Unauthorized - no session token provided")
			return
		}

		userCtx, err := sessionValidator(HashToken(token))
		if err == sql.ErrNoRows {
			httpx.Error(w, http.StatusUnauthorized, "Unauthorized - invalid or expired session")
			return
		}
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Authentication error")
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, userCtx)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AuthWithRedirect middleware validates session tokens and redirects to /login for dashboard routes.
func AuthWithRedirect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractSessionToken(r)
		if token == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		userCtx, err := sessionValidator(HashToken(token))
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, userCtx)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUser retrieves the authenticated user from context.
func GetUser(r *http.Request) *UserContext {
	if user, ok := r.Context().Value(userContextKey).(*UserContext); ok {
		return user
	}
	return nil
}

// ContextWithUser attaches a user context to the provided context.
func ContextWithUser(ctx context.Context, user *UserContext) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

func extractSessionToken(r *http.Request) string {
	if cookie, err := r.Cookie("kaunta_session"); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return ""
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
