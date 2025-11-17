package cli

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/contrib/v3/websocket"
	zapmiddleware "github.com/gofiber/contrib/v3/zap"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/extractors"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/gofiber/fiber/v3/middleware/healthcheck"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/static"
	"github.com/spf13/cobra"

	"github.com/seuros/kaunta/internal/config"
	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/geoip"
	"github.com/seuros/kaunta/internal/handlers"
	"github.com/seuros/kaunta/internal/logging"
	"github.com/seuros/kaunta/internal/middleware"
	"github.com/seuros/kaunta/internal/realtime"
	"go.uber.org/zap"
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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadWithOverrides(databaseURL, port, dataDir)
		if err != nil {
			logging.L().Warn("failed to load config overrides", zap.Error(err))
			return nil
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
		_ = os.Setenv("SECURE_COOKIES", strconv.FormatBool(cfg.SecureCookies))
		return nil
	},
	// Default to serve command if no subcommand provided
	RunE: func(cmd *cobra.Command, args []string) error {
		// If no arguments provided, run serve
		if len(args) == 0 {
			return serveAnalytics(
				AssetsFS,
				TrackerScript,
				VendorJS,
				VendorCSS,
				CountriesGeoJSON,
				DashboardTemplate,
				MapTemplate,
				IndexTemplate,
			)
		}
		return cmd.Help()
	},
}

// Execute is called by main
func Execute(
	version string,
	assetsFS interface{},
	trackerScript,
	vendorJS,
	vendorCSS,
	countriesGeoJSON,
	dashboardTemplate,
	mapTemplate,
	indexTemplate []byte,
) error {
	Version = version
	AssetsFS = assetsFS
	TrackerScript = trackerScript
	VendorJS = vendorJS
	VendorCSS = vendorCSS
	CountriesGeoJSON = countriesGeoJSON
	DashboardTemplate = dashboardTemplate
	MapTemplate = mapTemplate
	IndexTemplate = indexTemplate

	RootCmd.Version = version

	// Hide self-upgrade flags in dev builds (after Version is set)
	hideSelfUpgradeFlagsIfDevBuild()

	return RootCmd.Execute()
}

// Embedded assets passed from main
var (
	AssetsFS          interface{} // embed.FS
	TrackerScript     []byte
	VendorJS          []byte
	VendorCSS         []byte
	CountriesGeoJSON  []byte
	DashboardTemplate []byte
	MapTemplate       []byte
	IndexTemplate     []byte
)

