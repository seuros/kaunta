package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/httpx"
	"github.com/seuros/kaunta/internal/middleware"
)

// Website represents a website for the dashboard selector
type WebsiteInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

func selectedWebsiteFromValues(get func(string) string) string {
	if value := get("website"); value != "" {
		return value
	}
	if value := get("selectedWebsite"); value != "" {
		return value
	}
	if ds := get("datastar"); ds != "" {
		var signals map[string]any
		if err := json.Unmarshal([]byte(ds), &signals); err == nil {
			if stored, ok := signals["selectedWebsite"].(string); ok && stored != "" {
				return stored
			}
		}
	}
	return ""
}

func selectedWebsiteFromRequest(r *http.Request) string {
	query := r.URL.Query()
	return selectedWebsiteFromValues(query.Get)
}

func setSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

type sseResponseWriter struct {
	http.ResponseWriter
	flusher http.Flusher
}

func (w *sseResponseWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if err == nil {
		w.flusher.Flush()
	}
	return n, err
}

func streamDatastar(w http.ResponseWriter, fn func(*DatastarSSE)) {
	setSSEHeaders(w)

	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.Error(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	writer := bufio.NewWriter(&sseResponseWriter{ResponseWriter: w, flusher: flusher})
	sse := NewDatastarSSE(writer)
	fn(sse)
	_ = writer.Flush()
	flusher.Flush()
}

func buildWebsiteSelectorHTML(websites []WebsiteInfo, selectedWebsite, context string) string {
	if len(websites) == 0 {
		return ""
	}

	selectClass := "btn btn-sm"
	readonlyClass := "btn btn-sm"
	changeHandler := `const value = (event.target && event.target.value) ? event.target.value : ''; if (value) { localStorage.setItem('kaunta_website', value); } else { localStorage.removeItem('kaunta_website'); } $selectedWebsite = value;`

	switch context {
	case "dashboard":
		selectClass = "input focus-ring"
		readonlyClass = "input"
		changeHandler = `const value = (event.target && event.target.value) ? event.target.value : ''; if (value) { localStorage.setItem('kaunta_website', value); } else { localStorage.removeItem('kaunta_website'); } $selectedWebsite = value; $lastBreakdownKey = ''; $lastChartWebsite = ''; @get('/api/dashboard/stats?website=' + encodeURIComponent(value));`
	case "campaigns":
		changeHandler = `const value = (event.target && event.target.value) ? event.target.value : ''; if (value) { localStorage.setItem('kaunta_website', value); } else { localStorage.removeItem('kaunta_website'); } $selectedWebsite = value; @get('/api/dashboard/campaigns?website_id=' + encodeURIComponent(value));`
	case "map":
		// default change handler already updates localStorage and signal; map effect handles fetching
	default:
		// keep defaults
	}

	var builder strings.Builder

	if context == "campaigns" {
		label := findWebsiteLabel(websites, selectedWebsite)
		if label == "" {
			label = "Select a website in the main dashboard"
		}
		fmt.Fprintf(&builder, `<span class="%s" style="cursor: default; pointer-events: none">%s</span>`, escapeHTML(readonlyClass), escapeHTML(label))
		return builder.String()
	}

	if len(websites) > 1 {
		fmt.Fprintf(&builder, `<select class="%s" data-on:change="%s">`, escapeHTML(selectClass), escapeHTML(changeHandler))
		for _, w := range websites {
			label := websiteLabel(w)
			selectedAttr := ""
			if w.ID == selectedWebsite {
				selectedAttr = " selected"
			}
			fmt.Fprintf(&builder, `<option value="%s"%s>%s</option>`, escapeHTML(w.ID), selectedAttr, escapeHTML(label))
		}
		builder.WriteString(`</select>`)
	} else {
		label := websiteLabel(websites[0])
		fmt.Fprintf(&builder, `<span class="%s" style="cursor: default; pointer-events: none">%s</span>`, escapeHTML(readonlyClass), escapeHTML(label))
	}

	return builder.String()
}

func websiteLabel(site WebsiteInfo) string {
	label := strings.TrimSpace(site.Name)
	if label == "" {
		label = site.Domain
	}
	if label == "" {
		label = "Unnamed Website"
	}
	return label
}

func findWebsiteLabel(websites []WebsiteInfo, selected string) string {
	if selected != "" {
		for _, w := range websites {
			if w.ID == selected {
				return websiteLabel(w)
			}
		}
	}
	if len(websites) > 0 {
		return websiteLabel(websites[0])
	}
	return ""
}

type GoalInfo struct {
	ID        string `json:"id"`
	WebsiteID string `json:"website_id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Value     string `json:"value"`
}

// HandleDashboardInit initializes the dashboard with websites list and initial data
// GET /api/dashboard/init
func HandleDashboardInit(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	writer := bufio.NewWriter(w)
	sse := NewDatastarSSE(writer)

	flush := func() {
		_ = writer.Flush()
		flusher.Flush()
	}

	if user == nil {
		_ = sse.PatchSignals(map[string]any{
			"websitesError":   "Not authenticated",
			"websitesLoading": false,
		})
		flush()
		return
	}

	// Query websites for this user BEFORE streaming
	var websites []WebsiteInfo
	var queryErr error
	query := `
		SELECT website_id, COALESCE(name, ''), domain
		FROM website
		WHERE user_id = $1 -- Direct user_id column
		ORDER BY domain
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
	selectedWebsite := selectedWebsiteFromRequest(r)
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

	if queryErr != nil {
		_ = sse.PatchSignals(map[string]any{
			"websitesError":   "Failed to load websites",
			"websites":        []WebsiteInfo{},
			"websitesLoading": false,
		})
		flush()
		return
	}

	if html := buildWebsiteSelectorHTML(websites, selectedWebsite, "dashboard"); html != "" {
		_ = sse.PatchElements("#websites-container", html)
	}

	bounceRate := "0%"
	if statsErr == nil {
		bounceRate = fmt.Sprintf("%.1f%%", bounceRateNumeric)
	}

	_ = sse.PatchSignals(map[string]any{
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
	flush()
}

// HandleDashboardStats returns dashboard stats via Datastar SSE
// GET /api/dashboard/stats
func HandleDashboardStats(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	websiteIDStr := query.Get("website_id")
	if websiteIDStr == "" {
		websiteIDStr = query.Get("website")
	}
	country := query.Get("country")
	browser := query.Get("browser")
	device := query.Get("device")
	page := query.Get("page")

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

	streamDatastar(w, func(sse *DatastarSSE) {
		if parseErr != "" {
			_ = sse.PatchSignals(map[string]any{
				"statsError":   parseErr,
				"statsLoading": false,
			})
			return
		}

		if queryErr != nil {
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
func HandleTimeSeries(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	websiteIDStr := query.Get("website_id")
	if websiteIDStr == "" {
		websiteIDStr = query.Get("website")
	}
	days := httpx.QueryInt(r, "days", 7)
	if days > 90 {
		days = 90
	}
	country := query.Get("country")
	browser := query.Get("browser")
	device := query.Get("device")
	page := query.Get("page")

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

	streamDatastar(w, func(sse *DatastarSSE) {
		if parseErr != "" {
			_ = sse.PatchSignals(map[string]any{
				"chartLoading": false,
			})
			return
		}

		if queryErr != nil {
			_ = sse.ExecuteScript("window.destroyChart && window.destroyChart();")
			_ = sse.PatchSignals(map[string]any{
				"chartLoading": false,
			})
			return
		}

		timestamps := make([]string, 0, len(points))
		values := make([]int, 0, len(points))

		for _, point := range points {
			timestamps = append(timestamps, point.Timestamp)
			values = append(values, point.Value)
		}

		labelsJSON, _ := json.Marshal(timestamps)
		valuesJSON, _ := json.Marshal(values)

		var script string
		if len(values) == 0 {
			script = "window.destroyChart && window.destroyChart();"
		} else {
			script = fmt.Sprintf(`(function(){const _kauntaLabels=%s.map(ts=>new Date(ts).toLocaleString());const _kauntaValues=%s;window.initChart&&window.initChart(_kauntaLabels,_kauntaValues);})();`,
				string(labelsJSON),
				string(valuesJSON),
			)
		}

		_ = sse.ExecuteScript(script)
		_ = sse.PatchSignals(map[string]any{
			"chartLoading": false,
		})
	})
}

// HandleBreakdown returns breakdown data via Datastar SSE
// GET /api/dashboard/breakdown
func HandleBreakdown(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	datastarParam := query.Get("datastar")

	var websiteIDStr, breakdownType string

	if datastarParam != "" {
		var signals map[string]interface{}
		if err := json.Unmarshal([]byte(datastarParam), &signals); err == nil {
			// Get website ID
			if ws, ok := signals["selectedWebsite"].(string); ok && ws != "" {
				websiteIDStr = ws
			}
			// Get breakdown type (activeTab)
			if tab, ok := signals["activeTab"].(string); ok && tab != "" {
				breakdownType = tab
			}
		}
	}

	if websiteIDStr == "" {
		websiteIDStr = query.Get("website_id")
		if websiteIDStr == "" {
			websiteIDStr = query.Get("website")
		}
	}

	if breakdownType == "" {
		breakdownType = query.Get("type")
		if breakdownType == "" {
			breakdownType = query.Get("tab")
			if breakdownType == "" {
				breakdownType = "pages"
			}
		}
	}

	if websiteIDStr == "" {
		streamDatastar(w, func(sse *DatastarSSE) {
			patchBreakdownErrorState(sse, "Website ID is required")
		})
		return
	}

	var websiteID uuid.UUID
	var parseErr error
	websiteID, parseErr = uuid.Parse(websiteIDStr)
	if parseErr != nil {
		streamDatastar(w, func(sse *DatastarSSE) {
			patchBreakdownErrorState(sse, "Invalid website ID")
		})
		return
	}

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
		"entry-pages":  "entry_page",
		"exit-pages":   "exit_page",
	}

	dimension, ok := dimensionMap[breakdownType]
	if !ok {
		streamDatastar(w, func(sse *DatastarSSE) {
			patchBreakdownErrorState(sse, "Invalid breakdown type: "+breakdownType)
		})
		return
	}

	pagination := ParsePaginationParamsWithValidation(r, "breakdown")

	country := query.Get("country")
	browser := query.Get("browser")
	device := query.Get("device")
	page := query.Get("page")

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

	var items []BreakdownItem
	var totalCount int64
	var queryErr error

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

	streamDatastar(w, func(sse *DatastarSSE) {
		if queryErr != nil {
			fmt.Printf("DEBUG: Database query error: %v\n", queryErr)
			patchBreakdownErrorState(sse, "Database error: "+queryErr.Error())
			return
		}

		meta := BuildPaginationMeta(pagination, totalCount)

		_ = sse.PatchElementsWithMode("#breakdown-content-body", buildBreakdownTableHTML(breakdownType, items), "inner")
		_ = sse.PatchSignals(map[string]any{
			"breakdownLoading": false,
			"breakdownError":   false,
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
func HandleMapData(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	websiteIDStr := query.Get("website_id")
	days := min(max(httpx.QueryInt(r, "days", 7), 1), 90)
	country := query.Get("country")
	browser := query.Get("browser")
	device := query.Get("device")
	page := query.Get("page")

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

	streamDatastar(w, func(sse *DatastarSSE) {
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
func HandleRealtimeVisitors(w http.ResponseWriter, r *http.Request) {
	websiteIDStr := r.URL.Query().Get("website_id")

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

	streamDatastar(w, func(sse *DatastarSSE) {
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
// GET /api/dashboard/campaigns-init
func HandleCampaignsInit(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"websitesError":   "Not authenticated",
				"websitesLoading": false,
			})
		})
		return
	}

	// Query websites for this user BEFORE streaming
	var websites []WebsiteInfo
	var queryErr error

	query := `
		SELECT website_id, COALESCE(name, ''), domain
		FROM website
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY domain
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
	selectedWebsite := selectedWebsiteFromRequest(r)
	if selectedWebsite == "" && len(websites) > 0 {
		selectedWebsite = websites[0].ID
	}

	streamDatastar(w, func(sse *DatastarSSE) {
		if queryErr != nil {
			log.Printf("HandleCampaignsInit: query error: %v", queryErr)
			msg := fmt.Sprintf("Failed to load websites: %v", queryErr)
			_ = sse.PatchSignals(map[string]any{
				"websitesError":   msg,
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

		if html := buildWebsiteSelectorHTML(websites, selectedWebsite, "campaigns"); html != "" {
			_ = sse.PatchElements("#website-selector-container", html)
		}
	})
}

// HandleCampaigns handles campaign data requests via Datastar SSE
// GET /api/dashboard/campaigns-ds?website_id=...&dimension=...&sort_by=...&sort_order=...
func HandleCampaigns(w http.ResponseWriter, r *http.Request) {
	websiteID := selectedWebsiteFromRequest(r)
	if websiteID == "" {
		websiteID = r.URL.Query().Get("website_id")
	}
	query := r.URL.Query()
	dimension := query.Get("dimension")
	sortBy := query.Get("sort_by")
	if sortBy == "" {
		sortBy = "count"
	}
	sortOrder := query.Get("sort_order")
	if sortOrder == "" {
		sortOrder = "desc"
	}

	streamDatastar(w, func(sse *DatastarSSE) {
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

	// Show loader for this dimension until the table is patched
	_ = sse.PatchSignals(map[string]any{
		"loading": map[string]any{
			dimension: true,
		},
	})

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

	tableHTML := buildUTMTableHTML(dimension, items, sortBy, sortOrder)

	// USE ID SELECTOR LIKE THE WORKING CODE
	selector := fmt.Sprintf("#utm-%s-content", dimension)
	_ = sse.PatchElements(selector, tableHTML)

	_ = sse.PatchSignals(map[string]any{
		"loading": map[string]any{
			dimension: false,
		},
	})
}

// buildUTMTableHTML generates HTML for a UTM breakdown table
func buildUTMTableHTML(dimension string, items []BreakdownItem, sortBy, sortOrder string) string {
	if len(items) == 0 {
		return `<div class="empty-state-mini"><div>[=]</div><div>No UTM ` + dimension + ` data yet</div></div>`
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

	var rows strings.Builder
	for _, item := range items {
		rows.WriteString(fmt.Sprintf(`<tr><td>%s</td><td style="text-align:right;font-weight:500;color:var(--accent-color)">%s</td></tr>`,
			escapeHTML(item.Name), formatNumber(item.Count)))
	}

	// This is the compact, single-line version — no newlines/spaces between tags
	return fmt.Sprintf(`<table id="utm-%s-content" class="glass card"><thead><tr><th data-on:click="@get('/api/dashboard/campaigns?website_id='+$selectedWebsite+'&dimension=%s&sort_by=name&sort_order='+($sort.%s.column==='name'&&$sort.%s.direction==='desc'?'asc':'desc'))" style="cursor:pointer;user-select:none" class="sortable-header"><span>%s</span><span style="opacity:0.7">%s</span></th><th data-on:click="@get('/api/dashboard/campaigns?website_id='+$selectedWebsite+'&dimension=%s&sort_by=count&sort_order='+($sort.%s.column==='count'&&$sort.%s.direction==='desc'?'asc':'desc'))" style="text-align:right;cursor:pointer;user-select:none" class="sortable-header"><span>Count</span><span style="opacity:0.7">%s</span></th></tr></thead><tbody>%s</tbody></table>`,
		dimension, dimension, dimension, dimension, nameLabel, nameArrow,
		dimension, dimension, dimension, countArrow,
		rows.String(),
	)
}

var breakdownLabels = map[string]string{
	"pages":        "Pages",
	"referrers":    "Referrers",
	"referrer":     "Referrers",
	"browsers":     "Browsers",
	"browser":      "Browsers",
	"devices":      "Devices",
	"device":       "Devices",
	"countries":    "Countries",
	"country":      "Countries",
	"cities":       "Cities",
	"regions":      "Regions",
	"os":           "Operating Systems",
	"entry-pages":  "Entry Pages",
	"exit-pages":   "Exit Pages",
	"entry_page":   "Entry Pages",
	"exit_page":    "Exit Pages",
	"utm_source":   "UTM Source",
	"utm_medium":   "UTM Medium",
	"utm_campaign": "UTM Campaign",
	"utm_term":     "UTM Term",
	"utm_content":  "UTM Content",
}

func buildBreakdownTableHTML(breakdownType string, items []BreakdownItem) string {
	if len(items) == 0 {
		return `<div class="empty-state"><div class="empty-state-icon">[=]</div><div class="empty-state-title">No data yet</div><div class="empty-state-text">Start tracking to see breakdown data</div></div>`
	}

	header := breakdownHeaderLabel(breakdownType)
	var rows strings.Builder
	for _, item := range items {
		label := strings.TrimSpace(item.Name)
		if label == "" {
			label = "Unknown"
		}
		rows.WriteString(fmt.Sprintf(`<tr><td style="display:flex;align-items:center;gap:8px">%s<span>%s</span></td><td style="text-align:right;font-weight:500;color:var(--accent-color)">%s</td></tr>`,
			breakdownRowPrefix(breakdownType, item),
			escapeHTML(label),
			formatNumber(item.Count),
		))
	}

	return fmt.Sprintf(`<table class="breakdown-table"><thead><tr><th>%s</th><th style="text-align:right">Count</th></tr></thead><tbody>%s</tbody></table>`,
		escapeHTML(header),
		rows.String(),
	)
}

func breakdownHeaderLabel(breakdownType string) string {
	if label, ok := breakdownLabels[breakdownType]; ok {
		return label
	}

	value := strings.ReplaceAll(breakdownType, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")

	parts := strings.Fields(value)
	for i, part := range parts {
		if len(part) == 0 {
			continue
		}
		if len(part) == 1 {
			parts[i] = strings.ToUpper(part)
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}

	if len(parts) == 0 {
		return "Value"
	}
	return strings.Join(parts, " ")
}

func breakdownRowPrefix(breakdownType string, item BreakdownItem) string {
	switch breakdownType {
	case "country", "countries":
		if flag := countryFlagFromCode(item.Code); flag != "" {
			return `<span style="font-size:1.2em">` + flag + `</span>`
		}
	case "browser", "browsers":
		if icon := browserIconHTML(item.Name); icon != "" {
			return `<span class="breakdown-icon">` + icon + `</span>`
		}
	case "os":
		if icon := osIconHTML(item.Name); icon != "" {
			return `<span class="breakdown-icon">` + icon + `</span>`
		}
	case "device", "devices":
		if icon := deviceIconHTML(item.Name); icon != "" {
			return `<span class="breakdown-icon">` + icon + `</span>`
		}
	}
	return ""
}

func buildBreakdownErrorHTML(message string) string {
	if strings.TrimSpace(message) == "" {
		message = "Unable to load breakdown data."
	}
	return fmt.Sprintf(`<div class="empty-state"><div class="empty-state-icon">[!]</div><div class="empty-state-title">Unable to load breakdown</div><div class="empty-state-text">%s</div></div>`, escapeHTML(message))
}

func patchBreakdownErrorState(sse *DatastarSSE, message string) {
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "Unable to load breakdown data."
	}
	_ = sse.PatchElementsWithMode("#breakdown-content-body", buildBreakdownErrorHTML(msg), "inner")
	_ = sse.PatchSignals(map[string]any{
		"breakdownError":   msg,
		"breakdownLoading": false,
	})
}

var browserIcons = map[string]string{
	"Chrome":  `<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="12" cy="12" r="4" fill="currentColor"/><path d="M21.17 8H12M3.95 6.06L8.54 14M14.34 14l-4.63 8" stroke="currentColor" stroke-width="2" fill="none"/></svg>`,
	"Firefox": `<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"/></svg>`,
	"Safari":  `<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="2"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M16.24 7.76l-1.41 1.41M9.17 14.83l-1.41 1.41M7.76 7.76l1.41 1.41M14.83 14.83l1.41 1.41" stroke="currentColor" stroke-width="1.5"/><polygon points="12,6 9,15 12,12 15,15" fill="currentColor"/></svg>`,
	"Edge":    `<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M21 12c0 4.97-4.03 9-9 9-1.5 0-2.91-.37-4.15-1.02.25.02.5.02.75.02 3.31 0 6-2.69 6-6 0-2.49-1.52-4.63-3.68-5.54A8.03 8.03 0 0 1 21 12zM12 3c4.97 0 9 4.03 9 9 0 1.5-.37 2.91-1.02 4.15.02-.25.02-.5.02-.75 0-3.31-2.69-6-6-6-2.49 0-4.63 1.52-5.54 3.68A8.03 8.03 0 0 1 12 3z"/><circle cx="9" cy="15" r="4" fill="none" stroke="currentColor" stroke-width="2"/></svg>`,
	"Opera":   `<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><ellipse cx="12" cy="12" rx="4" ry="8" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="2"/></svg>`,
	"Brave":   `<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2L4 6v6c0 5.55 3.84 10.74 8 12 4.16-1.26 8-6.45 8-12V6l-8-4zm0 4l4 2v4c0 2.96-1.46 5.74-4 7.47-2.54-1.73-4-4.51-4-7.47V8l4-2z"/></svg>`,
	"Samsung": `<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="2"/><path d="M8 12h8M12 8v8" stroke="currentColor" stroke-width="2"/></svg>`,
}

func browserIconHTML(name string) string {
	if icon, ok := browserIcons[name]; ok {
		return icon
	}
	return ""
}

var osIcons = map[string]string{
	"Windows":   `<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M3 12V6.5l7-1v6.5H3zm8-7.5V11h10V3L11 4.5zM3 13v5.5l7 1V13H3zm8 .5V19l10 2v-8H11z"/></svg>`,
	"macOS":     `<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M18.71 19.5c-.83 1.24-1.71 2.45-3.05 2.47-1.34.03-1.77-.79-3.29-.79-1.53 0-2 .77-3.27.82-1.31.05-2.3-1.32-3.14-2.53C4.25 17 2.94 12.45 4.7 9.39c.87-1.52 2.43-2.48 4.12-2.51 1.28-.02 2.5.87 3.29.87.78 0 2.26-1.07 3.81-.91.65.03 2.47.26 3.64 1.98-.09.06-2.17 1.28-2.15 3.81.03 3.02 2.65 4.03 2.68 4.04-.03.07-.42 1.44-1.38 2.83M13 3.5c.73-.83 1.94-1.46 2.94-1.5.13 1.17-.34 2.35-1.04 3.19-.69.85-1.83 1.51-2.95 1.42-.15-1.15.41-2.35 1.05-3.11z"/></svg>`,
	"Mac OS X":  `<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M18.71 19.5c-.83 1.24-1.71 2.45-3.05 2.47-1.34.03-1.77-.79-3.29-.79-1.53 0-2 .77-3.27.82-1.31.05-2.3-1.32-3.14-2.53C4.25 17 2.94 12.45 4.7 9.39c.87-1.52 2.43-2.48 4.12-2.51 1.28-.02 2.5.87 3.29.87.78 0 2.26-1.07 3.81-.91.65.03 2.47.26 3.64 1.98-.09.06-2.17 1.28-2.15 3.81.03 3.02 2.65 4.03 2.68 4.04-.03.07-.42 1.44-1.38 2.83M13 3.5c.73-.83 1.94-1.46 2.94-1.5.13 1.17-.34 2.35-1.04 3.19-.69.85-1.83 1.51-2.95 1.42-.15-1.15.41-2.35 1.05-3.11z"/></svg>`,
	"Linux":     `<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M12.5 2c-1.66 0-3 1.57-3 3.5 0 .66.15 1.27.41 1.81L8.04 9.19C7.12 8.75 6.09 8.5 5 8.5c-2.76 0-5 2.24-5 5s2.24 5 5 5c1.09 0 2.1-.35 2.93-.95l1.91 1.91c-.55.83-.84 1.79-.84 2.79 0 2.76 2.24 5 5 5s5-2.24 5-5c0-1-.29-1.96-.84-2.79l1.91-1.91c.83.6 1.84.95 2.93.95 2.76 0 5-2.24 5-5s-2.24-5-5-5c-1.09 0-2.12.25-3.04.69l-1.87-1.88c.26-.54.41-1.15.41-1.81 0-1.93-1.34-3.5-3-3.5zm0 2c.55 0 1 .67 1 1.5S13.05 7 12.5 7s-1-.67-1-1.5.45-1.5 1-1.5z"/></svg>`,
	"Android":   `<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M6 18c0 .55.45 1 1 1h1v3.5c0 .83.67 1.5 1.5 1.5s1.5-.67 1.5-1.5V19h2v3.5c0 .83.67 1.5 1.5 1.5s1.5-.67 1.5-1.5V19h1c.55 0 1-.45 1-1V8H6v10zM3.5 8C2.67 8 2 8.67 2 9.5v7c0 .83.67 1.5 1.5 1.5S5 17.33 5 16.5v-7C5 8.67 4.33 8 3.5 8zm17 0c-.83 0-1.5.67-1.5 1.5v7c0 .83.67 1.5 1.5 1.5s1.5-.67 1.5-1.5v-7c0-.83-.67-1.5-1.5-1.5zm-4.97-5.84l1.3-1.3c.2-.2.2-.51 0-.71-.2-.2-.51-.2-.71 0l-1.48 1.48A5.84 5.84 0 0 0 12 1c-.96 0-1.86.23-2.66.63L7.85.15c-.2-.2-.51-.2-.71 0-.2.2-.2.51 0 .71l1.31 1.31A5.983 5.983 0 0 0 6 7h12c0-1.99-.97-3.75-2.47-4.84zM10 5H9V4h1v1zm5 0h-1V4h1v1z"/></svg>`,
	"iOS":       `<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M18.71 19.5c-.83 1.24-1.71 2.45-3.05 2.47-1.34.03-1.77-.79-3.29-.79-1.53 0-2 .77-3.27.82-1.31.05-2.3-1.32-3.14-2.53C4.25 17 2.94 12.45 4.7 9.39c.87-1.52 2.43-2.48 4.12-2.51 1.28-.02 2.5.87 3.29.87.78 0 2.26-1.07 3.81-.91.65.03 2.47.26 3.64 1.98-.09.06-2.17 1.28-2.15 3.81.03 3.02 2.65 4.03 2.68 4.04-.03.07-.42 1.44-1.38 2.83M13 3.5c.73-.83 1.94-1.46 2.94-1.5.13 1.17-.34 2.35-1.04 3.19-.69.85-1.83 1.51-2.95 1.42-.15-1.15.41-2.35 1.05-3.11z"/></svg>`,
	"Chrome OS": `<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="12" cy="12" r="4" fill="currentColor"/><path d="M21.17 8H12M3.95 6.06L8.54 14M14.34 14l-4.63 8" stroke="currentColor" stroke-width="2" fill="none"/></svg>`,
}

func osIconHTML(name string) string {
	if icon, ok := osIcons[name]; ok {
		return icon
	}
	return ""
}

var deviceIcons = map[string]string{
	"desktop": `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>`,
	"mobile":  `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="5" y="2" width="14" height="20" rx="2"/><line x1="12" y1="18" x2="12" y2="18.01"/></svg>`,
	"tablet":  `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="2" width="16" height="20" rx="2"/><line x1="12" y1="18" x2="12" y2="18.01"/></svg>`,
}

func deviceIconHTML(device string) string {
	key := strings.ToLower(strings.TrimSpace(device))
	if icon, ok := deviceIcons[key]; ok {
		return icon
	}
	return ""
}

func countryFlagFromCode(code string) string {
	if len(code) != 2 {
		return ""
	}

	upper := strings.ToUpper(code)
	r1 := upper[0]
	r2 := upper[1]

	if r1 < 'A' || r1 > 'Z' || r2 < 'A' || r2 > 'Z' {
		return ""
	}

	flag := []rune{
		rune(r1-'A') + 0x1F1E6,
		rune(r2-'A') + 0x1F1E6,
	}
	return string(flag)
}

func buildGoalsTableHTML(goals []GoalInfo) string {
	if len(goals) == 0 {
		return ` <div id="goals-empty-state" class="empty-state" data-attr:hidden="$goalsLoading || $goalsError || $goals.length !== 0"><div class="empty-state-icon">🎯</div><div class="empty-state-title">No goals yet</div><div class="empty-state-text">Create your first goal to start tracking conversions</div><button data-on:click="$showCreateModal = true" class="btn btn-primary" style="margin-top: 16px"> <svg class="icon-sm" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"></path></svg>Add Your First Goal</button></div>`
	}

	const editAction = `const btn = evt.currentTarget || evt.target; if (!btn) { return; } const data = btn.dataset || {}; $currentGoal = { id: data.goalId || '', name: data.goalName || '', type: data.goalType || '', value: data.goalValue || '' }; $goalForm = { name: data.goalName || '', type: data.goalType || '', value: data.goalValue || '' }; $formError = ''; $showEditModal = true;`
	const analyticsAction = `const btn = evt.currentTarget || evt.target; if (!btn) { return; } const data = btn.dataset || {}; $currentGoal = { id: data.goalId || '', name: data.goalName || '', type: data.goalType || '', value: data.goalValue || '' }; $analytics = { completions: 0, unique_sessions: 0, conversion_rate: '0.00', total_sessions: 0 }; $breakdownData = []; destroyGoalChart(); $analyticsLoading = true; $breakdownLoading = true; $lastAnalyticsRequestKey = ''; $lastBreakdownRequestKey = ''; $showAnalyticsModal = true;`
	const deleteAction = `const btn = evt.currentTarget || evt.target; if (!btn) { return; } const data = btn.dataset || {}; if (confirm('Delete goal &ldquo;' + (data.goalName || '') + '&rdquo;?')) { @delete('/api/dashboard/goals/' + (data.goalId || ''), { headers: { 'X-CSRF-Token': getGoalsCsrfToken() } }); }`

	var rows strings.Builder
	for _, g := range goals {
		typeLabel := "Page View"
		if g.Type == "custom_event" {
			typeLabel = "Custom Event"
		}
		rows.WriteString(fmt.Sprintf(`<tr><td><div class="goal-name">%s</div><div class="goal-meta">%s</div></td><td><span class="badge">%s</span></td><td><code class="goal-target">%s</code></td><td class="goal-actions"><button class="btn btn-xs btn-secondary" data-goal-id="%s" data-goal-name="%s" data-goal-type="%s" data-goal-value="%s" data-on:click="%s">Edit</button><button class="btn btn-xs btn-primary" data-goal-id="%s" data-goal-name="%s" data-goal-type="%s" data-goal-value="%s" data-on:click="%s">Analytics</button><button class="btn btn-xs btn-danger" data-goal-id="%s" data-goal-name="%s" data-on:click="%s">Delete</button></td></tr>`,
			escapeHTML(g.Name),
			escapeHTML(fmt.Sprintf("ID: %s", g.ID)),
			typeLabel,
			escapeHTML(g.Value),
			escapeHTML(g.ID),
			escapeHTML(g.Name),
			escapeHTML(g.Type),
			escapeHTML(g.Value),
			editAction,
			escapeHTML(g.ID),
			escapeHTML(g.Name),
			escapeHTML(g.Type),
			escapeHTML(g.Value),
			analyticsAction,
			escapeHTML(g.ID),
			escapeHTML(g.Name),
			deleteAction,
		))
	}

	return fmt.Sprintf(`<table class="glass card goals-table"><thead><tr><th>Goal</th><th>Type</th><th>Target</th><th style="text-align:right">Actions</th></tr></thead><tbody>%s</tbody></table>`, rows.String())
}

func buildGoalAnalyticsStatsHTML(conversionRate float64, completions, uniqueSessions, totalSessions int) string {
	convText := fmt.Sprintf("%.2f%%", conversionRate)
	sessionSummary := fmt.Sprintf("%s of %s sessions", formatNumber(uniqueSessions), formatNumber(totalSessions))

	var b strings.Builder
	fmt.Fprintf(&b, `<div class="stat-card glass card"><div class="stat-label">Conversion Rate</div><div class="stat-value">%s</div><div class="stat-footer">%s</div></div>`, escapeHTML(convText), escapeHTML(sessionSummary))
	fmt.Fprintf(&b, `<div class="stat-card glass card"><div class="stat-label">Total Completions</div><div class="stat-value">%s</div></div>`, escapeHTML(formatNumber(completions)))
	fmt.Fprintf(&b, `<div class="stat-card glass card"><div class="stat-label">Unique Sessions</div><div class="stat-value">%s</div></div>`, escapeHTML(formatNumber(uniqueSessions)))
	fmt.Fprintf(&b, `<div class="stat-card glass card"><div class="stat-label">Total Sessions</div><div class="stat-value">%s</div></div>`, escapeHTML(formatNumber(totalSessions)))

	return b.String()
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
	value := fmt.Sprintf("%d", n)
	sign := ""
	if strings.HasPrefix(value, "-") {
		sign = "-"
		value = value[1:]
	}

	if len(value) <= 3 {
		return sign + value
	}

	var b strings.Builder
	for i, ch := range value {
		if i > 0 && (len(value)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(ch)
	}

	return sign + b.String()
}

// HandleWebsitesInit initializes the websites management page
// GET /api/dashboard/websites-init-ds
func HandleWebsitesInit(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"websitesError":   "Not authenticated",
				"websitesLoading": false,
			})
		})
		return
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
		SELECT website_id, domain, COALESCE(name, ''), allowed_domains, public_stats_enabled
		FROM website
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY domain
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

	streamDatastar(w, func(sse *DatastarSSE) {
		if queryErr != nil {
			log.Printf("HandleWebsitesInit: query error: %v", queryErr)
			msg := fmt.Sprintf("Failed to load websites: %v", queryErr)
			_ = sse.PatchSignals(map[string]any{
				"websitesError":   msg,
				"websitesLoading": false,
			})
			return
		}

		gridSelector := "[data-element='websites-grid-content']"

		if len(websites) == 0 {
			emptyHTML := `
				<div class="empty-state-mini" style="grid-column: 1/-1;">
					<div>[+]</div>
					<div>No websites yet. Create one to start tracking.</div>
				</div>
			`
			_ = sse.PatchElementsWithMode(gridSelector, emptyHTML, "inner")
		} else {
			var cardsHTML strings.Builder
			for _, ws := range websites {
				displayName := strings.TrimSpace(ws.Name)
				if displayName == "" {
					displayName = ws.Domain
				}

				trackingCode := fmt.Sprintf(`<script async src="/k.js" data-website-id="%s"></script>`, ws.ID)
				copyAction := fmt.Sprintf("copyTrackingCode('%s', $signals)", ws.ID)

				var domains strings.Builder
				if len(ws.AllowedDomains) == 0 {
					domains.WriteString(`<div class="domain-item"><span>No allowed domains yet</span></div>`)
				} else {
					for _, domain := range ws.AllowedDomains {
						domains.WriteString(fmt.Sprintf(`<div class="domain-item"><span>%s</span></div>`, escapeHTML(domain)))
					}
				}

				card := fmt.Sprintf(`<div class="glass card website-card"><div class="website-header"><div class="website-title-group"><div class="website-name">%s</div></div></div><div class="website-info"><div class="info-row"><div class="info-label">Website ID</div><div class="info-value">%s</div></div><div class="info-row"><div class="info-label">Tracking Code</div><div class="info-value"><code class="tracking-code" id="code-%s">%s</code></div></div></div><div class="domains-section"><div class="domains-header"><div class="domains-label">Allowed Domains</div><div class="domains-count">%d</div></div><div class="domains-list">%s</div></div><div class="website-actions"><button class="btn btn-xs btn-ghost transition-standard" data-on:click="%s" title="Copy tracking snippet"><svg class="icon-sm" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16h8a2 2 0 002-2V6a2 2 0 00-2-2H8a2 2 0 00-2 2v8a2 2 0 002 2z"></path><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 8H5a2 2 0 00-2 2v8a2 2 0 002 2h8a2 2 0 002-2v-1"></path></svg></button><a href="/dashboard?website=%s" class="btn btn-xs btn-primary transition-standard">Analytics</a></div></div>`,
					escapeHTML(displayName), escapeHTML(ws.ID), ws.ID, escapeHTML(trackingCode), len(ws.AllowedDomains), domains.String(), copyAction, ws.ID)

				cardsHTML.WriteString(card)
			}

			_ = sse.PatchElementsWithMode(gridSelector, cardsHTML.String(), "inner")
		}

		_ = sse.PatchSignals(map[string]any{
			"websites":        websites,
			"websitesError":   false,
			"websitesLoading": false,
		})
	})
}

// HandleWebsitesCreate creates a new website via Datastar SSE
// POST /api/dashboard/websites-create-ds
func HandleWebsitesCreate(w http.ResponseWriter, r *http.Request) {
	domain := r.FormValue("domain")
	name := r.FormValue("name")

	var createErr string
	if domain == "" {
		createErr = "Domain is required"
	}

	if createErr != "" {
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"createError": createErr,
				"creating":    false,
			})
		})
		return
	}

	user := middleware.GetUser(r)
	if user == nil {
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"createError": "Not authenticated",
				"creating":    false,
			})
		})
		return
	}

	// Create website
	if name == "" {
		name = domain
	}

	websiteID := uuid.New().String()
	allowedDomains := []string{domain, "www." + domain}
	allowedDomainsJSON, _ := json.Marshal(allowedDomains)

	_, err := database.DB.Exec(`
		INSERT INTO website (website_id, user_id, domain, name, allowed_domains, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, NOW(), NOW())
	`, websiteID, user.UserID, domain, name, string(allowedDomainsJSON))

	if err != nil {
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"createError": "Failed to create website. Domain may already exist.",
				"creating":    false,
			})
		})
		return
	}

	streamDatastar(w, func(sse *DatastarSSE) {
		_ = sse.PatchSignals(map[string]any{
			"showCreateModal": false,
			"creating":        false,
			"createError":     "",
			"newWebsite": map[string]any{
				"domain": "",
				"name":   "",
			},
			"toast": map[string]any{
				"show":    true,
				"message": "Website created successfully!",
				"type":    "success",
			},
			"websitesReload": true,
		})
		_ = sse.ExecuteScript(`
			clearTimeout(window.__kauntaToastTimer || 0);
			window.__kauntaToastTimer = setTimeout(() => {
				if (window.Datastar?.store) {
					window.Datastar.store.toast = { show: false, message: "", type: "" };
				}
			}, 3000);
		`)
	})
}

