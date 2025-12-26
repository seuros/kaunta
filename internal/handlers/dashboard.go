package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/middleware"
)

// Website represents a website for the dashboard selector
type WebsiteInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

// HandleDashboardInit initializes the dashboard with websites list and initial data
// GET /api/dashboard/init-ds
func HandleDashboardInit(c fiber.Ctx) error {
	// Get user from context
	user := middleware.GetUser(c)
	if user == nil {
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"websitesError":   "Not authenticated",
				"websitesLoading": false,
			})
		})
	}

	// Query websites for this user BEFORE streaming
	var websites []WebsiteInfo
	var queryErr error

	query := `
		SELECT w.website_id, COALESCE(w.name, ''), w.domain
		FROM website w
		JOIN user_website uw ON w.website_id = uw.website_id
		WHERE uw.user_id = $1
		ORDER BY w.domain
	`
	rows, err := database.DB.Query(query, user.UserID)
	if err != nil {
		queryErr = err
	} else {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var w WebsiteInfo
			if err := rows.Scan(&w.ID, &w.Name, &w.Domain); err != nil {
				continue
			}
			websites = append(websites, w)
		}
	}

	// Determine selected website
	selectedWebsite := c.Query("website")
	if selectedWebsite == "" && len(websites) > 0 {
		selectedWebsite = websites[0].ID
	}

	// Query stats if we have a selected website
	var currentVisitors, todayPageviews, todayVisitors int64
	var bounceRateNumeric float64
	var statsErr error

	if selectedWebsite != "" {
		websiteID, parseErr := uuid.Parse(selectedWebsite)
		if parseErr == nil {
			statsQuery := `SELECT * FROM get_dashboard_stats($1, 1, $2, $3, $4, $5)`
			statsErr = database.DB.QueryRow(
				statsQuery,
				websiteID,
				nil, // country
				nil, // browser
				nil, // device
				nil, // page
			).Scan(&currentVisitors, &todayPageviews, &todayVisitors, &bounceRateNumeric)
		}
	}

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)

		if queryErr != nil {
			_ = sse.PatchSignals(map[string]any{
				"websitesError":   "Failed to load websites",
				"websitesLoading": false,
				"websites":        []WebsiteInfo{},
			})
			return
		}

		// Build stats object
		bounceRate := "0%"
		if statsErr == nil {
			bounceRate = fmt.Sprintf("%.1f%%", bounceRateNumeric)
		}

		_ = sse.PatchSignals(map[string]any{
			"websites":        websites,
			"selectedWebsite": selectedWebsite,
			"websitesLoading": false,
			"websitesError":   false,
			"stats": map[string]any{
				"current_visitors":  currentVisitors,
				"today_pageviews":   todayPageviews,
				"today_visitors":    todayVisitors,
				"today_bounce_rate": bounceRate,
			},
		})
	})
}

// HandleDashboardStats returns dashboard stats via Datastar SSE
// GET /api/dashboard/stats-ds?website_id=...&country=...&browser=...&device=...&page=...
func HandleDashboardStats(c fiber.Ctx) error {
	// Extract all context values BEFORE entering stream
	websiteIDStr := c.Query("website_id")
	country := c.Query("country")
	browser := c.Query("browser")
	device := c.Query("device")
	page := c.Query("page")

	// Parse and validate website ID before streaming
	var parseErr string
	var websiteID uuid.UUID
	if websiteIDStr == "" {
		parseErr = "Website ID is required"
	} else {
		var err error
		websiteID, err = uuid.Parse(websiteIDStr)
		if err != nil {
			parseErr = "Invalid website ID"
		}
	}

	// Convert empty strings to NULL for SQL
	var countryParam, browserParam, deviceParam, pageParam interface{}
	if country != "" {
		countryParam = country
	}
	if browser != "" {
		browserParam = browser
	}
	if device != "" {
		deviceParam = device
	}
	if page != "" {
		pageParam = page
	}

	// Query database BEFORE streaming
	var currentVisitors, todayPageviews, todayVisitors int64
	var bounceRateNumeric float64
	var queryErr error

	if parseErr == "" {
		query := `SELECT * FROM get_dashboard_stats($1, 1, $2, $3, $4, $5)`
		queryErr = database.DB.QueryRow(
			query,
			websiteID,
			countryParam,
			browserParam,
			deviceParam,
			pageParam,
		).Scan(&currentVisitors, &todayPageviews, &todayVisitors, &bounceRateNumeric)
	}

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)

		if parseErr != "" {
			_ = sse.PatchSignals(map[string]any{
				"statsError":   parseErr,
				"statsLoading": false,
			})
			return
		}

		if queryErr != nil {
			// On error, return zero values
			_ = sse.PatchSignals(map[string]any{
				"stats": map[string]any{
					"current_visitors":  0,
					"today_pageviews":   0,
					"today_visitors":    0,
					"today_bounce_rate": "0%",
				},
				"statsLoading": false,
			})
			return
		}

		bounceRate := fmt.Sprintf("%.1f%%", bounceRateNumeric)

		_ = sse.PatchSignals(map[string]any{
			"stats": map[string]any{
				"current_visitors":  currentVisitors,
				"today_pageviews":   todayPageviews,
				"today_visitors":    todayVisitors,
				"today_bounce_rate": bounceRate,
			},
			"statsLoading": false,
		})
	})
}

