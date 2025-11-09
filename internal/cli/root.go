package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/csrf"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/spf13/cobra"

	"github.com/seuros/kaunta/internal/config"
	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/geoip"
	"github.com/seuros/kaunta/internal/handlers"
	"github.com/seuros/kaunta/internal/middleware"
)

var Version string
var databaseURL string
var port string
var dataDir string

// RootCmd represents the root command
var RootCmd = &cobra.Command{
	Use:   "kaunta",
	Short: "Analytics without bloat",
	Long: `Kaunta - A lightweight analytics solution.

Kaunta is a privacy-focused visitor tracking solution with minimal resource usage.
It provides real-time analytics and a clean dashboard interface.`,
	Version: Version,
	// Load config from file/env/flags (runs before all commands)
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadWithOverrides(databaseURL, port, dataDir)
		if err != nil {
			log.Printf("Warning: failed to load config: %v", err)
			return
		}

		// Set environment variables from config (for backward compatibility)
		if cfg.DatabaseURL != "" {
			_ = os.Setenv("DATABASE_URL", cfg.DatabaseURL)
		}
		if cfg.Port != "" {
			_ = os.Setenv("PORT", cfg.Port)
		}
		if cfg.DataDir != "" {
			_ = os.Setenv("DATA_DIR", cfg.DataDir)
		}
	},
	// Default to serve command if no subcommand provided
	RunE: func(cmd *cobra.Command, args []string) error {
		// If no arguments provided, run serve
		if len(args) == 0 {
			return serveAnalytics(
				TrackerScript,
				VendorJS,
				VendorCSS,
				CountriesGeoJSON,
				DashboardTemplate,
				IndexTemplate,
			)
		}
		return cmd.Help()
	},
}

// Execute is called by main
func Execute(
	version string,
	trackerScript,
	vendorJS,
	vendorCSS,
	countriesGeoJSON,
	dashboardTemplate,
	indexTemplate []byte,
) error {
	Version = version
	TrackerScript = trackerScript
	VendorJS = vendorJS
	VendorCSS = vendorCSS
	CountriesGeoJSON = countriesGeoJSON
	DashboardTemplate = dashboardTemplate
	IndexTemplate = indexTemplate

	RootCmd.Version = version

	return RootCmd.Execute()
}

// Embedded assets passed from main
var (
	TrackerScript     []byte
	VendorJS          []byte
	VendorCSS         []byte
	CountriesGeoJSON  []byte
	DashboardTemplate []byte
	IndexTemplate     []byte
)

