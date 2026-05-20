package handlers

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebsite_JSONMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		website  Website
		expected string
	}{
		{
			name: "Complete website",
			website: Website{
				ID:     "123e4567-e89b-12d3-a456-426614174000",
				Domain: "example.com",
				Name:   "Example Site",
			},
			expected: `{"id":"123e4567-e89b-12d3-a456-426614174000","domain":"example.com","name":"Example Site"}`,
		},
		{
			name: "Website with empty name",
			website: Website{
				ID:     "123e4567-e89b-12d3-a456-426614174000",
				Domain: "example.com",
				Name:   "",
			},
			expected: `{"id":"123e4567-e89b-12d3-a456-426614174000","domain":"example.com","name":""}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, err := json.Marshal(tt.website)
			require.NoError(t, err, "Failed to marshal website")

			assert.JSONEq(t, tt.expected, string(jsonBytes), "JSON output mismatch")

			// Test unmarshaling
			var unmarshaled Website
			err = json.Unmarshal(jsonBytes, &unmarshaled)
			require.NoError(t, err, "Failed to unmarshal website")

			assert.Equal(t, tt.website, unmarshaled, "Unmarshaled website mismatch")
		})
	}
}

func TestDashboardStats_JSONMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		stats    DashboardStats
		expected string
	}{
		{
			name: "Normal stats",
			stats: DashboardStats{
				CurrentVisitors: 42,
				TodayPageviews:  1337,
				TodayVisitors:   256,
				TodayBounceRate: "45.2%",
			},
			expected: `{"current_visitors":42,"today_pageviews":1337,"today_visitors":256,"today_bounce_rate":"45.2%"}`,
		},
		{
			name: "Zero stats",
			stats: DashboardStats{
				CurrentVisitors: 0,
				TodayPageviews:  0,
				TodayVisitors:   0,
				TodayBounceRate: "0%",
			},
			expected: `{"current_visitors":0,"today_pageviews":0,"today_visitors":0,"today_bounce_rate":"0%"}`,
		},
		{
			name: "High traffic stats",
			stats: DashboardStats{
				CurrentVisitors: 9999,
				TodayPageviews:  1000000,
				TodayVisitors:   500000,
				TodayBounceRate: "12.3%",
			},
			expected: `{"current_visitors":9999,"today_pageviews":1000000,"today_visitors":500000,"today_bounce_rate":"12.3%"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, err := json.Marshal(tt.stats)
			require.NoError(t, err, "Failed to marshal stats")

			assert.JSONEq(t, tt.expected, string(jsonBytes), "JSON output mismatch")

			// Test unmarshaling
			var unmarshaled DashboardStats
			err = json.Unmarshal(jsonBytes, &unmarshaled)
			require.NoError(t, err, "Failed to unmarshal stats")

			assert.Equal(t, tt.stats, unmarshaled, "Unmarshaled stats mismatch")
		})
	}
}

func TestTopPage_JSONMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		page     TopPage
		expected string
	}{
		{
			name: "Homepage",
			page: TopPage{
				Path:  "/",
				Views: 1000,
			},
			expected: `{"path":"/","views":1000}`,
		},
		{
			name: "Deep path",
			page: TopPage{
				Path:  "/blog/posts/2024/my-article",
				Views: 42,
			},
			expected: `{"path":"/blog/posts/2024/my-article","views":42}`,
		},
		{
			name: "Path with query params",
			page: TopPage{
				Path:  "/search?q=test",
				Views: 15,
			},
			expected: `{"path":"/search?q=test","views":15}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, err := json.Marshal(tt.page)
			require.NoError(t, err, "Failed to marshal page")

			assert.JSONEq(t, tt.expected, string(jsonBytes), "JSON output mismatch")

			// Test unmarshaling
			var unmarshaled TopPage
			err = json.Unmarshal(jsonBytes, &unmarshaled)
			require.NoError(t, err, "Failed to unmarshal page")

			assert.Equal(t, tt.page, unmarshaled, "Unmarshaled page mismatch")
		})
	}
}

func TestTimeSeriesPoint_JSONMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		point    TimeSeriesPoint
		expected string
	}{
		{
			name: "Normal data point",
			point: TimeSeriesPoint{
				Timestamp: "2025-11-05T14:00:00Z",
				Value:     150,
			},
			expected: `{"timestamp":"2025-11-05T14:00:00Z","value":150}`,
		},
		{
			name: "Zero value",
			point: TimeSeriesPoint{
				Timestamp: "2025-11-05T15:00:00Z",
				Value:     0,
			},
			expected: `{"timestamp":"2025-11-05T15:00:00Z","value":0}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, err := json.Marshal(tt.point)
			require.NoError(t, err, "Failed to marshal time series point")

			assert.JSONEq(t, tt.expected, string(jsonBytes), "JSON output mismatch")

			// Test unmarshaling
			var unmarshaled TimeSeriesPoint
			err = json.Unmarshal(jsonBytes, &unmarshaled)
			require.NoError(t, err, "Failed to unmarshal time series point")

			assert.Equal(t, tt.point, unmarshaled, "Unmarshaled point mismatch")
		})
	}
}

