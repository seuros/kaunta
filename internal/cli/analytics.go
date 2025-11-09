package cli

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/seuros/kaunta/internal/database"
	"github.com/spf13/cobra"
)

// Data structures for analytics

type OverviewStats struct {
	TotalVisitors       int64            `json:"total_visitors"`
	TotalPageviews      int64            `json:"total_pageviews"`
	TopPage             *PageStat        `json:"top_page,omitempty"`
	TopReferrer         *ReferrerStat    `json:"top_referrer,omitempty"`
	BrowserDistribution map[string]int64 `json:"browser_distribution"`
	DeviceDistribution  map[string]int64 `json:"device_distribution"`
	CountryDistribution map[string]int64 `json:"country_distribution"`
	AvgEngagement       float64          `json:"avg_engagement_seconds"`
}

type PageStat struct {
	Path           string  `json:"path"`
	Pageviews      int64   `json:"pageviews"`
	UniqueVisitors int64   `json:"unique_visitors"`
	BounceRate     float64 `json:"bounce_rate"`
	AvgTime        float64 `json:"avg_time_seconds"`
}

type ReferrerStat struct {
	Domain    string `json:"domain"`
	Visitors  int64  `json:"visitors"`
	Pageviews int64  `json:"pageviews"`
}

type BreakdownStat struct {
	Dimension string                   `json:"dimension"`
	Items     []map[string]interface{} `json:"items"`
}

type LiveStatsData struct {
	Timestamp           time.Time                `json:"timestamp"`
	ActiveVisitorsNow   int64                    `json:"active_visitors_now"`
	PageviewsLastMinute int64                    `json:"pageviews_last_minute"`
	TopPageNow          *PageStat                `json:"top_page_now,omitempty"`
	RecentReferrers     []map[string]interface{} `json:"recent_referrers,omitempty"`
	RecentEvents        int64                    `json:"recent_events"`
}

// Stats command structure
var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "View analytics statistics",
	Long: `View analytics statistics and reports.

Stats commands allow you to view analytics data and generate reports from the command line.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(cmd.Help())
	},
}

var (
	getWebsiteIDByDomainFn = GetWebsiteIDByDomain
	getOverviewStats       = GetOverviewStats
	getTopPagesFn          = GetTopPages
	getBreakdownStatsFn    = GetBreakdownStats
	getLiveStatsFn         = GetLiveStats
)

// Overview command flags
var (
	overviewDays   int
	overviewFormat string
)

var statsOverviewCmd = &cobra.Command{
	Use:   "overview <website-domain> [--days <N>] [--format json|table|text]",
	Short: "Show analytics overview dashboard",
	Long: `Display a quick overview/dashboard for a website with key metrics.

Shows:
  - Total Visitors (unique sessions)
  - Total Pageviews (sum of events)
  - Top Page (most visited page)
  - Top Referrer (most common referrer_domain)
  - Browser Distribution (top 3)
  - Device Distribution (mobile, desktop, tablet)
  - Country Distribution (top 3)
  - Average Engagement (in seconds)

Options:
  --days N     Time period in days (1-365, default 7)
  --format     Output format: json, table, text (default table)`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatsOverview(args[0], overviewDays, overviewFormat)
	},
}

// Pages command flags
var (
	pagesDays   int
	pagesTop    int
	pagesFormat string
)

var statsPagesCmd = &cobra.Command{
	Use:   "pages <website-domain> [--days <N>] [--top <N>] [--format json|table|csv]",
	Short: "Show top pages by pageview count",
	Long: `Display top pages sorted by pageview count.

Columns: URL Path, Pageviews, Unique Visitors, Bounce Rate, Avg Time

Options:
  --days N      Time period in days (1-365, default 7)
  --top N       Number of pages to show (1-100, default 10)
  --format      Output format: json, table, csv (default table)`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatsPages(args[0], pagesDays, pagesTop, pagesFormat)
	},
}

// Breakdown command flags
var (
	breakdownDimension string
	breakdownDays      int
	breakdownTop       int
	breakdownFormat    string
)

var statsBreakdownCmd = &cobra.Command{
	Use:   "breakdown <website-domain> --by <dimension> [--days <N>] [--top <N>] [--format json|table|csv]",
	Short: "Show metrics breakdown by dimension",
	Long: `Display metrics broken down by a specific dimension.

Valid dimensions:
  country  - Country Name, Visitors, Pageviews, Bounce Rate
  browser  - Browser, Visitors, Pageviews, Bounce Rate
  device   - Device Type, Visitors, Pageviews, Bounce Rate
  referrer - Referrer Domain, Visitors, Pageviews, Bounce Rate
  os       - OS, Visitors, Pageviews, Bounce Rate

Options:
  --by          Dimension to break down by (required)
  --days N      Time period in days (1-365, default 7)
  --top N       Number of items to show (1-100, default 10)
  --format      Output format: json, table, csv (default table)

