package cli

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
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
	"github.com/gofiber/template/html/v2"
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
				ViewsFS,
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
	countriesGeoJSON []byte,
	viewsFS interface{},
	setupTemplate,
	setupCompleteTemplate []byte,
) error {
	Version = version
	AssetsFS = assetsFS
	TrackerScript = trackerScript
	VendorJS = vendorJS
	VendorCSS = vendorCSS
	CountriesGeoJSON = countriesGeoJSON
	ViewsFS = viewsFS
	SetupTemplate = setupTemplate
	SetupCompleteTemplate = setupCompleteTemplate

	RootCmd.Version = version

	// Hide self-upgrade flags in dev builds (after Version is set)
	hideSelfUpgradeFlagsIfDevBuild()

	return RootCmd.Execute()
}

// ErrSetupComplete signals that setup wizard completed successfully
var ErrSetupComplete = fmt.Errorf("setup complete")

// Embedded assets passed from main
var (
	AssetsFS              interface{} // embed.FS
	TrackerScript         []byte
	VendorJS              []byte
	VendorCSS             []byte
	CountriesGeoJSON      []byte
	ViewsFS               interface{} // embed.FS for template views
	SetupTemplate         []byte
	SetupCompleteTemplate []byte
)

// serveAnalytics runs the Kaunta server
func serveAnalytics(
	assetsFS interface{},
	trackerScript, vendorJS, vendorCSS, countriesGeoJSON []byte,
	viewsFS interface{},
) error {
	// Ensure logger is flushed on exit
	defer func() {
		_ = logging.Sync() // Ignore sync errors on stderr (expected)
	}()

	// Check setup status (loop to allow restart after setup)
	for {
		setupStatus, err := config.CheckSetupStatus()
		if err != nil {
			logging.L().Error("failed to check setup status", zap.Error(err))
		}

		if setupStatus != nil && setupStatus.NeedsSetup {
			// Run setup wizard
			logging.L().Info("setup required", zap.String("reason", setupStatus.Reason))
			if err := runSetupServer(); err != nil && err != ErrSetupComplete {
				return err
			}

			// Reload configuration from saved file and update environment
			logging.L().Info("setup completed, reloading configuration")
			cfg, err := config.Load()
			if err != nil {
				logging.L().Error("failed to reload config after setup", zap.Error(err))
				return fmt.Errorf("failed to reload config after setup: %w", err)
			}

			// Update environment variables from newly saved config
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

			logging.L().Info("configuration reloaded, restarting server")
			continue
		}
		break
	}

	// Normal server startup - setup is complete
	logging.L().Info("starting normal server")

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

	// Ensure self website exists for dogfooding (creates if missing for existing installations)
	ensureSelfWebsite()

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

	// Initialize HTML template engine
	viewsEmbedFS, ok := viewsFS.(embed.FS)
	if !ok {
		logging.Fatal("viewsFS is not embed.FS")
	}
	// Convert embed.FS to http.FileSystem using http.FS
	httpFS := http.FS(viewsEmbedFS)
	engine := html.NewFileSystem(httpFS, ".html")

	// Create Fiber app
	appName := "Kaunta - Analytics without bloat"
	if Version != "" {
		appName = fmt.Sprintf("Kaunta v%s - Analytics without bloat", Version)
	}
	app := fiber.New(createFiberConfig(appName, engine))

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

	// Transform domain strings to full URLs for CSRF middleware and validate format
	trustedOriginURLs := make([]string, 0, len(trustedOrigins))
	for _, domain := range trustedOrigins {
		if normalized, ok := normalizeOriginForCSRF(domain); ok {
			trustedOriginURLs = append(trustedOriginURLs, normalized)
		} else {
			logging.L().Warn("skipping invalid trusted origin", zap.String("origin", domain))
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
		// Skip CSRF protection for public endpoints and static assets
		Next: func(c fiber.Ctx) bool {
			// Skip for tracking API endpoint
			if c.Path() == "/api/send" {
				return true
			}
			// Skip for GET requests to static assets (JS, CSS)
			if c.Method() == "GET" {
				path := c.Path()
				if strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".css") {
					return true
				}
			}
			return false
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
	app.Get("/", func(c fiber.Ctx) error {
		return c.Render("views/index", fiber.Map{
			"Title": "Kaunta - Analytics without bloat",
		}, "views/layouts/base")
	})
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

	// Pixel tracking (for email, RSS, no-JS environments)
	app.Get("/p/:id.gif", handlers.HandlePixelTracking)

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
		return c.Render("views/login", fiber.Map{
			"Title": "Login - Kaunta",
		}, "views/layouts/base")
	})

	// Dashboard UI (protected)
	app.Get("/dashboard", middleware.AuthWithRedirect, func(c fiber.Ctx) error {
		return c.Render("views/dashboard/home", fiber.Map{
			"Title":         "Dashboard",
			"Version":       Version,
			"SelfWebsiteID": config.SelfWebsiteID,
		}, "views/layouts/dashboard")
	})

	// Map UI (protected)
	app.Get("/dashboard/map", middleware.AuthWithRedirect, func(c fiber.Ctx) error {
		return c.Render("views/dashboard/map", fiber.Map{
			"Title":         "Map",
			"Version":       Version,
			"SelfWebsiteID": config.SelfWebsiteID,
		})
	})

	// Campaigns UI (protected)
	app.Get("/dashboard/campaigns", middleware.AuthWithRedirect, func(c fiber.Ctx) error {
		return c.Render("views/dashboard/campaigns", fiber.Map{
			"Title":         "Campaigns",
			"Version":       Version,
			"SelfWebsiteID": config.SelfWebsiteID,
		})
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
	app.Get("/api/dashboard/os/:website_id", middleware.Auth, handlers.HandleTopOS)
	app.Get("/api/dashboard/countries/:website_id", middleware.Auth, handlers.HandleTopCountries)
	app.Get("/api/dashboard/cities/:website_id", middleware.Auth, handlers.HandleTopCities)
	app.Get("/api/dashboard/regions/:website_id", middleware.Auth, handlers.HandleTopRegions)
	app.Get("/api/dashboard/map/:website_id", middleware.Auth, handlers.HandleMapData)

	// UTM Campaign Parameter endpoints (protected)
	app.Get("/api/dashboard/utm-source/:website_id", middleware.Auth, handlers.HandleUTMSource)
	app.Get("/api/dashboard/utm-medium/:website_id", middleware.Auth, handlers.HandleUTMMedium)
	app.Get("/api/dashboard/utm-campaign/:website_id", middleware.Auth, handlers.HandleUTMCampaign)
	app.Get("/api/dashboard/utm-term/:website_id", middleware.Auth, handlers.HandleUTMTerm)
	app.Get("/api/dashboard/utm-content/:website_id", middleware.Auth, handlers.HandleUTMContent)

	// Entry/Exit Page endpoints (protected)
	app.Get("/api/dashboard/entry-pages/:website_id", middleware.Auth, handlers.HandleEntryPages)
	app.Get("/api/dashboard/exit-pages/:website_id", middleware.Auth, handlers.HandleExitPages)

	// Website Management API (protected)
	app.Get("/api/websites/list", middleware.Auth, handlers.HandleWebsiteList)
	app.Get("/api/websites/:website_id", middleware.Auth, handlers.HandleWebsiteShow)
	app.Post("/api/websites", middleware.Auth, handlers.HandleWebsiteCreate)
	app.Put("/api/websites/:website_id", middleware.Auth, handlers.HandleWebsiteUpdate)
	app.Post("/api/websites/:website_id/domains", middleware.Auth, handlers.HandleAddDomain)
	app.Delete("/api/websites/:website_id/domains", middleware.Auth, handlers.HandleRemoveDomain)

	// Website Management Dashboard page (protected)
	app.Get("/dashboard/websites", middleware.AuthWithRedirect, func(c fiber.Ctx) error {
		return c.Render("views/dashboard/websites", fiber.Map{
			"Title":         "Websites",
			"Version":       Version,
			"SelfWebsiteID": config.SelfWebsiteID,
		})
	})

	// Goals Management Dashboard page (proctected)
	app.Get("/dashboard/goals", middleware.AuthWithRedirect, func(c fiber.Ctx) error {
		return c.Render("views/dashboard/goals", fiber.Map{
			"Title":         "Goals",
			"Version":       Version,
			"SelfWebsiteID": config.SelfWebsiteID,
		})
	})

	// Goals API (protected)
	app.Get("/api/goals/:website_id", middleware.Auth, handlers.HandleGoalList)
	app.Post("/api/goals", middleware.Auth, handlers.HandleGoalCreate)
	app.Put("/api/goals/:id", middleware.Auth, handlers.HandleGoalUpdate)
	app.Delete("/api/goals/:id", middleware.Auth, handlers.HandleGoalDelete)

	// Start server
	port := getEnv("PORT", "3000")
	logging.L().Info("starting kaunta server", zap.String("port", port))
	if err := app.Listen(":" + port); err != nil {
		logging.Fatal("fiber server exited", zap.Error(err))
	}

	return nil
}

// Handler functions

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

// normalizeOriginForCSRF converts a domain (with or without scheme) into a full origin URL
// acceptable by Fiber's CSRF middleware. It rejects paths, queries, fragments, wildcards,
// and empty values.
func normalizeOriginForCSRF(domain string) (string, bool) {
	trimmed := strings.TrimSpace(domain)
	if trimmed == "" {
		return "", false
	}

	origin := trimmed
	if !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://") {
		// Use https:// for non-localhost domains (safer default for production)
		if strings.HasPrefix(origin, "localhost") || strings.HasPrefix(origin, "127.0.0.1") {
			origin = "http://" + origin
		} else {
			origin = "https://" + origin
		}
	}

	origin = strings.TrimSuffix(origin, "/")

	u, err := url.Parse(origin)
	if err != nil {
		return "", false
	}

	if u.Scheme == "" || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}

	if strings.Contains(u.Host, "*") {
		return "", false
	}

	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host), true
}

