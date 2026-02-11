package handlers

import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/httpx"
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
func HandlePublicStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	websiteIDStr := chi.URLParam(r, "website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid website ID")
		return
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
		httpx.Error(w, http.StatusNotFound, "Website not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "Database error")
		return
	}

	if !publicStatsEnabled {
		httpx.Error(w, http.StatusNotFound, "Public stats not enabled for this website")
		return
	}

	stats, err := getPublicStatsData(websiteID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "Failed to fetch stats")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, stats)
}

// HandleAPIStats returns stats for a website via API key (always available)
// Requires API key with 'stats' scope
// GET /api/v1/stats/:website_id
func HandleAPIStats(w http.ResponseWriter, r *http.Request) {
	websiteIDStr := chi.URLParam(r, "website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid website ID")
		return
	}

	apiKey := middleware.GetAPIKey(r)
	if apiKey == nil {
		httpx.Error(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Check if API key has stats scope
	if !apiKey.HasScope("stats") {
		httpx.Error(w, http.StatusForbidden, "API key does not have stats permission")
		return
	}

	// Verify website matches API key's website
	if apiKey.WebsiteID != websiteID {
		httpx.Error(w, http.StatusForbidden, "API key not authorized for this website")
		return
	}

	// Check if website exists
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM website WHERE website_id = $1 AND deleted_at IS NULL)`
	if err := database.DB.QueryRow(query, websiteID).Scan(&exists); err != nil || !exists {
		httpx.Error(w, http.StatusNotFound, "Website not found")
		return
	}

	stats, err := getPublicStatsData(websiteID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "Failed to fetch stats")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, stats)
}
