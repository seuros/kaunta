package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/seuros/kaunta/internal/config"
	"github.com/seuros/kaunta/internal/database"
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
	AdminEmail           string `form:"admin_email" json:"admin_email"`
	AdminPassword        string `form:"admin_password" json:"admin_password"`
	AdminPasswordConfirm string `form:"admin_password_confirm" json:"admin_password_confirm"`
}

// ShowSetup displays the setup page
func ShowSetup(setupTemplate []byte) fiber.Handler {
	return func(c fiber.Ctx) error {
		// Check if setup is actually needed
		status, err := config.CheckSetupStatus()
		if err != nil {
			logging.L().Error("failed to check setup status", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).SendString("Setup check failed")
		}

		if !status.NeedsSetup {
			// Setup not needed, redirect to dashboard
			return c.Redirect().To("/")
		}

		// Prepare template data
		data := fiber.Map{
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
		data["ServerPort"] = cfg.Port
		if data["ServerPort"] == "" {
			data["ServerPort"] = "3000"
		}
		data["DataDir"] = cfg.DataDir
		if data["DataDir"] == "" {
			data["DataDir"] = "./data"
		}

		// Render setup page
		return c.Type("html").Send(setupTemplate)
	}
}

// SubmitSetup processes the setup form submission
func SubmitSetup() fiber.Handler {
	return func(c fiber.Ctx) error {
		// Parse form
		var form SetupForm
		if err := c.Bind().Body(&form); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid form data",
			})
		}

		// Validate form fields
		if err := validateSetupForm(&form); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": err.Error(),
			})
		}

		// Build database URL
		dbURL := buildDatabaseURL(&form)

		// Test database connection
		db, err := sql.Open("postgres", dbURL)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("Invalid database configuration: %v", err),
			})
		}
		defer func() { _ = db.Close() }()

		if err := db.Ping(); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("Cannot connect to database: %v", err),
			})
		}

		// Check if users already exist
		hasUsers, err := models.HasAnyUsers(context.Background(), db)
		if err == nil && hasUsers {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Setup already completed. Users already exist in the database.",
			})
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
			form.AdminEmail,
		)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": fmt.Sprintf("Failed to create admin user: %v", err),
			})
		}

		// Save configuration
		cfg := &config.Config{
			DatabaseURL:    dbURL,
			Port:           form.ServerPort,
			DataDir:        form.DataDir,
			SecureCookies:  true,
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

		_, err = db.Exec(
			`INSERT INTO user_sessions (session_id, user_id, expires_at)
			VALUES ($1, $2, $3)`,
			sessionID, user.UserID, expiresAt,
		)
		if err != nil {
			logging.L().Warn("failed to create session after setup", zap.Error(err))
			// Don't fail, user can login manually
		} else {
			// Set session cookie
			c.Cookie(&fiber.Cookie{
				Name:     "kaunta_session",
				Value:    sessionID.String(),
				Path:     "/",
				HTTPOnly: true,
				Secure:   cfg.SecureCookies,
				SameSite: "Lax",
				Expires:  expiresAt,
			})
		}

		// Return success response
		return c.JSON(fiber.Map{
			"success": true,
			"message": "Setup completed successfully",
			"user": fiber.Map{
				"id":       user.UserID.String(),
				"username": user.Username,
			},
		})
	}
}

// TestDatabase tests the database connection with provided credentials
func TestDatabase() fiber.Handler {
	return func(c fiber.Ctx) error {
		// Parse form
		var form SetupForm
		if err := c.Bind().Body(&form); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid form data",
			})
		}

		// Validate database fields
		if form.DBHost == "" || form.DBPort == "" || form.DBName == "" || form.DBUser == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Missing required database fields",
			})
		}

		// Build database URL
		dbURL := buildDatabaseURL(&form)

		// Test connection
		db, err := sql.Open("postgres", dbURL)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"success": false,
				"error":   fmt.Sprintf("Invalid configuration: %v", err),
			})
		}
		defer func() { _ = db.Close() }()

		if err := db.Ping(); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"success": false,
				"error":   fmt.Sprintf("Connection failed: %v", err),
			})
		}

		// Check PostgreSQL version
		var version string
		err = db.QueryRow("SELECT version()").Scan(&version)
		if err != nil {
			version = "Unknown"
		}

		return c.JSON(fiber.Map{
			"success": true,
			"message": "Database connection successful",
			"version": version,
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

	if form.AdminEmail == "" {
		return fmt.Errorf("admin email is required")
	}
	if !isValidEmail(form.AdminEmail) {
		return fmt.Errorf("invalid email format")
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

// isValidEmail checks if the email format is valid
func isValidEmail(email string) bool {
	// Reject emails with whitespace
	if strings.ContainsAny(email, " \t\n\r") {
		return false
	}

	// Basic email validation
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	if len(parts[0]) == 0 || len(parts[1]) == 0 {
		return false
	}
	// Check for at least one dot in domain
	return strings.Contains(parts[1], ".")
}
