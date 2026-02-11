//go:build integration

package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/seuros/kaunta/internal/config"
	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/models"
	"github.com/seuros/kaunta/internal/test"
)

func TestSetupFlow_Integration(t *testing.T) {
	// Get test database
	testDB := test.GetTestDatabase(t)
	defer test.CleanupTestDatabase(t, testDB)

	// Set DATABASE_URL for the test
	os.Setenv("DATABASE_URL", testDB.URL)
	defer os.Unsetenv("DATABASE_URL")

	// Connect to test database
	err := database.Connect()
	require.NoError(t, err)
	defer database.Close()

	// Run migrations
	err = database.RunMigrations(testDB.URL)
	require.NoError(t, err)

	// Create temp directory for config
	tempDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(origDir)

	// Verify no users exist initially
	hasUsers, err := models.HasAnyUsers(context.Background(), database.DB)
	require.NoError(t, err)
	assert.False(t, hasUsers)

	// Create router for testing
	router := chi.NewRouter()

	// Add setup routes
	setupTemplate := []byte(`<!DOCTYPE html><html><body>Setup Page</body></html>`)
	router.Get("/setup", ShowSetup(setupTemplate))
	router.Post("/setup", SubmitSetup())
	router.Post("/setup/test-db", TestDatabase())

	// Test 1: GET /setup should show the setup page
	req := httptest.NewRequest("GET", "/setup", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	assert.Equal(t, 200, resp.Code)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "Setup Page")

	// Test 2: Test database connection
	testDBForm := SetupForm{
		DBHost:     testDB.Host,
		DBPort:     testDB.Port,
		DBName:     testDB.Name,
		DBUser:     testDB.User,
		DBPassword: testDB.Password,
		DBSSLMode:  "disable",
	}

	jsonBody, _ := json.Marshal(testDBForm)
	req = httptest.NewRequest("POST", "/setup/test-db", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	assert.Equal(t, 200, resp.Code)

	var testResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&testResult)
	assert.True(t, testResult["success"].(bool))
	assert.Contains(t, testResult, "version")

	// Test 3: Complete setup with admin user
	setupForm := SetupForm{
		DBHost:               testDB.Host,
		DBPort:               testDB.Port,
		DBName:               testDB.Name,
		DBUser:               testDB.User,
		DBPassword:           testDB.Password,
		DBSSLMode:            "disable",
		ServerPort:           "3000",
		DataDir:              "./data",
		AdminUsername:        "testadmin",
		AdminEmail:           "admin@test.com",
		AdminPassword:        "TestPassword123!",
		AdminPasswordConfirm: "TestPassword123!",
	}

	jsonBody, _ = json.Marshal(setupForm)
	req = httptest.NewRequest("POST", "/setup", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	body, _ = io.ReadAll(resp.Body)
	if resp.Code != 200 {
		t.Logf("Response body: %s", string(body))
	}
	assert.Equal(t, 200, resp.Code)

	var setupResult map[string]interface{}
	err = json.Unmarshal(body, &setupResult)
	require.NoError(t, err)

	assert.True(t, setupResult["success"].(bool))
	assert.Contains(t, setupResult, "user")

	// Verify user was created
	hasUsers, err = models.HasAnyUsers(context.Background(), database.DB)
	require.NoError(t, err)
	assert.True(t, hasUsers)

	// Verify we can validate the created user
	user, err := models.ValidateUser(context.Background(), database.DB, "testadmin", "TestPassword123!")
	require.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, "testadmin", user.Username)

	// Verify config was saved
	configPath := filepath.Join(tempDir, "kaunta.toml")
	assert.FileExists(t, configPath)

	// Load and verify config
	cfg, err := config.Load()
	require.NoError(t, err)
	assert.True(t, cfg.InstallLock)
	assert.Equal(t, "3000", cfg.Port)
	assert.Equal(t, "./data", cfg.DataDir)

	// Test 4: Try to setup again - should fail
	jsonBody, _ = json.Marshal(setupForm)
	req = httptest.NewRequest("POST", "/setup", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	assert.Equal(t, 400, resp.Code)

	var errorResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&errorResult)
	assert.Contains(t, errorResult["error"], "already")
}

