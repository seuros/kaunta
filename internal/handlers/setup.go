package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/seuros/kaunta/internal/config"
	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/httpx"
	"github.com/seuros/kaunta/internal/logging"
	"github.com/seuros/kaunta/internal/models"
	"go.uber.org/zap"
)

// SetupForm represents the setup form data
type SetupForm struct {
	// Database configuration
	DBHost     string `form:"db_host" json:"db_host"`
	DBPort     string `form:"db_port" json:"db_port"`
	DBName     string `form:"db_name" json:"db_name"`
	DBUser     string `form:"db_user" json:"db_user"`
	DBPassword string `form:"db_password" json:"db_password"`
	DBSSLMode  string `form:"db_ssl_mode" json:"db_ssl_mode"`

	// Server configuration
	ServerPort string `form:"server_port" json:"server_port"`
	DataDir    string `form:"data_dir" json:"data_dir"`

	// Admin user
	AdminUsername        string `form:"admin_username" json:"admin_username"`
	AdminName            string `form:"admin_name" json:"admin_name"`
	AdminPassword        string `form:"admin_password" json:"admin_password"`
	AdminPasswordConfirm string `form:"admin_password_confirm" json:"admin_password_confirm"`
}

// DatastarRequest represent the request format from setup page
type DatastarRequest struct {
	Form SetupForm `json:"form"`
}