// serveAnalytics runs the Kaunta server
func serveAnalytics(
	trackerScript, vendorJS, vendorCSS, countriesGeoJSON, dashboardTemplate, indexTemplate []byte,
) error {
	// Get database URL
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	// Run migrations
	log.Println("Running database migrations...")
	if err := database.RunMigrations(databaseURL); err != nil {
		log.Printf("⚠️  Migration warning: %v", err)
	} else {
		log.Println("✓ Migrations completed")
	}

	// Connect to database
	if err := database.Connect(); err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
	}()

	// Initialize GeoIP database (downloads if missing)
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	if err := geoip.Init(dataDir); err != nil {
		log.Fatalf("GeoIP initialization failed: %v", err)
	}
	defer func() {
		if err := geoip.Close(); err != nil {
			log.Printf("Error closing GeoIP: %v", err)
		}
	}()

	// Create Fiber app
	appName := "Kaunta - Analytics without bloat"
	if Version != "" {
		appName = fmt.Sprintf("Kaunta v%s - Analytics without bloat", Version)
	}
	app := fiber.New(fiber.Config{
		AppName: appName,
	})

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, X-CSRF-Token",
		AllowMethods: "GET, POST, OPTIONS",
	}))

	// Add version header to all responses
	app.Use(func(c *fiber.Ctx) error {
		c.Set("X-Kaunta-Version", Version)
		return c.Next()
	})

	// CSRF protection middleware
	app.Use(csrf.New(csrf.Config{
		KeyLookup:      "header:X-CSRF-Token",
		CookieName:     "kaunta_csrf",
		CookieSameSite: "Lax",
		CookieHTTPOnly: true,
		CookieSecure:   false, // Will be set dynamically based on protocol
		Expiration:     7 * 24 * time.Hour,
		// Skip CSRF protection for tracking endpoint (public API)
		Next: func(c *fiber.Ctx) bool {
			return c.Path() == "/api/send"
		},
	}))

	// Static assets - serve embedded JS/CSS files
	app.Get("/assets/vendor/:filename<*>", func(c *fiber.Ctx) error {
		filename := c.Params("filename")
		// Strip query string if present
		if idx := strings.Index(filename, "?"); idx > -1 {
			filename = filename[:idx]
		}

		c.Set("Cache-Control", "public, max-age=31536000, immutable")
		c.Set("CF-Cache-Tag", "kaunta-assets")

		switch filename {
		case "vendor.js":
			c.Set("Content-Type", "application/javascript; charset=utf-8")
			return c.Send(vendorJS)
		case "vendor.css":
			c.Set("Content-Type", "text/css; charset=utf-8")
			return c.Send(vendorCSS)
		default:
			return c.Status(404).SendString("Not found")
		}
	})

	// Static data files
	app.Get("/assets/data/:filename<*>", func(c *fiber.Ctx) error {
		filename := c.Params("filename")
		// Strip query string if present
		if idx := strings.Index(filename, "?"); idx > -1 {
			filename = filename[:idx]
		}

		c.Set("Cache-Control", "public, max-age=86400, immutable")
		c.Set("CF-Cache-Tag", "kaunta-data")

		switch filename {
		case "countries-110m.json":
			c.Set("Content-Type", "application/json; charset=utf-8")
			return c.Send(countriesGeoJSON)
		default:
			return c.Status(404).SendString("Not found")
		}
	})

	// Routes
	app.Get("/", handleIndex(indexTemplate))
	app.Get("/health", handleHealth)
	app.Get("/up", handleUp) // Docker health check
	app.Get("/api/version", handleVersion)

	// Tracker script
	app.Get("/k.js", handleTrackerScript(trackerScript))
	app.Get("/kaunta.js", handleTrackerScript(trackerScript)) // Long form
	app.Get("/script.js", handleTrackerScript(trackerScript)) // Umami-compatible alias

	// Tracking API (Umami-compatible)
	app.Options("/api/send", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	app.Post("/api/send", handlers.HandleTracking)

	// Stats API (Plausible-inspired)
	app.Get("/api/stats/realtime/:website_id", handlers.HandleCurrentVisitors)

	// Auth API endpoints (public)
	// CSRF token endpoint
	app.Get("/api/auth/csrf", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"token": c.Locals("csrf").(string),
		})
	})

	// Rate limiter for login endpoint (5 requests per minute per IP)
	loginLimiter := limiter.New(limiter.Config{
		Max:        5,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"success": false,
				"error":   "Too many login attempts. Please try again later.",
			})
		},
	})

	app.Post("/api/auth/login", loginLimiter, handlers.HandleLogin)

	// Login page (public)
	app.Get("/login", func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.SendString(loginPageHTML())
	})

	// Dashboard UI (protected)
	app.Get("/dashboard", middleware.Auth, func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/html; charset=utf-8")
		// Replace Go template variables in embedded HTML
		html := strings.ReplaceAll(string(dashboardTemplate), "{{.Title}}", "Kaunta Dashboard")
		html = strings.ReplaceAll(html, "{{.Version}}", Version)
		return c.SendString(html)
	})

	// Protected API endpoints
	app.Post("/api/auth/logout", middleware.Auth, handlers.HandleLogout)
	app.Get("/api/auth/me", middleware.Auth, handlers.HandleMe)

	// Dashboard API endpoints (protected)
	app.Get("/api/websites", middleware.Auth, handlers.HandleWebsites)
	app.Get("/api/dashboard/stats/:website_id", middleware.Auth, handlers.HandleDashboardStats)
	app.Get("/api/dashboard/pages/:website_id", middleware.Auth, handlers.HandleTopPages)
	app.Get("/api/dashboard/timeseries/:website_id", middleware.Auth, handlers.HandleTimeSeries)
	app.Get("/api/dashboard/referrers/:website_id", middleware.Auth, handlers.HandleTopReferrers)
	app.Get("/api/dashboard/browsers/:website_id", middleware.Auth, handlers.HandleTopBrowsers)
	app.Get("/api/dashboard/devices/:website_id", middleware.Auth, handlers.HandleTopDevices)
	app.Get("/api/dashboard/countries/:website_id", middleware.Auth, handlers.HandleTopCountries)
	app.Get("/api/dashboard/cities/:website_id", middleware.Auth, handlers.HandleTopCities)
	app.Get("/api/dashboard/regions/:website_id", middleware.Auth, handlers.HandleTopRegions)
	app.Get("/api/dashboard/map/:website_id", middleware.Auth, handlers.HandleMapData)

	// Start server
	port := getEnv("PORT", "3000")
	log.Printf("Kaunta starting on port %s", port)
	log.Fatal(app.Listen(":" + port))

	return nil
}

// Handler functions

func handleIndex(indexTemplate []byte) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.Send(indexTemplate)
	}
}

func handleHealth(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":  "healthy",
		"service": "kaunta",
		"react":   false,
		"bloat":   false,
	})
}

