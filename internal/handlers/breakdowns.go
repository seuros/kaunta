package handlers

import (
	"fmt"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/seuros/kaunta/internal/database"
)

// HandleTopReferrers returns top referrers breakdown
func HandleTopReferrers(c fiber.Ctx) error {
	websiteIDStr := c.Params("website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid website ID"})
	}

	limit := fiber.Query[int](c, "limit", 10)
	filterClause, filterArgs := buildFilterClause(c, []interface{}{websiteID})

	// Add limit to args
	filterArgs = append(filterArgs, limit)
	limitParam := fmt.Sprintf("$%d", len(filterArgs))

	query := `
		SELECT COALESCE(e.referrer_domain, 'Direct / None') as name, COUNT(*) as count
		FROM website_event e
		JOIN session s ON e.session_id = s.session_id
		WHERE e.website_id = $1
		  AND e.created_at >= CURRENT_DATE
		  AND e.event_type = 1` + filterClause + `
		GROUP BY e.referrer_domain
		ORDER BY count DESC
		LIMIT ` + limitParam

	rows, err := database.DB.Query(query, filterArgs...)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to query referrers"})
	}
	defer func() {
		_ = rows.Close()
	}()

	items := make([]BreakdownItem, 0)
	for rows.Next() {
		var item BreakdownItem
		if err := rows.Scan(&item.Name, &item.Count); err != nil {
			continue
		}
		items = append(items, item)
	}

	return c.JSON(items)
}

// HandleTopBrowsers returns top browsers breakdown
func HandleTopBrowsers(c fiber.Ctx) error {
	websiteIDStr := c.Params("website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid website ID"})
	}

	limit := fiber.Query[int](c, "limit", 10)
	filterClause, filterArgs := buildFilterClause(c, []interface{}{websiteID})

	// Add limit to args
	filterArgs = append(filterArgs, limit)
	limitParam := fmt.Sprintf("$%d", len(filterArgs))

	query := `
		SELECT COALESCE(s.browser, 'Unknown') as name, COUNT(*) as count
		FROM website_event e
		JOIN session s ON e.session_id = s.session_id
		WHERE e.website_id = $1
		  AND e.created_at >= CURRENT_DATE
		  AND e.event_type = 1` + filterClause + `
		GROUP BY s.browser
		ORDER BY count DESC
		LIMIT ` + limitParam

	rows, err := database.DB.Query(query, filterArgs...)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to query browsers"})
	}
	defer func() { _ = rows.Close() }()

	items := make([]BreakdownItem, 0)
	for rows.Next() {
		var item BreakdownItem
		if err := rows.Scan(&item.Name, &item.Count); err != nil {
			continue
		}
		items = append(items, item)
	}

	return c.JSON(items)
}

// HandleTopDevices returns top devices breakdown
func HandleTopDevices(c fiber.Ctx) error {
	websiteIDStr := c.Params("website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid website ID"})
	}

	limit := fiber.Query[int](c, "limit", 10)
	filterClause, filterArgs := buildFilterClause(c, []interface{}{websiteID})

	// Add limit to args
	filterArgs = append(filterArgs, limit)
	limitParam := fmt.Sprintf("$%d", len(filterArgs))

	query := `
		SELECT COALESCE(s.device, 'Unknown') as name, COUNT(*) as count
		FROM website_event e
		JOIN session s ON e.session_id = s.session_id
		WHERE e.website_id = $1
		  AND e.created_at >= CURRENT_DATE
		  AND e.event_type = 1` + filterClause + `
		GROUP BY s.device
		ORDER BY count DESC
		LIMIT ` + limitParam

	rows, err := database.DB.Query(query, filterArgs...)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to query devices"})
	}
	defer func() { _ = rows.Close() }()

	items := make([]BreakdownItem, 0)
	for rows.Next() {
		var item BreakdownItem
		if err := rows.Scan(&item.Name, &item.Count); err != nil {
			continue
		}
		items = append(items, item)
	}

	return c.JSON(items)
}

// HandleTopCountries returns top countries breakdown
func HandleTopCountries(c fiber.Ctx) error {
	websiteIDStr := c.Params("website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid website ID"})
	}

	limit := fiber.Query[int](c, "limit", 10)
	filterClause, filterArgs := buildFilterClause(c, []interface{}{websiteID})

	// Add limit to args
	filterArgs = append(filterArgs, limit)
	limitParam := fmt.Sprintf("$%d", len(filterArgs))

	query := `
		SELECT COALESCE(s.country, 'Unknown') as name, COUNT(*) as count
		FROM website_event e
		JOIN session s ON e.session_id = s.session_id
		WHERE e.website_id = $1
		  AND e.created_at >= CURRENT_DATE
		  AND e.event_type = 1` + filterClause + `
		GROUP BY s.country
		ORDER BY count DESC
		LIMIT ` + limitParam

	rows, err := database.DB.Query(query, filterArgs...)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to query countries"})
	}
	defer func() { _ = rows.Close() }()

	items := make([]BreakdownItem, 0)
	for rows.Next() {
		var item BreakdownItem
		if err := rows.Scan(&item.Name, &item.Count); err != nil {
			continue
		}
		items = append(items, item)
	}

	return c.JSON(items)
}

