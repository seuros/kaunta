package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleGoalList_Success(t *testing.T) {
	websiteID := uuid.New().String()
	goalID1 := uuid.New().String()
	goalID2 := uuid.New().String()
	now := time.Now()

	responses := []mockResponse{
		{
			match:   "SELECT id, website_id, name, target_url, target_event, created_at, updated_at FROM goals WHERE website_id = $1",
			args:    []interface{}{websiteID},
			columns: []string{"id", "website_id", "name", "target_url", "target_event", "created_at", "updated_at"},
			rows: [][]interface{}{
				{goalID1, websiteID, "Signup Goal", "/signup", nil, now, now},
				{goalID2, websiteID, "Purchase Goal", nil, "purchase", now, now},
			},
		},
	}

	queue := newMockQueue(responses)
	driverName, err := registerMockDriver(queue)
	require.NoError(t, err)

	db, err := sql.Open(driverName, "")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	original := database.DB
	database.DB = db
	defer func() { database.DB = original }()

	app := fiber.New()
	app.Get("/api/goals/:website_id", HandleGoalList)

	req := httptest.NewRequest(http.MethodGet, "/api/goals/"+websiteID, nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var goals []models.Goal
	err = json.NewDecoder(resp.Body).Decode(&goals)
	require.NoError(t, err)
	assert.Len(t, goals, 2)
	assert.Equal(t, "Signup Goal", goals[0].Name)

	require.NoError(t, queue.expectationsMet())
}

func TestHandleGoalList_InvalidWebsiteID(t *testing.T) {
	app := fiber.New()
	app.Get("/api/goals/:website_id", HandleGoalList)

	req := httptest.NewRequest(http.MethodGet, "/api/goals/invalid-uuid", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "invalid website_id", result["error"])
}

func TestHandleGoalList_DatabaseError(t *testing.T) {
	websiteID := uuid.New().String()
	responses := []mockResponse{
		{
			match: "SELECT id, website_id, name, target_url, target_event, created_at, updated_at FROM goals WHERE website_id = $1",
			args:  []interface{}{websiteID},
			err:   assert.AnError,
		},
	}

	queue := newMockQueue(responses)
	driverName, err := registerMockDriver(queue)
	require.NoError(t, err)

	db, err := sql.Open(driverName, "")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	original := database.DB
	database.DB = db
	defer func() { database.DB = original }()

	app := fiber.New()
	app.Get("/api/goals/:website_id", HandleGoalList)

	req := httptest.NewRequest(http.MethodGet, "/api/goals/"+websiteID, nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "database error", result["error"])

	require.NoError(t, queue.expectationsMet())
}
