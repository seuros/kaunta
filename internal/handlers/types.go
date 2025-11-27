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
	ID             string   `json:"id"`
	Domain         string   `json:"domain"`
	Name           string   `json:"name"`
	AllowedDomains []string `json:"allowed_domains"`
	CreatedAt      string   `json:"created_at"`
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
