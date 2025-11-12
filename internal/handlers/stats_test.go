package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleDashboardStats_Success(t *testing.T) {
	websiteID := uuid.New()

	responses := []mockResponse{
		{
			match:   "SELECT * FROM get_dashboard_stats",
			args:    []interface{}{websiteID, nil, nil, nil, nil},
			columns: []string{"current_visitors", "today_pageviews", "today_visitors", "bounce_rate"},
			rows:    [][]interface{}{{int64(3), int64(12), int64(6), 33.3}},
		},
	}

	app, queue, cleanup := setupFiberTest(t, "/api/dashboard/stats/:website_id", HandleDashboardStats, responses)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/stats/"+websiteID.String(), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var stats DashboardStats
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&stats))
	assert.Equal(t, 3, stats.CurrentVisitors)
	assert.Equal(t, 12, stats.TodayPageviews)
	assert.Equal(t, 6, stats.TodayVisitors)
	assert.Equal(t, "33.3%", stats.TodayBounceRate)

	require.NoError(t, queue.expectationsMet())
}

func TestHandleDashboardStats_InvalidWebsiteID(t *testing.T) {
	app := fiber.New()
	app.Get("/api/dashboard/stats/:website_id", HandleDashboardStats)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/stats/not-a-uuid", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHandleDashboardStats_QueryErrors(t *testing.T) {
	websiteID := uuid.New()

	responses := []mockResponse{
		{
			match: "SELECT * FROM get_dashboard_stats",
			args:  []interface{}{websiteID, nil, nil, nil, nil},
			err:   assert.AnError,
		},
	}

	app, queue, cleanup := setupFiberTest(t, "/api/dashboard/stats/:website_id", HandleDashboardStats, responses)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/stats/"+websiteID.String(), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var stats DashboardStats
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&stats))
	assert.Equal(t, 0, stats.CurrentVisitors)
	assert.Equal(t, 0, stats.TodayPageviews)
	assert.Equal(t, 0, stats.TodayVisitors)
	assert.Equal(t, "0%", stats.TodayBounceRate)

	require.NoError(t, queue.expectationsMet())
}
