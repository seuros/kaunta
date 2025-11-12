package handlers

import (
	"fmt"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/seuros/kaunta/internal/database"
)

// HandleDashboardStats returns aggregated stats for the dashboard
// Uses PostgreSQL function get_dashboard_stats() for optimized query execution
func HandleDashboardStats(c fiber.Ctx) error {
	websiteIDStr := c.Params("website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid website ID",
		})
	}

	// Extract filter parameters from query string
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

	// Call get_dashboard_stats() function - replaces 4 separate queries
	var currentVisitors, todayPageviews, todayVisitors int64
	var bounceRateNumeric float64

	query := `SELECT * FROM get_dashboard_stats($1, 1, $2, $3, $4, $5)`
	err = database.DB.QueryRow(
		query,
		websiteID,
		countryParam,
		browserParam,
		deviceParam,
		pageParam,
	).Scan(&currentVisitors, &todayPageviews, &todayVisitors, &bounceRateNumeric)

	if err != nil {
		// On error, return zero values
		return c.JSON(DashboardStats{
			CurrentVisitors: 0,
			TodayPageviews:  0,
			TodayVisitors:   0,
			TodayBounceRate: "0%",
		})
	}

	// Format bounce rate as percentage string
	bounceRate := fmt.Sprintf("%.1f%%", bounceRateNumeric)

	return c.JSON(DashboardStats{
		CurrentVisitors: int(currentVisitors),
		TodayPageviews:  int(todayPageviews),
		TodayVisitors:   int(todayVisitors),
		TodayBounceRate: bounceRate,
	})
}