// serveAnalytics runs the Kaunta server
func serveAnalytics(
	assetsFS interface{},
	trackerScript, vendorJS, vendorCSS, countriesGeoJSON, dashboardTemplate, mapTemplate, indexTemplate []byte,
) error {
	// Ensure logger is flushed on exit
	defer func() {
		_ = logging.Sync() // Ignore sync errors on stderr (expected)
	}()

	// Get database URL
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		logging.Fatal("DATABASE_URL environment variable is required")
	}

	// Run migrations
	logging.L().Info("running database migrations")
	if err := database.RunMigrations(databaseURL); err != nil {
		logging.L().Warn("migration warning", zap.Error(err))
	} else {
		logging.L().Info("migrations completed")
	}

	// Connect to database
	if err := database.Connect(); err != nil {
		logging.Fatal("database connection failed", zap.Error(err))
	}
	defer func() {
		if err := database.Close(); err != nil {
			logging.L().Warn("error closing database", zap.Error(err))
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	realtimeHub := realtime.NewHub()
	logging.L().Info("starting realtime websocket listener")
	if err := realtime.StartListener(ctx, databaseURL, realtimeHub); err != nil {
		logging.L().Error("failed to start realtime listener", zap.Error(err))
	} else {
		logging.L().Info("realtime websocket listener started successfully")
	}

	// Sync trusted origins from config to database
	cfg, err := config.Load()
	if err != nil {
		logging.L().Warn("failed to load config for trusted origins", zap.Error(err))
	} else if len(cfg.TrustedOrigins) > 0 {
		syncTrustedOrigins(cfg.TrustedOrigins)
	}

	// Initialize trusted origins cache from database
	logging.L().Info("initializing trusted origins cache")
	if err := middleware.InitTrustedOriginsCache(); err != nil {
		logging.L().Warn("failed to initialize trusted origins cache", zap.Error(err))
	}

	// Initialize GeoIP database (downloads if missing)
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	if err := geoip.Init(dataDir); err != nil {
		logging.Fatal("geoip initialization failed", zap.Error(err))
	}
	defer func() {
		if err := geoip.Close(); err != nil {
			logging.L().Warn("error closing geoip", zap.Error(err))
		}
	}()

	// Create Fiber app
	appName := "Kaunta - Analytics without bloat"
	if Version != "" {
		appName = fmt.Sprintf("Kaunta v%s - Analytics without bloat", Version)
	}
	app := fiber.New(createFiberConfig(appName))

	// Middleware
	app.Use(recover.New())
	app.Use(zapmiddleware.New(zapmiddleware.Config{
		Logger: logging.L(),
		Next: func(c fiber.Ctx) bool {
			path := c.Path()
			return path == "/up" || path == "/health" // Skip healthcheck logs
		},
	}))
	app.Use(cors.New(cors.Config{
		AllowOriginsFunc: func(origin string) bool {
			return true // Allow all origins
		},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "X-CSRF-Token"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowCredentials: true,
	}))

	// Add version header to all responses
	app.Use(func(c fiber.Ctx) error {
		c.Set("X-Kaunta-Version", Version)
		return c.Next()
	})

	// Realtime WebSocket endpoint
	app.Use("/ws/realtime", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	app.Get("/ws/realtime", realtimeHub.Handler())

	// CSRF protection middleware - use database-backed trusted origins
	// Get initial trusted origins from cache
	trustedOrigins, err := middleware.GetTrustedOrigins()
	if err != nil {
		logging.L().Warn("failed to get trusted origins", zap.Error(err))
		trustedOrigins = []string{} // Empty list if error
	}

	// Transform domain strings to full URLs for CSRF middleware
	trustedOriginURLs := make([]string, 0, len(trustedOrigins))
	for _, domain := range trustedOrigins {
		if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
			trustedOriginURLs = append(trustedOriginURLs, "http://"+domain)
		} else {
			trustedOriginURLs = append(trustedOriginURLs, domain)
		}
	}

	// Determine if we should use secure cookies (HTTPS required)
	secureEnabled := secureCookiesEnabled(cfg)

	app.Use(csrf.New(csrf.Config{
		Extractor:      extractors.FromHeader("X-CSRF-Token"),
		CookieName:     "kaunta_csrf",
		CookieSameSite: "Lax",         // Lax works for same-site requests
		CookieHTTPOnly: false,         // Must be false to allow JavaScript to read token
		CookieSecure:   secureEnabled, // Only enable for HTTPS deployments
		IdleTimeout:    7 * 24 * time.Hour,
		Session:        nil,               // Use cookie-based tokens, not session
		TrustedOrigins: trustedOriginURLs, // Loaded from database, transformed to URLs
		// Skip CSRF protection for tracking endpoint (public API)
		Next: func(c fiber.Ctx) bool {
			return c.Path() == "/api/send"
		},
	}))

	// Static assets - serve embedded JS/CSS files
	app.Get("/assets/vendor/:filename<*>", func(c fiber.Ctx) error {
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
	app.Get("/assets/data/:filename<*>", func(c fiber.Ctx) error {
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
	app.Get("/up", healthcheck.New(healthcheck.Config{
		Probe: func(c fiber.Ctx) bool {
			return pingDatabase() == nil
		},
	}))
	app.Get("/api/version", handleVersion)

	// Tracker script
	app.Get("/k.js", handleTrackerScript(trackerScript))
	app.Get("/kaunta.js", handleTrackerScript(trackerScript)) // Long form
	app.Get("/script.js", handleTrackerScript(trackerScript)) // Umami-compatible alias

	// Static assets (favicon, etc.) from embedded FS
	assetsSubFS, err := fs.Sub(assetsFS.(embed.FS), "assets")
	if err != nil {
		return fmt.Errorf("failed to create sub filesystem: %w", err)
	}
	app.Get("/assets/*", static.New("", static.Config{
		FS:            assetsSubFS,
		MaxAge:        31536000, // 1 year cache
		CacheDuration: 365 * 24 * time.Hour,
	}))
	// Serve favicon.ico from root
	app.Get("/favicon.ico", func(c fiber.Ctx) error {
		data, err := fs.ReadFile(assetsFS.(embed.FS), "assets/favicon.ico")
		if err != nil {
			return c.Status(404).SendString("Not found")
		}
		c.Set("Content-Type", "image/x-icon")
		c.Set("Cache-Control", "public, max-age=31536000, immutable")
		return c.Send(data)
	})

	// Tracking API (Umami-compatible)
	app.Options("/api/send", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	app.Post("/api/send", handlers.HandleTracking)

	// Stats API (Plausible-inspired) - protected
	app.Get("/api/stats/realtime/:website_id", middleware.Auth, handlers.HandleCurrentVisitors)

	// Auth API endpoints (public)
	// Rate limiter for login endpoint (5 requests per minute per IP)
	loginLimiter := limiter.New(limiter.Config{
		Max:        5,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"success": false,
				"error":   "Too many login attempts. Please try again later.",
			})
		},
	})

	app.Post("/api/auth/login", loginLimiter, handlers.HandleLogin)

	// Login page (public)
	app.Get("/login", func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.SendString(loginPageHTML())
	})

	// Dashboard UI (protected)
	app.Get("/dashboard", middleware.AuthWithRedirect, func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/html; charset=utf-8")
		// Replace Go template variables in embedded HTML
		html := strings.ReplaceAll(string(dashboardTemplate), "{{.Title}}", "Kaunta Dashboard")
		html = strings.ReplaceAll(html, "{{.Version}}", Version)
		return c.SendString(html)
	})

	// Map UI (protected)
	app.Get("/map", middleware.AuthWithRedirect, func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/html; charset=utf-8")
		// Replace Go template variables in embedded HTML
		html := strings.ReplaceAll(string(mapTemplate), "{{.Title}}", "Kaunta Map")
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
	logging.L().Info("starting kaunta server", zap.String("port", port))
	if err := app.Listen(":" + port); err != nil {
		logging.Fatal("fiber server exited", zap.Error(err))
	}

	return nil
}

// Handler functions

func handleIndex(indexTemplate []byte) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.Send(indexTemplate)
	}
}

