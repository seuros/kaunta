package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/spf13/cobra"

	"github.com/seuros/kaunta/internal/config"
	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/geoip"
	"github.com/seuros/kaunta/internal/handlers"
	"github.com/seuros/kaunta/internal/httpx"
	"github.com/seuros/kaunta/internal/logging"
	appmiddleware "github.com/seuros/kaunta/internal/middleware"
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

// render creates an isolated template instance for each request to prevent definition clashes.
func render(w http.ResponseWriter, pagePath, layoutPath string, data map[string]any) error {
	viewsEmbedFS, ok := ViewsFS.(embed.FS)
	if !ok {
		return fmt.Errorf("ViewsFS is not an embed.FS")
	}

	layoutFile := layoutPath + ".html"
	pageFile := pagePath + ".html"

	// Read layout and template from the embedded FS
	layoutBytes, err := viewsEmbedFS.ReadFile(layoutFile)
	if err != nil {
		return fmt.Errorf("could not read layout %s: %w", layoutFile, err)
	}
	pageBytes, err := viewsEmbedFS.ReadFile(pageFile)
	if err != nil {
		return fmt.Errorf("could not read page %s: %w", pageFile, err)
	}

	// Create a new template set for each request.
	// The name of the root template must match the layout's filename.
	tmpl, err := template.New(layoutFile).Parse(string(layoutBytes))
	if err != nil {
		return fmt.Errorf("could not parse layout %s: %w", layoutFile, err)
	}

	// Parse the page template into the same set.
	_, err = tmpl.Parse(string(pageBytes))
	if err != nil {
		return fmt.Errorf("could not parse page %s: %w", pageFile, err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.ExecuteTemplate(w, layoutFile, data)
}

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
	if err := appmiddleware.InitTrustedOriginsCache(); err != nil {
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

	r := chi.NewRouter()
	r.Use(chimiddleware.Recoverer)
	r.Use(requestLoggerMiddleware())
	r.Use(corsMiddleware())
	r.Use(addVersionHeader())

	// CSRF protection middleware - use database-backed trusted origins
	// Get initial trusted origins from cache
	trustedOrigins, err := appmiddleware.GetTrustedOrigins()
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

	r.Use(csrfMiddleware(csrfOptions{
		Secure:        secureEnabled,
		TrustedOrigin: trustedOriginURLs,
	}))

	// Static assets - serve embedded JS/CSS files
	r.Get("/ws/realtime", realtimeHub.Handler())
	r.Get("/assets/vendor/{filename:.+}", vendorAssetHandler(vendorJS, vendorCSS))
	r.Get("/assets/data/{filename:.+}", staticDataHandler(countriesGeoJSON))

	// Routes
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		if err := render(w, "views/index", "views/layouts/base", map[string]any{
			"Title": "Kaunta - Analytics without bloat",
		}); err != nil {
			http.Error(w, "Failed to render index", http.StatusInternalServerError)
		}
	})
	r.Get("/health", handleHealth)
	r.Get("/up", upHandler)
	r.Get("/api/version", handleVersion)

	// Tracker script
	trackerHandler := handleTrackerScript(trackerScript)
	r.Handle("/k.js", trackerHandler)
	r.Handle("/kaunta.js", trackerHandler)
	r.Handle("/script.js", trackerHandler)

	// Static assets (favicon, etc.) from embedded FS
	assetsSubFS, err := fs.Sub(assetsFS.(embed.FS), "assets")
	if err != nil {
		return fmt.Errorf("failed to create sub filesystem: %w", err)
	}
	fileServer := http.StripPrefix("/assets", http.FileServer(http.FS(assetsSubFS)))
	r.Get("/assets/*", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		fileServer.ServeHTTP(w, req)
	})
	r.Get("/favicon.ico", func(w http.ResponseWriter, req *http.Request) {
		data, err := fs.ReadFile(assetsFS.(embed.FS), "assets/favicon.ico")
		if err != nil {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "image/x-icon")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		_, _ = w.Write(data)
	})

	// Tracking API (Umami-compatible)
	r.Options("/api/send", optionsOK)
	r.Post("/api/send", handlers.HandleTracking)

	// Pixel tracking (for email, RSS, no-JS environments)
	r.Get("/p/{id}.gif", handlers.HandlePixelTracking)

	// Server-side Ingest API (for backend event ingestion)
	// Uses API key authentication instead of session-based auth
	r.Options("/api/ingest", optionsOK)
	r.Options("/api/ingest/batch", optionsOK)
	r.With(appmiddleware.APIKeyAuth).Post("/api/ingest", handlers.HandleIngest)
	r.With(appmiddleware.APIKeyAuth).Post("/api/ingest/batch", handlers.HandleIngestBatch)

	// Stats API (Plausible-inspired) - protected
	r.With(appmiddleware.Auth).Get("/api/stats/realtime/{website_id}", handlers.HandleCurrentVisitors)

	// Auth API endpoints (public)
	// Rate limiter for login endpoint (5 requests per minute per IP)
	loginLimiter := rateLimitMiddleware(rateLimitConfig{
		Limit:  5,
		Window: time.Minute,
		KeyFunc: func(r *http.Request) string {
			return httpx.ClientIP(r)
		},
		OnLimit: func(w http.ResponseWriter, r *http.Request) {
			httpx.WriteJSON(w, http.StatusTooManyRequests, map[string]any{
				"success": false,
				"error":   "Too many login attempts. Please try again later.",
			})
		},
	})
	r.With(loginLimiter).Post("/api/auth/login", handlers.HandleLogin)
	r.With(loginLimiter).Get("/api/auth/login", handlers.HandleLoginSSE)

	// Login page (public)
	r.Get("/login", func(w http.ResponseWriter, r *http.Request) {
		if err := render(w, "views/login", "views/layouts/base", map[string]any{
			"Title": "Login - Kaunta",
		}); err != nil {
			http.Error(w, "Failed to render login view", http.StatusInternalServerError)
		}
	})

	// Dashboard UI (protected)
	r.With(appmiddleware.AuthWithRedirect).Get("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		if err := render(w, "views/dashboard/home", "views/layouts/dashboard", map[string]any{
			"Title":         "Dashboard",
			"Version":       Version,
			"SelfWebsiteID": config.SelfWebsiteID,
		}); err != nil {
			http.Error(w, "Failed to render dashboard", http.StatusInternalServerError)
		}
	})

	// Map UI (protected)
	r.With(appmiddleware.AuthWithRedirect).Get("/dashboard/map", func(w http.ResponseWriter, r *http.Request) {
		if err := render(w, "views/dashboard/map", "views/layouts/dashboard", map[string]any{
			"Title":         "Map",
			"Version":       Version,
			"SelfWebsiteID": config.SelfWebsiteID,
		}); err != nil {
			http.Error(w, "Failed to render map view", http.StatusInternalServerError)
		}
	})

	// Campaigns UI (protected)
	r.With(appmiddleware.AuthWithRedirect).Get("/dashboard/campaigns", func(w http.ResponseWriter, r *http.Request) {
		if err := render(w, "views/dashboard/campaigns", "views/layouts/dashboard", map[string]any{
			"Title":         "Campaigns",
			"Version":       Version,
			"SelfWebsiteID": config.SelfWebsiteID,
		}); err != nil {
			http.Error(w, "Failed to render campaigns view", http.StatusInternalServerError)
		}
	})

	// Protected API endpoints
	authProtected := r.With(appmiddleware.Auth)
	authProtected.Post("/api/auth/logout", handlers.HandleLogoutSSE)
	authProtected.Get("/api/auth/me", handlers.HandleMe)

	// Dashboard API endpoints (protected, SSE-based)
	authProtected.Get("/api/websites", handlers.HandleWebsites)
	authProtected.Get("/api/dashboard/init", handlers.HandleDashboardInit)
	authProtected.Get("/api/dashboard/stats", handlers.HandleDashboardStats)
	authProtected.Get("/api/dashboard/timeseries", handlers.HandleTimeSeries)
	authProtected.Get("/api/dashboard/chart", handlers.HandleTimeSeries)
	authProtected.Get("/api/dashboard/breakdown", handlers.HandleBreakdown)
	authProtected.Get("/api/dashboard/map", handlers.HandleMapData)
	authProtected.Get("/api/dashboard/realtime", handlers.HandleRealtimeVisitors)
	authProtected.Get("/api/dashboard/campaigns-init", handlers.HandleCampaignsInit)
	authProtected.Get("/api/dashboard/campaigns", handlers.HandleCampaigns)
	authProtected.Get("/api/dashboard/websites-init", handlers.HandleWebsitesInit)
	authProtected.Post("/api/dashboard/websites-create", handlers.HandleWebsitesCreate)
	authProtected.Get("/api/dashboard/map-init", handlers.HandleMapInit)
	authProtected.Get("/api/dashboard/goals", handlers.HandleGoals)
	authProtected.Post("/api/dashboard/goals", handlers.HandleGoalsCreate)
	authProtected.Put("/api/dashboard/goals/{id}", handlers.HandleGoalsUpdate)
	authProtected.Delete("/api/dashboard/goals/{id}", handlers.HandleGoalsDelete)
	authProtected.Get("/api/dashboard/goals/{id}/analytics", handlers.HandleGoalsAnalytics)
	authProtected.Get("/api/dashboard/goals/{id}/breakdown/{type}", handlers.HandleGoalsBreakdown)

	// Website Management API (protected)
	authProtected.Get("/api/websites/list", handlers.HandleWebsiteList)
	authProtected.Get("/api/websites/{website_id}", handlers.HandleWebsiteShow)
	authProtected.Post("/api/websites", handlers.HandleWebsiteCreate)
	authProtected.Put("/api/websites/{website_id}", handlers.HandleWebsiteUpdate)
	authProtected.Post("/api/websites/{website_id}/domains", handlers.HandleAddDomain)
	authProtected.Delete("/api/websites/{website_id}/domains", handlers.HandleRemoveDomain)
	authProtected.Patch("/api/websites/{website_id}/public-stats", handlers.HandleSetPublicStats)

	// Public Stats API (no auth, opt-in per website)
	r.Get("/api/public/stats/{website_id}", handlers.HandlePublicStats)

	// API Key Stats API (requires API key with stats scope)
	r.With(appmiddleware.APIKeyAuthAny).Get("/api/v1/stats/{website_id}", handlers.HandleAPIStats)

	// Website Management Dashboard page (protected)
	r.With(appmiddleware.AuthWithRedirect).Get("/dashboard/websites", func(w http.ResponseWriter, r *http.Request) {
		if err := render(w, "views/dashboard/websites", "views/layouts/dashboard", map[string]any{
			"Title":         "Websites",
			"Version":       Version,
			"SelfWebsiteID": config.SelfWebsiteID,
		}); err != nil {
			http.Error(w, "Failed to render websites view", http.StatusInternalServerError)
		}
	})

	r.With(appmiddleware.AuthWithRedirect).Get("/dashboard/goals", func(w http.ResponseWriter, r *http.Request) {
		if err := render(w, "views/dashboard/goals", "views/layouts/dashboard", map[string]any{
			"Title":         "Goals",
			"Version":       Version,
			"SelfWebsiteID": config.SelfWebsiteID,
		}); err != nil {
			http.Error(w, "Failed to render goals view", http.StatusInternalServerError)
		}
	})

	port := getEnv("PORT", "3000")
	server := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}
	logging.L().Info("starting kaunta server", zap.String("port", port))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logging.Fatal("http server exited", zap.Error(err))
	}
	return nil
}

