package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/spf13/cobra"

	"github.com/seuros/kaunta/internal/config"
	"github.com/seuros/kaunta/internal/database"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run health checks on Kaunta installation",
	Long: `Run comprehensive health checks on Kaunta installation.

Checks performed:
  - Data directory writable
  - GeoIP database exists
  - Database connection
  - PostgreSQL version ≥17
  - Database migrations completed
  - PostgreSQL functions exist
  - PostgreSQL triggers exist
  - Materialized views exist

Example:
  kaunta doctor
  kaunta doctor --json`,
	RunE: runDoctor,
}

type CheckResult struct {
	Name       string `json:"name"`
	Pass       bool   `json:"pass"`
	Error      string `json:"error,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
	Details    string `json:"details,omitempty"`
}

var requiredFunctions = []string{
	// Auth functions
	"hash_password",
	"verify_password",
	"validate_session",
	"cleanup_expired_sessions",

	// Partition management
	"cleanup_old_partitions",
	"cleanup_old_bot_logs",
	"get_partition_stats",
	"reset_stale_request_counters",

	// Origin validation
	"is_trusted_origin",
	"update_trusted_origin_timestamp",
	"get_trusted_origins",

	// Analytics
	"get_dashboard_stats",
	"get_top_pages",
	"get_timeseries",
	"get_breakdown",
	"validate_origin",
}

var requiredTriggers = []struct {
	name  string
	table string
}{
	{"trg_website_event_realtime_stats", "website_event"},
	{"trigger_update_trusted_origin_timestamp", "trusted_origin"},
}

var requiredMatViews = []string{
	"daily_website_stats",
	"hourly_website_stats",
	"realtime_website_stats",
	"bot_stats_by_country",
}

func checkDataDirectory(cfg *config.Config) CheckResult {
	// Test write access to DATA_DIR
	testFile := filepath.Join(cfg.DataDir, ".kaunta-write-test")
	err := os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		return CheckResult{
			Name:       "Data Directory Writable",
			Pass:       false,
			Error:      err.Error(),
			Suggestion: "Ensure DATA_DIR has write permissions",
		}
	}
	_ = os.Remove(testFile)
	return CheckResult{Name: "Data Directory Writable", Pass: true}
}

func checkGeoIPDatabase(cfg *config.Config) CheckResult {
	geoipPath := filepath.Join(cfg.DataDir, "GeoLite2-City.mmdb")

	info, err := os.Stat(geoipPath)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{
				Name:       "GeoIP Database",
				Pass:       false,
				Error:      "GeoLite2-City.mmdb not found",
				Suggestion: "Database will auto-download on first server start",
			}
		}
		return CheckResult{Name: "GeoIP Database", Pass: false, Error: err.Error()}
	}

	// Check if file is readable
	file, err := os.Open(geoipPath)
	if err != nil {
		return CheckResult{
			Name:       "GeoIP Database",
			Pass:       false,
			Error:      "Cannot read GeoLite2-City.mmdb",
			Suggestion: "Check file permissions",
		}
	}
	_ = file.Close()

	return CheckResult{
		Name:    "GeoIP Database",
		Pass:    true,
		Details: fmt.Sprintf("%.1f MB", float64(info.Size())/(1024*1024)),
	}
}

func checkDatabaseConnection(db *sql.DB) CheckResult {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return CheckResult{
			Name:       "Database Connection",
			Pass:       false,
			Error:      err.Error(),
			Suggestion: "Verify DATABASE_URL and ensure PostgreSQL is running",
		}
	}
	return CheckResult{Name: "Database Connection", Pass: true}
}

func checkPostgreSQLVersion(db *sql.DB) CheckResult {
	var version string
	err := db.QueryRow("SHOW server_version").Scan(&version)
	if err != nil {
		return CheckResult{Name: "PostgreSQL Version", Pass: false, Error: err.Error()}
	}

	// Parse version (e.g., "17.1 (Debian 17.1-1)")
	parts := strings.Split(version, " ")
	versionNum := strings.Split(parts[0], ".")
	major, _ := strconv.Atoi(versionNum[0])

	if major < 17 {
		return CheckResult{
			Name:       "PostgreSQL Version",
			Pass:       false,
			Error:      fmt.Sprintf("Version %s found, need ≥17", parts[0]),
			Suggestion: "Upgrade PostgreSQL to version 17 or higher",
		}
	}
	return CheckResult{Name: "PostgreSQL Version", Pass: true, Details: parts[0]}
}

func checkMigrations(cfg *config.Config) CheckResult {
	version, dirty, err := database.GetMigrationVersion(cfg.DatabaseURL)
	if err != nil {
		return CheckResult{
			Name:       "Database Migrations",
			Pass:       false,
			Error:      err.Error(),
			Suggestion: "Run migrations with: kaunta migrate up",
		}
	}

	expectedVersion := uint(12)
	if version != expectedVersion {
		return CheckResult{
			Name:       "Database Migrations",
			Pass:       false,
			Error:      fmt.Sprintf("Migration version %d, expected %d", version, expectedVersion),
			Suggestion: "Run migrations with: kaunta migrate up",
		}
	}

	if dirty {
		return CheckResult{
			Name:       "Database Migrations",
			Pass:       false,
			Error:      "Migration state is dirty",
			Suggestion: "Fix dirty migration state, may need manual intervention",
		}
	}

	return CheckResult{Name: "Database Migrations", Pass: true, Details: fmt.Sprintf("v%d", version)}
}

func checkPostgreSQLFunctions(db *sql.DB) CheckResult {
	query := `
		SELECT proname
		FROM pg_proc
		JOIN pg_namespace ON pg_proc.pronamespace = pg_namespace.oid
		WHERE nspname = 'public' AND proname = ANY($1)
	`

	rows, err := db.Query(query, pq.Array(requiredFunctions))
	if err != nil {
		return CheckResult{Name: "PostgreSQL Functions", Pass: false, Error: err.Error()}
	}
	defer func() { _ = rows.Close() }()

	foundFunctions := make(map[string]bool)
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		foundFunctions[name] = true
	}

	var missing []string
	for _, fn := range requiredFunctions {
		if !foundFunctions[fn] {
			missing = append(missing, fn)
		}
	}

	if len(missing) > 0 {
		return CheckResult{
			Name:       "PostgreSQL Functions",
			Pass:       false,
			Error:      fmt.Sprintf("Missing %d functions: %s", len(missing), strings.Join(missing, ", ")),
			Suggestion: "Run migrations to create missing functions",
		}
	}

	return CheckResult{
		Name:    "PostgreSQL Functions",
		Pass:    true,
		Details: fmt.Sprintf("%d/%d functions found", len(requiredFunctions), len(requiredFunctions)),
	}
}

func checkPostgreSQLTriggers(db *sql.DB) CheckResult {
	query := `
		SELECT tgname, tgrelid::regclass::text
		FROM pg_trigger
		WHERE tgname = ANY($1)
	`

	triggerNames := make([]string, len(requiredTriggers))
	for i, t := range requiredTriggers {
		triggerNames[i] = t.name
	}

	rows, err := db.Query(query, pq.Array(triggerNames))
	if err != nil {
		return CheckResult{Name: "PostgreSQL Triggers", Pass: false, Error: err.Error()}
	}
	defer func() { _ = rows.Close() }()

	foundTriggers := make(map[string]string)
	for rows.Next() {
		var name, table string
		_ = rows.Scan(&name, &table)
		foundTriggers[name] = table
	}

	var missing []string
	for _, trigger := range requiredTriggers {
		if _, found := foundTriggers[trigger.name]; !found {
			missing = append(missing, trigger.name)
		}
	}

	if len(missing) > 0 {
		return CheckResult{
			Name:       "PostgreSQL Triggers",
			Pass:       false,
			Error:      fmt.Sprintf("Missing triggers: %s", strings.Join(missing, ", ")),
			Suggestion: "Run migrations to create missing triggers",
		}
	}

	return CheckResult{
		Name:    "PostgreSQL Triggers",
		Pass:    true,
		Details: fmt.Sprintf("%d/%d triggers found", len(requiredTriggers), len(requiredTriggers)),
	}
}

func checkMaterializedViews(db *sql.DB) CheckResult {
	query := `
		SELECT matviewname
		FROM pg_matviews
		WHERE schemaname = 'public' AND matviewname = ANY($1)
	`

	rows, err := db.Query(query, pq.Array(requiredMatViews))
	if err != nil {
		return CheckResult{Name: "Materialized Views", Pass: false, Error: err.Error()}
	}
	defer func() { _ = rows.Close() }()

	foundViews := make(map[string]bool)
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		foundViews[name] = true
	}

	var missing []string
	for _, view := range requiredMatViews {
		if !foundViews[view] {
			missing = append(missing, view)
		}
	}

	if len(missing) > 0 {
		return CheckResult{
			Name:       "Materialized Views",
			Pass:       false,
			Error:      fmt.Sprintf("Missing views: %s", strings.Join(missing, ", ")),
			Suggestion: "Run migrations to create missing materialized views",
		}
	}

	return CheckResult{
		Name:    "Materialized Views",
		Pass:    true,
		Details: fmt.Sprintf("%d/%d views found", len(requiredMatViews), len(requiredMatViews)),
	}
}

func runDoctor(cmd *cobra.Command, args []string) error {
	jsonOutput, _ := cmd.Flags().GetBool("json")

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("✗ Configuration Error: %v\n", err)
		return err
	}

	results := []CheckResult{}

	// Non-DB checks first
	results = append(results, checkDataDirectory(cfg))
	results = append(results, checkGeoIPDatabase(cfg))

	// Connect to database for remaining checks
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		results = append(results, CheckResult{
			Name:       "Database Connection",
			Pass:       false,
			Error:      err.Error(),
			Suggestion: "Verify DATABASE_URL is valid",
		})
	} else {
		defer func() { _ = db.Close() }()

		results = append(results, checkDatabaseConnection(db))
		results = append(results, checkPostgreSQLVersion(db))
		results = append(results, checkMigrations(cfg))
		results = append(results, checkPostgreSQLFunctions(db))
		results = append(results, checkPostgreSQLTriggers(db))
		results = append(results, checkMaterializedViews(db))
	}

	// Output results
	if jsonOutput {
		outputDoctorJSON(results)
	} else {
		outputDoctorHuman(results)
	}

	// Determine exit code
	allPassed := true
	for _, r := range results {
		if !r.Pass {
			allPassed = false
			break
		}
	}

	if !allPassed {
		os.Exit(1)
	}

	return nil
}

func outputDoctorHuman(results []CheckResult) {
	fmt.Println("\n🏥 Kaunta Health Check")

	for _, r := range results {
		icon := "✓"
		if !r.Pass {
			icon = "✗"
		}

		fmt.Printf("%s %s", icon, r.Name)
		if r.Details != "" {
			fmt.Printf(" (%s)", r.Details)
		}
		fmt.Println()

		if !r.Pass {
			if r.Error != "" {
				fmt.Printf("  Error: %s\n", r.Error)
			}
			if r.Suggestion != "" {
				fmt.Printf("  💡 %s\n", r.Suggestion)
			}
		}
	}

	// Summary
	passed := 0
	for _, r := range results {
		if r.Pass {
			passed++
		}
	}

	fmt.Printf("\n%d/%d checks passed\n\n", passed, len(results))
}

func outputDoctorJSON(results []CheckResult) {
	data, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(data))
}

func init() {
	doctorCmd.Flags().Bool("json", false, "Output results as JSON")
	RootCmd.AddCommand(doctorCmd)
}