// HandleTimeSeries returns time series data via Datastar SSE
// GET /api/dashboard/timeseries-ds?website_id=...&days=7&country=...&browser=...&device=...&page=...
// Also supports: website (alias for website_id)
func HandleTimeSeries(c fiber.Ctx) error {
	// Extract all context values BEFORE entering stream
	// Support both website_id and website params
	websiteIDStr := c.Query("website_id")
	if websiteIDStr == "" {
		websiteIDStr = c.Query("website")
	}
	days := fiber.Query[int](c, "days", 7)
	if days > 90 {
		days = 90
	}
	country := c.Query("country")
	browser := c.Query("browser")
	device := c.Query("device")
	page := c.Query("page")

	// Parse and validate website ID before streaming
	var parseErr string
	var websiteID uuid.UUID
	if websiteIDStr == "" {
		parseErr = "Website ID is required"
	} else {
		var err error
		websiteID, err = uuid.Parse(websiteIDStr)
		if err != nil {
			parseErr = "Invalid website ID"
		}
	}

	// Convert empty strings to NULL for SQL
	var countryParam, browserParam, deviceParam, pageParam interface{}
	if country != "" {
		countryParam = country
	}
	if browser != "" {
		browserParam = browser
	}
	if device != "" {
		deviceParam = device
	}
	if page != "" {
		pageParam = page
	}

	// Query database BEFORE streaming
	var points []TimeSeriesPoint
	var queryErr error

	if parseErr == "" {
		query := `SELECT * FROM get_timeseries($1, $2, $3, $4, $5, $6)`
		rows, err := database.DB.Query(
			query,
			websiteID,
			days,
			countryParam,
			browserParam,
			deviceParam,
			pageParam,
		)
		if err != nil {
			queryErr = err
		} else {
			defer func() { _ = rows.Close() }()
			points = make([]TimeSeriesPoint, 0)
			for rows.Next() {
				var timestamp string
				var value int64
				if err := rows.Scan(&timestamp, &value); err != nil {
					continue
				}
				points = append(points, TimeSeriesPoint{
					Timestamp: timestamp,
					Value:     int(value),
				})
			}
		}
	}

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)

		if parseErr != "" {
			_ = sse.PatchSignals(map[string]any{
				"timeseriesError":   parseErr,
				"timeseriesLoading": false,
			})
			return
		}

		if queryErr != nil {
			_ = sse.PatchSignals(map[string]any{
				"timeseries":        []TimeSeriesPoint{},
				"timeseriesLoading": false,
			})
			return
		}

		_ = sse.PatchSignals(map[string]any{
			"timeseries":        points,
			"timeseriesLoading": false,
		})
	})
}

// HandleBreakdown returns breakdown data via Datastar SSE
// GET /api/dashboard/breakdown-ds?website_id=...&type=pages|referrers|browsers|devices|countries|cities|regions|os|utm_source|utm_medium|utm_campaign|utm_term|utm_content|entry_page|exit_page
// Also supports: website (alias for website_id), tab (alias for type)
func HandleBreakdown(c fiber.Ctx) error {
	// Extract all context values BEFORE entering stream
	// Support both website_id and website params
	websiteIDStr := c.Query("website_id")
	if websiteIDStr == "" {
		websiteIDStr = c.Query("website")
	}
	// Support both type and tab params
	breakdownType := c.Query("type")
	if breakdownType == "" {
		breakdownType = c.Query("tab", "pages")
	}
	country := c.Query("country")
	browser := c.Query("browser")
	device := c.Query("device")
	page := c.Query("page")

	// Parse pagination parameters
	pagination := ParsePaginationParamsWithValidation(c, "breakdown")

	// Parse and validate website ID before streaming
	var parseErr string
	var websiteID uuid.UUID
	if websiteIDStr == "" {
		parseErr = "Website ID is required"
	} else {
		var err error
		websiteID, err = uuid.Parse(websiteIDStr)
		if err != nil {
			parseErr = "Invalid website ID"
		}
	}

	// Map breakdown types to dimension names
	// Support both underscore and hyphen variants (entry_page and entry-pages)
	dimensionMap := map[string]string{
		"pages":        "pages",
		"referrers":    "referrer",
		"browsers":     "browser",
		"devices":      "device",
		"countries":    "country",
		"cities":       "city",
		"regions":      "region",
		"os":           "os",
		"utm_source":   "utm_source",
		"utm_medium":   "utm_medium",
		"utm_campaign": "utm_campaign",
		"utm_term":     "utm_term",
		"utm_content":  "utm_content",
		"entry_page":   "entry_page",
		"exit_page":    "exit_page",
		"entry-pages":  "entry_page", // Alias with hyphen
		"exit-pages":   "exit_page",  // Alias with hyphen
	}

	dimension, ok := dimensionMap[breakdownType]
	if !ok {
		parseErr = "Invalid breakdown type"
	}

	// Convert empty strings to NULL for SQL
	var countryParam, browserParam, deviceParam, pageParam interface{}
	if country != "" {
		countryParam = country
	}
	if browser != "" {
		browserParam = browser
	}
	if device != "" {
		deviceParam = device
	}
	if page != "" {
		pageParam = page
	}

	// Query database BEFORE streaming
	var items []BreakdownItem
	var totalCount int64
	var queryErr error

	if parseErr == "" {
		if breakdownType == "pages" {
			// Use get_top_pages() for pages breakdown
			query := `SELECT * FROM get_top_pages($1, 1, $2, $3, $4, $5, $6, $7, $8)`
			rows, err := database.DB.Query(
				query,
				websiteID,
				pagination.Per,
				pagination.Offset,
				countryParam,
				browserParam,
				deviceParam,
				pagination.SortBy,
				string(pagination.SortOrder),
			)
			if err != nil {
				queryErr = err
			} else {
				defer func() { _ = rows.Close() }()
				items = make([]BreakdownItem, 0)
				for rows.Next() {
					var path string
					var views int64
					var uniqueVisitors int64
					var avgEngagement *float64
					var rowTotal int64
					if err := rows.Scan(&path, &views, &uniqueVisitors, &avgEngagement, &rowTotal); err != nil {
						continue
					}
					totalCount = rowTotal
					items = append(items, BreakdownItem{
						Name:  path,
						Count: int(views),
					})
				}
			}
		} else if breakdownType == "countries" {
			// Special handling for countries to include ISO code and name conversion
			query := `SELECT * FROM get_breakdown($1, $2, 1, $3, $4, $5, $6, $7, $8, $9, $10)`
			rows, err := database.DB.Query(
				query,
				websiteID,
				dimension,
				pagination.Per,
				pagination.Offset,
				nil, // country filter not applicable when querying countries
				browserParam,
				deviceParam,
				pageParam,
				pagination.SortBy,
				string(pagination.SortOrder),
			)
			if err != nil {
				queryErr = err
			} else {
				defer func() { _ = rows.Close() }()
				items = make([]BreakdownItem, 0)
				for rows.Next() {
					var isoCode string
					var count int64
					var rowTotal int64
					if err := rows.Scan(&isoCode, &count, &rowTotal); err != nil {
						continue
					}
					totalCount = rowTotal
					items = append(items, BreakdownItem{
						Name:  getCountryName(isoCode),
						Code:  isoCode,
						Count: int(count),
					})
				}
			}
		} else {
			// Generic breakdown handler
			query := `SELECT * FROM get_breakdown($1, $2, 1, $3, $4, $5, $6, $7, $8, $9, $10)`
			rows, err := database.DB.Query(
				query,
				websiteID,
				dimension,
				pagination.Per,
				pagination.Offset,
				countryParam,
				browserParam,
				deviceParam,
				pageParam,
				pagination.SortBy,
				string(pagination.SortOrder),
			)
			if err != nil {
				queryErr = err
			} else {
				defer func() { _ = rows.Close() }()
				items = make([]BreakdownItem, 0)
				for rows.Next() {
					var item BreakdownItem
					var rowTotal int64
					if err := rows.Scan(&item.Name, &item.Count, &rowTotal); err != nil {
						continue
					}
					totalCount = rowTotal
					items = append(items, item)
				}
			}
		}
	}

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)

		signalPrefix := "breakdown" + breakdownType

		if parseErr != "" {
			_ = sse.PatchSignals(map[string]any{
				signalPrefix + "Error":   parseErr,
				signalPrefix + "Loading": false,
			})
			return
		}

		if queryErr != nil {
			_ = sse.PatchSignals(map[string]any{
				signalPrefix:             []BreakdownItem{},
				signalPrefix + "Loading": false,
				"pagination": map[string]any{
					"page":        pagination.Page,
					"per":         pagination.Per,
					"total":       0,
					"total_pages": 0,
					"has_more":    false,
				},
			})
			return
		}

		meta := BuildPaginationMeta(pagination, totalCount)

		_ = sse.PatchSignals(map[string]any{
			signalPrefix:             items,
			signalPrefix + "Loading": false,
			"pagination": map[string]any{
				"page":        meta.Page,
				"per":         meta.Per,
				"total":       meta.Total,
				"total_pages": meta.TotalPages,
				"has_more":    meta.HasMore,
			},
		})
	})
}

