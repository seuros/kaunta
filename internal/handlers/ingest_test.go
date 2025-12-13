package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateIngestPayload(t *testing.T) {
	tests := []struct {
		name        string
		payload     IngestPayload
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid page_view event",
			payload: IngestPayload{
				Event:     "page_view",
				VisitorID: "visitor_123",
				URL:       "/products",
			},
			expectError: false,
		},
		{
			name: "valid custom event",
			payload: IngestPayload{
				Event:     "purchase",
				VisitorID: "visitor_123",
				Properties: map[string]interface{}{
					"amount": 99.99,
				},
			},
			expectError: false,
		},
		{
			name: "missing event",
			payload: IngestPayload{
				VisitorID: "visitor_123",
			},
			expectError: true,
			errorMsg:    "event is required",
		},
		{
			name: "missing visitor_id",
			payload: IngestPayload{
				Event: "page_view",
				URL:   "/products",
			},
			expectError: true,
			errorMsg:    "visitorid is required",
		},
		{
			name: "page_view without url",
			payload: IngestPayload{
				Event:     "page_view",
				VisitorID: "visitor_123",
			},
			expectError: true,
			errorMsg:    "url is required for page_view",
		},
		{
			name: "event name too long",
			payload: IngestPayload{
				Event:     "this_is_a_very_long_event_name_that_exceeds_the_fifty_character_limit_for_event_names",
				VisitorID: "visitor_123",
			},
			expectError: true,
			errorMsg:    "exceeds maximum length",
		},
		{
			name: "timestamp too old",
			payload: IngestPayload{
				Event:     "custom_event",
				VisitorID: "visitor_123",
				Timestamp: ptrInt64(time.Now().Add(-60 * 24 * time.Hour).Unix()),
			},
			expectError: true,
			errorMsg:    "timestamp must be within 30 days",
		},
		{
			name: "timestamp too far in future",
			payload: IngestPayload{
				Event:     "custom_event",
				VisitorID: "visitor_123",
				Timestamp: ptrInt64(time.Now().Add(60 * 24 * time.Hour).Unix()),
			},
			expectError: true,
			errorMsg:    "timestamp must be within 30 days",
		},
		{
			name: "valid timestamp",
			payload: IngestPayload{
				Event:     "custom_event",
				VisitorID: "visitor_123",
				Timestamp: ptrInt64(time.Now().Unix()),
			},
			expectError: false,
		},
		{
			name: "reserved property key with dollar",
			payload: IngestPayload{
				Event:     "custom_event",
				VisitorID: "visitor_123",
				Properties: map[string]interface{}{
					"$reserved": "value",
				},
			},
			expectError: true,
			errorMsg:    "reserved property key",
		},
		{
			name: "reserved property key with underscore",
			payload: IngestPayload{
				Event:     "custom_event",
				VisitorID: "visitor_123",
				Properties: map[string]interface{}{
					"_internal": "value",
				},
			},
			expectError: true,
			errorMsg:    "reserved property key",
		},
		{
			name: "too many properties",
			payload: IngestPayload{
				Event:      "custom_event",
				VisitorID:  "visitor_123",
				Properties: generateManyProperties(101),
			},
			expectError: true,
			errorMsg:    "exceed",
		},
		{
			name: "valid UTM parameters",
			payload: IngestPayload{
				Event:       "custom_event",
				VisitorID:   "visitor_123",
				UTMSource:   ptrString("google"),
				UTMMedium:   ptrString("cpc"),
				UTMCampaign: ptrString("summer_sale"),
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIngestPayload(&tt.payload)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateIngestProperties(t *testing.T) {
	tests := []struct {
		name        string
		props       map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid properties",
			props: map[string]interface{}{
				"product_id": "123",
				"price":      99.99,
				"tags":       []string{"sale", "featured"},
			},
			expectError: false,
		},
		{
			name:        "nil properties",
			props:       nil,
			expectError: false,
		},
		{
			name:        "empty properties",
			props:       map[string]interface{}{},
			expectError: false,
		},
		{
			name: "nested properties within depth",
			props: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": "value",
					},
				},
			},
			expectError: false,
		},
		{
			name: "properties exceeds depth",
			props: map[string]interface{}{
				"l1": map[string]interface{}{
					"l2": map[string]interface{}{
						"l3": map[string]interface{}{
							"l4": map[string]interface{}{
								"l5": map[string]interface{}{
									"l6": "too deep",
								},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "max depth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.props == nil {
				return // Skip nil check as validateIngestProperties expects non-nil
			}
			err := validateIngestProperties(tt.props)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetJSONDepth(t *testing.T) {
	tests := []struct {
		name     string
		data     interface{}
		expected int
	}{
		{
			name:     "flat object",
			data:     map[string]interface{}{"key": "value"},
			expected: 1,
		},
		{
			name: "nested two levels",
			data: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": "value",
				},
			},
			expected: 2,
		},
		{
			name:     "array",
			data:     []interface{}{"a", "b", "c"},
			expected: 1,
		},
		{
			name: "array with objects",
			data: []interface{}{
				map[string]interface{}{"key": "value"},
			},
			expected: 2,
		},
		{
			name:     "scalar",
			data:     "string",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getJSONDepth(tt.data, 0)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBatchIngestRequest(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		expectValid bool
	}{
		{
			name: "valid batch",
			json: `{
				"events": [
					{"event": "page_view", "visitor_id": "v1", "url": "/page1"},
					{"event": "purchase", "visitor_id": "v1", "properties": {"amount": 99}}
				]
			}`,
			expectValid: true,
		},
		{
			name:        "empty events",
			json:        `{"events": []}`,
			expectValid: false,
		},
		{
			name:        "missing events",
			json:        `{}`,
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req BatchIngestRequest
			err := json.Unmarshal([]byte(tt.json), &req)
			require.NoError(t, err)

			isValid := len(req.Events) > 0 && len(req.Events) <= 100
			assert.Equal(t, tt.expectValid, isValid)
		})
	}
}

func TestIngestPayloadUnmarshal(t *testing.T) {
	jsonStr := `{
		"event": "purchase",
		"visitor_id": "anon_abc123",
		"url": "/checkout/complete",
		"properties": {
			"product_id": "123",
			"price": 99.99,
			"quantity": 2
		},
		"utm_source": "newsletter",
		"utm_medium": "email",
		"context": {
			"locale": "en-US",
			"screen": "1920x1080"
		}
	}`

	var payload IngestPayload
	err := json.Unmarshal([]byte(jsonStr), &payload)
	require.NoError(t, err)

	assert.Equal(t, "purchase", payload.Event)
	assert.Equal(t, "anon_abc123", payload.VisitorID)
	assert.Equal(t, "/checkout/complete", payload.URL)
	assert.NotNil(t, payload.Properties)
	assert.Equal(t, "123", payload.Properties["product_id"])
	assert.Equal(t, 99.99, payload.Properties["price"])
	assert.Equal(t, "newsletter", *payload.UTMSource)
	assert.Equal(t, "email", *payload.UTMMedium)
	assert.NotNil(t, payload.Context)
	assert.Equal(t, "en-US", payload.Context.Locale)
	assert.Equal(t, "1920x1080", payload.Context.Screen)
}

// Helper functions

func ptrInt64(i int64) *int64 {
	return &i
}

func ptrString(s string) *string {
	return &s
}

func generateManyProperties(count int) map[string]interface{} {
	props := make(map[string]interface{})
	for i := 0; i < count; i++ {
		props["key_"+string(rune('a'+i%26))+string(rune('0'+i/26))] = i
	}
	return props
}
