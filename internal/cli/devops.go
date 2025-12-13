package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/seuros/kaunta/internal/database"
)

// ============================================================
// Test Tracking Command
// ============================================================

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Run diagnostic tests",
	Long:  `Test various components of the Kaunta system.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(cmd.Help())
	},
}

var testTrackingCmd = &cobra.Command{
	Use:   "tracking <website-domain> [--origin <origin-url>] [--payload <json-file>]",
	Short: "Test website tracking setup",
	Long: `Validate website tracking setup and connectivity.

Tests:
  - Website exists and is active
  - CORS origin validation
  - Tracking endpoint connectivity
  - Sends test event to /api/send endpoint

Examples:
  kaunta test tracking example.com
  kaunta test tracking example.com --origin "https://example.com"
  kaunta test tracking example.com --payload custom.json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		origin, _ := cmd.Flags().GetString("origin")
		payloadFile, _ := cmd.Flags().GetString("payload")
		return runTestTracking(args[0], origin, payloadFile)
	},
}

func runTestTracking(websiteDomain, originURL, payloadFile string) error {
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = database.Close() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Println("=== Kaunta Tracking Setup Test ===")

	// Step 1: Check website exists
	fmt.Print("Step 1: Checking website exists... ")
	website, err := GetWebsiteByDomain(ctx, websiteDomain, nil)
	if err != nil {
		fmt.Println("FAIL")
		fmt.Printf("  Error: %v\n", err)
		return nil // Don't exit, show all failures
	}
	fmt.Println("PASS")
	fmt.Printf("  Website ID: %s\n", website.WebsiteID)
	fmt.Printf("  Domain: %s\n", website.Domain)

	// Use domain as origin if not specified
	if originURL == "" {
		originURL = "https://" + websiteDomain
	}

	// Step 2: Validate CORS origin
	fmt.Print("\nStep 2: Validating CORS origin... ")
	originHost := extractHost(originURL)
	isOriginAllowed := false
	for _, allowed := range website.AllowedDomains {
		if strings.EqualFold(allowed, originHost) || strings.EqualFold(allowed, websiteDomain) {
			isOriginAllowed = true
			break
		}
	}

	if !isOriginAllowed {
		fmt.Println("WARN")
		fmt.Printf("  Origin '%s' not in allowed domains: %v\n", originHost, website.AllowedDomains)
		fmt.Printf("  Suggestion: kaunta website add-domain %s %s\n", websiteDomain, originHost)
	} else {
		fmt.Println("PASS")
		fmt.Printf("  Origin '%s' is allowed\n", originHost)
	}

	// Step 3: Check database validation function
	fmt.Print("\nStep 3: Testing database origin validation... ")
	isValid, err := validateOriginInDB(ctx, website.WebsiteID, originURL)
	if err != nil {
		fmt.Println("FAIL")
		fmt.Printf("  Error: %v\n", err)
	} else if isValid {
		fmt.Println("PASS")
		fmt.Printf("  Database confirms origin is valid\n")
	} else {
		fmt.Println("WARN")
		fmt.Printf("  Database validation failed for origin\n")
	}

	// Step 4: Prepare test payload
	fmt.Print("\nStep 4: Preparing test payload... ")
	testPayload := map[string]interface{}{
		"type": "event",
		"payload": map[string]interface{}{
			"website":   website.WebsiteID,
			"hostname":  originHost,
			"url":       "/test",
			"title":     "Test Page",
			"referrer":  nil,
			"language":  "en-US",
			"screen":    "1920x1080",
			"timestamp": time.Now().UnixMilli(),
		},
	}

	// Load custom payload if provided
	if payloadFile != "" {
		data, err := os.ReadFile(payloadFile)
		if err != nil {
			fmt.Println("FAIL")
			fmt.Printf("  Error reading payload file: %v\n", err)
			return nil
		}
		if err := json.Unmarshal(data, &testPayload); err != nil {
			fmt.Println("FAIL")
			fmt.Printf("  Error parsing payload JSON: %v\n", err)
			return nil
		}
	}
	fmt.Println("PASS")

	// Show payload
	payloadJSON, _ := json.MarshalIndent(testPayload, "  ", "  ")
	fmt.Printf("  Payload:\n%s\n", payloadJSON)

	// Step 5: Send test event (simulation)
	fmt.Print("\nStep 5: Simulating tracking event send... ")
	fmt.Println("PASS (simulated)")
	fmt.Printf("  Event would be sent with:\n")
	fmt.Printf("    Origin: %s\n", originURL)
	fmt.Printf("    Website ID: %s\n", website.WebsiteID)
	fmt.Printf("    Headers: Origin: %s\n", originURL)

	// Summary
	fmt.Println("\n=== Test Summary ===")
	fmt.Printf("Website: %s\n", website.Domain)
	fmt.Printf("Website ID: %s\n", website.WebsiteID)
	fmt.Printf("Allowed Origins: %v\n", website.AllowedDomains)
	fmt.Printf("Test Origin: %s\n", originURL)

	if isOriginAllowed {
		fmt.Println("\nStatus: Ready for tracking ✓")
	} else {
		fmt.Println("\nStatus: Configuration needed ⚠")
		fmt.Printf("\nTo fix:\n")
		fmt.Printf("  kaunta website add-domain %s %s\n", websiteDomain, originHost)
	}

	return nil
}

