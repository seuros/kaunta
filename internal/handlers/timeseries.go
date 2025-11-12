package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/seuros/kaunta/internal/database"
)

// HandleTimeSeries returns time-series data for charts
// Uses PostgreSQL function get_timeseries() for optimized hourly aggregation
func HandleTimeSeries(c fiber.Ctx) error {
	websiteIDStr := c.Params("website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid website ID",
		})
	}

	// Get date range (default 7 days, max 90)
	days := fiber.Query[int](c, "days", 7)
	if days > 90 {
		days = 90
	}

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

	// Call get_timeseries() function
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
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to query time series",
		})
	}
	defer func() { _ = rows.Close() }()

	points := make([]TimeSeriesPoint, 0)
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

	return c.JSON(points)
}