// HandleMapData returns map data via Datastar SSE
// GET /api/dashboard/map-ds?website_id=...&days=7&country=...&browser=...&device=...&page=...
func HandleMapData(c fiber.Ctx) error {
	// Extract all context values BEFORE entering stream
	websiteIDStr := c.Query("website_id")
	days := min(max(fiber.Query[int](c, "days", 7), 1), 90)
	country := c.Query("country")
	browser := c.Query("browser")
	device := c.Query("device")
	page := c.Query("page")

	// Parse and validate website ID before streaming
	var parseErr string
	var websiteID uuid.UUID
	if websiteIDStr == "" {
		parseErr = "Website ID is required"
	} else {
		var err error
		websiteID, err = uuid.Parse(websiteIDStr)
		if err != nil {
			parseErr = "Invalid website ID"
		}
	}

	// Convert empty strings to NULL for SQL
	var countryParam, browserParam, deviceParam, pageParam interface{}
	if country != "" {
		countryParam = country
	}
	if browser != "" {
		browserParam = browser
	}
	if device != "" {
		deviceParam = device
	}
	if page != "" {
		pageParam = page
	}

	// Query database BEFORE streaming
	var data []MapDataPoint
	var totalVisitors int64
	var queryErr error

	if parseErr == "" {
		query := `SELECT * FROM get_map_data($1, $2, $3, $4, $5, $6)`
		rows, err := database.DB.Query(
			query,
			websiteID,
			days,
			countryParam,
			browserParam,
			deviceParam,
			pageParam,
		)
		if err != nil {
			queryErr = err
		} else {
			defer func() { _ = rows.Close() }()
			data = make([]MapDataPoint, 0)
			for rows.Next() {
				var countryCode string
				var visitors int64
				var percentage float64
				if err := rows.Scan(&countryCode, &visitors, &percentage); err != nil {
					continue
				}
				totalVisitors += visitors
				data = append(data, MapDataPoint{
					Country:     countryCode,
					CountryName: getCountryName(countryCode),
					Code:        getTopoJSONCode(countryCode),
					Visitors:    int(visitors),
					Percentage:  percentage,
				})
			}
		}
	}

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)

		if parseErr != "" {
			_ = sse.PatchSignals(map[string]any{
				"mapError":   parseErr,
				"mapLoading": false,
			})
			return
		}

		if queryErr != nil {
			_ = sse.PatchSignals(map[string]any{
				"mapData":          []MapDataPoint{},
				"mapTotalVisitors": 0,
				"mapPeriodDays":    days,
				"mapLoading":       false,
			})
			return
		}

		_ = sse.PatchSignals(map[string]any{
			"mapData":          data,
			"mapTotalVisitors": totalVisitors,
			"mapPeriodDays":    days,
			"mapLoading":       false,
		})
	})
}