// ShowSetup displays the setup page
func ShowSetup(setupTemplate []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if setup is actually needed
		status, err := config.CheckSetupStatus()
		if err != nil {
			logging.L().Error("failed to check setup status", zap.Error(err))
			http.Error(w, "Setup check failed", http.StatusInternalServerError)
			return
		}

		if !status.NeedsSetup {
			// Setup not needed, redirect to dashboard
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		// Prepare template data
		data := map[string]any{
			"Title":             "Setup",
			"NeedsSetup":        status.NeedsSetup,
			"HasDatabaseConfig": status.HasDatabaseConfig,
			"Reason":            status.Reason,
		}

		// Pre-fill database config if available
		cfg, _ := config.Load()
		if cfg != nil && cfg.DatabaseURL != "" {
			dbConfig := config.ParseDatabaseURL(cfg.DatabaseURL)
			data["DBHost"] = dbConfig.Host
			data["DBPort"] = dbConfig.Port
			data["DBName"] = dbConfig.Name
			data["DBUser"] = dbConfig.User
			data["DBSSLMode"] = dbConfig.SSLMode
		} else {
			// Set defaults
			data["DBHost"] = "localhost"
			data["DBPort"] = 5432
			data["DBSSLMode"] = "disable"
		}

		// Set server defaults
		if cfg != nil && cfg.Port != "" {
			data["ServerPort"] = cfg.Port
		} else {
			data["ServerPort"] = "3000"
		}
		if cfg != nil && cfg.DataDir != "" {
			data["DataDir"] = cfg.DataDir
		} else {
			data["DataDir"] = "./data"
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(setupTemplate)
	}
}

// SubmitSetup processes the setup form submission
// onComplete is called after successful setup to signal server restart
func SubmitSetup(onComplete func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var reqBody DatastarRequest
		if err := httpx.ReadJSON(r, &reqBody); err != nil {
			logging.L().Error("Bind failed for setup", zap.Error(err))
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]any{
				"submitting":  false,
				"message":     "Invalid request: " + err.Error(),
				"messageType": "error",
			})
			return
		}
		form := reqBody.Form
		logging.L().Info("Parsed form for setup")

		// Validate form fields
		if err := validateSetupForm(&form); err != nil {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]any{
				"submitting":  false,
				"message":     err.Error(),
				"messageType": "error",
			})
			return
		}

		// Build database URL
		dbURL := buildDatabaseURL(&form)

		// Test database connection
		db, err := sql.Open("postgres", dbURL)
		if err != nil {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]any{
				"error": fmt.Sprintf("Invalid database configuration: %v", err),
			})
			return
		}
		defer func() { _ = db.Close() }()

		if err := db.Ping(); err != nil {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]any{
				"error": fmt.Sprintf("Cannot connect to database: %v", err),
			})
			return
		}

		// Check if users already exist
		hasUsers, err := models.HasAnyUsers(context.Background(), db)
		if err == nil && hasUsers {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]any{
				"error": "Setup already completed. Users already exist in the database.",
			})
			return
		}

		// Run migrations
		logging.L().Info("running database migrations during setup")
		if err := database.RunMigrations(dbURL); err != nil {
			logging.L().Warn("migration warning during setup", zap.Error(err))
			// Don't fail setup if migrations have issues, they might already be applied
		}

		// Create admin user
		user, err := models.CreateUser(
			context.Background(),
			db,
			form.AdminUsername,
			form.AdminPassword,
			form.AdminName,
		)
		if err != nil {
			httpx.WriteJSON(w, http.StatusInternalServerError, map[string]any{
				"error": fmt.Sprintf("Failed to create admin user: %v", err),
			})
			return
		}

		// Create "self" website for dogfooding (tracking Kaunta dashboard itself)
		// Uses hardcoded nil UUID for deterministic, identifiable ID across all installations
		allowedDomains := []string{
			"localhost",
			"localhost:" + form.ServerPort,
			"http://localhost",
			"http://localhost:" + form.ServerPort,
			"https://localhost",
			"https://localhost:" + form.ServerPort,
		}
		allowedDomainsJSON, _ := json.Marshal(allowedDomains)

		_, err = db.Exec(`
			INSERT INTO website (website_id, domain, name, allowed_domains, user_id, created_at, updated_at)
			VALUES ($1, 'self', 'Kaunta Dashboard', $2::jsonb, $3, NOW(), NOW())
			ON CONFLICT (website_id) DO NOTHING
		`, config.SelfWebsiteID, string(allowedDomainsJSON), user.UserID)
		if err != nil {
			logging.L().Warn("failed to create self website", zap.Error(err))
			// Don't fail setup, self-tracking is optional
		}

		// Save configuration
		// SecureCookies defaults to false for localhost setup compatibility
		// Users should enable it for HTTPS deployments
		cfg := &config.Config{
			DatabaseURL:    dbURL,
			Port:           form.ServerPort,
			DataDir:        form.DataDir,
			SecureCookies:  false,
			TrustedOrigins: []string{"localhost"},
			InstallLock:    true,
		}

		if err := config.SaveConfig(cfg); err != nil {
			logging.L().Error("failed to save config file", zap.Error(err))
			// Don't fail, config saving is not critical
		}

		// Create session for the new admin user
		sessionID := uuid.New()
		expiresAt := time.Now().Add(7 * 24 * time.Hour)

		// Generate session token
		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			logging.L().Warn("failed to generate session token", zap.Error(err))
		} else {
			token := hex.EncodeToString(tokenBytes)
			hashBytes := sha256.Sum256([]byte(token))
			tokenHash := hex.EncodeToString(hashBytes[:])

			_, err = db.Exec(
				`INSERT INTO user_sessions (session_id, user_id, token_hash, expires_at)
				VALUES ($1, $2, $3, $4)`,
				sessionID, user.UserID, tokenHash, expiresAt,
			)
			if err != nil {
				logging.L().Warn("failed to create session after setup", zap.Error(err))
				// Don't fail, user can login manually
			} else {
				// Set session cookie (won't survive server restart, but code is correct)
				http.SetCookie(w, &http.Cookie{
					Name:     "kaunta_session",
					Value:    token,
					Path:     "/",
					HttpOnly: true,
					Secure:   cfg.SecureCookies,
					SameSite: http.SameSiteLaxMode,
					Expires:  expiresAt,
				})
			}
		}

		response := map[string]any{
			"success":    true,
			"submitting": false,
			"message":    "Setup completed successfully. Server is restarting...",
			"user": map[string]any{
				"id":       user.UserID.String(),
				"username": user.Username,
			},
			"redirect": "/dashboard",
		}

		httpx.WriteJSON(w, http.StatusOK, response)

		// Signal setup completion (triggers server restart) after response is flushed
		if onComplete != nil {
			go onComplete()
		}
	}
}

