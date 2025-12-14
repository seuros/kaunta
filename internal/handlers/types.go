package handlers

// Website represents a website in the system
type Website struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
	Name   string `json:"name"`
}

// DashboardStats holds basic stats for the dashboard
type DashboardStats struct {
	CurrentVisitors int    `json:"current_visitors"`
	TodayPageviews  int    `json:"today_pageviews"`
	TodayVisitors   int    `json:"today_visitors"`
	TodayBounceRate string `json:"today_bounce_rate"`
}

// TopPage represents a page with stats
type TopPage struct {
	Path  string `json:"path"`
	Views int    `json:"views"`
}

// TimeSeriesPoint represents a data point in time series
type TimeSeriesPoint struct {
	Timestamp string `json:"timestamp"`
	Value     int    `json:"value"`
}

// BreakdownItem represents a breakdown metric with count
type BreakdownItem struct {
	Name  string `json:"name"`
	Code  string `json:"code,omitempty"` // ISO code for countries
	Count int    `json:"count"`
}

// MapDataPoint represents a country on the choropleth map
type MapDataPoint struct {
	Country     string  `json:"country"`      // ISO 3166-1 alpha-2 (e.g., "US")
	CountryName string  `json:"country_name"` // Human-readable name
	Code        string  `json:"code"`         // ISO 3166-1 alpha-3 (e.g., "USA")
	Visitors    int     `json:"visitors"`     // Unique visitor count
	Percentage  float64 `json:"percentage"`   // Percentage of total
}

// MapResponse wraps the map data with metadata
type MapResponse struct {
	Data          []MapDataPoint `json:"data"`
	TotalVisitors int            `json:"total_visitors"`
	PeriodDays    int            `json:"period_days"`
}

// WebsiteDetailResponse represents a website with its allowed domains
type WebsiteDetailResponse struct {
	ID                 string   `json:"id"`
	Domain             string   `json:"domain"`
	Name               string   `json:"name"`
	AllowedDomains     []string `json:"allowed_domains"`
	PublicStatsEnabled bool     `json:"public_stats_enabled"`
	CreatedAt          string   `json:"created_at"`
}

// CreateWebsiteRequest is the payload for creating a new website
type CreateWebsiteRequest struct {
	Domain string `json:"domain"`
	Name   string `json:"name,omitempty"`
}

// UpdateWebsiteRequest is the payload for updating a website
type UpdateWebsiteRequest struct {
	Name string `json:"name"`
}

// DomainRequest is the payload for adding/removing allowed domains
type DomainRequest struct {
	Domain string `json:"domain"`
}

// GoalAnalytics holds conversion metrics for a goal
type GoalAnalytics struct {
	Completions    int     `json:"completions"`     // Total goal completions
	UniqueSessions int     `json:"unique_sessions"` // Unique sessions that completed goal
	ConversionRate float64 `json:"conversion_rate"` // Percentage (0-100)
	TotalSessions  int     `json:"total_sessions"`  // Total sessions in period
}
