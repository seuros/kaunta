package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/seuros/kaunta/internal/middleware"
)

func stubFetchUser(t *testing.T, fn func(username string) (*userRecord, error)) {
	t.Helper()
	original := fetchUserByUsername
	fetchUserByUsername = fn
	t.Cleanup(func() {
		fetchUserByUsername = original
	})
}

func stubVerifyPassword(t *testing.T, fn func(password, passwordHash string) (bool, error)) {
	t.Helper()
	original := verifyPasswordHashFunc
	verifyPasswordHashFunc = fn
	t.Cleanup(func() {
		verifyPasswordHashFunc = original
	})
}

func stubInsertSession(
	t *testing.T,
	fn func(sessionID, userID uuid.UUID, tokenHash string, expiresAt time.Time, userAgent, ipAddress string) error,
) {
	t.Helper()
	original := insertSessionFunc
	insertSessionFunc = fn
	t.Cleanup(func() {
		insertSessionFunc = original
	})
}

func stubSessionTokenGenerator(t *testing.T, fn func() (string, string, error)) {
	t.Helper()
	original := sessionTokenGenerator
	sessionTokenGenerator = fn
	t.Cleanup(func() {
		sessionTokenGenerator = original
	})
}

func stubFetchUserDetails(t *testing.T, fn func(userID uuid.UUID) (sql.NullString, time.Time, error)) {
	t.Helper()
	original := fetchUserDetailsFunc
	fetchUserDetailsFunc = fn
	t.Cleanup(func() {
		fetchUserDetailsFunc = original
	})
}

func TestHandleLoginSuccess(t *testing.T) {
	userID := uuid.New()
	stubFetchUser(t, func(username string) (*userRecord, error) {
		assert.Equal(t, "demo", username)
		return &userRecord{
			UserID:       userID,
			Username:     username,
			Name:         sql.NullString{String: "Demo", Valid: true},
			PasswordHash: "hashed",
		}, nil
	})
	stubVerifyPassword(t, func(password, passwordHash string) (bool, error) {
		assert.Equal(t, "secret", password)
		assert.Equal(t, "hashed", passwordHash)
		return true, nil
	})

	insertCalled := false
	stubInsertSession(t, func(
		sessionID, gotUserID uuid.UUID, tokenHash string, expiresAt time.Time, userAgent, ipAddress string,
	) error {
		insertCalled = true
		assert.Equal(t, userID, gotUserID)
		assert.Equal(t, "hashed-token", tokenHash)
		assert.Equal(t, "TestAgent", userAgent)
		assert.NotEmpty(t, ipAddress)
		assert.WithinDuration(t, time.Now().Add(7*24*time.Hour), expiresAt, 5*time.Second)
		return nil
	})

	stubSessionTokenGenerator(t, func() (string, string, error) {
		return "plain-token", "hashed-token", nil
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"demo","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TestAgent")
	req.RemoteAddr = "1.2.3.4:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")

	resp := httptest.NewRecorder()
	HandleLogin(resp, req)
	result := resp.Result()
	defer func() {
		_ = result.Body.Close()
	}()

	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.True(t, insertCalled)

	body, err := io.ReadAll(result.Body)
	require.NoError(t, err)
	var data LoginResponse
	require.NoError(t, json.Unmarshal(body, &data))
	assert.True(t, data.Success)
	require.NotNil(t, data.User)
	assert.Equal(t, userID, data.User.UserID)
	assert.Equal(t, "Demo", *data.User.Name)

	cookie := result.Cookies()
	require.NotEmpty(t, cookie)
	found := false
	for _, c := range cookie {
		if c.Name == "kaunta_session" {
			found = true
			assert.Equal(t, "plain-token", c.Value)
			assert.True(t, c.HttpOnly)
			assert.True(t, c.Secure)
			break
		}
	}
	assert.True(t, found, "session cookie should be set")
}

func TestHandleLoginInvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	HandleLogin(resp, req)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestHandleLoginMissingCredentials(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"demo"}`))
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	HandleLogin(resp, req)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestHandleLoginUnknownUser(t *testing.T) {
	stubFetchUser(t, func(username string) (*userRecord, error) {
		return nil, sql.ErrNoRows
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"demo","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	HandleLogin(resp, req)
	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestHandleLoginDatabaseError(t *testing.T) {
	stubFetchUser(t, func(username string) (*userRecord, error) {
		return nil, errors.New("db error")
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"demo","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	HandleLogin(resp, req)
	assert.Equal(t, http.StatusInternalServerError, resp.Code)
}

func TestHandleLoginInvalidPassword(t *testing.T) {
	stubFetchUser(t, func(username string) (*userRecord, error) {
		return &userRecord{
			UserID:       uuid.New(),
			Username:     username,
			PasswordHash: "hashed",
		}, nil
	})
	stubVerifyPassword(t, func(password, passwordHash string) (bool, error) {
		return false, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"demo","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	HandleLogin(resp, req)
	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestHandleLoginVerifyPasswordError(t *testing.T) {
	stubFetchUser(t, func(username string) (*userRecord, error) {
		return &userRecord{
			UserID:       uuid.New(),
			Username:     username,
			PasswordHash: "hashed",
		}, nil
	})
	stubVerifyPassword(t, func(password, passwordHash string) (bool, error) {
		return false, errors.New("boom")
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"demo","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	HandleLogin(resp, req)
	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestHandleLoginTokenGenerationFailure(t *testing.T) {
	stubFetchUser(t, func(username string) (*userRecord, error) {
		return &userRecord{
			UserID:       uuid.New(),
			Username:     username,
			PasswordHash: "hashed",
		}, nil
	})
	stubVerifyPassword(t, func(password, passwordHash string) (bool, error) {
		return true, nil
	})
	stubSessionTokenGenerator(t, func() (string, string, error) {
		return "", "", errors.New("token error")
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"demo","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	HandleLogin(resp, req)
	assert.Equal(t, http.StatusInternalServerError, resp.Code)
}

func TestHandleLoginInsertSessionFailure(t *testing.T) {
	stubFetchUser(t, func(username string) (*userRecord, error) {
		return &userRecord{
			UserID:       uuid.New(),
			Username:     username,
			PasswordHash: "hashed",
		}, nil
	})
	stubVerifyPassword(t, func(password, passwordHash string) (bool, error) {
		return true, nil
	})
	stubSessionTokenGenerator(t, func() (string, string, error) {
		return "token", "hash", nil
	})
	stubInsertSession(t, func(
		sessionID, userID uuid.UUID, tokenHash string, expiresAt time.Time, userAgent, ipAddress string,
	) error {
		return errors.New("insert error")
	})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"demo","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	HandleLogin(resp, req)
	assert.Equal(t, http.StatusInternalServerError, resp.Code)
}

func TestHandleMeSuccess(t *testing.T) {
	userID := uuid.New()
	stubFetchUserDetails(t, func(id uuid.UUID) (sql.NullString, time.Time, error) {
		assert.Equal(t, userID, id)
		return sql.NullString{String: "Demo", Valid: true}, time.Unix(0, 0), nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req = req.WithContext(middleware.ContextWithUser(req.Context(), &middleware.UserContext{
		UserID:   userID,
		Username: "demo",
	}))

	resp := httptest.NewRecorder()
	HandleMe(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "demo")
	assert.Contains(t, string(body), "Demo")
}

func TestHandleMeUnauthenticated(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	resp := httptest.NewRecorder()
	HandleMe(resp, req)
	assert.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestHandleMeDatabaseError(t *testing.T) {
	stubFetchUserDetails(t, func(id uuid.UUID) (sql.NullString, time.Time, error) {
		return sql.NullString{}, time.Time{}, errors.New("db error")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req = req.WithContext(middleware.ContextWithUser(req.Context(), &middleware.UserContext{
		UserID:   uuid.New(),
		Username: "demo",
	}))

	resp := httptest.NewRecorder()
	HandleMe(resp, req)

	assert.Equal(t, http.StatusInternalServerError, resp.Code)
}