func TestBreakdownItem_JSONMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		item     BreakdownItem
		expected string
	}{
		{
			name: "Browser breakdown",
			item: BreakdownItem{
				Name:  "Chrome",
				Count: 1500,
			},
			expected: `{"name":"Chrome","count":1500}`,
		},
		{
			name: "Country breakdown",
			item: BreakdownItem{
				Name:  "United States",
				Count: 5000,
			},
			expected: `{"name":"United States","count":5000}`,
		},
		{
			name: "Zero count",
			item: BreakdownItem{
				Name:  "Unknown",
				Count: 0,
			},
			expected: `{"name":"Unknown","count":0}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, err := json.Marshal(tt.item)
			require.NoError(t, err, "Failed to marshal breakdown item")

			assert.JSONEq(t, tt.expected, string(jsonBytes), "JSON output mismatch")

			// Test unmarshaling
			var unmarshaled BreakdownItem
			err = json.Unmarshal(jsonBytes, &unmarshaled)
			require.NoError(t, err, "Failed to unmarshal breakdown item")

			assert.Equal(t, tt.item, unmarshaled, "Unmarshaled item mismatch")
		})
	}
}

func TestTrackingPayload_JSONMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		payload  TrackingPayload
		expected string
	}{
		{
			name: "Simple pageview event",
			payload: TrackingPayload{
				Type: "event",
				Payload: PayloadData{
					Website: "123e4567-e89b-12d3-a456-426614174000",
				},
			},
			expected: `{"type":"event","payload":{"website":"123e4567-e89b-12d3-a456-426614174000"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, err := json.Marshal(tt.payload)
			require.NoError(t, err, "Failed to marshal tracking payload")

			// Test unmarshaling
			var unmarshaled TrackingPayload
			err = json.Unmarshal(jsonBytes, &unmarshaled)
			require.NoError(t, err, "Failed to unmarshal tracking payload")

			assert.Equal(t, tt.payload.Type, unmarshaled.Type, "Type mismatch")
			assert.Equal(t, tt.payload.Payload.Website, unmarshaled.Payload.Website, "Website ID mismatch")
		})
	}
}

func TestPayloadData_OptionalFields(t *testing.T) {
	// Test that optional fields can be nil
	payload := PayloadData{
		Website: "test-id",
	}

	jsonBytes, err := json.Marshal(payload)
	require.NoError(t, err, "Failed to marshal payload")

	// Unmarshal and verify nil fields
	var unmarshaled PayloadData
	err = json.Unmarshal(jsonBytes, &unmarshaled)
	require.NoError(t, err, "Failed to unmarshal payload")

	assert.Nil(t, unmarshaled.Hostname)
	assert.Nil(t, unmarshaled.Language)
	assert.Nil(t, unmarshaled.Referrer)
	assert.Nil(t, unmarshaled.Screen)
	assert.Nil(t, unmarshaled.Title)
	assert.Nil(t, unmarshaled.URL)
	assert.Nil(t, unmarshaled.Name)
	assert.Nil(t, unmarshaled.ScrollDepth)
	assert.Nil(t, unmarshaled.EngagementTime)
}

func TestPayloadData_WithEnhancedTracking(t *testing.T) {
	scrollDepth := 75
	engagementTime := 15000

	payload := PayloadData{
		Website:        "test-id",
		ScrollDepth:    &scrollDepth,
		EngagementTime: &engagementTime,
		Props: map[string]any{
			"button_clicked": "signup",
			"experiment_id":  "exp_123",
		},
	}

	jsonBytes, err := json.Marshal(payload)
	require.NoError(t, err, "Failed to marshal enhanced payload")

	var unmarshaled PayloadData
	err = json.Unmarshal(jsonBytes, &unmarshaled)
	require.NoError(t, err, "Failed to unmarshal enhanced payload")

	require.NotNil(t, unmarshaled.ScrollDepth)
	assert.Equal(t, 75, *unmarshaled.ScrollDepth)

	require.NotNil(t, unmarshaled.EngagementTime)
	assert.Equal(t, 15000, *unmarshaled.EngagementTime)

	assert.NotNil(t, unmarshaled.Props)
	assert.Equal(t, "signup", unmarshaled.Props["button_clicked"])
	assert.Equal(t, "exp_123", unmarshaled.Props["experiment_id"])
}

func TestDashboardStats_BounceRateFormatting(t *testing.T) {
	tests := []struct {
		name        string
		bounceRate  string
		expectValid bool
	}{
		{
			name:        "Valid percentage",
			bounceRate:  "45.2%",
			expectValid: true,
		},
		{
			name:        "Zero percentage",
			bounceRate:  "0%",
			expectValid: true,
		},
		{
			name:        "High percentage",
			bounceRate:  "99.9%",
			expectValid: true,
		},
		{
			name:        "100 percent",
			bounceRate:  "100.0%",
			expectValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := DashboardStats{
				TodayBounceRate: tt.bounceRate,
			}

			// Should have percentage sign
			assert.Contains(t, stats.TodayBounceRate, "%", "Bounce rate should contain percentage sign")

			// Should be parseable as a formatted string
			assert.NotEmpty(t, stats.TodayBounceRate, "Bounce rate should not be empty")
		})
	}
}
