package middleware

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stubSessionValidator(t *testing.T, stub func(tokenHash string) (*UserContext, error)) {
	t.Helper()
	original := sessionValidator
	sessionValidator = stub
	t.Cleanup(func() {
		sessionValidator = original
	})
}

func executeAuth(t *testing.T, req *http.Request, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	if handler == nil {
		handler = func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}
	}

	recorder := httptest.NewRecorder()
	Auth(handler).ServeHTTP(recorder, req)
	return recorder
}

func executeAuthWithRedirect(t *testing.T, req *http.Request, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	if handler == nil {
		handler = func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}
	}

	recorder := httptest.NewRecorder()
	AuthWithRedirect(handler).ServeHTTP(recorder, req)
	return recorder
}

func TestHashTokenDeterministic(t *testing.T) {
	token := "test-token"
	expected := HashToken(token)
	assert.Equal(t, expected, HashToken(token))
	assert.NotEmpty(t, expected)
}

func TestGetUserWithoutContextReturnsNil(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	assert.Nil(t, GetUser(req))
}

func TestAuthMissingTokenReturnsUnauthorized(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := executeAuth(t, req, nil)

	assert.Equal(t, http.StatusUnauthorized, resp.Code)

	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)
	assert.Contains(t, string(body), "no session token")
}

func TestAuthInvalidSessionFromDB(t *testing.T) {
	token := "invalid-token"
	stubSessionValidator(t, func(tokenHash string) (*UserContext, error) {
		assert.Equal(t, HashToken(token), tokenHash)
		return nil, sql.ErrNoRows
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "kaunta_session", Value: token})

	resp := executeAuth(t, req, nil)

	assert.Equal(t, http.StatusUnauthorized, resp.Code)

	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)
	assert.Contains(t, string(body), "invalid or expired session")
}

func TestAuthDatabaseError(t *testing.T) {
	stubSessionValidator(t, func(tokenHash string) (*UserContext, error) {
		return nil, errors.New("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "kaunta_session", Value: "token"})

	resp := executeAuth(t, req, nil)

	assert.Equal(t, http.StatusInternalServerError, resp.Code)

	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)
	assert.Contains(t, string(body), "Authentication error")
}

func TestAuthSuccessStoresUserContext(t *testing.T) {
	expectedUser := &UserContext{
		UserID:    uuid.New(),
		Username:  "demo",
		SessionID: uuid.New(),
	}

	stubSessionValidator(t, func(tokenHash string) (*UserContext, error) {
		assert.Equal(t, HashToken("good-token"), tokenHash)
		return expectedUser, nil
	})

	var capturedUser *UserContext

	handler := func(w http.ResponseWriter, r *http.Request) {
		capturedUser = GetUser(r)
		w.WriteHeader(http.StatusOK)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "kaunta_session", Value: "good-token"})

	resp := executeAuth(t, req, handler)

	assert.Equal(t, http.StatusOK, resp.Code)
	require.NotNil(t, capturedUser)
	assert.Equal(t, expectedUser.UserID, capturedUser.UserID)
	assert.Equal(t, expectedUser.Username, capturedUser.Username)
	assert.Equal(t, expectedUser.SessionID, capturedUser.SessionID)
}

func TestAuthUsesAuthorizationHeader(t *testing.T) {
	stubSessionValidator(t, func(tokenHash string) (*UserContext, error) {
		assert.Equal(t, HashToken("bearer-token"), tokenHash)
		return &UserContext{
			UserID:    uuid.New(),
			Username:  "api-user",
			SessionID: uuid.New(),
		}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer bearer-token")

	resp := executeAuth(t, req, nil)

	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestAuthWithRedirectNoToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := executeAuthWithRedirect(t, req, nil)

	assert.Equal(t, http.StatusSeeOther, resp.Code)
	assert.Equal(t, "/login", resp.Header().Get("Location"))
}

func TestAuthWithRedirectValidToken(t *testing.T) {
	stubSessionValidator(t, func(tokenHash string) (*UserContext, error) {
		return &UserContext{UserID: uuid.New(), Username: "test"}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "kaunta_session", Value: "token"})

	resp := executeAuthWithRedirect(t, req, nil)

	assert.Equal(t, http.StatusOK, resp.Code)
}