// ensureSelfWebsite creates or migrates the self-tracking website
// This handles existing installations that upgrade to a version with dogfooding support
func ensureSelfWebsite() {
	// Check if self website with correct UUID already exists
	var existsWithCorrectID bool
	err := database.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM website WHERE website_id = $1)`, config.SelfWebsiteID).Scan(&existsWithCorrectID)
	if err != nil {
		logging.L().Warn("failed to check for self website", zap.Error(err))
		return
	}

	if existsWithCorrectID {
		logging.L().Debug("self website already exists with correct ID")
		return
	}

	// Check if there's an existing self website with a different UUID (from older setup)
	var oldID string
	err = database.DB.QueryRow(`SELECT website_id FROM website WHERE domain = 'self' LIMIT 1`).Scan(&oldID)
	if err == nil && oldID != "" {
		// Migrate existing self website to use the standard nil UUID
		// Must update related tables first due to foreign key constraints
		tx, err := database.DB.Begin()
		if err != nil {
			logging.L().Warn("failed to start transaction for self website migration", zap.Error(err))
			return
		}
		defer func() { _ = tx.Rollback() }()

		// Update related sessions
		_, _ = tx.Exec(`UPDATE session SET website_id = $1 WHERE website_id = $2`, config.SelfWebsiteID, oldID)
		// Update related events
		_, _ = tx.Exec(`UPDATE website_event SET website_id = $1 WHERE website_id = $2`, config.SelfWebsiteID, oldID)
		// Update the website itself
		_, err = tx.Exec(`UPDATE website SET website_id = $1, updated_at = NOW() WHERE domain = 'self'`, config.SelfWebsiteID)
		if err != nil {
			logging.L().Warn("failed to migrate self website to nil UUID", zap.Error(err))
			return
		}

		if err = tx.Commit(); err != nil {
			logging.L().Warn("failed to commit self website migration", zap.Error(err))
		} else {
			logging.L().Info("migrated self website to standard UUID", zap.String("old_id", oldID), zap.String("new_id", config.SelfWebsiteID))
		}
		return
	}

	// Create new self website with default allowed domains
	allowedDomains := `["localhost", "http://localhost", "https://localhost"]`
	_, err = database.DB.Exec(`
		INSERT INTO website (website_id, domain, name, allowed_domains, created_at, updated_at)
		VALUES ($1, 'self', 'Kaunta Dashboard', $2::jsonb, NOW(), NOW())
		ON CONFLICT (website_id) DO NOTHING
	`, config.SelfWebsiteID, allowedDomains)
	if err != nil {
		logging.L().Warn("failed to create self website", zap.Error(err))
	} else {
		logging.L().Info("created self website for dogfooding", zap.String("website_id", config.SelfWebsiteID))
	}
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

// runSetupServer runs a minimal server for the setup wizard
func runSetupServer() error {
	logging.L().Info("starting setup wizard server")

	// Channel to signal setup completion
	setupDone := make(chan struct{})

	// Create minimal Fiber app for setup
	app := fiber.New(createFiberConfig("Kaunta Setup", nil))

	// Middleware
	app.Use(recover.New())
	app.Use(zapmiddleware.New(zapmiddleware.Config{
		Logger: logging.L(),
	}))

	// Rate limiter for setup endpoints (5 requests per minute per IP)
	setupLimiter := limiter.New(limiter.Config{
		Max:        5,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c fiber.Ctx) string {
			return c.IP()
		},
	})

	// Setup routes
	app.Get("/setup", handlers.ShowSetup(SetupTemplate))
	app.Post("/setup", setupLimiter, handlers.SubmitSetup(func() {
		// Signal setup completion after response is sent
		go func() {
			time.Sleep(500 * time.Millisecond) // Allow response to be sent
			close(setupDone)
		}()
	}))
	app.Post("/setup/test-db", setupLimiter, handlers.TestDatabase())
	app.Get("/setup/complete", func(c fiber.Ctx) error {
		return c.Type("html").Send(SetupCompleteTemplate)
	})

	// Redirect root to setup
	app.Get("/", func(c fiber.Ctx) error {
		return c.Redirect().To("/setup")
	})

	// Health check endpoint (for Docker healthcheck during setup)
	app.Get("/up", func(c fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Static assets (for favicon and CSS)
	app.Get("/assets/favicon.ico", func(c fiber.Ctx) error {
		data, err := fs.ReadFile(AssetsFS.(embed.FS), "assets/favicon.ico")
		if err != nil {
			return c.Status(404).SendString("Not found")
		}
		c.Set("Content-Type", "image/x-icon")
		return c.Send(data)
	})

	app.Get("/assets/global.css", func(c fiber.Ctx) error {
		data, err := fs.ReadFile(AssetsFS.(embed.FS), "assets/global.css")
		if err != nil {
			return c.Status(404).SendString("Not found")
		}
		c.Set("Content-Type", "text/css")
		c.Set("Cache-Control", "public, max-age=3600")
		return c.Send(data)
	})

	// Get port from config or environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	// Start server in goroutine
	addr := fmt.Sprintf(":%s", port)
	logging.L().Info("setup wizard available", zap.String("url", fmt.Sprintf("http://localhost:%s/setup", port)))

	go func() {
		if err := app.Listen(addr); err != nil {
			logging.L().Debug("setup server stopped", zap.Error(err))
		}
	}()

	// Wait for setup completion
	<-setupDone
	logging.L().Info("setup completed, shutting down setup server")

	// Graceful shutdown
	if err := app.ShutdownWithTimeout(5 * time.Second); err != nil {
		logging.L().Warn("error shutting down setup server", zap.Error(err))
	}

	return ErrSetupComplete
}