func TestSetupValidation_Integration(t *testing.T) {
	// Get test database
	testDB := test.GetTestDatabase(t)
	defer test.CleanupTestDatabase(t, testDB)

	// Create router
	router := chi.NewRouter()
	router.Post("/setup", SubmitSetup())

	// Test various validation errors
	tests := []struct {
		name        string
		form        SetupForm
		expectedErr string
	}{
		{
			name: "invalid database credentials",
			form: SetupForm{
				DBHost:               "invalid-host",
				DBPort:               "5432",
				DBName:               "invalid",
				DBUser:               "invalid",
				DBPassword:           "invalid",
				AdminUsername:        "admin",
				AdminEmail:           "admin@test.com",
				AdminPassword:        "password123",
				AdminPasswordConfirm: "password123",
			},
			expectedErr: "Cannot connect to database",
		},
		{
			name: "invalid email format",
			form: SetupForm{
				DBHost:               testDB.Host,
				DBPort:               testDB.Port,
				DBName:               testDB.Name,
				DBUser:               testDB.User,
				DBPassword:           testDB.Password,
				AdminUsername:        "admin",
				AdminEmail:           "not-an-email",
				AdminPassword:        "password123",
				AdminPasswordConfirm: "password123",
			},
			expectedErr: "invalid email format",
		},
		{
			name: "password too short",
			form: SetupForm{
				DBHost:               testDB.Host,
				DBPort:               testDB.Port,
				DBName:               testDB.Name,
				DBUser:               testDB.User,
				DBPassword:           testDB.Password,
				AdminUsername:        "admin",
				AdminEmail:           "admin@test.com",
				AdminPassword:        "short",
				AdminPasswordConfirm: "short",
			},
			expectedErr: "at least 8 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBody, _ := json.Marshal(tt.form)
			req := httptest.NewRequest("POST", "/setup", bytes.NewReader(jsonBody))
			req.Header.Set("Content-Type", "application/json")

			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			assert.Equal(t, 400, resp.Code)

			var result map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&result)
			assert.Contains(t, result["error"], tt.expectedErr)
		})
	}
}

func TestCheckSetupStatus_Integration(t *testing.T) {
	// Get test database
	testDB := test.GetTestDatabase(t)
	defer test.CleanupTestDatabase(t, testDB)

	// Set DATABASE_URL
	os.Setenv("DATABASE_URL", testDB.URL)
	defer os.Unsetenv("DATABASE_URL")

	// Create temp directory for config
	tempDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(origDir)

	// Test 1: Fresh database - needs setup
	status, err := config.CheckSetupStatus()
	require.NoError(t, err)
	assert.True(t, status.NeedsSetup)
	assert.True(t, status.HasDatabaseConfig)
	assert.False(t, status.HasUsers)
	assert.Contains(t, status.Reason, "No users")

	// Connect and run migrations
	db, err := sql.Open("postgres", testDB.URL)
	require.NoError(t, err)
	defer db.Close()

	err = database.RunMigrations(testDB.URL)
	require.NoError(t, err)

	// Create a user
	_, err = models.CreateUser(context.Background(), db, "testuser", "password123", "Test User")
	require.NoError(t, err)

	// Test 2: Has users but no install lock - still needs setup
	status, err = config.CheckSetupStatus()
	require.NoError(t, err)
	assert.False(t, status.NeedsSetup)

	// Test 3: Save config with install lock
	cfg := &config.Config{
		DatabaseURL: testDB.URL,
		InstallLock: true,
	}
	err = config.SaveConfig(cfg)
	require.NoError(t, err)

	// Test 4: With install lock - no setup needed
	status, err = config.CheckSetupStatus()
	require.NoError(t, err)
	assert.False(t, status.NeedsSetup)
}