// HandleTopCities returns top cities breakdown
func HandleTopCities(c fiber.Ctx) error {
	websiteIDStr := c.Params("website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid website ID"})
	}

	limit := fiber.Query[int](c, "limit", 10)
	filterClause, filterArgs := buildFilterClause(c, []interface{}{websiteID})

	// Add limit to args
	filterArgs = append(filterArgs, limit)
	limitParam := fmt.Sprintf("$%d", len(filterArgs))

	query := `
		SELECT COALESCE(s.city, 'Unknown') as name, COUNT(*) as count
		FROM website_event e
		JOIN session s ON e.session_id = s.session_id
		WHERE e.website_id = $1
		  AND e.created_at >= CURRENT_DATE
		  AND e.event_type = 1` + filterClause + `
		GROUP BY s.city
		ORDER BY count DESC
		LIMIT ` + limitParam

	rows, err := database.DB.Query(query, filterArgs...)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to query cities"})
	}
	defer func() { _ = rows.Close() }()

	items := make([]BreakdownItem, 0)
	for rows.Next() {
		var item BreakdownItem
		if err := rows.Scan(&item.Name, &item.Count); err != nil {
			continue
		}
		items = append(items, item)
	}

	return c.JSON(items)
}

// HandleTopRegions returns top regions breakdown
func HandleTopRegions(c fiber.Ctx) error {
	websiteIDStr := c.Params("website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid website ID"})
	}

	limit := fiber.Query[int](c, "limit", 10)
	filterClause, filterArgs := buildFilterClause(c, []interface{}{websiteID})

	// Add limit to args
	filterArgs = append(filterArgs, limit)
	limitParam := fmt.Sprintf("$%d", len(filterArgs))

	query := `
		SELECT COALESCE(s.region, 'Unknown') as name, COUNT(*) as count
		FROM website_event e
		JOIN session s ON e.session_id = s.session_id
		WHERE e.website_id = $1
		  AND e.created_at >= CURRENT_DATE
		  AND e.event_type = 1` + filterClause + `
		GROUP BY s.region
		ORDER BY count DESC
		LIMIT ` + limitParam

	rows, err := database.DB.Query(query, filterArgs...)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to query regions"})
	}
	defer func() { _ = rows.Close() }()

	items := make([]BreakdownItem, 0)
	for rows.Next() {
		var item BreakdownItem
		if err := rows.Scan(&item.Name, &item.Count); err != nil {
			continue
		}
		items = append(items, item)
	}

	return c.JSON(items)
}

// HandleMapData returns visitor data aggregated by country for choropleth maps
func HandleMapData(c fiber.Ctx) error {
	websiteIDStr := c.Params("website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid website ID"})
	}

	// Get date range (default 7 days, clamp between 1 and 90)
	days := min(max(fiber.Query[int](c, "days", 7), 1), 90)

	// Build filter clause
	baseArgs := []interface{}{websiteID, days}
	filterClause, filterArgs := buildFilterClause(c, baseArgs)

	// First get total visitor count
	var totalVisitors int
	totalQuery := `
		SELECT COUNT(DISTINCT e.session_id)
		FROM website_event e
		JOIN session s ON e.session_id = s.session_id
		WHERE e.website_id = $1
		  AND e.created_at >= NOW() - INTERVAL '1 day' * $2
		  AND e.event_type = 1` + filterClause

	if err := database.DB.QueryRow(totalQuery, filterArgs...).Scan(&totalVisitors); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to query total visitors"})
	}

	// Query country aggregation
	query := `
		SELECT
			COALESCE(s.country, 'Unknown') as country,
			COUNT(DISTINCT e.session_id) as visitors
		FROM website_event e
		JOIN session s ON e.session_id = s.session_id
		WHERE e.website_id = $1
		  AND e.created_at >= NOW() - INTERVAL '1 day' * $2
		  AND e.event_type = 1` + filterClause + `
		GROUP BY s.country
		ORDER BY visitors DESC`

	rows, err := database.DB.Query(query, filterArgs...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to query map data"})
	}
	defer func() { _ = rows.Close() }()

	var data []MapDataPoint
	for rows.Next() {
		var country string
		var visitors int
		if err := rows.Scan(&country, &visitors); err != nil {
			continue
		}

		percentage := 0.0
		if totalVisitors > 0 {
			percentage = (float64(visitors) / float64(totalVisitors)) * 100
		}

		data = append(data, MapDataPoint{
			Country:     country,
			CountryName: getCountryName(country),
			Code:        getAlpha3Code(country),
			Visitors:    visitors,
			Percentage:  percentage,
		})
	}

	return c.JSON(MapResponse{
		Data:          data,
		TotalVisitors: totalVisitors,
		PeriodDays:    days,
	})
}