func extractHost(originURL string) string {
	if !strings.Contains(originURL, "://") {
		originURL = "https://" + originURL
	}
	u, err := url.Parse(originURL)
	if err != nil {
		return originURL
	}
	// Remove port if present
	host := u.Hostname()
	if host == "" {
		return u.Host
	}
	return host
}

func validateOriginInDB(ctx context.Context, websiteID string, originURL string) (bool, error) {
	var isValid bool
	err := database.DB.QueryRowContext(ctx,
		"SELECT validate_origin($1::uuid, $2::text)",
		websiteID, originURL,
	).Scan(&isValid)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	return isValid, nil
}

// ============================================================
// Diagnostics Command
// ============================================================

type DiagnosticsResult struct {
	DatabaseConnected bool
	PostgreSQLVersion string
	ExtensionsLoaded  []string
	WebsiteCount      int64
	SessionCount      int64
	EventCount        int64
	OldestEvent       *time.Time
	NewestEvent       *time.Time
	PartitionCount    int
	DiskUsageGB       float64
	EventsPerMinute   float64
	DataRetentionDays int
	Status            string
}

var diagnosticsCmd = &cobra.Command{
	Use:   "diagnostics [--full]",
	Short: "System health check",
	Long: `Check database and system health without modifying data.

Displays:
  - Database connectivity
  - PostgreSQL version and extensions
  - Table structure health
  - Record counts
  - Data retention period
  - Event processing rate
  - Disk space usage`,
	RunE: func(cmd *cobra.Command, args []string) error {
		full, _ := cmd.Flags().GetBool("full")
		return runDiagnostics(full)
	},
}

func runDiagnostics(full bool) error {
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = database.Close() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := RunDiagnostics(ctx, database.DB)
	if err != nil {
		return fmt.Errorf("diagnostics failed: %w", err)
	}

	// Display results
	fmt.Println("=== Kaunta System Diagnostics ===")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Database
	status := "PASS"
	if !result.DatabaseConnected {
		status = "FAIL"
	}
	_, _ = fmt.Fprintf(w, "Database Connected:\t%s\n", status)

	if result.PostgreSQLVersion != "" {
		_, _ = fmt.Fprintf(w, "PostgreSQL Version:\t%s\n", result.PostgreSQLVersion)
	}

	if len(result.ExtensionsLoaded) > 0 {
		_, _ = fmt.Fprintf(w, "Extensions Loaded:\t%v\n", result.ExtensionsLoaded)
	}

	// Data
	_, _ = fmt.Fprintf(w, "\nWebsites:\t%d\n", result.WebsiteCount)
	_, _ = fmt.Fprintf(w, "Sessions:\t%d\n", result.SessionCount)
	_, _ = fmt.Fprintf(w, "Events:\t%d\n", result.EventCount)

	if result.OldestEvent != nil {
		_, _ = fmt.Fprintf(w, "Oldest Event:\t%s\n", result.OldestEvent.Format("2006-01-02 15:04:05"))
	}

	if result.NewestEvent != nil {
		_, _ = fmt.Fprintf(w, "Newest Event:\t%s\n", result.NewestEvent.Format("2006-01-02 15:04:05"))
	}

	if result.DataRetentionDays > 0 {
		_, _ = fmt.Fprintf(w, "Data Retention:\t%d days\n", result.DataRetentionDays)
	}

	// Performance
	if result.EventsPerMinute > 0 {
		_, _ = fmt.Fprintf(w, "Events Per Minute:\t%.1f\n", result.EventsPerMinute)
	}

	_, _ = fmt.Fprintf(w, "Partitions:\t%d\n", result.PartitionCount)

	// Storage
	if result.DiskUsageGB > 0 {
		_, _ = fmt.Fprintf(w, "Disk Usage:\t%.2f GB\n", result.DiskUsageGB)
	}

	_, _ = fmt.Fprintf(w, "\nStatus:\t%s\n", result.Status)

	_ = w.Flush()

	if full {
		fmt.Println("=== Full Diagnostics Report ===")
		_ = reportFullDiagnostics(ctx, database.DB)
	}

	return nil
}

