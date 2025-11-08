package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/spf13/cobra"

	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/geoip"
	"github.com/seuros/kaunta/internal/handlers"
	"github.com/seuros/kaunta/internal/middleware"
)

var Version string

// RootCmd represents the root command
var RootCmd = &cobra.Command{
	Use:   "kaunta",
	Short: "Analytics without bloat",
	Long: `Kaunta - A lightweight analytics solution.

Kaunta is a privacy-focused visitor tracking solution with minimal resource usage.
It provides real-time analytics and a clean dashboard interface.`,
	Version: Version,
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
		AllowHeaders: "Origin, Content-Type, Accept",
		AllowMethods: "GET, POST, OPTIONS",
	}))

	// Add version header to all responses
	app.Use(func(c *fiber.Ctx) error {
		c.Set("X-Kaunta-Version", Version)
		return c.Next()
	})

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
	app.Post("/api/auth/login", handlers.HandleLogin)

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

func handleUp(c *fiber.Ctx) error {
	// Simple Docker health check endpoint
	// Returns 200 OK if server is running and can connect to database
	if err := database.DB.Ping(); err != nil {
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
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Login - Kaunta</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .container {
            background: white;
            padding: 2rem;
            border-radius: 8px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.2);
            width: 100%;
            max-width: 400px;
        }
        h1 {
            margin-bottom: 1.5rem;
            color: #333;
            font-size: 1.8rem;
        }
        .form-group {
            margin-bottom: 1rem;
        }
        label {
            display: block;
            margin-bottom: 0.5rem;
            color: #555;
            font-weight: 500;
        }
        input {
            width: 100%;
            padding: 0.75rem;
            border: 1px solid #ddd;
            border-radius: 4px;
            font-size: 1rem;
        }
        input:focus {
            outline: none;
            border-color: #667eea;
        }
        button {
            width: 100%;
            padding: 0.75rem;
            background: #667eea;
            color: white;
            border: none;
            border-radius: 4px;
            font-size: 1rem;
            font-weight: 600;
            cursor: pointer;
            transition: background 0.2s;
        }
        button:hover {
            background: #5568d3;
        }
        button:disabled {
            background: #ccc;
            cursor: not-allowed;
        }
        .error {
            background: #fee;
            color: #c33;
            padding: 0.75rem;
            border-radius: 4px;
            margin-bottom: 1rem;
            display: none;
        }
        .error.show {
            display: block;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Login to Kaunta</h1>
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
            <button type="submit" id="submitBtn">Login</button>
        </form>
    </div>

    <script>
        const form = document.getElementById('loginForm');
        const errorDiv = document.getElementById('error');
        const submitBtn = document.getElementById('submitBtn');

        form.addEventListener('submit', async (e) => {
            e.preventDefault();

            const username = document.getElementById('username').value;
            const password = document.getElementById('password').value;

            errorDiv.classList.remove('show');
            submitBtn.disabled = true;
            submitBtn.textContent = 'Logging in...';

            try {
                const response = await fetch('/api/auth/login', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
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
                submitBtn.textContent = 'Login';
            }
        });
    </script>
</body>
</html>`
}

func init() {
	// Add subcommands
	RootCmd.AddCommand(serveCmd)
	RootCmd.AddCommand(websiteCmd)
	RootCmd.AddCommand(statsCmd)
	// DevOps commands added in devops.go init()

	setupSelfUpgrade()

	// Set version output
	RootCmd.Version = Version
}