Examples:
  kaunta stats breakdown mysite.com --by country
  kaunta stats breakdown mysite.com --by browser --top 5 --days 30`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatsBreakdown(args[0], breakdownDimension, breakdownDays, breakdownTop, breakdownFormat)
	},
}

// Live command flags
var (
	liveInterval int
	liveFormat   string
)

var statsLiveCmd = &cobra.Command{
	Use:   "live <website-domain> [--interval <seconds>] [--format json|text]",
	Short: "Real-time streaming stats",
	Long: `Display real-time streaming statistics that update every N seconds.

Shows:
  - Active visitors (last 5 minutes)
  - Pageviews in last minute
  - Top page right now
  - Recent referrers
  - Recent events

Options:
  --interval N  Update interval in seconds (2-60, default 5)
  --format      Output format: json, text (default text)

Press Ctrl+C to stop.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatsLive(args[0], liveInterval, liveFormat)
	},
}

// Command implementations

func runStatsOverview(domain string, days int, format string) error {
	if days < 1 || days > 365 {
		return fmt.Errorf("days must be between 1 and 365")
	}

	if format == "" {
		format = "table"
	}

	if database.DB == nil {
		if err := connectDatabase(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = closeDatabase() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get website ID
	websiteID, err := getWebsiteIDByDomainFn(ctx, domain)
	if err != nil {
		return err
	}

	stats, err := getOverviewStats(ctx, database.DB, websiteID, days)
	if err != nil {
		return err
	}

	switch format {
	case "json":
		return outputOverviewJSON(stats)
	case "text":
		return outputOverviewText(stats, domain, days)
	case "table":
		return outputOverviewTable(stats, domain, days)
	default:
		return fmt.Errorf("invalid format: %s (use json, table, or text)", format)
	}
}

func runStatsPages(domain string, days int, top int, format string) error {
	if days < 1 || days > 365 {
		return fmt.Errorf("days must be between 1 and 365")
	}

	if top < 1 || top > 100 {
		return fmt.Errorf("top must be between 1 and 100")
	}

	if format == "" {
		format = "table"
	}

	if database.DB == nil {
		if err := connectDatabase(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = closeDatabase() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	websiteID, err := getWebsiteIDByDomainFn(ctx, domain)
	if err != nil {
		return err
	}

	pages, err := getTopPagesFn(ctx, database.DB, websiteID, days, top)
	if err != nil {
		return err
	}

	switch format {
	case "json":
		return outputPagesJSON(pages)
	case "csv":
		return outputPagesCSV(pages)
	case "table":
		return outputPagesTable(pages)
	default:
		return fmt.Errorf("invalid format: %s (use json, table, or csv)", format)
	}
}

func runStatsBreakdown(domain string, dimension string, days int, top int, format string) error {
	if dimension == "" {
		return fmt.Errorf("--by dimension is required (valid: country, browser, device, referrer, os)")
	}

	validDimensions := map[string]bool{
		"country":  true,
		"browser":  true,
		"device":   true,
		"referrer": true,
		"os":       true,
	}

	if !validDimensions[dimension] {
		return fmt.Errorf("invalid dimension: %s (valid: country, browser, device, referrer, os)", dimension)
	}

	if days < 1 || days > 365 {
		return fmt.Errorf("days must be between 1 and 365")
	}

	if top < 1 || top > 100 {
		return fmt.Errorf("top must be between 1 and 100")
	}

	if format == "" {
		format = "table"
	}

	if database.DB == nil {
		if err := connectDatabase(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = closeDatabase() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	websiteID, err := getWebsiteIDByDomainFn(ctx, domain)
	if err != nil {
		return err
	}

	stats, err := getBreakdownStatsFn(ctx, database.DB, websiteID, dimension, days, top)
	if err != nil {
		return err
	}

	switch format {
	case "json":
		return outputBreakdownJSON(stats)
	case "csv":
		return outputBreakdownCSV(stats)
	case "table":
		return outputBreakdownTable(stats)
	default:
		return fmt.Errorf("invalid format: %s (use json, table, or csv)", format)
	}
}

func runStatsLive(domain string, interval int, format string) error {
	if interval < 2 || interval > 60 {
		interval = 5
	}

	if format == "" {
		format = "text"
	}

	if database.DB == nil {
		if err := connectDatabase(); err != nil {
			return fmt.Errorf("database connection failed: %w", err)
		}
		defer func() { _ = closeDatabase() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()

	websiteID, err := getWebsiteIDByDomainFn(ctx, domain)
	if err != nil {
		return err
	}

	// Setup signal handler for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	fmt.Printf("Live stats for %s (updating every %d seconds, press Ctrl+C to exit)\n\n", domain, interval)

	// Display initial stats
	liveData, _ := getLiveStatsFn(ctx, database.DB, websiteID)
	if format == "json" {
		_ = outputLiveJSON(liveData)
	} else {
		_ = outputLiveTerm(liveData)
	}

	for {
		select {
		case <-sigChan:
			fmt.Println("\n\nExiting live stats...")
			return nil
		case <-ticker.C:
			liveData, err := getLiveStatsFn(ctx, database.DB, websiteID)
			if err != nil {
				fmt.Printf("Error fetching live stats: %v\n", err)
				continue
			}

			if format == "json" {
				_ = outputLiveJSON(liveData)
			} else {
				_ = outputLiveTerm(liveData)
			}
		}
	}
}

// Helper functions to query database

func GetWebsiteIDByDomain(ctx context.Context, domain string) (string, error) {
	var websiteID string
	query := `SELECT website_id FROM website WHERE domain = $1 AND deleted_at IS NULL`
	err := database.DB.QueryRowContext(ctx, query, domain).Scan(&websiteID)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("website not found: %s", domain)
	}
	if err != nil {
		return "", fmt.Errorf("failed to query website: %w", err)
	}
	return websiteID, nil
}

func GetOverviewStats(ctx context.Context, db *sql.DB, websiteID string, days int) (*OverviewStats, error) {
	stats := &OverviewStats{
		BrowserDistribution: make(map[string]int64),
		DeviceDistribution:  make(map[string]int64),
		CountryDistribution: make(map[string]int64),
	}

	// Parse UUID
	parsedID, err := uuid.Parse(websiteID)
	if err != nil {
		return nil, fmt.Errorf("invalid website ID: %w", err)
	}

	// Total unique visitors
	query := `
		SELECT COUNT(DISTINCT e.session_id)
		FROM website_event e
		WHERE e.website_id = $1
		  AND e.created_at >= NOW() - INTERVAL '1 day' * $2
		  AND e.event_type = 1`

	err = db.QueryRowContext(ctx, query, parsedID, days).Scan(&stats.TotalVisitors)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query visitors: %w", err)
	}

	// Total pageviews
	query = `
		SELECT COUNT(*)
		FROM website_event e
		WHERE e.website_id = $1
		  AND e.created_at >= NOW() - INTERVAL '1 day' * $2
		  AND e.event_type = 1`

	err = db.QueryRowContext(ctx, query, parsedID, days).Scan(&stats.TotalPageviews)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query pageviews: %w", err)
	}

	// Top page
	topPage, err := getTopPageDetail(ctx, db, parsedID, days)
	if err == nil && topPage != nil {
		stats.TopPage = topPage
	}

	// Top referrer
	topRef, err := getTopReferrer(ctx, db, parsedID, days)
	if err == nil && topRef != nil {
		stats.TopReferrer = topRef
	}

	// Browser distribution (top 3)
	browsers, err := getBrowserDistribution(ctx, db, parsedID, days, 3)
	if err == nil {
		stats.BrowserDistribution = browsers
	}

	// Device distribution
	devices, err := getDeviceDistribution(ctx, db, parsedID, days)
	if err == nil {
		stats.DeviceDistribution = devices
	}

	// Country distribution (top 3)
	countries, err := getCountryDistribution(ctx, db, parsedID, days, 3)
	if err == nil {
		stats.CountryDistribution = countries
	}

	// Average engagement time
	avgTime, err := getAverageEngagement(ctx, db, parsedID, days)
	if err == nil {
		stats.AvgEngagement = avgTime
	}

	return stats, nil
}

func GetTopPages(ctx context.Context, db *sql.DB, websiteID string, days int, limit int) ([]*PageStat, error) {
	parsedID, err := uuid.Parse(websiteID)
	if err != nil {
		return nil, fmt.Errorf("invalid website ID: %w", err)
	}

	query := `
		SELECT
			e.url_path,
			COUNT(*) as pageviews,
			COUNT(DISTINCT e.session_id) as unique_visitors
		FROM website_event e
		WHERE e.website_id = $1
		  AND e.created_at >= NOW() - INTERVAL '1 day' * $2
		  AND e.event_type = 1
		  AND e.url_path IS NOT NULL
		GROUP BY e.url_path
		ORDER BY pageviews DESC
		LIMIT $3`

	rows, err := db.QueryContext(ctx, query, parsedID, days, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top pages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var pages []*PageStat
	for rows.Next() {
		var path string
		var pageviews, uniqueVisitors int64

		if err := rows.Scan(&path, &pageviews, &uniqueVisitors); err != nil {
			continue
		}

		// Calculate bounce rate for this page
		bounceRate := calculatePageBounceRate(ctx, db, parsedID, path, days)

		// Calculate average time on page
		avgTime := calculatePageAvgTime(ctx, db, parsedID, path, days)

		pages = append(pages, &PageStat{
			Path:           path,
			Pageviews:      pageviews,
			UniqueVisitors: uniqueVisitors,
			BounceRate:     bounceRate,
			AvgTime:        avgTime,
		})
	}

	return pages, rows.Err()
}

func GetBreakdownStats(ctx context.Context, db *sql.DB, websiteID string, dimension string, days int, limit int) (*BreakdownStat, error) {
	parsedID, err := uuid.Parse(websiteID)
	if err != nil {
		return nil, fmt.Errorf("invalid website ID: %w", err)
	}

	var query string
	var column string

	switch dimension {
	case "country":
		column = "COALESCE(s.country, 'Unknown')"
	case "browser":
		column = "COALESCE(s.browser, 'Unknown')"
	case "device":
		column = "COALESCE(s.device, 'Unknown')"
	case "referrer":
		column = "COALESCE(e.referrer_domain, 'Direct / None')"
	case "os":
		column = "COALESCE(s.os, 'Unknown')"
	default:
		return nil, fmt.Errorf("invalid dimension: %s", dimension)
	}

	// Join with session if needed
	var joinClause string
	if dimension != "referrer" {
		joinClause = "JOIN session s ON e.session_id = s.session_id"
	} else {
		joinClause = "JOIN session s ON e.session_id = s.session_id"
	}

	query = fmt.Sprintf(`
		SELECT
			%s as name,
			COUNT(DISTINCT e.session_id) as visitors,
			COUNT(*) as pageviews
		FROM website_event e
		%s
		WHERE e.website_id = $1
		  AND e.created_at >= NOW() - INTERVAL '1 day' * $2
		  AND e.event_type = 1
		GROUP BY %s
		ORDER BY visitors DESC
		LIMIT $3`, column, joinClause, column)

	rows, err := db.QueryContext(ctx, query, parsedID, days, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query breakdown: %w", err)
	}
	defer func() { _ = rows.Close() }()

	stats := &BreakdownStat{
		Dimension: dimension,
		Items:     []map[string]interface{}{},
	}

	for rows.Next() {
		var name string
		var visitors, pageviews int64

		if err := rows.Scan(&name, &visitors, &pageviews); err != nil {
			continue
		}

		// Calculate bounce rate for this dimension value
		bounceRate := calculateDimensionBounceRate(ctx, db, parsedID, dimension, name, days)

		item := map[string]interface{}{
			"name":        name,
			"visitors":    visitors,
			"pageviews":   pageviews,
			"bounce_rate": bounceRate,
		}

		stats.Items = append(stats.Items, item)
	}

	return stats, rows.Err()
}

func GetLiveStats(ctx context.Context, db *sql.DB, websiteID string) (*LiveStatsData, error) {
	parsedID, err := uuid.Parse(websiteID)
	if err != nil {
		return nil, fmt.Errorf("invalid website ID: %w", err)
	}

	liveData := &LiveStatsData{
		Timestamp: time.Now(),
	}

	// Active visitors (last 5 minutes)
	query := `
		SELECT COUNT(DISTINCT e.session_id)
		FROM website_event e
		WHERE e.website_id = $1
		  AND e.created_at >= NOW() - INTERVAL '5 minutes'
		  AND e.event_type = 1`

	_ = db.QueryRowContext(ctx, query, parsedID).Scan(&liveData.ActiveVisitorsNow)

	// Pageviews last minute
	query = `
		SELECT COUNT(*)
		FROM website_event e
		WHERE e.website_id = $1
		  AND e.created_at >= NOW() - INTERVAL '1 minute'
		  AND e.event_type = 1`

	_ = db.QueryRowContext(ctx, query, parsedID).Scan(&liveData.PageviewsLastMinute)

	// Top page right now
	topPage, _ := getTopPageDetail(ctx, db, parsedID, 0) // 0 = last 5 minutes
	liveData.TopPageNow = topPage

	// Recent referrers
	liveData.RecentReferrers, _ = getRecentReferrers(ctx, db, parsedID)

	// Recent events count
	query = `
		SELECT COUNT(*)
		FROM website_event e
		WHERE e.website_id = $1
		  AND e.created_at >= NOW() - INTERVAL '5 minutes'
		  AND e.event_type = 1`

	_ = db.QueryRowContext(ctx, query, parsedID).Scan(&liveData.RecentEvents)

	return liveData, nil
}

// Helper utility functions

func getTopPageDetail(ctx context.Context, db *sql.DB, websiteID uuid.UUID, days int) (*PageStat, error) {
	var query string

	if days == 0 {
		// Last 5 minutes
		query = `
			SELECT e.url_path, COUNT(*) as pageviews, COUNT(DISTINCT e.session_id) as unique_visitors
			FROM website_event e
			WHERE e.website_id = $1
			  AND e.created_at >= NOW() - INTERVAL '5 minutes'
			  AND e.event_type = 1
			  AND e.url_path IS NOT NULL
			GROUP BY e.url_path
			ORDER BY pageviews DESC
			LIMIT 1`

		var path string
		var pageviews, uniqueVisitors int64

		err := db.QueryRowContext(ctx, query, websiteID).Scan(&path, &pageviews, &uniqueVisitors)
		if err != nil {
			return nil, err
		}

		return &PageStat{
			Path:           path,
			Pageviews:      pageviews,
			UniqueVisitors: uniqueVisitors,
		}, nil
	}

	query = `
		SELECT e.url_path, COUNT(*) as pageviews, COUNT(DISTINCT e.session_id) as unique_visitors
		FROM website_event e
		WHERE e.website_id = $1
		  AND e.created_at >= NOW() - INTERVAL '1 day' * $2
		  AND e.event_type = 1
		  AND e.url_path IS NOT NULL
		GROUP BY e.url_path
		ORDER BY pageviews DESC
		LIMIT 1`

	var path string
	var pageviews, uniqueVisitors int64

	err := db.QueryRowContext(ctx, query, websiteID, days).Scan(&path, &pageviews, &uniqueVisitors)
	if err != nil {
		return nil, err
	}

	return &PageStat{
		Path:           path,
		Pageviews:      pageviews,
		UniqueVisitors: uniqueVisitors,
	}, nil
}

func getTopReferrer(ctx context.Context, db *sql.DB, websiteID uuid.UUID, days int) (*ReferrerStat, error) {
	query := `
		SELECT
			COALESCE(e.referrer_domain, 'Direct / None') as domain,
			COUNT(DISTINCT e.session_id) as visitors,
			COUNT(*) as pageviews
		FROM website_event e
		WHERE e.website_id = $1
		  AND e.created_at >= NOW() - INTERVAL '1 day' * $2
		  AND e.event_type = 1
		GROUP BY e.referrer_domain
		ORDER BY visitors DESC
		LIMIT 1`

	var domain string
	var visitors, pageviews int64

	err := db.QueryRowContext(ctx, query, websiteID, days).Scan(&domain, &visitors, &pageviews)
	if err != nil {
		return nil, err
	}

	return &ReferrerStat{
		Domain:    domain,
		Visitors:  visitors,
		Pageviews: pageviews,
	}, nil
}

func getBrowserDistribution(ctx context.Context, db *sql.DB, websiteID uuid.UUID, days int, limit int) (map[string]int64, error) {
	query := `
		SELECT COALESCE(s.browser, 'Unknown') as browser, COUNT(DISTINCT e.session_id) as visitors
		FROM website_event e
		JOIN session s ON e.session_id = s.session_id
		WHERE e.website_id = $1
		  AND e.created_at >= NOW() - INTERVAL '1 day' * $2
		  AND e.event_type = 1
		GROUP BY s.browser
		ORDER BY visitors DESC
		LIMIT $3`

	rows, err := db.QueryContext(ctx, query, websiteID, days, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	distribution := make(map[string]int64)
	for rows.Next() {
		var browser string
		var visitors int64

		if err := rows.Scan(&browser, &visitors); err != nil {
			continue
		}

		distribution[browser] = visitors
	}

	return distribution, rows.Err()
}

func getDeviceDistribution(ctx context.Context, db *sql.DB, websiteID uuid.UUID, days int) (map[string]int64, error) {
	query := `
		SELECT COALESCE(s.device, 'Unknown') as device, COUNT(DISTINCT e.session_id) as visitors
		FROM website_event e
		JOIN session s ON e.session_id = s.session_id
		WHERE e.website_id = $1
		  AND e.created_at >= NOW() - INTERVAL '1 day' * $2
		  AND e.event_type = 1
		GROUP BY s.device
		ORDER BY visitors DESC`

	rows, err := db.QueryContext(ctx, query, websiteID, days)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	distribution := make(map[string]int64)
	for rows.Next() {
		var device string
		var visitors int64

		if err := rows.Scan(&device, &visitors); err != nil {
			continue
		}

		distribution[device] = visitors
	}

	return distribution, rows.Err()
}

func getCountryDistribution(ctx context.Context, db *sql.DB, websiteID uuid.UUID, days int, limit int) (map[string]int64, error) {
	query := `
		SELECT COALESCE(s.country, 'Unknown') as country, COUNT(DISTINCT e.session_id) as visitors
		FROM website_event e
		JOIN session s ON e.session_id = s.session_id
		WHERE e.website_id = $1
		  AND e.created_at >= NOW() - INTERVAL '1 day' * $2
		  AND e.event_type = 1
		GROUP BY s.country
		ORDER BY visitors DESC
		LIMIT $3`

	rows, err := db.QueryContext(ctx, query, websiteID, days, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	distribution := make(map[string]int64)
	for rows.Next() {
		var country string
		var visitors int64

		if err := rows.Scan(&country, &visitors); err != nil {
			continue
		}

		distribution[country] = visitors
	}

	return distribution, rows.Err()
}

func getAverageEngagement(ctx context.Context, db *sql.DB, websiteID uuid.UUID, days int) (float64, error) {
	// Calculate average time between first and last pageview per session
	query := `
		SELECT AVG(engagement_time)
		FROM (
			SELECT
				e.session_id,
				EXTRACT(EPOCH FROM (MAX(e.created_at) - MIN(e.created_at))) as engagement_time
			FROM website_event e
			WHERE e.website_id = $1
			  AND e.created_at >= NOW() - INTERVAL '1 day' * $2
			  AND e.event_type = 1
			GROUP BY e.session_id
		) session_engagement`

	var avgTime sql.NullFloat64
	err := db.QueryRowContext(ctx, query, websiteID, days).Scan(&avgTime)
	if err != nil || !avgTime.Valid {
		return 0, nil
	}

	return avgTime.Float64, nil
}

func calculatePageBounceRate(ctx context.Context, db *sql.DB, websiteID uuid.UUID, path string, days int) float64 {
	query := `
		SELECT
			COUNT(DISTINCT CASE WHEN pageview_count = 1 THEN e.session_id END)::float / NULLIF(COUNT(DISTINCT e.session_id), 0) * 100 as bounce_rate
		FROM website_event e
		LEFT JOIN (
			SELECT session_id, COUNT(*) as pageview_count
			FROM website_event
			WHERE website_id = $1
			  AND created_at >= NOW() - INTERVAL '1 day' * $2
			  AND event_type = 1
			GROUP BY session_id
		) pv ON e.session_id = pv.session_id
		WHERE e.website_id = $1
		  AND e.url_path = $3
		  AND e.created_at >= NOW() - INTERVAL '1 day' * $2
		  AND e.event_type = 1`

	var bounceRate sql.NullFloat64
	_ = db.QueryRowContext(ctx, query, websiteID, days, path).Scan(&bounceRate)

	if bounceRate.Valid {
		return bounceRate.Float64
	}
	return 0
}

func calculatePageAvgTime(ctx context.Context, db *sql.DB, websiteID uuid.UUID, path string, days int) float64 {
	query := `
		SELECT AVG(engagement_time)
		FROM (
			SELECT
				e.session_id,
				EXTRACT(EPOCH FROM (MAX(e.created_at) - MIN(e.created_at))) as engagement_time
			FROM website_event e
			WHERE e.website_id = $1
			  AND e.url_path = $2
			  AND e.created_at >= NOW() - INTERVAL '1 day' * $3
			  AND e.event_type = 1
			GROUP BY e.session_id
		) session_engagement`

	var avgTime sql.NullFloat64
	_ = db.QueryRowContext(ctx, query, websiteID, path, days).Scan(&avgTime)

	if avgTime.Valid {
		return avgTime.Float64
	}
	return 0
}

func calculateDimensionBounceRate(ctx context.Context, db *sql.DB, websiteID uuid.UUID, dimension string, value string, days int) float64 {
	var column string
	var table string

	switch dimension {
	case "country":
		column = "s.country"
		table = "JOIN session s ON e.session_id = s.session_id"
	case "browser":
		column = "s.browser"
		table = "JOIN session s ON e.session_id = s.session_id"
	case "device":
		column = "s.device"
		table = "JOIN session s ON e.session_id = s.session_id"
	case "referrer":
		column = "e.referrer_domain"
		table = "JOIN session s ON e.session_id = s.session_id"
	case "os":
		column = "s.os"
		table = "JOIN session s ON e.session_id = s.session_id"
	default:
		return 0
	}

	var whereClause string
	if dimension == "referrer" {
		whereClause = fmt.Sprintf("COALESCE(%s, 'Direct / None') = $3", column)
	} else {
		whereClause = fmt.Sprintf("COALESCE(%s, 'Unknown') = $3", column)
	}

	query := fmt.Sprintf(`
		SELECT
			COUNT(DISTINCT CASE WHEN pageview_count = 1 THEN e.session_id END)::float / NULLIF(COUNT(DISTINCT e.session_id), 0) * 100 as bounce_rate
		FROM website_event e
		%s
		LEFT JOIN (
			SELECT session_id, COUNT(*) as pageview_count
			FROM website_event
			WHERE website_id = $1
			  AND created_at >= NOW() - INTERVAL '1 day' * $2
			  AND event_type = 1
			GROUP BY session_id
		) pv ON e.session_id = pv.session_id
		WHERE e.website_id = $1
		  AND %s
		  AND e.created_at >= NOW() - INTERVAL '1 day' * $2
		  AND e.event_type = 1`, table, whereClause)

	var bounceRate sql.NullFloat64
	_ = db.QueryRowContext(ctx, query, websiteID, days, value).Scan(&bounceRate)

	if bounceRate.Valid {
		return bounceRate.Float64
	}
	return 0
}

func getRecentReferrers(ctx context.Context, db *sql.DB, websiteID uuid.UUID) ([]map[string]interface{}, error) {
	query := `
		SELECT
			COALESCE(e.referrer_domain, 'Direct / None') as referrer,
			COUNT(*) as count
		FROM website_event e
		WHERE e.website_id = $1
		  AND e.created_at >= NOW() - INTERVAL '5 minutes'
		  AND e.event_type = 1
		GROUP BY e.referrer_domain
		ORDER BY count DESC
		LIMIT 5`

	rows, err := db.QueryContext(ctx, query, websiteID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var referrers []map[string]interface{}
	for rows.Next() {
		var referrer string
		var count int64

		if err := rows.Scan(&referrer, &count); err != nil {
			continue
		}

		referrers = append(referrers, map[string]interface{}{
			"referrer": referrer,
			"count":    count,
		})
	}

	return referrers, rows.Err()
}

// Output formatting functions

func outputOverviewJSON(stats *OverviewStats) error {
	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func outputOverviewText(stats *OverviewStats, domain string, days int) error {
	fmt.Printf("Analytics Overview for %s (last %d days)\n", domain, days)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("\nTotal Visitors:        %d\n", stats.TotalVisitors)
	fmt.Printf("Total Pageviews:       %d\n", stats.TotalPageviews)

	if stats.TotalVisitors > 0 {
		fmt.Printf("Avg Pageviews/Visitor: %.1f\n", float64(stats.TotalPageviews)/float64(stats.TotalVisitors))
	}

	fmt.Printf("Avg Engagement Time:   %.1f seconds\n\n", stats.AvgEngagement)

	if stats.TopPage != nil {
		fmt.Printf("Top Page:              %s (%d pageviews)\n\n", stats.TopPage.Path, stats.TopPage.Pageviews)
	}

	if stats.TopReferrer != nil {
		fmt.Printf("Top Referrer:          %s (%d visitors)\n\n", stats.TopReferrer.Domain, stats.TopReferrer.Visitors)
	}

	fmt.Println("Browser Distribution:")
	for browser, count := range stats.BrowserDistribution {
		fmt.Printf("  %s: %d\n", browser, count)
	}

	fmt.Println("\nDevice Distribution:")
	for device, count := range stats.DeviceDistribution {
		fmt.Printf("  %s: %d\n", device, count)
	}

	fmt.Println("\nTop Countries:")
	for country, count := range stats.CountryDistribution {
		fmt.Printf("  %s: %d\n", country, count)
	}

	return nil
}

func outputOverviewTable(stats *OverviewStats, domain string, days int) error {
	fmt.Printf("Analytics Overview for %s (last %d days)\n", domain, days)
	fmt.Println(strings.Repeat("=", 60))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	_, _ = fmt.Fprintf(w, "Total Visitors:\t%d\n", stats.TotalVisitors)
	_, _ = fmt.Fprintf(w, "Total Pageviews:\t%d\n", stats.TotalPageviews)
	_, _ = fmt.Fprintf(w, "Avg Engagement Time:\t%.1f seconds\n\n", stats.AvgEngagement)

	if stats.TopPage != nil {
		_, _ = fmt.Fprintf(w, "Top Page:\t%s (%d pageviews)\n", stats.TopPage.Path, stats.TopPage.Pageviews)
	}

	if stats.TopReferrer != nil {
		_, _ = fmt.Fprintf(w, "Top Referrer:\t%s (%d visitors)\n\n", stats.TopReferrer.Domain, stats.TopReferrer.Visitors)
	}

	_ = w.Flush()

	// Browser distribution
	fmt.Println("Browser Distribution:")
	for browser, count := range stats.BrowserDistribution {
		fmt.Printf("  %s: %d\n", browser, count)
	}

	// Device distribution
	fmt.Println("\nDevice Distribution:")
	for device, count := range stats.DeviceDistribution {
		fmt.Printf("  %s: %d\n", device, count)
	}

	// Country distribution
	fmt.Println("\nTop Countries:")
	for country, count := range stats.CountryDistribution {
		fmt.Printf("  %s: %d\n", country, count)
	}

	return nil
}

func outputPagesJSON(pages []*PageStat) error {
	data, err := json.MarshalIndent(pages, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func outputPagesTable(pages []*PageStat) error {
	if len(pages) == 0 {
		fmt.Println("No page data available")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }()

	_, _ = fmt.Fprintln(w, "PATH\tPAGEVIEWS\tUNIQUE VISITORS\tBOUNCE RATE\tAVG TIME")
	_, _ = fmt.Fprintln(w, "----\t----------\t---------------\t-----------\t--------")

	for _, page := range pages {
		_, _ = fmt.Fprintf(w, "%s\t%d\t%d\t%.1f%%\t%.1fs\n",
			page.Path,
			page.Pageviews,
			page.UniqueVisitors,
			page.BounceRate,
			page.AvgTime,
		)
	}

	return nil
}

func outputPagesCSV(pages []*PageStat) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	// Write header
	err := w.Write([]string{"path", "pageviews", "unique_visitors", "bounce_rate", "avg_time_seconds"})
	if err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write rows
	for _, page := range pages {
		err := w.Write([]string{
			page.Path,
			fmt.Sprintf("%d", page.Pageviews),
			fmt.Sprintf("%d", page.UniqueVisitors),
			fmt.Sprintf("%.1f", page.BounceRate),
			fmt.Sprintf("%.1f", page.AvgTime),
		})
		if err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	return nil
}

func outputBreakdownJSON(stats *BreakdownStat) error {
	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func outputBreakdownTable(stats *BreakdownStat) error {
	if len(stats.Items) == 0 {
		fmt.Printf("No data available for dimension: %s\n", stats.Dimension)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }()

	_, _ = fmt.Fprintf(w, "NAME\tVISITORS\tPAGEVIEWS\tBOUNCE RATE\n")
	_, _ = fmt.Fprintf(w, "----\t--------\t---------\t-----------\n")

	for _, item := range stats.Items {
		_, _ = fmt.Fprintf(w, "%v\t%v\t%v\t%.1f%%\n",
			item["name"],
			item["visitors"],
			item["pageviews"],
			item["bounce_rate"],
		)
	}

	return nil
}

func outputBreakdownCSV(stats *BreakdownStat) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	// Write header
	err := w.Write([]string{"name", "visitors", "pageviews", "bounce_rate"})
	if err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write rows
	for _, item := range stats.Items {
		err := w.Write([]string{
			fmt.Sprintf("%v", item["name"]),
			fmt.Sprintf("%v", item["visitors"]),
			fmt.Sprintf("%v", item["pageviews"]),
			fmt.Sprintf("%.1f", item["bounce_rate"]),
		})
		if err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	return nil
}

func outputLiveJSON(data *LiveStatsData) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Printf("Error marshaling JSON: %v\n", err)
		return nil
	}
	fmt.Println(string(jsonData))
	return nil
}

func outputLiveTerm(data *LiveStatsData) error {
	// Clear screen (works on Unix-like systems)
	fmt.Print("\033[2J\033[H")

	fmt.Printf("Live Analytics - %s\n", data.Timestamp.Format("15:04:05"))
	fmt.Println(strings.Repeat("=", 60))

	fmt.Printf("\nActive Visitors (last 5 min): %d\n", data.ActiveVisitorsNow)
	fmt.Printf("Pageviews (last minute):      %d\n", data.PageviewsLastMinute)
	fmt.Printf("Recent Events (last 5 min):   %d\n\n", data.RecentEvents)

	if data.TopPageNow != nil {
		fmt.Printf("Top Page Now: %s (%d pageviews)\n\n", data.TopPageNow.Path, data.TopPageNow.Pageviews)
	}

	if len(data.RecentReferrers) > 0 {
		fmt.Println("Recent Referrers:")
		for _, ref := range data.RecentReferrers {
			fmt.Printf("  %v: %v\n", ref["referrer"], ref["count"])
		}
	}

	fmt.Printf("\nPress Ctrl+C to exit\n")
	return nil
}

func init() {
	// Add subcommands to stats
	statsCmd.AddCommand(statsOverviewCmd)
	statsCmd.AddCommand(statsPagesCmd)
	statsCmd.AddCommand(statsBreakdownCmd)
	statsCmd.AddCommand(statsLiveCmd)

	// Overview command flags
	statsOverviewCmd.Flags().IntVarP(&overviewDays, "days", "d", 7, "Time period in days (1-365)")
	statsOverviewCmd.Flags().StringVarP(&overviewFormat, "format", "f", "table", "Output format (json, table, text)")

	// Pages command flags
	statsPagesCmd.Flags().IntVarP(&pagesDays, "days", "d", 7, "Time period in days (1-365)")
	statsPagesCmd.Flags().IntVarP(&pagesTop, "top", "t", 10, "Number of pages to show (1-100)")
	statsPagesCmd.Flags().StringVarP(&pagesFormat, "format", "f", "table", "Output format (json, table, csv)")

	// Breakdown command flags
	statsBreakdownCmd.Flags().StringVarP(&breakdownDimension, "by", "b", "", "Dimension to break down by (required: country, browser, device, referrer, os)")
	statsBreakdownCmd.Flags().IntVarP(&breakdownDays, "days", "d", 7, "Time period in days (1-365)")
	statsBreakdownCmd.Flags().IntVarP(&breakdownTop, "top", "t", 10, "Number of items to show (1-100)")
	statsBreakdownCmd.Flags().StringVarP(&breakdownFormat, "format", "f", "table", "Output format (json, table, csv)")

	// Live command flags
	statsLiveCmd.Flags().IntVarP(&liveInterval, "interval", "i", 5, "Update interval in seconds (2-60)")
	statsLiveCmd.Flags().StringVarP(&liveFormat, "format", "f", "text", "Output format (json, text)")
}
