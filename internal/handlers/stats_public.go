package handlers

import (
	"database/sql"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/middleware"
)

// PublicStats represents the public-facing stats response
type PublicStats struct {
	Online    int   `json:"online"`
	Pageviews int64 `json:"pageviews"`
	Visitors  int64 `json:"visitors"`
}

// getPublicStatsData fetches online users, total pageviews, and visitors for a website
func getPublicStatsData(websiteID uuid.UUID) (*PublicStats, error) {
	stats := &PublicStats{}

	// Get online users (distinct sessions in last 5 minutes)
	onlineQuery := `
		SELECT COUNT(DISTINCT session_id)
		FROM website_event
		WHERE website_id = $1
		  AND created_at >= NOW() - INTERVAL '5 minutes'
		  AND event_type = 1
	`
	if err := database.DB.QueryRow(onlineQuery, websiteID).Scan(&stats.Online); err != nil {
		stats.Online = 0
	}

	// Get total pageviews and unique visitors (all time)
	totalsQuery := `
		SELECT
			COUNT(*) as pageviews,
			COUNT(DISTINCT session_id) as visitors
		FROM website_event
		WHERE website_id = $1
		  AND event_type = 1
	`
	if err := database.DB.QueryRow(totalsQuery, websiteID).Scan(&stats.Pageviews, &stats.Visitors); err != nil {
		stats.Pageviews = 0
		stats.Visitors = 0
	}

	return stats, nil
}

// HandlePublicStats returns public stats for a website (no auth required)
// Only works if public_stats_enabled is true for the website
// GET /api/public/stats/:website_id
func HandlePublicStats(c fiber.Ctx) error {
	// Set CORS headers for public endpoint
	c.Set("Access-Control-Allow-Origin", "*")
	c.Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	c.Set("Access-Control-Allow-Headers", "Content-Type")

	// Handle preflight
	if c.Method() == "OPTIONS" {
		return c.SendStatus(204)
	}

	websiteIDStr := c.Params("website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid website ID",
		})
	}

	// Check if website exists and has public stats enabled
	var publicStatsEnabled bool
	query := `
		SELECT public_stats_enabled
		FROM website
		WHERE website_id = $1
		  AND deleted_at IS NULL
	`
	err = database.DB.QueryRow(query, websiteID).Scan(&publicStatsEnabled)
	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{
			"error": "Website not found",
		})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Database error",
		})
	}

	if !publicStatsEnabled {
		return c.Status(404).JSON(fiber.Map{
			"error": "Public stats not enabled for this website",
		})
	}

	stats, err := getPublicStatsData(websiteID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to fetch stats",
		})
	}

	return c.JSON(stats)
}

// HandleAPIStats returns stats for a website via API key (always available)
// Requires API key with 'stats' scope
// GET /api/v1/stats/:website_id
func HandleAPIStats(c fiber.Ctx) error {
	websiteIDStr := c.Params("website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid website ID",
		})
	}

	// Verify API key has access to this website
	apiKey := middleware.GetAPIKey(c)
	if apiKey == nil {
		return c.Status(401).JSON(fiber.Map{
			"error": "Unauthorized",
		})
	}

	// Check if API key has stats scope
	if !apiKey.HasScope("stats") {
		return c.Status(403).JSON(fiber.Map{
			"error": "API key does not have stats permission",
		})
	}

	// Verify website matches API key's website
	if apiKey.WebsiteID != websiteID {
		return c.Status(403).JSON(fiber.Map{
			"error": "API key not authorized for this website",
		})
	}

	// Check if website exists
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM website WHERE website_id = $1 AND deleted_at IS NULL)`
	if err := database.DB.QueryRow(query, websiteID).Scan(&exists); err != nil || !exists {
		return c.Status(404).JSON(fiber.Map{
			"error": "Website not found",
		})
	}

	stats, err := getPublicStatsData(websiteID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to fetch stats",
		})
	}

	return c.JSON(stats)
}
