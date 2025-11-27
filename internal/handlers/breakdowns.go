package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/seuros/kaunta/internal/database"
)

// handleBreakdown is a generic handler for all breakdown dimensions
// Uses PostgreSQL function get_breakdown() to reduce code duplication
func handleBreakdown(c fiber.Ctx, dimension string) error {
	websiteIDStr := c.Params("website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid website ID"})
	}

	// Parse pagination parameters with validation for breakdown endpoints
	pagination := ParsePaginationParamsWithValidation(c, "breakdown")

	// Extract query parameters
	country := c.Query("country")
	browser := c.Query("browser")
	device := c.Query("device")
	page := c.Query("page")

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

	// Call get_breakdown() function with appropriate dimension, pagination, and sorting
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
		return c.Status(500).JSON(fiber.Map{"error": "Failed to query " + dimension})
	}
	defer func() { _ = rows.Close() }()

	items := make([]BreakdownItem, 0)
	var totalCount int64
	for rows.Next() {
		var item BreakdownItem
		var rowTotal int64
		if err := rows.Scan(&item.Name, &item.Count, &rowTotal); err != nil {
			continue
		}
		totalCount = rowTotal // Capture total count from function
		items = append(items, item)
	}

	// Return paginated response
	return c.JSON(NewPaginatedResponse(items, pagination, totalCount))
}

// HandleTopReferrers returns top referrers breakdown
func HandleTopReferrers(c fiber.Ctx) error {
	return handleBreakdown(c, "referrer")
}

// HandleTopBrowsers returns top browsers breakdown
func HandleTopBrowsers(c fiber.Ctx) error {
	return handleBreakdown(c, "browser")
}

// HandleTopDevices returns top devices breakdown
func HandleTopDevices(c fiber.Ctx) error {
	return handleBreakdown(c, "device")
}

// HandleTopCountries returns top countries breakdown
func HandleTopCountries(c fiber.Ctx) error {
	return handleBreakdown(c, "country")
}

// HandleTopCities returns top cities breakdown
func HandleTopCities(c fiber.Ctx) error {
	return handleBreakdown(c, "city")
}

// HandleTopRegions returns top regions breakdown
func HandleTopRegions(c fiber.Ctx) error {
	return handleBreakdown(c, "region")
}

// HandleMapData returns visitor data aggregated by country for choropleth maps
// Uses PostgreSQL function get_map_data() for optimized aggregation with percentage calculation
func HandleMapData(c fiber.Ctx) error {
	websiteIDStr := c.Params("website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid website ID"})
	}

	// Get date range (default 7 days, clamp between 1 and 90)
	days := min(max(fiber.Query[int](c, "days", 7), 1), 90)

	// Extract filter parameters
	country := c.Query("country")
	browser := c.Query("browser")
	device := c.Query("device")
	page := c.Query("page")

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

	// Call get_map_data() function - replaces 2 queries + percentage calculation
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
		return c.Status(500).JSON(fiber.Map{"error": "Failed to query map data"})
	}
	defer func() { _ = rows.Close() }()

	var data []MapDataPoint
	var totalVisitors int64 = 0
	for rows.Next() {
		var countryCode string
		var visitors int64
		var percentage float64
		if err := rows.Scan(&countryCode, &visitors, &percentage); err != nil {
			continue
		}

		// Accumulate total visitors
		totalVisitors += visitors

		data = append(data, MapDataPoint{
			Country:     countryCode,
			CountryName: getCountryName(countryCode),
			Code:        getTopoJSONCode(countryCode),
			Visitors:    int(visitors),
			Percentage:  percentage,
		})
	}

	return c.JSON(MapResponse{
		Data:          data,
		TotalVisitors: int(totalVisitors),
		PeriodDays:    days,
	})
}