// TestDatabase tests the database connection with provided credentials
func TestDatabase() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		raw := string(bodyBytes)
		logging.L().Info("Raw body for test-db", zap.String("raw", raw))

		var reqBody DatastarRequest
		if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
			logging.L().Error("Bind failed for test-db", zap.Error(err))
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]any{
				"testing":     false,
				"submitting":  false,
				"message":     "Invalid request: " + err.Error(),
				"messageType": "error",
			})
			return
		}

		form := reqBody.Form
		logging.L().Info("Parsed form for test-db", zap.Any("form", form))

		if form.DBHost == "" || form.DBPort == "" || form.DBName == "" || form.DBUser == "" {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]any{
				"testing":     false,
				"submitting":  false,
				"message":     "Missing required database fields",
				"messageType": "error",
			})
			return
		}

		dbURL := buildDatabaseURL(&form)
		db, err := sql.Open("postgres", dbURL)
		if err != nil {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]any{
				"testing":     false,
				"submitting":  false,
				"message":     fmt.Sprintf("Invalid config: %v", err),
				"messageType": "error",
			})
			return
		}
		defer func() { _ = db.Close() }()

		if err := db.Ping(); err != nil {
			httpx.WriteJSON(w, http.StatusBadRequest, map[string]any{
				"testing":     false,
				"submitting":  false,
				"message":     fmt.Sprintf("Connection failed: %v", err),
				"messageType": "error",
			})
			return
		}

		var version string
		if err := db.QueryRow("SELECT version()").Scan(&version); err != nil {
			version = "Unknown"
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"testing":     false,
			"submitting":  false,
			"message":     "Database connection successful! Version: " + version,
			"messageType": "success",
			"version":     version,
		})
	}
}

// validateSetupForm validates the setup form data
func validateSetupForm(form *SetupForm) error {
	// Apply defaults first
	if form.DBPort == "" {
		form.DBPort = "5432"
	}
	if form.ServerPort == "" {
		form.ServerPort = "3000"
	}
	if form.DataDir == "" {
		form.DataDir = "./data"
	}

	// Validate database fields
	if form.DBHost == "" {
		return fmt.Errorf("database host is required")
	}
	if form.DBName == "" {
		return fmt.Errorf("database name is required")
	}
	if form.DBUser == "" {
		return fmt.Errorf("database user is required")
	}

	// Validate admin user fields
	if form.AdminUsername == "" {
		return fmt.Errorf("admin username is required")
	}
	if len(form.AdminUsername) < 3 || len(form.AdminUsername) > 30 {
		return fmt.Errorf("username must be between 3 and 30 characters")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(form.AdminUsername) {
		return fmt.Errorf("username can only contain letters, numbers, and underscores")
	}

	if form.AdminPassword == "" {
		return fmt.Errorf("admin password is required")
	}
	if len(form.AdminPassword) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if form.AdminPassword != form.AdminPasswordConfirm {
		return fmt.Errorf("passwords do not match")
	}

	return nil
}

// buildDatabaseURL constructs a PostgreSQL connection URL from form data
func buildDatabaseURL(form *SetupForm) string {
	sslMode := form.DBSSLMode
	if sslMode == "" {
		sslMode = "disable"
	}

	port := form.DBPort
	if port == "" {
		port = "5432"
	}

	// Build the connection string
	url := fmt.Sprintf("postgres://%s", form.DBUser)
	if form.DBPassword != "" {
		url += fmt.Sprintf(":%s", form.DBPassword)
	}
	url += fmt.Sprintf("@%s:%s/%s?sslmode=%s",
		form.DBHost,
		port,
		form.DBName,
		sslMode,
	)

	return url
}
