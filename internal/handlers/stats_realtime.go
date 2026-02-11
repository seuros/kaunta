package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/httpx"
)

// HandleCurrentVisitors returns count of visitors in last 5 minutes
// GET /api/stats/realtime/:website_id
func HandleCurrentVisitors(w http.ResponseWriter, r *http.Request) {
	websiteIDStr := chi.URLParam(r, "website_id")
	websiteID, err := uuid.Parse(websiteIDStr)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid website ID")
		return
	}

	// Count distinct sessions from last 5 minutes
	// (Plausible uses last 5 minutes as default)
	query := `
		SELECT COUNT(DISTINCT session_id)
		FROM website_event
		WHERE website_id = $1
		  AND created_at >= NOW() - INTERVAL '5 minutes'
		  AND event_type = 1
	`

	var count int
	if err := database.DB.QueryRow(query, websiteID).Scan(&count); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "Failed to query current visitors")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"value": count,
	})
}