var pingDatabase = func() error {
	if database.DB == nil {
		return fmt.Errorf("database connection not initialized")
	}
	return database.DB.Ping()
}

func handleUp(c *fiber.Ctx) error {
	// Simple Docker health check endpoint
	// Returns 200 OK if server is running and can connect to database
	if err := pingDatabase(); err != nil {
		return c.Status(503).SendString("database unavailable")
	}
	return c.SendStatus(200)
}

func handleVersion(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"version": Version,
	})
}

func handleTrackerScript(trackerScript []byte) fiber.Handler {
	// Compute ETag once from actual content hash
	hash := sha256.Sum256(trackerScript)
	etag := "\"" + hex.EncodeToString(hash[:8]) + "\""

	return func(c *fiber.Ctx) error {
		// Security headers
		c.Set("Content-Type", "application/javascript; charset=utf-8")
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")

		// Cache headers (1 hour)
		c.Set("Cache-Control", "public, max-age=3600, immutable")
		c.Set("ETag", etag)

		// CORS headers - allow from anywhere (JS file is public)
		// Origin validation happens at /api/send endpoint
		c.Set("Access-Control-Allow-Origin", "*")
		c.Set("Access-Control-Allow-Methods", "GET, OPTIONS")

		// Timing headers
		c.Set("Timing-Allow-Origin", "*")

		return c.Send(trackerScript)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// loginPageHTML returns a simple login page
func loginPageHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Login - Kaunta</title>
    <style>
      :root {
        --bg-primary: #ffffff;
        --bg-secondary: #f8f9fa;
        --bg-accent: #e6f0ff;
        --bg-glass: rgba(230, 240, 255, 0.7);
        --text-primary: #1a1a1a;
        --text-secondary: #4b5e7a;
        --text-tertiary: #7b8aa6;
        --border-color: #d6e2ff;
        --accent-color: #3b82f6;
        --accent-dark: #1e3a8a;
      }

      * {
        margin: 0;
        padding: 0;
        box-sizing: border-box;
      }

      body {
        font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "Roboto",
          sans-serif;
        background: var(--bg-primary);
        color: var(--text-primary);
        line-height: 1.6;
        min-height: 100vh;
        display: flex;
        flex-direction: column;
      }

      .container {
        max-width: 1440px;
        margin: 0 auto;
        padding: 32px;
        flex: 1;
        display: flex;
        flex-direction: column;
        justify-content: center;
        align-items: center;
        text-align: center;
      }

      .main-content {
        flex: 1;
        display: flex;
        flex-direction: column;
        justify-content: center;
        align-items: center;
      }

      .glass {
        background: var(--bg-glass);
        backdrop-filter: blur(10px);
        border: 1px solid var(--border-color);
      }

      .card {
        padding: 32px;
        border-radius: 16px;
        box-shadow: 0 8px 32px rgba(59, 130, 246, 0.08);
      }

      .login-card {
        max-width: 400px;
        width: 100%;
        margin-bottom: 48px;
      }

      .hero {
        margin-bottom: 32px;
      }

      .hero h1 {
        font-size: 48px;
        font-weight: 500;
        color: var(--accent-dark);
        margin-bottom: 16px;
      }

      .hero .subtitle {
        font-size: 20px;
        color: var(--text-secondary);
        font-weight: 400;
      }

      .login-form h2 {
        font-size: 24px;
        font-weight: 500;
        color: var(--accent-dark);
        margin-bottom: 24px;
        text-align: center;
      }

      .form-group {
        margin-bottom: 20px;
        text-align: left;
      }

      label {
        display: block;
        margin-bottom: 8px;
        color: var(--text-secondary);
        font-weight: 500;
        font-size: 14px;
      }

      input {
        width: 100%;
        padding: 12px 16px;
        border: 1px solid var(--border-color);
        border-radius: 12px;
        font-size: 14px;
        color: var(--text-primary);
        background: var(--bg-glass);
        backdrop-filter: blur(10px);
        transition: all 0.2s ease;
      }

      input:focus {
        outline: none;
        border-color: var(--accent-color);
        box-shadow: 0 0 0 4px rgba(59, 130, 246, 0.25);
        background: rgba(230, 240, 255, 0.95);
      }

      .btn {
        width: 100%;
        padding: 12px 24px;
        border-radius: 12px;
        text-decoration: none;
        font-weight: 500;
        transition: all 0.2s ease;
        display: inline-flex;
        align-items: center;
        justify-content: center;
        gap: 8px;
        border: none;
        cursor: pointer;
        font-size: 14px;
      }

      .btn-primary {
        background: var(--accent-color);
        color: white;
        border: 1px solid var(--accent-color);
      }

      .btn-primary:hover:not(:disabled) {
        background: var(--accent-dark);
        transform: translateY(-1px);
        box-shadow: 0 4px 16px rgba(59, 130, 246, 0.3);
      }

      .btn-primary:disabled {
        background: var(--text-tertiary);
        border-color: var(--text-tertiary);
        cursor: not-allowed;
        transform: none;
        box-shadow: none;
      }

      .error {
        background: rgba(239, 68, 68, 0.1);
        color: #dc2626;
        padding: 12px 16px;
        border-radius: 12px;
        margin-bottom: 20px;
        display: none;
        border: 1px solid rgba(239, 68, 68, 0.2);
        font-size: 14px;
      }

      .error.show {
        display: block;
      }

      .footer {
        margin-top: auto;
        padding: 24px;
        background: var(--bg-glass);
        backdrop-filter: blur(10px);
        border: 1px solid var(--border-color);
        border-radius: 16px;
        width: 100%;
        max-width: 800px;
      }

      .footer p {
        color: var(--text-tertiary);
        font-size: 14px;
      }

      /* Responsive */
      @media (max-width: 768px) {
        .container {
          padding: 20px;
        }

        .hero h1 {
          font-size: 36px;
        }

        .hero .subtitle {
          font-size: 18px;
        }

        .login-card {
          padding: 24px;
        }
      }
    </style>
  </head>
  <body>
    <div class="container">
      <div class="main-content">
        <div class="hero">
          <h1>Kaunta</h1>
          <p class="subtitle">Analytics without bloat</p>
        </div>

        <div class="login-card glass card">
          <h2>Login to Dashboard</h2>
          <div id="error" class="error"></div>
          <form id="loginForm">
            <div class="form-group">
              <label for="username">Username</label>
              <input type="text" id="username" name="username" required autocomplete="username">
            </div>
            <div class="form-group">
              <label for="password">Password</label>
              <input type="password" id="password" name="password" required autocomplete="current-password">
            </div>
            <button type="submit" id="submitBtn" class="btn btn-primary">
              <span>🔐</span>
              Login
            </button>
          </form>
        </div>
      </div>

      <div class="footer">
        <p>
          <strong>Kaunta Analytics</strong> - Built with Go, Fiber, Alpine.js,
          PostgreSQL, and Leaflet
        </p>
        <p>
          <a
            href="https://github.com/seuros/kaunta"
            style="color: var(--accent-color); text-decoration: none"
            >View on GitHub</a
          >
        </p>
      </div>
    </div>

    <script>
      const form = document.getElementById('loginForm');
      const errorDiv = document.getElementById('error');
      const submitBtn = document.getElementById('submitBtn');
      let csrfToken = '';

      // Fetch CSRF token on page load
      async function fetchCSRFToken() {
        try {
          const response = await fetch('/api/auth/csrf');
          const data = await response.json();
          csrfToken = data.token;
        } catch (error) {
          console.error('Failed to fetch CSRF token:', error);
          errorDiv.textContent = 'Security initialization failed. Please reload the page.';
          errorDiv.classList.add('show');
          submitBtn.disabled = true;
        }
      }

      // Initialize CSRF token
      fetchCSRFToken();

      form.addEventListener('submit', async (e) => {
        e.preventDefault();

        const username = document.getElementById('username').value;
        const password = document.getElementById('password').value;

        errorDiv.classList.remove('show');
        submitBtn.disabled = true;
        submitBtn.innerHTML = '<span>⏳</span> Logging in...';

        try {
          const response = await fetch('/api/auth/login', {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'X-CSRF-Token': csrfToken,
            },
            body: JSON.stringify({ username, password }),
          });

          const data = await response.json();

          if (response.ok && data.success) {
            window.location.href = '/dashboard';
          } else {
            errorDiv.textContent = data.error || 'Login failed';
            errorDiv.classList.add('show');
          }
        } catch (error) {
          errorDiv.textContent = 'Network error. Please try again.';
          errorDiv.classList.add('show');
        } finally {
          submitBtn.disabled = false;
          submitBtn.innerHTML = '<span>🔐</span> Login';
        }
      });
    </script>
  </body>
</html>`
}

func init() {
	// Global flags available to all commands
	RootCmd.PersistentFlags().StringVar(&databaseURL, "database-url", "", "PostgreSQL connection URL (overrides config file and env)")
	RootCmd.PersistentFlags().StringVar(&port, "port", "", "Server port (overrides config file and env)")
	RootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", "", "Data directory for GeoIP database (overrides config file and env)")

	// Add subcommands
	RootCmd.AddCommand(serveCmd)
	RootCmd.AddCommand(websiteCmd)
	RootCmd.AddCommand(statsCmd)
	// DevOps commands added in devops.go init()

	setupSelfUpgrade()

	// Set version output
	RootCmd.Version = Version
}
