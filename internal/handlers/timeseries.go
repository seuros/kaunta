package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/seuros/kaunta/internal/database"
)

// HandleTimeSeries returns time-series data for charts
func HandleTimeSeries(c fiber.Ctx) error {
	websiteIDStr := c.Params("website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid website ID",
		})
	}

	// Get date range (default 7 days)
	days := fiber.Query[int](c, "days", 7)
	if days > 90 {
		days = 90 // Max 90 days
	}

	filterClause, filterArgs := buildFilterClause(c, []interface{}{websiteID, days})

	// Query for hourly pageview counts
	query := `
		SELECT
			DATE_TRUNC('hour', e.created_at) as hour,
			COUNT(*) as views
		FROM website_event e
		JOIN session s ON e.session_id = s.session_id
		WHERE e.website_id = $1
		  AND e.created_at >= NOW() - INTERVAL '1 day' * $2
		  AND e.event_type = 1` + filterClause + `
		GROUP BY hour
		ORDER BY hour ASC`

	rows, err := database.DB.Query(query, filterArgs...)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to query time series",
		})
	}
	defer func() { _ = rows.Close() }()

	points := make([]TimeSeriesPoint, 0)
	for rows.Next() {
		var timestamp string
		var value int
		if err := rows.Scan(&timestamp, &value); err != nil {
			continue
		}
		points = append(points, TimeSeriesPoint{
			Timestamp: timestamp,
			Value:     value,
		})
	}

	return c.JSON(points)
}
