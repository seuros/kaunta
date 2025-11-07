package cli

import (
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
			)
		}
		return cmd.Help()
	},
}

// Execute is called by main
func Execute(version string, trackerScript, vendorJS, vendorCSS, countriesGeoJSON, dashboardTemplate []byte) error {
	Version = version
	TrackerScript = trackerScript
	VendorJS = vendorJS
	VendorCSS = vendorCSS
	CountriesGeoJSON = countriesGeoJSON
	DashboardTemplate = dashboardTemplate

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
)

// serveAnalytics runs the Kaunta server
func serveAnalytics(
	trackerScript, vendorJS, vendorCSS, countriesGeoJSON, dashboardTemplate []byte,
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
	app := fiber.New(fiber.Config{
		AppName: "Kaunta - Analytics without bloat",
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
	app.Get("/", handleIndex)
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

	// Dashboard UI (Alpine.js)
	app.Get("/dashboard", func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/html; charset=utf-8")
		// Replace Go template variables in embedded HTML
		html := strings.ReplaceAll(string(dashboardTemplate), "{{.Title}}", "Kaunta Dashboard")
		html = strings.ReplaceAll(html, "{{.Version}}", Version)
		return c.SendString(html)
	})

	// Dashboard API endpoints
	app.Get("/api/websites", handlers.HandleWebsites)
	app.Get("/api/dashboard/stats/:website_id", handlers.HandleDashboardStats)
	app.Get("/api/dashboard/pages/:website_id", handlers.HandleTopPages)
	app.Get("/api/dashboard/timeseries/:website_id", handlers.HandleTimeSeries)
	app.Get("/api/dashboard/referrers/:website_id", handlers.HandleTopReferrers)
	app.Get("/api/dashboard/browsers/:website_id", handlers.HandleTopBrowsers)
	app.Get("/api/dashboard/devices/:website_id", handlers.HandleTopDevices)
	app.Get("/api/dashboard/countries/:website_id", handlers.HandleTopCountries)
	app.Get("/api/dashboard/cities/:website_id", handlers.HandleTopCities)
	app.Get("/api/dashboard/regions/:website_id", handlers.HandleTopRegions)
	app.Get("/api/dashboard/map/:website_id", handlers.HandleMapData)

	// Start server
	port := getEnv("PORT", "3000")
	log.Printf("Kaunta starting on port %s", port)
	log.Fatal(app.Listen(":" + port))

	return nil
}

// Handler functions

func handleIndex(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(`
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Kaunta - Analytics without bloat</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
            max-width: 800px;
            margin: 100px auto;
            padding: 20px;
            line-height: 1.6;
        }
        h1 { color: #2c3e50; }
        .subtitle { color: #7f8c8d; font-style: italic; }
        .status {
            background: #fff3cd;
            border: 1px solid #ffc107;
            padding: 15px;
            border-radius: 5px;
            margin: 20px 0;
        }
    </style>
</head>
<body>
    <h1>Kaunta (カウンタ)</h1>
    <p class="subtitle">Analytics without bloat.</p>

    <div class="status">
        <strong>Status:</strong> Production<br>
        <strong>Memory:</strong> ~12MB (vs Umami's 200-500MB)<br>
        <strong>Size:</strong> Single Go binary (vs 300MB node_modules)<br>
        <strong>React:</strong> None (that's the point)
    </div>

    <h2>Features</h2>
    <ul>
        <li>Privacy-focused visitor tracking</li>
        <li>Minimal resource usage</li>
        <li>PostgreSQL backend</li>
        <li>Real-time analytics dashboard</li>
    </ul>

    <p style="text-align: center; margin-top: 40px;">
        <a href="https://github.com/seuros/kaunta">View on GitHub</a>
    </p>
</body>
</html>
	`)
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
	return func(c *fiber.Ctx) error {
		// Security headers
		c.Set("Content-Type", "application/javascript; charset=utf-8")
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")

		// Cache headers (1 hour)
		c.Set("Cache-Control", "public, max-age=3600, immutable")
		c.Set("ETag", "\"kaunta-v1.0.0\"")

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

func init() {
	// Add subcommands
	RootCmd.AddCommand(serveCmd)
	RootCmd.AddCommand(websiteCmd)
	RootCmd.AddCommand(statsCmd)
	// DevOps commands added in devops.go init()

	// Set version output
	RootCmd.Version = Version
}
