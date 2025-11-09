package middleware

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func stubSessionValidator(t *testing.T, stub func(tokenHash string) (*UserContext, error)) {
	t.Helper()
	original := sessionValidator
	sessionValidator = stub
	t.Cleanup(func() {
		sessionValidator = original
	})
}

func newTestApp(handler fiber.Handler) *fiber.App {
	app := fiber.New()
	app.Use(Auth)
	app.Get("/", handler)
	return app
}

func newTestAppWithRedirect(handler fiber.Handler) *fiber.App {
	app := fiber.New()
	app.Use(AuthWithRedirect)
	app.Get("/", handler)
	app.Get("/login", func(c *fiber.Ctx) error {
		return c.SendString("login page")
	})
	return app
}

func TestHashTokenDeterministic(t *testing.T) {
	token := "test-token"
	expected := hashToken(token)
	assert.Equal(t, expected, hashToken(token))
	assert.NotEmpty(t, expected)
}

func TestGetUserWithoutContextReturnsNil(t *testing.T) {
	app := fiber.New()
	ctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(ctx)
	assert.Nil(t, GetUser(ctx))
}

func TestAuthMissingTokenReturnsUnauthorized(t *testing.T) {
	app := newTestApp(func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)
	assert.Contains(t, string(body), "no session token")
}

func TestAuthInvalidSessionFromDB(t *testing.T) {
	token := "invalid-token"
	stubSessionValidator(t, func(tokenHash string) (*UserContext, error) {
		assert.Equal(t, hashToken(token), tokenHash)
		return nil, sql.ErrNoRows
	})

	app := newTestApp(func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "kaunta_session", Value: token})

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)

	body, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)
	assert.Contains(t, string(body), "invalid or expired session")
}

func TestAuthDatabaseError(t *testing.T) {
	stubSessionValidator(t, func(tokenHash string) (*UserContext, error) {
		return nil, errors.New("boom")
	})

	app := newTestApp(func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "kaunta_session", Value: "token"})

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)

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
		assert.Equal(t, hashToken("good-token"), tokenHash)
		return expectedUser, nil
	})

	var capturedUser *UserContext

	app := newTestApp(func(c *fiber.Ctx) error {
		capturedUser = GetUser(c)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "kaunta_session", Value: "good-token"})

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	require.NotNil(t, capturedUser)
	assert.Equal(t, expectedUser.UserID, capturedUser.UserID)
	assert.Equal(t, expectedUser.Username, capturedUser.Username)
	assert.Equal(t, expectedUser.SessionID, capturedUser.SessionID)
}

func TestAuthUsesAuthorizationHeader(t *testing.T) {
	stubSessionValidator(t, func(tokenHash string) (*UserContext, error) {
		assert.Equal(t, hashToken("bearer-token"), tokenHash)
		return &UserContext{
			UserID:    uuid.New(),
			Username:  "api-user",
			SessionID: uuid.New(),
		}, nil
	})

	app := newTestApp(func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer bearer-token")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestAuthWithRedirectNoToken(t *testing.T) {
	app := newTestAppWithRedirect(func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusFound, resp.StatusCode)
	assert.Equal(t, "/login", resp.Header.Get("Location"))
}

func TestAuthWithRedirectValidToken(t *testing.T) {
	stubSessionValidator(t, func(tokenHash string) (*UserContext, error) {
		return &UserContext{UserID: uuid.New(), Username: "test"}, nil
	})

	app := newTestAppWithRedirect(func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "kaunta_session", Value: "token"})

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}