// Handler functions

func handleHealth(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
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

func handleVersion(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"version": Version,
	})
}

func handleTrackerScript(trackerScript []byte) http.Handler {
	hash := sha256.Sum256(trackerScript)
	etag := `"` + hex.EncodeToString(hash[:8]) + `"`

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Timing-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "public, max-age=3600, immutable")
		w.Header().Set("ETag", etag)

		if match := r.Header.Get("If-None-Match"); match == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		_, _ = w.Write(trackerScript)
	})
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

func upHandler(w http.ResponseWriter, r *http.Request) {
	if err := pingDatabase(); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func optionsOK(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func addVersionHeader() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Kaunta-Version", Version)
			next.ServeHTTP(w, r)
		})
	}
}

type responseLogger struct {
	http.ResponseWriter
	status int
	size   int64
}

func (l *responseLogger) WriteHeader(statusCode int) {
	l.status = statusCode
	l.ResponseWriter.WriteHeader(statusCode)
}

func (l *responseLogger) Write(b []byte) (int, error) {
	if l.status == 0 {
		l.status = http.StatusOK
	}
	n, err := l.ResponseWriter.Write(b)
	l.size += int64(n)
	return n, err
}

func (l *responseLogger) Flush() {
	if flusher, ok := l.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func requestLoggerMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/up" || r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			lrw := &responseLogger{ResponseWriter: w}
			next.ServeHTTP(lrw, r)

			logging.L().Info("http request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", lrw.status),
				zap.Int64("bytes", lrw.size),
				zap.String("ip", httpx.ClientIP(r)),
				zap.Duration("duration", time.Since(start)),
			)
		})
	}
}

func corsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type rateLimitConfig struct {
	Limit   int
	Window  time.Duration
	KeyFunc func(*http.Request) string
	OnLimit func(http.ResponseWriter, *http.Request)
}

func rateLimitMiddleware(cfg rateLimitConfig) func(http.Handler) http.Handler {
	type bucket struct {
		count int
		reset time.Time
	}

	var (
		mu      sync.Mutex
		buckets = make(map[string]*bucket)
	)

	if cfg.KeyFunc == nil {
		cfg.KeyFunc = func(*http.Request) string { return "default" }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := cfg.KeyFunc(r)
			if key == "" {
				key = "default"
			}

			now := time.Now()

			mu.Lock()
			b := buckets[key]
			if b == nil || now.After(b.reset) {
				b = &bucket{reset: now.Add(cfg.Window)}
				buckets[key] = b
			}
			b.count++
			count := b.count
			reset := b.reset
			mu.Unlock()

			if cfg.Limit > 0 {
				w.Header().Set("X-RateLimit-Limit", strconv.Itoa(cfg.Limit))
				remaining := cfg.Limit - count
				if remaining < 0 {
					remaining = 0
				}
				w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			}
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))

			if cfg.Limit > 0 && count > cfg.Limit {
				if cfg.OnLimit != nil {
					cfg.OnLimit(w, r)
				} else {
					http.Error(w, "Too many requests", http.StatusTooManyRequests)
				}
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

type csrfOptions struct {
	Secure        bool
	TrustedOrigin []string
}

func csrfMiddleware(opts csrfOptions) func(http.Handler) http.Handler {
	trusted := make(map[string]struct{}, len(opts.TrustedOrigin))
	for _, origin := range opts.TrustedOrigin {
		trusted[strings.ToLower(origin)] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ensureCSRFToken(w, r, opts.Secure)

			if shouldSkipCSRF(r) {
				next.ServeHTTP(w, r)
				return
			}

			if isSafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			if origin := r.Header.Get("Origin"); origin != "" && len(trusted) > 0 {
				if _, ok := trusted[strings.ToLower(origin)]; !ok {
					http.Error(w, "origin not allowed", http.StatusForbidden)
					return
				}
			}

			headerToken := r.Header.Get("X-CSRF-Token")
			if headerToken == "" || headerToken != token {
				http.Error(w, "invalid CSRF token", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func ensureCSRFToken(w http.ResponseWriter, r *http.Request, secure bool) string {
	if cookie, err := r.Cookie("kaunta_csrf"); err == nil && cookie.Value != "" {
		return cookie.Value
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		panic(err)
	}
	token := base64.RawStdEncoding.EncodeToString(tokenBytes)

	http.SetCookie(w, &http.Cookie{
		Name:     "kaunta_csrf",
		Value:    token,
		Path:     "/",
		Secure:   secure,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(7 * 24 * time.Hour),
	})
	return token
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func shouldSkipCSRF(r *http.Request) bool {
	path := r.URL.Path
	if path == "/api/send" {
		return true
	}
	if strings.HasPrefix(path, "/api/ingest") {
		return true
	}
	if isSafeMethod(r.Method) && (strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".css")) {
		return true
	}
	return false
}

func vendorAssetHandler(vendorJS, vendorCSS []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filename := chi.URLParam(r, "filename")
		if idx := strings.Index(filename, "?"); idx > -1 {
			filename = filename[:idx]
		}
		base := path.Base(filename)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("CF-Cache-Tag", "kaunta-assets")

		switch base {
		case "vendor.js":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
			_, _ = w.Write(vendorJS)
		case "vendor.css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			_, _ = w.Write(vendorCSS)
		default:
			http.NotFound(w, r)
		}
	}
}

func staticDataHandler(countriesGeoJSON []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filename := chi.URLParam(r, "filename")
		if idx := strings.Index(filename, "?"); idx > -1 {
			filename = filename[:idx]
		}

		w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
		w.Header().Set("CF-Cache-Tag", "kaunta-data")

		switch filename {
		case "countries-110m.json":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write(countriesGeoJSON)
		default:
			http.NotFound(w, r)
		}
	}
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
          Built with Go, Fiber, Datastar, PostgreSQL, and Leaflet
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

	r := chi.NewRouter()
	r.Use(chimiddleware.Recoverer)
	r.Use(requestLoggerMiddleware())

	setupLimiter := rateLimitMiddleware(rateLimitConfig{
		Limit:  5,
		Window: time.Minute,
		KeyFunc: func(r *http.Request) string {
			return httpx.ClientIP(r)
		},
		OnLimit: func(w http.ResponseWriter, r *http.Request) {
			httpx.WriteJSON(w, http.StatusTooManyRequests, map[string]any{
				"error": "Too many requests, slow down.",
			})
		},
	})

	r.Get("/setup", handlers.ShowSetup(SetupTemplate))
	r.With(setupLimiter).Post("/setup", handlers.SubmitSetup(func() {
		go func() {
			time.Sleep(500 * time.Millisecond)
			close(setupDone)
		}()
	}))
	r.With(setupLimiter).Post("/setup/test-db", handlers.TestDatabase())
	r.Get("/setup/complete", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(SetupCompleteTemplate)
	})

	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/setup", http.StatusFound)
	})

	r.Get("/up", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	r.Get("/assets/favicon.ico", func(w http.ResponseWriter, req *http.Request) {
		data, err := fs.ReadFile(AssetsFS.(embed.FS), "assets/favicon.ico")
		if err != nil {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "image/x-icon")
		_, _ = w.Write(data)
	})

	r.Get("/assets/global.css", func(w http.ResponseWriter, req *http.Request) {
		data, err := fs.ReadFile(AssetsFS.(embed.FS), "assets/global.css")
		if err != nil {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/css")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(data)
	})

	r.Get("/assets/vendor/vendor.js", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		_, _ = w.Write(VendorJS)
	})

	r.Get("/assets/vendor/vendor.css", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		_, _ = w.Write(VendorCSS)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	addr := fmt.Sprintf(":%s", port)
	logging.L().Info("setup wizard available", zap.String("url", fmt.Sprintf("http://localhost:%s/setup", port)))

	server := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.L().Debug("setup server stopped", zap.Error(err))
		}
	}()

	<-setupDone
	logging.L().Info("setup completed, shutting down setup server")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logging.L().Warn("error shutting down setup server", zap.Error(err))
	}

	return ErrSetupComplete
}
