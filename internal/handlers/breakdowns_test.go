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

func TestHandleTopReferrers_Success(t *testing.T) {
	websiteID := uuid.New()
	responses := []mockResponse{
		{
			match:   "SELECT * FROM get_breakdown(",
			columns: []string{"name", "count", "total_count"},
			rows:    [][]interface{}{{"example.com", int64(12), int64(1)}},
		},
	}

	app, queue, cleanup := setupFiberTest(t, "/api/dashboard/referrers/:website_id", HandleTopReferrers, responses)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/referrers/"+websiteID.String(), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var paginatedResp PaginatedResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&paginatedResp))

	itemsJSON, err := json.Marshal(paginatedResp.Data)
	require.NoError(t, err)
	var items []BreakdownItem
	require.NoError(t, json.Unmarshal(itemsJSON, &items))

	assert.Len(t, items, 1)
	assert.Equal(t, "example.com", items[0].Name)
	assert.Equal(t, int64(1), paginatedResp.Pagination.Total)

	require.NoError(t, queue.expectationsMet())
}

func TestHandleTopReferrers_Filtered(t *testing.T) {
	websiteID := uuid.New()
	responses := []mockResponse{
		{
			match:   "SELECT * FROM get_breakdown(",
			columns: []string{"name", "count", "total_count"},
			rows:    [][]interface{}{{"example.com", int64(5), int64(1)}},
		},
	}

	app, queue, cleanup := setupFiberTest(t, "/api/dashboard/referrers/:website_id", HandleTopReferrers, responses)
	defer cleanup()

	url := "/api/dashboard/referrers/" + websiteID.String() + "?per=5&country=US&browser=Chrome&device=mobile"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, queue.expectationsMet())
}

func TestHandleTopBrowsers_Success(t *testing.T) {
	websiteID := uuid.New()
	responses := []mockResponse{
		{
			match:   "SELECT * FROM get_breakdown(",
			columns: []string{"name", "count", "total_count"},
			rows:    [][]interface{}{{"Chrome", int64(20), int64(1)}},
		},
	}

	app, queue, cleanup := setupFiberTest(t, "/api/dashboard/browsers/:website_id", HandleTopBrowsers, responses)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/browsers/"+websiteID.String(), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, queue.expectationsMet())
}

func TestHandleTopDevices_Success(t *testing.T) {
	websiteID := uuid.New()
	responses := []mockResponse{
		{
			match:   "SELECT * FROM get_breakdown(",
			columns: []string{"name", "count", "total_count"},
			rows:    [][]interface{}{{"mobile", int64(8), int64(1)}},
		},
	}

	app, queue, cleanup := setupFiberTest(t, "/api/dashboard/devices/:website_id", HandleTopDevices, responses)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/devices/"+websiteID.String(), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, queue.expectationsMet())
}

func TestHandleTopCountries_Success(t *testing.T) {
	websiteID := uuid.New()
	responses := []mockResponse{
		{
			match:   "SELECT * FROM get_breakdown(",
			columns: []string{"name", "count", "total_count"},
			rows:    [][]interface{}{{"US", int64(15), int64(1)}},
		},
	}

	app, queue, cleanup := setupFiberTest(t, "/api/dashboard/countries/:website_id", HandleTopCountries, responses)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/countries/"+websiteID.String(), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var paginatedResp PaginatedResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&paginatedResp))

	itemsJSON, err := json.Marshal(paginatedResp.Data)
	require.NoError(t, err)
	var items []BreakdownItem
	require.NoError(t, json.Unmarshal(itemsJSON, &items))

	assert.Len(t, items, 1)
	assert.Equal(t, "United States", items[0].Name) // ISO code converted to full name
	assert.Equal(t, "US", items[0].Code)            // ISO code preserved for flag emoji
	assert.Equal(t, 15, items[0].Count)

	require.NoError(t, queue.expectationsMet())
}

func TestHandleTopCountries_MultipleCountries(t *testing.T) {
	websiteID := uuid.New()
	responses := []mockResponse{
		{
			match:   "SELECT * FROM get_breakdown(",
			columns: []string{"name", "count", "total_count"},
			rows: [][]interface{}{
				{"DE", int64(20), int64(3)},
				{"FR", int64(15), int64(3)},
				{"NL", int64(10), int64(3)},
			},
		},
	}

	app, queue, cleanup := setupFiberTest(t, "/api/dashboard/countries/:website_id", HandleTopCountries, responses)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/countries/"+websiteID.String(), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var paginatedResp PaginatedResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&paginatedResp))

	itemsJSON, err := json.Marshal(paginatedResp.Data)
	require.NoError(t, err)
	var items []BreakdownItem
	require.NoError(t, json.Unmarshal(itemsJSON, &items))

	assert.Len(t, items, 3)

	// Verify ISO codes are preserved and names are converted
	assert.Equal(t, "Germany", items[0].Name)
	assert.Equal(t, "DE", items[0].Code)
	assert.Equal(t, "France", items[1].Name)
	assert.Equal(t, "FR", items[1].Code)
	assert.Equal(t, "Netherlands", items[2].Name)
	assert.Equal(t, "NL", items[2].Code)

	require.NoError(t, queue.expectationsMet())
}