func handleHealth(c fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":  "healthy",
		"service": "kaunta",
	})
}

var pingDatabase = func() error {
	if database.DB == nil {
		return fmt.Errorf("database connection not initialized")
	}
	return database.DB.Ping()
}

func handleVersion(c fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"version": Version,
	})
}

func handleTrackerScript(trackerScript []byte) fiber.Handler {
	// Compute ETag once from actual content hash
	hash := sha256.Sum256(trackerScript)
	etag := "\"" + hex.EncodeToString(hash[:8]) + "\""

	return func(c fiber.Ctx) error {
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

func secureCookiesEnabled(cfg *config.Config) bool {
	if cfg != nil {
		return cfg.SecureCookies
	}
	// Default to secure cookies unless explicitly disabled
	// This makes HTTPS reverse proxy setups work out of the box
	env := os.Getenv("SECURE_COOKIES")
	if env == "" {
		return true // Default to secure (safer for production)
	}
	return env == "true"
}

// loginPageHTML returns a simple login page with injected CSRF token
func loginPageHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Login - Kaunta</title>

    <link rel="icon" type="image/x-icon" href="/assets/favicon.ico" />

    <!-- Private page - not for indexing -->
    <meta name="robots" content="noindex, nofollow" />

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
        --gradient-primary: linear-gradient(135deg, var(--accent-color), var(--accent-dark));
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
        background: var(--gradient-primary);
        -webkit-background-clip: text;
        -webkit-text-fill-color: transparent;
        background-clip: text;
        margin-bottom: 16px;
      }

      h1 {
        background: var(--gradient-primary);
        -webkit-background-clip: text;
        -webkit-text-fill-color: transparent;
        background-clip: text;
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
        text-align: center;
        margin-bottom: 8px;
      }

      .footer .company-name {
        background: var(--gradient-primary);
        -webkit-background-clip: text;
        -webkit-text-fill-color: transparent;
        background-clip: text;
        font-weight: 600;
      }

      .footer .repo-link {
        color: var(--accent-color);
        text-decoration: none;
        font-weight: 500;
      }

      .footer .repo-link:hover {
        color: var(--accent-dark);
        text-decoration: underline;
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
          <div style="display: flex; justify-content: center; margin-bottom: 24px;">
            <img src="/assets/kaunta.svg" alt="Kaunta Analytics" style="height: 88px; width: auto;" />
          </div>
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
        <div style="display: flex; align-items: center; justify-content: center; gap: 12px; margin-bottom: 16px;">
          <img src="/assets/kaunta.svg" alt="Kaunta" style="height: 32px; width: auto;" />
          <span class="company-name">Kaunta Analytics</span>
        </div>
        <p>
          Built with Go, Fiber, Alpine.js, PostgreSQL, and Leaflet
        </p>
        <p>
          <a href="https://github.com/seuros/kaunta" class="repo-link">View on GitHub</a>
        </p>
      </div>
    </div>

    <script>
      const form = document.getElementById('loginForm');
      const errorDiv = document.getElementById('error');
      const submitBtn = document.getElementById('submitBtn');

      // Read CSRF token from cookie
      function getCsrfToken() {
        const value = '; ' + document.cookie;
        const parts = value.split('; kaunta_csrf=');
        if (parts.length === 2) return parts.pop().split(';').shift();
        return '';
      }
      const csrfToken = getCsrfToken();

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
            credentials: 'include',
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

// syncTrustedOrigins syncs trusted origins from config to database
func syncTrustedOrigins(origins []string) {
	logging.L().Info("syncing trusted origins from config", zap.Int("count", len(origins)))
	for _, origin := range origins {
		// Insert or update trusted origin (upsert)
		query := `
			INSERT INTO trusted_origin (domain, is_active, description)
			VALUES ($1, true, 'Auto-synced from TRUSTED_ORIGINS env var')
			ON CONFLICT (domain) DO UPDATE SET
				is_active = true,
				updated_at = NOW()
		`
		_, err := database.DB.Exec(query, origin)
		if err != nil {
			logging.L().Warn("failed to sync trusted origin", zap.String("origin", origin), zap.Error(err))
		} else {
			logging.L().Info("synced trusted origin", zap.String("origin", origin))
		}
	}
	logging.L().Info("finished syncing trusted origins")
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
