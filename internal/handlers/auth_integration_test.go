package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/lib/pq"

	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/middleware"
)

func integrationDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("INTEGRATION_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		dsn = "postgres://kaunta:kaunta@localhost:5432/kaunta?sslmode=disable"
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("integration DB unavailable: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Skipf("integration DB unreachable: %v", err)
	}

	if err := database.RunMigrations(dsn); err != nil {
		t.Fatalf("failed to run migrations for integration DB: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

func prepareIntegrationUser(t *testing.T, db *sql.DB, username, password string) uuid.UUID {
	t.Helper()

	userID := uuid.New()

	var passwordHash string
	require.NoError(t, db.QueryRow("SELECT hash_password($1)", password).Scan(&passwordHash))

	_, err := db.Exec(`DELETE FROM user_sessions WHERE user_id IN (SELECT user_id FROM users WHERE username = $1)`, username)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM users WHERE username = $1`, username)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO users (user_id, username, password_hash, name) VALUES ($1, $2, $3, $4)`,
		userID, username, passwordHash, "Integration User",
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM user_sessions WHERE user_id = $1`, userID)
		_, _ = db.Exec(`DELETE FROM users WHERE user_id = $1`, userID)
	})

	return userID
}

func TestAuthIntegration_LoginLogoutFlow(t *testing.T) {
	db := integrationDB(t)
	userID := prepareIntegrationUser(t, db, "integration-user", "integration-secret")

	originalDB := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = originalDB
	})

	app := newIntegrationAuthApp()
	sessionCookie, loginResp := loginIntegrationUser(t, app, "integration-user", "integration-secret")
	assert.Equal(t, userID, loginResp.User.UserID)

	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.AddCookie(sessionCookie)
	meResp := executeIntegrationRequest(app, meReq)
	assert.Equal(t, http.StatusOK, meResp.Code)

	var mePayload map[string]any
	meBody, err := io.ReadAll(meResp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(meBody, &mePayload))
	assert.Equal(t, "integration-user", mePayload["username"])

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutReq.AddCookie(sessionCookie)
	logoutResp := executeIntegrationRequest(app, logoutReq)
	assert.Equal(t, http.StatusOK, logoutResp.Code)

	meReqAfterLogout := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReqAfterLogout.AddCookie(sessionCookie)
	meRespAfterLogout := executeIntegrationRequest(app, meReqAfterLogout)
	assert.Equal(t, http.StatusUnauthorized, meRespAfterLogout.Code)
}

func TestAuthIntegration_ProtectedRouteAuthorization(t *testing.T) {
	db := integrationDB(t)
	prepareIntegrationUser(t, db, "integration-protected", "integration-secret")

	originalDB := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = originalDB
	})

	app := newIntegrationAuthApp()

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	resp := executeIntegrationRequest(app, req)
	assert.Equal(t, http.StatusUnauthorized, resp.Code)

	sessionCookie, _ := loginIntegrationUser(t, app, "integration-protected", "integration-secret")

	protectedReq := httptest.NewRequest(http.MethodGet, "/protected", nil)
	protectedReq.AddCookie(sessionCookie)
	protectedResp := executeIntegrationRequest(app, protectedReq)
	assert.Equal(t, http.StatusOK, protectedResp.Code)

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutReq.AddCookie(sessionCookie)
	_ = executeIntegrationRequest(app, logoutReq)

	protectedReqAfter := httptest.NewRequest(http.MethodGet, "/protected", nil)
	protectedReqAfter.AddCookie(sessionCookie)
	protectedRespAfter := executeIntegrationRequest(app, protectedReqAfter)
	assert.Equal(t, http.StatusUnauthorized, protectedRespAfter.Code)
}

func newIntegrationAuthApp() http.Handler {
	router := chi.NewRouter()
	router.Post("/api/auth/login", HandleLogin)
	router.With(middleware.Auth).Post("/api/auth/logout", HandleLogoutSSE)
	router.With(middleware.Auth).Get("/api/auth/me", HandleMe)
	router.With(middleware.Auth).Get("/protected", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	return router
}

func executeIntegrationRequest(handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}

func loginIntegrationUser(t *testing.T, handler http.Handler, username, password string) (*http.Cookie, LoginResponse) {
	t.Helper()

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(fmt.Sprintf(`{"username":"%s","password":"%s"}`, username, password)))
	loginReq.Header.Set("Content-Type", "application/json")

	resp := executeIntegrationRequest(handler, loginReq)
	result := resp.Result()
	defer func() {
		_ = result.Body.Close()
	}()

	require.Equal(t, http.StatusOK, result.StatusCode)

	body, err := io.ReadAll(result.Body)
	require.NoError(t, err)

	var loginResp LoginResponse
	require.NoError(t, json.Unmarshal(body, &loginResp))
	require.NotNil(t, loginResp.User)

	var sessionCookie *http.Cookie
	for _, c := range result.Cookies() {
		if c.Name == "kaunta_session" {
			sessionCookie = c
			break
		}
	}
	require.NotNil(t, sessionCookie, "session cookie should be present")

	return sessionCookie, loginResp
}