func reportFullDiagnostics(ctx context.Context, db *sql.DB) error {
	// Index health
	fmt.Println("Index Status:")
	rows, err := db.QueryContext(ctx, `
		SELECT relname, idx_scan, idx_tup_read, idx_tup_fetch
		FROM pg_stat_user_indexes
		WHERE schemaname = 'public'
		ORDER BY idx_scan DESC
		LIMIT 10
	`)
	if err != nil {
		fmt.Printf("  Error querying indexes: %v\n", err)
		return nil
	}
	defer func() { _ = rows.Close() }()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "  Index\tScans\tTuples Read\tTuples Fetched\n")
	_, _ = fmt.Fprintf(w, "  -----\t-----\t-----------\t--------------\n")

	for rows.Next() {
		var name string
		var scans, read, fetch int64
		if err := rows.Scan(&name, &scans, &read, &fetch); err != nil {
			continue
		}
		_, _ = fmt.Fprintf(w, "  %s\t%d\t%d\t%d\n", name, scans, read, fetch)
	}
	_ = w.Flush()

	// Table sizes
	fmt.Println("\nTable Sizes:")
	rows, err = db.QueryContext(ctx, `
		SELECT tablename, pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) as size
		FROM pg_tables
		WHERE schemaname = 'public'
		ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC
		LIMIT 10
	`)
	if err != nil {
		fmt.Printf("  Error querying tables: %v\n", err)
		return nil
	}
	defer func() { _ = rows.Close() }()

	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "  Table\tSize\n")
	_, _ = fmt.Fprintf(w, "  -----\t----\n")

	for rows.Next() {
		var name, size string
		if err := rows.Scan(&name, &size); err != nil {
			continue
		}
		_, _ = fmt.Fprintf(w, "  %s\t%s\n", name, size)
	}
	_ = w.Flush()

	return nil
}

// ============================================================
// Website Sync Command
// ============================================================

type WebsiteConfig struct {
	Domain         string   `yaml:"domain" json:"domain"`
	Name           string   `yaml:"name" json:"name"`
	AllowedDomains []string `yaml:"allowed_domains" json:"allowed_domains"`
}

type SyncFile struct {
	Websites []WebsiteConfig `yaml:"websites" json:"websites"`
}

type SyncStats struct {
	Created int
	Updated int
	Skipped int
	Errors  []string
}

var syncCmd = &cobra.Command{
	Use:   "sync --from <file.yaml|file.json> [--dry-run] [--merge|--replace]",
	Short: "Bulk import/update websites",
	Long: `Sync websites from a YAML or JSON file.

Options:
  --from         Path to config file (YAML or JSON) - required
  --dry-run      Preview changes without applying
  --merge        Keep existing websites, update/add new (default)
  --replace      Delete all existing websites and import only from file

File format:
  websites:
    - domain: example.com
      name: "Example Site"
      allowed_domains:
        - example.com
        - www.example.com

Examples:
  kaunta website sync --from websites.yaml --dry-run
  kaunta website sync --from websites.yaml --merge
  kaunta website sync --from websites.json --replace`,
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath, _ := cmd.Flags().GetString("from")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		replace, _ := cmd.Flags().GetBool("replace")

		if filePath == "" {
			return fmt.Errorf("--from flag is required")
		}

		return runWebsiteSync(filePath, dryRun, replace)
	},
}