func TestHandleTopOS_Success(t *testing.T) {
	websiteID := uuid.New()
	responses := []mockResponse{
		{
			match:   "SELECT * FROM get_breakdown(",
			columns: []string{"name", "count", "total_count"},
			rows:    [][]interface{}{{"Windows", int64(25), int64(1)}},
		},
	}

	app, queue, cleanup := setupFiberTest(t, "/api/dashboard/os/:website_id", HandleTopOS, responses)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/os/"+websiteID.String(), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var paginatedResp PaginatedResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&paginatedResp))

	itemsJSON, err := json.Marshal(paginatedResp.Data)
	require.NoError(t, err)
	var items []BreakdownItem
	require.NoError(t, json.Unmarshal(itemsJSON, &items))

	assert.Len(t, items, 1)
	assert.Equal(t, "Windows", items[0].Name)
	assert.Equal(t, 25, items[0].Count)
	assert.Equal(t, int64(1), paginatedResp.Pagination.Total)

	require.NoError(t, queue.expectationsMet())
}

func TestHandleTopOS_MultipleOS(t *testing.T) {
	websiteID := uuid.New()
	responses := []mockResponse{
		{
			match:   "SELECT * FROM get_breakdown(",
			columns: []string{"name", "count", "total_count"},
			rows: [][]interface{}{
				{"Windows", int64(40), int64(4)},
				{"macOS", int64(30), int64(4)},
				{"Linux", int64(20), int64(4)},
				{"iOS", int64(10), int64(4)},
			},
		},
	}

	app, queue, cleanup := setupFiberTest(t, "/api/dashboard/os/:website_id", HandleTopOS, responses)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/os/"+websiteID.String(), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var paginatedResp PaginatedResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&paginatedResp))

	itemsJSON, err := json.Marshal(paginatedResp.Data)
	require.NoError(t, err)
	var items []BreakdownItem
	require.NoError(t, json.Unmarshal(itemsJSON, &items))

	assert.Len(t, items, 4)
	assert.Equal(t, "Windows", items[0].Name)
	assert.Equal(t, "macOS", items[1].Name)
	assert.Equal(t, "Linux", items[2].Name)
	assert.Equal(t, "iOS", items[3].Name)
	assert.Equal(t, int64(4), paginatedResp.Pagination.Total)

	require.NoError(t, queue.expectationsMet())
}

func TestHandleTopOS_InvalidWebsiteID(t *testing.T) {
	app := fiber.New()
	app.Get("/api/dashboard/os/:website_id", HandleTopOS)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/os/invalid-uuid", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHandleTopOS_WithFilters(t *testing.T) {
	websiteID := uuid.New()
	responses := []mockResponse{
		{
			match:   "SELECT * FROM get_breakdown(",
			columns: []string{"name", "count", "total_count"},
			rows:    [][]interface{}{{"Windows", int64(15), int64(1)}},
		},
	}

	app, queue, cleanup := setupFiberTest(t, "/api/dashboard/os/:website_id", HandleTopOS, responses)
	defer cleanup()

	url := "/api/dashboard/os/" + websiteID.String() + "?per=10&country=MA&browser=Chrome&device=desktop"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, queue.expectationsMet())
}

func TestBreakdownHandlers_InvalidWebsiteID(t *testing.T) {
	type invalidCase struct {
		route   string
		handler fiber.Handler
	}
	cases := []invalidCase{
		{"/api/dashboard/referrers/:website_id", HandleTopReferrers},
		{"/api/dashboard/browsers/:website_id", HandleTopBrowsers},
		{"/api/dashboard/devices/:website_id", HandleTopDevices},
		{"/api/dashboard/countries/:website_id", HandleTopCountries},
		{"/api/dashboard/os/:website_id", HandleTopOS},
	}

	for _, tc := range cases {
		app := fiber.New()
		app.Get(tc.route, tc.handler)

		req := httptest.NewRequest(http.MethodGet, tc.route[:len(tc.route)-len(":website_id")]+"invalid", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		_ = resp.Body.Close()
	}
}