// HandleRealtimeVisitors returns current visitors count via Datastar SSE
// GET /api/dashboard/realtime-ds?website_id=...
func HandleRealtimeVisitors(c fiber.Ctx) error {
	// Extract all context values BEFORE entering stream
	websiteIDStr := c.Query("website_id")

	// Parse and validate website ID before streaming
	var parseErr string
	var websiteID uuid.UUID
	if websiteIDStr == "" {
		parseErr = "Website ID is required"
	} else {
		var err error
		websiteID, err = uuid.Parse(websiteIDStr)
		if err != nil {
			parseErr = "Invalid website ID"
		}
	}

	// Query database BEFORE streaming
	var count int
	var queryErr error

	if parseErr == "" {
		query := `
			SELECT COUNT(DISTINCT session_id)
			FROM website_event
			WHERE website_id = $1
			  AND created_at >= NOW() - INTERVAL '5 minutes'
			  AND event_type = 1
		`
		queryErr = database.DB.QueryRow(query, websiteID).Scan(&count)
	}

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)

		if parseErr != "" {
			_ = sse.PatchSignals(map[string]any{
				"realtimeError":   parseErr,
				"realtimeLoading": false,
			})
			return
		}

		if queryErr != nil {
			_ = sse.PatchSignals(map[string]any{
				"realtimeVisitors": 0,
				"realtimeLoading":  false,
			})
			return
		}

		_ = sse.PatchSignals(map[string]any{
			"realtimeVisitors": count,
			"realtimeLoading":  false,
		})
	})
}

// HandleCampaignsInit initializes the campaigns page with websites list
// GET /api/dashboard/campaigns-init-ds
func HandleCampaignsInit(c fiber.Ctx) error {
	// Get user from context
	user := middleware.GetUser(c)
	if user == nil {
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"websitesError":   "Not authenticated",
				"websitesLoading": false,
			})
		})
	}

	// Query websites for this user BEFORE streaming
	var websites []WebsiteInfo
	var queryErr error

	query := `
		SELECT w.website_id, COALESCE(w.name, ''), w.domain
		FROM website w
		JOIN user_website uw ON w.website_id = uw.website_id
		WHERE uw.user_id = $1
		ORDER BY w.domain
	`
	rows, err := database.DB.Query(query, user.UserID)
	if err != nil {
		queryErr = err
	} else {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var w WebsiteInfo
			if err := rows.Scan(&w.ID, &w.Name, &w.Domain); err != nil {
				continue
			}
			websites = append(websites, w)
		}
	}

	// Determine selected website
	selectedWebsite := c.Query("website")
	if selectedWebsite == "" && len(websites) > 0 {
		selectedWebsite = websites[0].ID
	}

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)

		if queryErr != nil {
			_ = sse.PatchSignals(map[string]any{
				"websitesError":   "Failed to load websites",
				"websitesLoading": false,
				"websites":        []WebsiteInfo{},
			})
			return
		}

		_ = sse.PatchSignals(map[string]any{
			"websites":        websites,
			"selectedWebsite": selectedWebsite,
			"websitesLoading": false,
			"websitesError":   false,
		})

		// If we have a selected website, load UTM data
		if selectedWebsite != "" {
			loadCampaignUTMData(sse, selectedWebsite, "source", "count", "desc")
			loadCampaignUTMData(sse, selectedWebsite, "medium", "count", "desc")
			loadCampaignUTMData(sse, selectedWebsite, "campaign", "count", "desc")
			loadCampaignUTMData(sse, selectedWebsite, "term", "count", "desc")
			loadCampaignUTMData(sse, selectedWebsite, "content", "count", "desc")
		}
	})
}

// HandleCampaigns handles campaign data requests via Datastar SSE
// GET /api/dashboard/campaigns-ds?website=...&dimension=...&sort_by=...&sort_order=...
func HandleCampaigns(c fiber.Ctx) error {
	websiteID := c.Query("website")
	dimension := c.Query("dimension")
	sortBy := c.Query("sort_by", "count")
	sortOrder := c.Query("sort_order", "desc")

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)

		if websiteID == "" {
			_ = sse.PatchSignals(map[string]any{
				"websitesError": "Website ID is required",
			})
			return
		}

		// If dimension is specified, load only that dimension
		if dimension != "" {
			loadCampaignUTMData(sse, websiteID, dimension, sortBy, sortOrder)
		} else {
			// Load all UTM dimensions
			loadCampaignUTMData(sse, websiteID, "source", sortBy, sortOrder)
			loadCampaignUTMData(sse, websiteID, "medium", sortBy, sortOrder)
			loadCampaignUTMData(sse, websiteID, "campaign", sortBy, sortOrder)
			loadCampaignUTMData(sse, websiteID, "term", sortBy, sortOrder)
			loadCampaignUTMData(sse, websiteID, "content", sortBy, sortOrder)
		}
	})
}

