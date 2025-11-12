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

func TestHandleTimeSeries_Success(t *testing.T) {
	websiteID := uuid.New()
	responses := []mockResponse{
		{
			match:   "SELECT * FROM get_timeseries",
			args:    []interface{}{websiteID, 7, nil, nil, nil, nil},
			columns: []string{"hour", "views"},
			rows: [][]interface{}{
				{"2025-11-05T14:00:00Z", int64(10)},
				{"2025-11-05T15:00:00Z", int64(7)},
			},
		},
	}

	app, queue, cleanup := setupFiberTest(t, "/api/dashboard/timeseries/:website_id", HandleTimeSeries, responses)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/timeseries/"+websiteID.String(), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var points []TimeSeriesPoint
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&points))
	assert.Len(t, points, 2)

	require.NoError(t, queue.expectationsMet())
}

func TestHandleTimeSeries_WithFilters(t *testing.T) {
	websiteID := uuid.New()
	responses := []mockResponse{
		{
			match:   "SELECT * FROM get_timeseries",
			args:    []interface{}{websiteID, 30, "US", "Chrome", "mobile", "/docs"},
			columns: []string{"hour", "views"},
			rows: [][]interface{}{
				{"2025-11-05T14:00:00Z", int64(5)},
			},
		},
	}

	app, queue, cleanup := setupFiberTest(t, "/api/dashboard/timeseries/:website_id", HandleTimeSeries, responses)
	defer cleanup()

	url := "/api/dashboard/timeseries/" + websiteID.String() + "?days=30&country=US&browser=Chrome&device=mobile&page=/docs"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, queue.expectationsMet())
}

func TestHandleTimeSeries_InvalidWebsiteID(t *testing.T) {
	app := fiber.New()
	app.Get("/api/dashboard/timeseries/:website_id", HandleTimeSeries)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/timeseries/not-a-uuid", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHandleTimeSeries_QueryError(t *testing.T) {
	websiteID := uuid.New()
	responses := []mockResponse{
		{
			match: "SELECT * FROM get_timeseries",
			args:  []interface{}{websiteID, 7, nil, nil, nil, nil},
			err:   assert.AnError,
		},
	}

	app, queue, cleanup := setupFiberTest(t, "/api/dashboard/timeseries/:website_id", HandleTimeSeries, responses)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/timeseries/"+websiteID.String(), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	require.NoError(t, queue.expectationsMet())
}