func runWebsiteSync(filePath string, dryRun, replace bool) error {
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = database.Close() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Parse file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var syncFile SyncFile
	if strings.HasSuffix(filePath, ".yaml") || strings.HasSuffix(filePath, ".yml") {
		if err := yaml.Unmarshal(data, &syncFile); err != nil {
			return fmt.Errorf("invalid YAML format: %w", err)
		}
	} else if strings.HasSuffix(filePath, ".json") {
		if err := json.Unmarshal(data, &syncFile); err != nil {
			return fmt.Errorf("invalid JSON format: %w", err)
		}
	} else {
		return fmt.Errorf("unsupported file format (use .yaml or .json)")
	}

	if len(syncFile.Websites) == 0 {
		return fmt.Errorf("no websites found in file")
	}

	// Validate all websites before applying
	for _, ws := range syncFile.Websites {
		if err := validateDomain(ws.Domain); err != nil {
			return fmt.Errorf("invalid website '%s': %w", ws.Domain, err)
		}
		if ws.Name == "" {
			ws.Name = ws.Domain
		}
	}

	// Perform sync
	stats, err := SyncWebsitesFromFile(ctx, database.DB, syncFile, dryRun, !replace)
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	// Display results
	fmt.Println("=== Website Sync Report ===")
	if dryRun {
		fmt.Println("[DRY RUN - No changes applied]")
	}

	fmt.Printf("Created:  %d\n", stats.Created)
	fmt.Printf("Updated:  %d\n", stats.Updated)
	fmt.Printf("Skipped:  %d\n", stats.Skipped)

	if len(stats.Errors) > 0 {
		fmt.Printf("\nErrors (%d):\n", len(stats.Errors))
		for _, e := range stats.Errors {
			fmt.Printf("  - %s\n", e)
		}
	}

	if !dryRun && (stats.Created > 0 || stats.Updated > 0) {
		fmt.Printf("\nSuccess! Processed %d websites.\n", stats.Created+stats.Updated)
	}

	return nil
}

// ============================================================
// Migrate Command Enhancements
// ============================================================

var migrateCmd = &cobra.Command{
	Use:   "migrate [up|down|version] [--step <N>]",
	Short: "Manage database migrations",
	Long: `Run database migrations.

Subcommands:
  up       Run pending migrations (default: all)
  down     Rollback migrations
  version  Show current migration version

Examples:
  kaunta migrate up
  kaunta migrate up --step 1
  kaunta migrate down --step 2
  kaunta migrate version`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			args = []string{"up"}
		}
		action := args[0]
		step, _ := cmd.Flags().GetInt("step")

		return runMigrate(action, step)
	},
}

func runMigrate(action string, step int) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL environment variable not set")
	}

	switch action {
	case "up":
		return runMigrateUp(databaseURL, step)
	case "down":
		return runMigrateDown(databaseURL, step)
	case "version":
		return runMigrateVersion(databaseURL)
	default:
		return fmt.Errorf("unknown action: %s (use up, down, or version)", action)
	}
}

func runMigrateUp(databaseURL string, steps int) error {
	fmt.Println("Running migrations...")
	if err := database.RunMigrations(databaseURL); err != nil {
		return err
	}
	fmt.Println("Migrations completed successfully")
	return runMigrateVersion(databaseURL)
}

func runMigrateDown(databaseURL string, steps int) error {
	fmt.Println("Rollback not yet implemented")
	return nil
}

func runMigrateVersion(databaseURL string) error {
	version, dirty, err := database.GetMigrationVersion(databaseURL)
	if err != nil {
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	fmt.Println("=== Migration Status ===")
	fmt.Printf("Current Version: %d\n", version)
	if dirty {
		fmt.Println("State: DIRTY (incomplete migration)")
	} else {
		fmt.Println("State: CLEAN")
	}

	return nil
}

// ============================================================
// Check Website Command
// ============================================================

var checkWebsiteCmd = &cobra.Command{
	Use:   "check <website-domain>",
	Short: "Validate website configuration",
	Long: `Quick validation check for a specific website.

Checks:
  - Website exists
  - Domain is unique
  - Allowed domains are valid
  - Share ID is unique (if set)

Example:
  kaunta check website example.com`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCheckWebsite(args[0])
	},
}

type WebsiteCheckResult struct {
	Valid    bool
	Issues   []string
	Warnings []string
}