// loadCampaignUTMData loads UTM data for a specific dimension and sends it via SSE
func loadCampaignUTMData(sse *DatastarSSE, websiteID, dimension, sortBy, sortOrder string) {
	websiteUUID, err := uuid.Parse(websiteID)
	if err != nil {
		return
	}

	// Set loading state
	_ = sse.PatchSignals(map[string]any{
		"loading": map[string]any{
			dimension: true,
		},
	})

	// Query UTM data
	query := `SELECT * FROM get_breakdown($1, $2, 1, 50, 0, NULL, NULL, NULL, NULL, $3, $4)`
	utmDimension := "utm_" + dimension
	rows, err := database.DB.Query(query, websiteUUID, utmDimension, sortBy, sortOrder)

	var items []BreakdownItem
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var item BreakdownItem
			var rowTotal int64
			if err := rows.Scan(&item.Name, &item.Count, &rowTotal); err != nil {
				continue
			}
			items = append(items, item)
		}
	}

	// Build table HTML
	tableHTML := buildUTMTableHTML(dimension, items, sortBy, sortOrder)

	// Update loading state and send HTML
	_ = sse.PatchSignals(map[string]any{
		"loading": map[string]any{
			dimension: false,
		},
		"sort": map[string]any{
			dimension: map[string]string{
				"column":    sortBy,
				"direction": sortOrder,
			},
		},
	})

	// Patch the table container with rendered HTML
	_ = sse.PatchElements("#utm-"+dimension+"-table", tableHTML)
}

// buildUTMTableHTML generates HTML for a UTM breakdown table
func buildUTMTableHTML(dimension string, items []BreakdownItem, sortBy, sortOrder string) string {
	if len(items) == 0 {
		return `<div class="empty-state-mini">
			<div>[=]</div>
			<div>No UTM ` + dimension + ` data yet</div>
		</div>`
	}

	nameLabel := strings.ToUpper(dimension[:1]) + dimension[1:]
	nameArrow := ""
	countArrow := ""

	if sortBy == "name" {
		if sortOrder == "asc" {
			nameArrow = " [^]"
		} else {
			nameArrow = " [v]"
		}
	}
	if sortBy == "count" {
		if sortOrder == "asc" {
			countArrow = " [^]"
		} else {
			countArrow = " [v]"
		}
	}

	var rows string
	for _, item := range items {
		rows += fmt.Sprintf(`<tr>
			<td>%s</td>
			<td style="text-align: right; font-weight: 500; color: var(--accent-color)">%s</td>
		</tr>`, escapeHTML(item.Name), formatNumber(item.Count))
	}

	return fmt.Sprintf(`<table class="glass card">
		<thead>
			<tr>
				<th
					data-on:click="@get('/api/dashboard/campaigns-ds?website=' + $selectedWebsite + '&dimension=%s&sort_by=name&sort_order=' + ($sort.%s.column === 'name' && $sort.%s.direction === 'desc' ? 'asc' : 'desc'))"
					style="cursor: pointer; user-select: none"
					class="sortable-header"
				>
					<span>%s</span>
					<span style="opacity: 0.7">%s</span>
				</th>
				<th
					data-on:click="@get('/api/dashboard/campaigns-ds?website=' + $selectedWebsite + '&dimension=%s&sort_by=count&sort_order=' + ($sort.%s.column === 'count' && $sort.%s.direction === 'desc' ? 'asc' : 'desc'))"
					style="text-align: right; cursor: pointer; user-select: none"
					class="sortable-header"
				>
					<span>Count</span>
					<span style="opacity: 0.7">%s</span>
				</th>
			</tr>
		</thead>
		<tbody>
			%s
		</tbody>
	</table>`, dimension, dimension, dimension, nameLabel, nameArrow, dimension, dimension, dimension, countArrow, rows)
}

// escapeHTML escapes special HTML characters
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// formatNumber formats an integer with thousand separators
func formatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d,%03d", n/1000, n%1000)
}

// HandleWebsitesInit initializes the websites management page
// GET /api/dashboard/websites-init-ds
func HandleWebsitesInit(c fiber.Ctx) error {
	// Get user from context
	user := middleware.GetUser(c)
	if user == nil {
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"websitesError":   "Not authenticated",
				"websitesLoading": false,
			})
		})
	}

	// Query websites for this user BEFORE streaming
	type websiteCard struct {
		ID                 string   `json:"id"`
		Domain             string   `json:"domain"`
		Name               string   `json:"name"`
		AllowedDomains     []string `json:"allowed_domains"`
		PublicStatsEnabled bool     `json:"public_stats_enabled"`
	}
	var websites []websiteCard
	var queryErr error

	query := `
		SELECT w.website_id, w.domain, COALESCE(w.name, ''), w.allowed_domains, w.public_stats_enabled
		FROM website w
		JOIN user_website uw ON w.website_id = uw.website_id
		WHERE uw.user_id = $1 AND w.deleted_at IS NULL
		ORDER BY w.domain
	`
	rows, err := database.DB.Query(query, user.UserID)
	if err != nil {
		queryErr = err
	} else {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var w websiteCard
			var allowedDomainsJSON []byte
			if err := rows.Scan(&w.ID, &w.Domain, &w.Name, &allowedDomainsJSON, &w.PublicStatsEnabled); err != nil {
				continue
			}
			w.AllowedDomains = []string{}
			if len(allowedDomainsJSON) > 0 {
				_ = json.Unmarshal(allowedDomainsJSON, &w.AllowedDomains)
			}
			websites = append(websites, w)
		}
	}

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)

		if queryErr != nil {
			_ = sse.PatchSignals(map[string]any{
				"websitesError":   "Failed to load websites",
				"websitesLoading": false,
			})
			return
		}

		// Build website cards HTML
		if len(websites) == 0 {
			_ = sse.PatchElements("#websites-grid-container", `
				<div class="empty-state-mini" style="grid-column: 1/-1;">
					<div>[+]</div>
					<div>No websites yet. Create one to start tracking.</div>
				</div>
			`)
		} else {
			var cardsHTML string
			for _, ws := range websites {
				trackingCode := fmt.Sprintf(`&lt;script defer src=&quot;%s/k.js&quot; data-website-id=&quot;%s&quot;&gt;&lt;/script&gt;`, "{{.BaseURL}}", ws.ID)
				cardsHTML += fmt.Sprintf(`
					<div class="glass card website-card">
						<div class="website-header">
							<h3>%s</h3>
							<span class="domain-badge">%s</span>
						</div>
						<div class="tracking-code-section">
							<label>Tracking Code:</label>
							<div class="code-wrapper">
								<code class="tracking-code" id="code-%s">%s</code>
								<button class="btn btn-xs btn-ghost copy-btn" data-on:click="navigator.clipboard.writeText(document.getElementById('code-%s').textContent.replace(/&lt;/g,'<').replace(/&gt;/g,'>').replace(/&amp;/g,'&').replace(/&quot;/g,'\"')); $toastMessage = 'Copied!'; $showToast = true; setTimeout(() => $showToast = false, 2000)">
									Copy
								</button>
							</div>
						</div>
						<div class="website-footer">
							<a href="/dashboard?website=%s" class="btn btn-xs btn-primary">View Analytics</a>
						</div>
					</div>
				`, escapeHTML(ws.Name), escapeHTML(ws.Domain), ws.ID, trackingCode, ws.ID, ws.ID)
			}
			_ = sse.PatchElements("#websites-grid-container", cardsHTML)
		}

		_ = sse.PatchSignals(map[string]any{
			"websitesLoading": false,
		})
	})
}

