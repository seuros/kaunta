package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/seuros/kaunta/internal/database"
)

// HandleGoalAnalytics returns conversion metrics for a goal
// GET /api/dashboard/goals/:goal_id/analytics?days=7&country=US&browser=Chrome&device=desktop&page=/landing
func HandleGoalAnalytics(c fiber.Ctx) error {
	goalIDStr := c.Params("goal_id")
	goalID, err := uuid.Parse(goalIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid goal id"})
	}

	days := fiber.Query[int](c, "days", 7)
	if days > 90 {
		days = 90
	}

	country := c.Query("country")
	browser := c.Query("browser")
	device := c.Query("device")
	page := c.Query("page")

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

	query := `SELECT * FROM get_goal_analytics($1, $2, $3, $4, $5, $6)`
	var completions, uniqueSessions, totalSessions int64
	var conversionRate float64

	err = database.DB.QueryRow(
		query,
		goalID,
		days,
		countryParam,
		browserParam,
		deviceParam,
		pageParam,
	).Scan(&completions, &uniqueSessions, &conversionRate, &totalSessions)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to query goal analytics"})
	}

	return c.JSON(GoalAnalytics{
		Completions:    int(completions),
		UniqueSessions: int(uniqueSessions),
		ConversionRate: conversionRate,
		TotalSessions:  int(totalSessions),
	})
}

// HandleGoalTimeSeries returns time series of goal completions
// GET /api/dashboard/goals/:goal_id/timeseries?days=7&country=US
func HandleGoalTimeSeries(c fiber.Ctx) error {
	goalIDStr := c.Params("goal_id")
	goalID, err := uuid.Parse(goalIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid goal id"})
	}

	days := fiber.Query[int](c, "days", 7)
	if days > 90 {
		days = 90
	}

	country := c.Query("country")
	browser := c.Query("browser")
	device := c.Query("device")
	page := c.Query("page")

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

	query := `SELECT * FROM get_goal_timeseries($1, $2, $3, $4, $5, $6)`
	rows, err := database.DB.Query(
		query,
		goalID,
		days,
		countryParam,
		browserParam,
		deviceParam,
		pageParam,
	)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to query goal timeseries"})
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

// HandleGoalBreakdown returns breakdown by dimension
// GET /api/dashboard/goals/:goal_id/breakdown/:dimension?days=7&per=10&offset=0
func HandleGoalBreakdown(c fiber.Ctx) error {
	goalIDStr := c.Params("goal_id")
	goalID, err := uuid.Parse(goalIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid goal id"})
	}

	dimension := c.Params("dimension")
	if dimension == "" {
		return c.Status(400).JSON(fiber.Map{"error": "dimension required"})
	}

	pagination := ParsePaginationParamsWithValidation(c, "breakdown")

	days := fiber.Query[int](c, "days", 7)
	if days > 90 {
		days = 90
	}

	country := c.Query("country")
	browser := c.Query("browser")
	device := c.Query("device")
	page := c.Query("page")

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

	query := `SELECT * FROM get_goal_breakdown($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	rows, err := database.DB.Query(
		query,
		goalID,
		dimension,
		days,
		pagination.Per,
		pagination.Offset,
		countryParam,
		browserParam,
		deviceParam,
		pageParam,
	)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to query goal breakdown"})
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
		totalCount = rowTotal
		items = append(items, item)
	}

	return c.JSON(NewPaginatedResponse(items, pagination, totalCount))
}

// HandleGoalConvertingPages returns pages visited before goal completion
// GET /api/dashboard/goals/:goal_id/converting-pages?days=7&per=10&offset=0
func HandleGoalConvertingPages(c fiber.Ctx) error {
	goalIDStr := c.Params("goal_id")
	goalID, err := uuid.Parse(goalIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid goal id"})
	}

	pagination := ParsePaginationParamsWithValidation(c, "pages")

	days := fiber.Query[int](c, "days", 7)
	if days > 90 {
		days = 90
	}

	country := c.Query("country")
	browser := c.Query("browser")
	device := c.Query("device")

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

	query := `SELECT * FROM get_goal_converting_pages($1, $2, $3, $4, $5, $6, $7)`
	rows, err := database.DB.Query(
		query,
		goalID,
		days,
		pagination.Per,
		pagination.Offset,
		countryParam,
		browserParam,
		deviceParam,
	)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to query converting pages"})
	}
	defer func() { _ = rows.Close() }()

	type ConvertingPage struct {
		Path        string `json:"path"`
		Conversions int    `json:"conversions"`
	}

	items := make([]ConvertingPage, 0)
	var totalCount int64
	for rows.Next() {
		var path string
		var conversions int64
		var rowTotal int64
		if err := rows.Scan(&path, &conversions, &rowTotal); err != nil {
			continue
		}
		totalCount = rowTotal
		items = append(items, ConvertingPage{
			Path:        path,
			Conversions: int(conversions),
		})
	}

	return c.JSON(NewPaginatedResponse(items, pagination, totalCount))
}
