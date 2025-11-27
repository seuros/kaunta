package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/seuros/kaunta/internal/database"
)

// HandleTopPages returns top pages for the dashboard
// Uses PostgreSQL function get_top_pages() for optimized query execution
func HandleTopPages(c fiber.Ctx) error {
	websiteIDStr := c.Params("website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid website ID",
		})
	}

	// Parse pagination parameters with validation for pages endpoints
	pagination := ParsePaginationParamsWithValidation(c, "pages")

	// Extract filter parameters
	country := c.Query("country")
	browser := c.Query("browser")
	device := c.Query("device")

	// Convert empty strings to NULL for SQL
	var countryParam, browserParam, deviceParam interface{}
	if country != "" {
		countryParam = country
	}
	if browser != "" {
		browserParam = browser
	}
	if device != "" {
		deviceParam = device
	}

	// Call get_top_pages() function with pagination and sorting
	// Function returns: (path, views, unique_visitors, avg_engagement_time, total_count)
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
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to query top pages",
		})
	}
	defer func() { _ = rows.Close() }()

	pages := make([]TopPage, 0)
	var totalCount int64
	for rows.Next() {
		var path string
		var views int64
		var uniqueVisitors int64   // Not used in response, but returned by function
		var avgEngagement *float64 // Not used in response, but returned by function
		var rowTotal int64

		if err := rows.Scan(&path, &views, &uniqueVisitors, &avgEngagement, &rowTotal); err != nil {
			continue
		}

		totalCount = rowTotal // Capture total count from function

		pages = append(pages, TopPage{
			Path:  path,
			Views: int(views),
		})
	}

	// Return paginated response
	return c.JSON(NewPaginatedResponse(pages, pagination, totalCount))
}