// HandleWebsitesCreate creates a new website via Datastar SSE
// POST /api/dashboard/websites-create-ds
func HandleWebsitesCreate(c fiber.Ctx) error {
	// Extract form data BEFORE streaming
	domain := c.FormValue("domain")
	name := c.FormValue("name")

	var createErr string
	if domain == "" {
		createErr = "Domain is required"
	}

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	if createErr != "" {
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"createError":   createErr,
				"createLoading": false,
			})
		})
	}

	// Get user
	user := middleware.GetUser(c)
	if user == nil {
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"createError":   "Not authenticated",
				"createLoading": false,
			})
		})
	}

	// Create website
	if name == "" {
		name = domain
	}

	websiteID := uuid.New().String()
	allowedDomains := []string{domain, "www." + domain}
	allowedDomainsJSON, _ := json.Marshal(allowedDomains)

	_, err := database.DB.Exec(`
		INSERT INTO website (website_id, domain, name, allowed_domains, created_at, updated_at)
		VALUES ($1, $2, $3, $4::jsonb, NOW(), NOW())
	`, websiteID, domain, name, string(allowedDomainsJSON))

	if err != nil {
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"createError":   "Failed to create website. Domain may already exist.",
				"createLoading": false,
			})
		})
	}

	// Link to user
	_, _ = database.DB.Exec(`
		INSERT INTO user_website (user_id, website_id, created_at)
		VALUES ($1, $2, NOW())
	`, user.UserID, websiteID)

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)
		_ = sse.PatchSignals(map[string]any{
			"showCreateModal": false,
			"createLoading":   false,
			"createError":     false,
			"newDomain":       "",
			"newName":         "",
			"toastMessage":    "Website created successfully!",
			"showToast":       true,
		})
		// Trigger refresh of website list
		_ = sse.ExecuteScript("setTimeout(() => { $showToast = false }, 2000)")
	})
}

// HandleMapInit initializes the map page with website selection
// GET /api/dashboard/map-init-ds
func HandleMapInit(c fiber.Ctx) error {
	// Get user from context
	user := middleware.GetUser(c)
	if user == nil {
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"mapError":   "Not authenticated",
				"mapLoading": false,
			})
		})
	}

	// Query websites for this user BEFORE streaming
	var websites []WebsiteInfo
	var queryErr error

	query := `
		SELECT w.website_id, COALESCE(w.name, ''), w.domain
		FROM website w
		JOIN user_website uw ON w.website_id = uw.website_id
		WHERE uw.user_id = $1
		ORDER BY w.domain
	`
	rows, err := database.DB.Query(query, user.UserID)
	if err != nil {
		queryErr = err
	} else {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var w WebsiteInfo
			if err := rows.Scan(&w.ID, &w.Name, &w.Domain); err != nil {
				continue
			}
			websites = append(websites, w)
		}
	}

	// Determine selected website
	selectedWebsite := c.Query("website")
	if selectedWebsite == "" && len(websites) > 0 {
		selectedWebsite = websites[0].ID
	}

	// Query map data if we have a selected website
	days := 7
	var mapData []MapDataPoint
	var totalVisitors int64

	if selectedWebsite != "" {
		websiteID, parseErr := uuid.Parse(selectedWebsite)
		if parseErr == nil {
			mapQuery := `SELECT * FROM get_map_data($1, $2, NULL, NULL, NULL, NULL)`
			mapRows, mapErr := database.DB.Query(mapQuery, websiteID, days)
			if mapErr == nil {
				defer func() { _ = mapRows.Close() }()
				for mapRows.Next() {
					var countryCode string
					var visitors int64
					var percentage float64
					if err := mapRows.Scan(&countryCode, &visitors, &percentage); err != nil {
						continue
					}
					totalVisitors += visitors
					mapData = append(mapData, MapDataPoint{
						Country:     countryCode,
						CountryName: getCountryName(countryCode),
						Code:        getTopoJSONCode(countryCode),
						Visitors:    int(visitors),
						Percentage:  percentage,
					})
				}
			}
		}
	}

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)

		if queryErr != nil {
			_ = sse.PatchSignals(map[string]any{
				"mapError":   "Failed to load websites",
				"mapLoading": false,
			})
			return
		}

		_ = sse.PatchSignals(map[string]any{
			"websites":         websites,
			"selectedWebsite":  selectedWebsite,
			"mapData":          mapData,
			"mapTotalVisitors": totalVisitors,
			"mapPeriodDays":    days,
			"mapLoading":       false,
		})

		// Initialize map via script execution
		_ = sse.ExecuteScript(`
			if (window.initChoroplethMap && $mapData) {
				setTimeout(() => window.initChoroplethMap($mapData), 100);
			}
		`)
	})
}

