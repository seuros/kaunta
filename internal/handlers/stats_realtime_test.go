package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleCurrentVisitors_Success(t *testing.T) {
	websiteID := uuid.New()
	responses := []mockResponse{
		{
			match:   "SELECT COUNT(DISTINCT session_id)",
			args:    []interface{}{websiteID},
			columns: []string{"value"},
			rows: [][]interface{}{
				{7},
			},
		},
	}

	handler, queue, cleanup := setupHTTPTest(t, "/api/stats/realtime/{website_id}", HandleCurrentVisitors, responses)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/realtime/"+websiteID.String(), nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var payload map[string]int
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	assert.Equal(t, 7, payload["value"])

	require.NoError(t, queue.expectationsMet())
}

func TestHandleCurrentVisitors_InvalidWebsiteID(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/api/stats/realtime/{website_id}", HandleCurrentVisitors)

	req := httptest.NewRequest(http.MethodGet, "/api/stats/realtime/not-a-uuid", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestHandleCurrentVisitors_QueryError(t *testing.T) {
	websiteID := uuid.New()
	responses := []mockResponse{
		{
			match: "SELECT COUNT(DISTINCT session_id)",
			args:  []interface{}{websiteID},
			err:   assert.AnError,
		},
	}

	handler, queue, cleanup := setupHTTPTest(t, "/api/stats/realtime/{website_id}", HandleCurrentVisitors, responses)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/stats/realtime/"+websiteID.String(), nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusInternalServerError, resp.Code)
	require.NoError(t, queue.expectationsMet())
}
