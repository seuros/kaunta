package handlers

import (
	"fmt"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/seuros/kaunta/internal/database"
)

// HandleTopPages returns top pages for the dashboard
func HandleTopPages(c fiber.Ctx) error {
	websiteIDStr := c.Params("website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid website ID",
		})
	}

	limit := fiber.Query[int](c, "limit", 10)
	filterClause, filterArgs := buildFilterClause(c, []interface{}{websiteID})

	// Add limit to args
	filterArgs = append(filterArgs, limit)
	limitParam := fmt.Sprintf("$%d", len(filterArgs))

	query := `
		SELECT e.url_path, COUNT(*) as views
		FROM website_event e
		JOIN session s ON e.session_id = s.session_id
		WHERE e.website_id = $1
		  AND e.created_at >= CURRENT_DATE
		  AND e.event_type = 1
		  AND e.url_path IS NOT NULL` + filterClause + `
		GROUP BY e.url_path
		ORDER BY views DESC
		LIMIT ` + limitParam

	rows, err := database.DB.Query(query, filterArgs...)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to query top pages",
		})
	}
	defer func() { _ = rows.Close() }()

	pages := make([]TopPage, 0)
	for rows.Next() {
		var page TopPage
		if err := rows.Scan(&page.Path, &page.Views); err != nil {
			continue
		}
		pages = append(pages, page)
	}

	return c.JSON(pages)
}