// HandleGoals returns goals list for a website via Datastar SSE
// GET /api/dashboard/goals-ds?website=...
func HandleGoals(c fiber.Ctx) error {
	websiteID := c.Query("website")

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	if websiteID == "" {
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"goalsError":   "Website ID is required",
				"goalsLoading": false,
			})
		})
	}

	if _, err := uuid.Parse(websiteID); err != nil {
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"goalsError":   "Invalid website ID",
				"goalsLoading": false,
			})
		})
	}

	// Query goals
	rows, err := database.DB.Query(`
		SELECT id, website_id, name, target_url, target_event, created_at, updated_at
		FROM goals
		WHERE website_id = $1
		ORDER BY created_at DESC
	`, websiteID)

	if err != nil {
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"goalsError":   "Failed to load goals",
				"goalsLoading": false,
			})
		})
	}
	defer func() { _ = rows.Close() }()

	type goal struct {
		ID        string `json:"id"`
		WebsiteID string `json:"website_id"`
		Name      string `json:"name"`
		Type      string `json:"type"`
		Value     string `json:"value"`
	}
	var goals []goal

	for rows.Next() {
		var g goal
		var targetURL, targetEvent *string
		var createdAt, updatedAt interface{}
		if err := rows.Scan(&g.ID, &g.WebsiteID, &g.Name, &targetURL, &targetEvent, &createdAt, &updatedAt); err != nil {
			continue
		}
		if targetEvent != nil && *targetEvent != "" {
			g.Type = "custom_event"
			g.Value = *targetEvent
		} else if targetURL != nil {
			g.Type = "page_view"
			g.Value = *targetURL
		}
		goals = append(goals, g)
	}

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)
		_ = sse.PatchSignals(map[string]any{
			"goals":        goals,
			"goalsLoading": false,
			"goalsError":   false,
		})
	})
}

// HandleGoalsCreate creates a new goal via Datastar SSE
// POST /api/dashboard/goals-ds
func HandleGoalsCreate(c fiber.Ctx) error {
	// Extract form data
	websiteID := c.FormValue("website_id")
	name := c.FormValue("name")
	goalType := c.FormValue("type")
	value := c.FormValue("value")

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	// Validate
	if websiteID == "" || name == "" || goalType == "" || value == "" {
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"goalError":   "All fields are required",
				"goalLoading": false,
			})
		})
	}

	var targetURL, targetEvent *string
	switch goalType {
	case "page_view":
		targetURL = &value
	case "custom_event":
		targetEvent = &value
	default:
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"goalError":   "Invalid goal type",
				"goalLoading": false,
			})
		})
	}

	id := uuid.New().String()
	_, err := database.DB.Exec(`
		INSERT INTO goals (id, website_id, name, target_url, target_event, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
	`, id, websiteID, name, targetURL, targetEvent)

	if err != nil {
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"goalError":   "Failed to create goal",
				"goalLoading": false,
			})
		})
	}

	// Invalidate cache
	websiteUUID, _ := uuid.Parse(websiteID)
	InvalidateGoalCache(websiteUUID)

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)
		_ = sse.PatchSignals(map[string]any{
			"showCreateModal": false,
			"goalLoading":     false,
			"goalError":       false,
			"newGoalName":     "",
			"newGoalType":     "page_view",
			"newGoalValue":    "",
			"toastMessage":    "Goal created successfully!",
			"showToast":       true,
		})
		_ = sse.ExecuteScript("setTimeout(() => { $showToast = false }, 2000)")
	})
}

// HandleGoalsUpdate updates a goal via Datastar SSE
// PUT /api/dashboard/goals-ds/:id
func HandleGoalsUpdate(c fiber.Ctx) error {
	goalID := c.Params("id")

	// Extract form data
	name := c.FormValue("name")
	goalType := c.FormValue("type")
	value := c.FormValue("value")

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	if _, err := uuid.Parse(goalID); err != nil {
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"goalError":   "Invalid goal ID",
				"goalLoading": false,
			})
		})
	}

	var targetURL, targetEvent *string
	switch goalType {
	case "page_view":
		targetURL = &value
	case "custom_event":
		targetEvent = &value
	default:
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"goalError":   "Invalid goal type",
				"goalLoading": false,
			})
		})
	}

	_, err := database.DB.Exec(`
		UPDATE goals SET name = $1, target_url = $2, target_event = $3, updated_at = NOW()
		WHERE id = $4
	`, name, targetURL, targetEvent, goalID)

	if err != nil {
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"goalError":   "Failed to update goal",
				"goalLoading": false,
			})
		})
	}

	// Invalidate cache
	var websiteID uuid.UUID
	_ = database.DB.QueryRow("SELECT website_id FROM goals WHERE id = $1", goalID).Scan(&websiteID)
	InvalidateGoalCache(websiteID)

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)
		_ = sse.PatchSignals(map[string]any{
			"showEditModal": false,
			"goalLoading":   false,
			"goalError":     false,
			"toastMessage":  "Goal updated successfully!",
			"showToast":     true,
		})
		_ = sse.ExecuteScript("setTimeout(() => { $showToast = false }, 2000)")
	})
}