// HandleMapInit initializes the map page with website selection
// GET /api/dashboard/map-init-ds
func HandleMapInit(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"mapError":   "Not authenticated",
				"mapLoading": false,
			})
		})
		return
	}

	// Query websites for this user BEFORE streaming
	var websites []WebsiteInfo
	var queryErr error

	query := `
		SELECT website_id, COALESCE(name, ''), domain
		FROM website
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY domain
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
	selectedWebsite := r.URL.Query().Get("website")
	if selectedWebsite == "" && len(websites) > 0 {
		selectedWebsite = websites[0].ID
	}

	// Query map data if we have a selected website
	days := 7
	mapData := make([]MapDataPoint, 0)
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

	streamDatastar(w, func(sse *DatastarSSE) {
		if queryErr != nil {
			log.Printf("HandleMapInit: query error: %v", queryErr)
			msg := fmt.Sprintf("Failed to load websites: %v", queryErr)
			_ = sse.PatchSignals(map[string]any{
				"mapError":   msg,
				"mapLoading": false,
			})
			return
		}

		_ = sse.PatchSignals(map[string]any{
			"websites":         websites,
			"selectedWebsite":  selectedWebsite,
			"websitesLoading":  false,
			"websitesError":    false,
			"mapData":          mapData,
			"mapTotalVisitors": totalVisitors,
			"mapPeriodDays":    days,
			"mapLoading":       false,
		})

		if html := buildWebsiteSelectorHTML(websites, selectedWebsite, "map"); html != "" {
			_ = sse.PatchElements("#website-selector-container", html)
		}
	})
}

// HandleGoals returns goals list for a website via Datastar SSE
// GET /api/dashboard/goals-ds?website=...
func HandleGoals(w http.ResponseWriter, r *http.Request) {
	websiteID := r.URL.Query().Get("website")

	if websiteID == "" {
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"goalsError":   "Website ID is required",
				"goalsLoading": false,
			})
		})
		return
	}

	if _, err := uuid.Parse(websiteID); err != nil {
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"goalsError":   "Invalid website ID",
				"goalsLoading": false,
			})
		})
		return
	}

	// Query goals
	rows, err := database.DB.Query(`
		SELECT id, website_id, name, target_url, target_event, created_at, updated_at
		FROM goals
		WHERE website_id = $1
		ORDER BY created_at DESC
	`, websiteID)

	if err != nil {
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"goalsError":   "Failed to load goals",
				"goalsLoading": false,
			})
		})
		return
	}
	defer func() { _ = rows.Close() }()

	var goals []GoalInfo

	for rows.Next() {
		var g GoalInfo
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

	streamDatastar(w, func(sse *DatastarSSE) {
		tableHTML := buildGoalsTableHTML(goals)
		_ = sse.PatchElementsWithMode("[data-element='goals-table-container']", tableHTML, "inner")
		_ = sse.PatchSignals(map[string]any{
			"goals":        goals,
			"goalsLoading": false,
			"goalsError":   false,
			"goalsReload":  false,
		})
	})
}

// HandleGoalsCreate creates a new goal via Datastar SSE
// POST /api/dashboard/goals-ds
func HandleGoalsCreate(w http.ResponseWriter, r *http.Request) {
	websiteID := r.FormValue("website_id")
	name := r.FormValue("name")
	goalType := r.FormValue("type")
	value := r.FormValue("value")

	if websiteID == "" || name == "" || goalType == "" || value == "" {
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"goalError":   "All fields are required",
				"goalLoading": false,
				"submitting":  false,
			})
		})
		return
	}

	var targetURL, targetEvent *string
	switch goalType {
	case "page_view":
		targetURL = &value
	case "custom_event":
		targetEvent = &value
	default:
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"goalError":   "Invalid goal type",
				"goalLoading": false,
				"submitting":  false,
			})
		})
		return
	}

	id := uuid.New().String()
	_, err := database.DB.Exec(`
		INSERT INTO goals (id, website_id, name, target_url, target_event, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
	`, id, websiteID, name, targetURL, targetEvent)

	if err != nil {
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"goalError":   "Failed to create goal",
				"goalLoading": false,
				"submitting":  false,
			})
		})
		return
	}

	// Invalidate cache
	websiteUUID, _ := uuid.Parse(websiteID)
	InvalidateGoalCache(websiteUUID)

	streamDatastar(w, func(sse *DatastarSSE) {
		_ = sse.PatchSignals(map[string]any{
			"showCreateModal": false,
			"goalLoading":     false,
			"goalError":       false,
			"goalForm": map[string]string{
				"name":  "",
				"type":  "",
				"value": "",
			},
			"submitting":   false,
			"toastMessage": "Goal created successfully!",
			"showToast":    true,
			"goalsReload":  true,
		})
		_ = sse.ExecuteScript("setTimeout(() => { $showToast = false }, 2000)")
	})
}

// HandleGoalsUpdate updates a goal via Datastar SSE
// PUT /api/dashboard/goals-ds/:id
func HandleGoalsUpdate(w http.ResponseWriter, r *http.Request) {
	goalID := chi.URLParam(r, "id")

	name := r.FormValue("name")
	goalType := r.FormValue("type")
	value := r.FormValue("value")

	if _, err := uuid.Parse(goalID); err != nil {
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"goalError":   "Invalid goal ID",
				"goalLoading": false,
			})
		})
		return
	}

	var targetURL, targetEvent *string
	switch goalType {
	case "page_view":
		targetURL = &value
	case "custom_event":
		targetEvent = &value
	default:
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"goalError":   "Invalid goal type",
				"goalLoading": false,
			})
		})
		return
	}

	_, err := database.DB.Exec(`
		UPDATE goals SET name = $1, target_url = $2, target_event = $3, updated_at = NOW()
		WHERE id = $4
	`, name, targetURL, targetEvent, goalID)

	if err != nil {
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"goalError":   "Failed to update goal",
				"goalLoading": false,
			})
		})
		return
	}

	// Invalidate cache
	var websiteID uuid.UUID
	_ = database.DB.QueryRow("SELECT website_id FROM goals WHERE id = $1", goalID).Scan(&websiteID)
	InvalidateGoalCache(websiteID)

	streamDatastar(w, func(sse *DatastarSSE) {
		_ = sse.PatchSignals(map[string]any{
			"showEditModal": false,
			"goalLoading":   false,
			"goalError":     false,
			"toastMessage":  "Goal updated successfully!",
			"showToast":     true,
			"submitting":    false,
			"goalsReload":   true,
			"goalForm": map[string]string{
				"name":  "",
				"type":  "",
				"value": "",
			},
			"currentGoal": nil,
		})
		_ = sse.ExecuteScript("setTimeout(() => { $showToast = false }, 2000)")
	})
}

// HandleGoalsDelete deletes a goal via Datastar SSE
// DELETE /api/dashboard/goals-ds/:id
func HandleGoalsDelete(w http.ResponseWriter, r *http.Request) {
	goalID := chi.URLParam(r, "id")

	if _, err := uuid.Parse(goalID); err != nil {
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"goalError":   "Invalid goal ID",
				"goalLoading": false,
			})
		})
		return
	}

	// Get website ID for cache invalidation
	var websiteID uuid.UUID
	_ = database.DB.QueryRow("SELECT website_id FROM goals WHERE id = $1", goalID).Scan(&websiteID)

	_, err := database.DB.Exec(`DELETE FROM goals WHERE id = $1`, goalID)
	if err != nil {
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"goalError":   "Failed to delete goal",
				"goalLoading": false,
			})
		})
		return
	}

	InvalidateGoalCache(websiteID)

	streamDatastar(w, func(sse *DatastarSSE) {
		_ = sse.PatchSignals(map[string]any{
			"goalLoading":  false,
			"goalError":    false,
			"toastMessage": "Goal deleted successfully!",
			"showToast":    true,
			"goalsReload":  true,
		})
		_ = sse.ExecuteScript("setTimeout(() => { $showToast = false }, 2000)")
	})
}

// HandleGoalsAnalytics returns analytics for a specific goal via Datastar SSE
// GET /api/dashboard/goals-ds/:id/analytics?days=7
func HandleGoalsAnalytics(w http.ResponseWriter, r *http.Request) {
	goalID := chi.URLParam(r, "id")
	days := httpx.QueryInt(r, "days", 7)

	if _, err := uuid.Parse(goalID); err != nil {
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"analyticsError":   "Invalid goal ID",
				"analyticsLoading": false,
			})
		})
		return
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
		streamDatastar(w, func(sse *DatastarSSE) {
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
		return
	}

	chartLabels := make([]string, 0, 32)
	chartValues := make([]int, 0, 32)
	timeRows, timeErr := database.DB.Query(`
		SELECT date_trunc('hour', gc.completed_at) AS bucket, COUNT(*) as count
		FROM goal_completions gc
		WHERE gc.goal_id = $1
		  AND gc.completed_at >= NOW() - ($2 || ' days')::INTERVAL
		GROUP BY bucket
		ORDER BY bucket
	`, goalID, days)
	if timeErr == nil {
		defer func() { _ = timeRows.Close() }()
		for timeRows.Next() {
			var bucket time.Time
			var count int
			if err := timeRows.Scan(&bucket, &count); err != nil {
				continue
			}
			chartLabels = append(chartLabels, bucket.UTC().Format(time.RFC3339))
			chartValues = append(chartValues, count)
		}
	}

	var goalName string
	_ = database.DB.QueryRow(`SELECT COALESCE(name, '') FROM goals WHERE id = $1`, goalID).Scan(&goalName)
	statsHTML := buildGoalAnalyticsStatsHTML(conversionRate, completions, uniqueSessions, totalSessions)
	labelJSON, _ := json.Marshal(chartLabels)
	valueJSON, _ := json.Marshal(chartValues)
	daysStr := fmt.Sprintf("%d", days)

	streamDatastar(w, func(sse *DatastarSSE) {
		_ = sse.PatchElementsWithMode("[data-element='analytics-stats-container']", statsHTML, "inner")
		if goalName != "" {
			_ = sse.ExecuteScript(fmt.Sprintf(`(function(){const el=document.getElementById('analytics-goal-name');if(el){el.textContent=%q;}})();`, goalName))
		}
		_ = sse.ExecuteScript(fmt.Sprintf(`initGoalChart(%s,%s,%q);`, labelJSON, valueJSON, daysStr))
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
func HandleGoalsBreakdown(w http.ResponseWriter, r *http.Request) {
	goalID := chi.URLParam(r, "id")
	breakdownType := chi.URLParam(r, "type")
	days := httpx.QueryInt(r, "days", 7)

	if _, err := uuid.Parse(goalID); err != nil {
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"breakdownError":   "Invalid goal ID",
				"breakdownLoading": false,
			})
		})
		return
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
		streamDatastar(w, func(sse *DatastarSSE) {
			_ = sse.PatchSignals(map[string]any{
				"breakdownError":   "Invalid breakdown type",
				"breakdownLoading": false,
			})
		})
		return
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
		streamDatastar(w, func(sse *DatastarSSE) {
			html := buildBreakdownErrorHTML("Failed to load breakdown data")
			_ = sse.PatchElementsWithMode("[data-element='analytics-breakdown-container']", html, "inner")
			_ = sse.PatchSignals(map[string]any{
				"breakdown":        []BreakdownItem{},
				"breakdownLoading": false,
			})
		})
		return
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

	breakdownHTML := buildBreakdownTableHTML(breakdownType, items)

	streamDatastar(w, func(sse *DatastarSSE) {
		_ = sse.PatchElementsWithMode("[data-element='analytics-breakdown-container']", breakdownHTML, "inner")
		_ = sse.PatchSignals(map[string]any{
			"breakdown":        items,
			"breakdownLoading": false,
		})
	})
}