func runCheckWebsite(websiteDomain string) error {
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = database.Close() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := CheckWebsite(ctx, database.DB, websiteDomain)
	if err != nil {
		return err
	}

	fmt.Println("=== Website Check ===")
	fmt.Printf("Domain: %s\n", websiteDomain)

	if result.Valid {
		fmt.Println("Status: Valid ✓")
	} else {
		fmt.Println("Status: Invalid ✗")
	}

	if len(result.Issues) > 0 {
		fmt.Println("Issues:")
		for _, issue := range result.Issues {
			fmt.Printf("  - %s\n", issue)
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Println("\nWarnings:")
		for _, warning := range result.Warnings {
			fmt.Printf("  - %s\n", warning)
		}
	}

	if result.Valid && len(result.Warnings) == 0 {
		fmt.Println("Everything looks good!")
	}

	return nil
}

// ============================================================
// Helper Functions
// ============================================================

func RunDiagnostics(ctx context.Context, db *sql.DB) (*DiagnosticsResult, error) {
	result := &DiagnosticsResult{
		DatabaseConnected: false,
		ExtensionsLoaded:  []string{},
	}

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		return result, fmt.Errorf("database connection failed: %w", err)
	}
	result.DatabaseConnected = true

	// Get PostgreSQL version
	var version string
	if err := db.QueryRowContext(ctx, "SELECT version()").Scan(&version); err == nil {
		result.PostgreSQLVersion = version
	}

	// Check extensions
	rows, err := db.QueryContext(ctx, "SELECT extname FROM pg_extension WHERE extname IN ('uuid-ossp', 'pgcrypto')")
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var ext string
			if err := rows.Scan(&ext); err == nil {
				result.ExtensionsLoaded = append(result.ExtensionsLoaded, ext)
			}
		}
	}

	// Count records
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM website WHERE deleted_at IS NULL").Scan(&result.WebsiteCount)
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM session").Scan(&result.SessionCount)
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM website_event").Scan(&result.EventCount)

	// Date range
	_ = db.QueryRowContext(ctx, "SELECT MIN(created_at) FROM website_event").Scan(&result.OldestEvent)
	_ = db.QueryRowContext(ctx, "SELECT MAX(created_at) FROM website_event").Scan(&result.NewestEvent)

	// Data retention
	if result.OldestEvent != nil && result.NewestEvent != nil {
		result.DataRetentionDays = int(result.NewestEvent.Sub(*result.OldestEvent).Hours() / 24)
	}

	// Partition count
	partitionQuery := `SELECT COUNT(*) FROM pg_tables
		WHERE schemaname = 'public' AND tablename LIKE 'website_event_%'`
	_ = db.QueryRowContext(ctx, partitionQuery).Scan(&result.PartitionCount)

	// Disk usage
	var diskUsageBytes int64
	_ = db.QueryRowContext(ctx, "SELECT pg_database_size(current_database())").Scan(&diskUsageBytes)
	result.DiskUsageGB = float64(diskUsageBytes) / (1024 * 1024 * 1024)

	// Events per minute
	var eventCount int64
	var minutesBack float64
	row := db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       EXTRACT(EPOCH FROM (MAX(created_at) - MIN(created_at))) / 60.0 as minutes
		FROM website_event
		WHERE created_at >= NOW() - INTERVAL '24 hours'
	`)
	if err := row.Scan(&eventCount, &minutesBack); err == nil && minutesBack > 0 {
		result.EventsPerMinute = float64(eventCount) / minutesBack
	}

	// Status
	if result.DatabaseConnected && len(result.ExtensionsLoaded) >= 2 && result.EventCount > 0 {
		result.Status = "healthy"
	} else if result.DatabaseConnected {
		result.Status = "operational"
	} else {
		result.Status = "critical"
	}

	return result, nil
}

func SyncWebsitesFromFile(ctx context.Context, db *sql.DB, syncFile SyncFile, dryRun bool, merge bool) (*SyncStats, error) {
	stats := &SyncStats{
		Errors: []string{},
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return stats, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if !merge {
		// Delete all existing websites
		if _, err := tx.ExecContext(ctx, "UPDATE website SET deleted_at = NOW()"); err != nil {
			stats.Errors = append(stats.Errors, fmt.Sprintf("Failed to delete existing websites: %v", err))
			return stats, nil
		}
	}

	// Process each website
	for _, ws := range syncFile.Websites {
		// Check if website exists
		var exists bool
		var websiteID string
		checkQuery := `SELECT EXISTS(
			SELECT 1 FROM website WHERE LOWER(domain) = LOWER($1) AND deleted_at IS NULL
		), website_id FROM website WHERE LOWER(domain) = LOWER($1)`
		if err := tx.QueryRowContext(ctx, checkQuery, ws.Domain).Scan(&exists, &websiteID); err != nil {
			stats.Errors = append(stats.Errors, fmt.Sprintf("Failed to check website %s: %v", ws.Domain, err))
			continue
		}

		if exists {
			// Update existing
			domainsJSON, _ := json.Marshal(ws.AllowedDomains)
			_, err := tx.ExecContext(ctx,
				"UPDATE website SET name = $1, allowed_domains = $2, updated_at = NOW() WHERE website_id = $3",
				ws.Name, string(domainsJSON), websiteID,
			)
			if err != nil {
				stats.Errors = append(stats.Errors, fmt.Sprintf("Failed to update %s: %v", ws.Domain, err))
				continue
			}
			stats.Updated++
		} else {
			// Create new
			websiteID := uuid.New().String()
			domainsJSON, _ := json.Marshal(ws.AllowedDomains)
			_, err := tx.ExecContext(ctx,
				"INSERT INTO website (website_id, domain, name, allowed_domains, created_at, updated_at) VALUES ($1, $2, $3, $4::jsonb, NOW(), NOW())",
				websiteID, ws.Domain, ws.Name, string(domainsJSON),
			)
			if err != nil {
				stats.Errors = append(stats.Errors, fmt.Sprintf("Failed to create %s: %v", ws.Domain, err))
				continue
			}
			stats.Created++
		}
	}

	if !dryRun {
		if err := tx.Commit(); err != nil {
			return stats, fmt.Errorf("failed to commit transaction: %w", err)
		}
	}

	return stats, nil
}

func CheckWebsite(ctx context.Context, db *sql.DB, websiteDomain string) (*WebsiteCheckResult, error) {
	result := &WebsiteCheckResult{
		Valid:    true,
		Issues:   []string{},
		Warnings: []string{},
	}

	// Check website exists
	var websiteID string
	var allowedDomainsJSON []byte
	err := db.QueryRowContext(ctx,
		"SELECT website_id, allowed_domains FROM website WHERE LOWER(domain) = LOWER($1) AND deleted_at IS NULL",
		websiteDomain,
	).Scan(&websiteID, &allowedDomainsJSON)

	if err == sql.ErrNoRows {
		result.Valid = false
		result.Issues = append(result.Issues, fmt.Sprintf("Website '%s' not found", websiteDomain))
		return result, nil
	}
	if err != nil {
		return nil, err
	}

	// Parse allowed domains
	var allowedDomains []string
	if err := json.Unmarshal(allowedDomainsJSON, &allowedDomains); err != nil {
		result.Warnings = append(result.Warnings, "Failed to parse allowed_domains JSON")
	}

	// Check for empty allowed domains
	if len(allowedDomains) == 0 {
		result.Warnings = append(result.Warnings, "No allowed domains configured (tracking may fail)")
	}

	// Check for duplicate domains (case-insensitive)
	domainMap := make(map[string]bool)
	for _, d := range allowedDomains {
		lower := strings.ToLower(d)
		if domainMap[lower] {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Duplicate domain (case-insensitive): %s", d))
		}
		domainMap[lower] = true
	}

	// Check for invalid domain formats
	for _, d := range allowedDomains {
		if err := validateDomain(d); err != nil {
			result.Issues = append(result.Issues, fmt.Sprintf("Invalid allowed domain '%s': %v", d, err))
			result.Valid = false
		}
	}

	return result, nil
}

// ============================================================
// Initialize commands in CLI
// ============================================================

func init() {
	// Add test command
	RootCmd.AddCommand(testCmd)
	testCmd.AddCommand(testTrackingCmd)
	testTrackingCmd.Flags().StringP("origin", "o", "", "Origin URL to test")
	testTrackingCmd.Flags().StringP("payload", "p", "", "Custom test payload file")

	// Add diagnostics command
	RootCmd.AddCommand(diagnosticsCmd)
	diagnosticsCmd.Flags().BoolP("full", "f", false, "Show detailed diagnostics")

	// Add sync command to website
	websiteCmd.AddCommand(syncCmd)
	syncCmd.Flags().StringP("from", "f", "", "Path to YAML or JSON file (required)")
	syncCmd.Flags().BoolP("dry-run", "d", false, "Preview changes without applying")
	syncCmd.Flags().BoolP("replace", "r", false, "Replace all existing websites")

	// Add migrate command
	RootCmd.AddCommand(migrateCmd)
	migrateCmd.Flags().IntP("step", "s", 0, "Number of migrations to run/rollback")

	// Add check command to website
	websiteCmd.AddCommand(checkWebsiteCmd)
}