// HandleGoalsDelete deletes a goal via Datastar SSE
// DELETE /api/dashboard/goals-ds/:id
func HandleGoalsDelete(c fiber.Ctx) error {
	goalID := c.Params("id")

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	if _, err := uuid.Parse(goalID); err != nil {
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"goalError":   "Invalid goal ID",
				"goalLoading": false,
			})
		})
	}

	// Get website ID for cache invalidation
	var websiteID uuid.UUID
	_ = database.DB.QueryRow("SELECT website_id FROM goals WHERE id = $1", goalID).Scan(&websiteID)

	_, err := database.DB.Exec(`DELETE FROM goals WHERE id = $1`, goalID)
	if err != nil {
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"goalError":   "Failed to delete goal",
				"goalLoading": false,
			})
		})
	}

	InvalidateGoalCache(websiteID)

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)
		_ = sse.PatchSignals(map[string]any{
			"goalLoading":  false,
			"goalError":    false,
			"toastMessage": "Goal deleted successfully!",
			"showToast":    true,
		})
		_ = sse.ExecuteScript("setTimeout(() => { $showToast = false }, 2000)")
	})
}

// HandleGoalsAnalytics returns analytics for a specific goal via Datastar SSE
// GET /api/dashboard/goals-ds/:id/analytics?days=7
func HandleGoalsAnalytics(c fiber.Ctx) error {
	goalID := c.Params("id")
	days := fiber.Query[int](c, "days", 7)

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	if _, err := uuid.Parse(goalID); err != nil {
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"analyticsError":   "Invalid goal ID",
				"analyticsLoading": false,
			})
		})
	}

	// Query goal analytics
	var completions, uniqueSessions, totalSessions int
	var conversionRate float64

	err := database.DB.QueryRow(`
		WITH goal_completions AS (
			SELECT COUNT(*) as completions, COUNT(DISTINCT session_id) as unique_sessions
			FROM goal_completions gc
			WHERE gc.goal_id = $1
			  AND gc.completed_at >= NOW() - ($2 || ' days')::INTERVAL
		),
		total_sessions AS (
			SELECT COUNT(DISTINCT session_id) as total
			FROM website_event we
			JOIN goals g ON we.website_id = g.website_id
			WHERE g.id = $1
			  AND we.created_at >= NOW() - ($2 || ' days')::INTERVAL
		)
		SELECT
			COALESCE(gc.completions, 0),
			COALESCE(gc.unique_sessions, 0),
			COALESCE(ts.total, 0),
			CASE WHEN ts.total > 0 THEN (gc.unique_sessions::float / ts.total * 100) ELSE 0 END
		FROM goal_completions gc, total_sessions ts
	`, goalID, days).Scan(&completions, &uniqueSessions, &totalSessions, &conversionRate)

	if err != nil {
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"analytics": map[string]any{
					"completions":     0,
					"unique_sessions": 0,
					"total_sessions":  0,
					"conversion_rate": 0,
				},
				"analyticsLoading": false,
			})
		})
	}

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)
		_ = sse.PatchSignals(map[string]any{
			"analytics": map[string]any{
				"completions":     completions,
				"unique_sessions": uniqueSessions,
				"total_sessions":  totalSessions,
				"conversion_rate": fmt.Sprintf("%.2f", conversionRate),
			},
			"analyticsLoading": false,
		})
	})
}

// HandleGoalsBreakdown returns breakdown data for a goal via Datastar SSE
// GET /api/dashboard/goals-ds/:id/breakdown/:type?days=7
func HandleGoalsBreakdown(c fiber.Ctx) error {
	goalID := c.Params("id")
	breakdownType := c.Params("type")
	days := fiber.Query[int](c, "days", 7)

	// Set SSE headers
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	if _, err := uuid.Parse(goalID); err != nil {
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"breakdownError":   "Invalid goal ID",
				"breakdownLoading": false,
			})
		})
	}

	// Map breakdown type to column
	columnMap := map[string]string{
		"pages":    "url_path",
		"referrer": "referrer_domain",
		"country":  "country",
		"device":   "device",
		"browser":  "browser",
	}

	column, ok := columnMap[breakdownType]
	if !ok {
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"breakdownError":   "Invalid breakdown type",
				"breakdownLoading": false,
			})
		})
	}

	// Query breakdown
	query := fmt.Sprintf(`
		SELECT COALESCE(%s, 'Unknown') as name, COUNT(*) as count
		FROM goal_completions gc
		JOIN website_event we ON gc.session_id = we.session_id
		WHERE gc.goal_id = $1
		  AND gc.completed_at >= NOW() - ($2 || ' days')::INTERVAL
		GROUP BY %s
		ORDER BY count DESC
		LIMIT 10
	`, column, column)

	rows, err := database.DB.Query(query, goalID, days)
	if err != nil {
		return c.SendStreamWriter(func(w *bufio.Writer) {
			sse := NewDatastarSSE(w)
			_ = sse.PatchSignals(map[string]any{
				"breakdown":        []BreakdownItem{},
				"breakdownLoading": false,
			})
		})
	}
	defer func() { _ = rows.Close() }()

	var items []BreakdownItem
	for rows.Next() {
		var item BreakdownItem
		if err := rows.Scan(&item.Name, &item.Count); err != nil {
			continue
		}
		items = append(items, item)
	}

	return c.SendStreamWriter(func(w *bufio.Writer) {
		sse := NewDatastarSSE(w)
		_ = sse.PatchSignals(map[string]any{
			"breakdown":        items,
			"breakdownLoading": false,
		})
	})
}
