package handlers

import (
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestParsePaginationParams(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    map[string]string
		expectedPage   int
		expectedPer    int
		expectedOffset int
	}{
		{
			name:           "default values",
			queryParams:    map[string]string{},
			expectedPage:   1,
			expectedPer:    10,
			expectedOffset: 0,
		},
		{
			name:           "custom page and per",
			queryParams:    map[string]string{"page": "2", "per": "25"},
			expectedPage:   2,
			expectedPer:    25,
			expectedOffset: 25,
		},
		{
			name:           "page 3 with per 10",
			queryParams:    map[string]string{"page": "3", "per": "10"},
			expectedPage:   3,
			expectedPer:    10,
			expectedOffset: 20,
		},
		{
			name:           "negative page defaults to 1",
			queryParams:    map[string]string{"page": "-1", "per": "10"},
			expectedPage:   1,
			expectedPer:    10,
			expectedOffset: 0,
		},
		{
			name:           "zero page defaults to 1",
			queryParams:    map[string]string{"page": "0", "per": "10"},
			expectedPage:   1,
			expectedPer:    10,
			expectedOffset: 0,
		},
		{
			name:           "per too large clamped to 100",
			queryParams:    map[string]string{"page": "1", "per": "500"},
			expectedPage:   1,
			expectedPer:    100,
			expectedOffset: 0,
		},
		{
			name:           "per zero defaults to 1",
			queryParams:    map[string]string{"page": "1", "per": "0"},
			expectedPage:   1,
			expectedPer:    1,
			expectedOffset: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := url.Values{}
			for k, v := range tt.queryParams {
				query.Set(k, v)
			}

			req := httptest.NewRequest("GET", "/test?"+query.Encode(), nil)
			params := ParsePaginationParams(req)

			if params.Page != tt.expectedPage {
				t.Errorf("Page = %d, want %d", params.Page, tt.expectedPage)
			}
			if params.Per != tt.expectedPer {
				t.Errorf("Per = %d, want %d", params.Per, tt.expectedPer)
			}
			if params.Offset != tt.expectedOffset {
				t.Errorf("Offset = %d, want %d", params.Offset, tt.expectedOffset)
			}
		})
	}
}

func TestBuildPaginationMeta(t *testing.T) {
	tests := []struct {
		name               string
		params             PaginationParams
		total              int64
		expectedTotalPages int
		expectedHasMore    bool
	}{
		{
			name:               "first page of 3",
			params:             PaginationParams{Page: 1, Per: 10, Offset: 0},
			total:              25,
			expectedTotalPages: 3,
			expectedHasMore:    true,
		},
		{
			name:               "middle page",
			params:             PaginationParams{Page: 2, Per: 10, Offset: 10},
			total:              25,
			expectedTotalPages: 3,
			expectedHasMore:    true,
		},
		{
			name:               "last page",
			params:             PaginationParams{Page: 3, Per: 10, Offset: 20},
			total:              25,
			expectedTotalPages: 3,
			expectedHasMore:    false,
		},
		{
			name:               "exact page boundary",
			params:             PaginationParams{Page: 1, Per: 10, Offset: 0},
			total:              10,
			expectedTotalPages: 1,
			expectedHasMore:    false,
		},
		{
			name:               "zero total",
			params:             PaginationParams{Page: 1, Per: 10, Offset: 0},
			total:              0,
			expectedTotalPages: 0,
			expectedHasMore:    false,
		},
		{
			name:               "single item",
			params:             PaginationParams{Page: 1, Per: 10, Offset: 0},
			total:              1,
			expectedTotalPages: 1,
			expectedHasMore:    false,
		},
		{
			name:               "page beyond total",
			params:             PaginationParams{Page: 5, Per: 10, Offset: 40},
			total:              25,
			expectedTotalPages: 3,
			expectedHasMore:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := BuildPaginationMeta(tt.params, tt.total)

			if meta.Page != tt.params.Page {
				t.Errorf("Page = %d, want %d", meta.Page, tt.params.Page)
			}
			if meta.Per != tt.params.Per {
				t.Errorf("Per = %d, want %d", meta.Per, tt.params.Per)
			}
			if meta.Total != tt.total {
				t.Errorf("Total = %d, want %d", meta.Total, tt.total)
			}
			if meta.TotalPages != tt.expectedTotalPages {
				t.Errorf("TotalPages = %d, want %d", meta.TotalPages, tt.expectedTotalPages)
			}
			if meta.HasMore != tt.expectedHasMore {
				t.Errorf("HasMore = %v, want %v", meta.HasMore, tt.expectedHasMore)
			}
		})
	}
}

func TestNewPaginatedResponse(t *testing.T) {
	data := []string{"item1", "item2", "item3"}
	params := PaginationParams{Page: 1, Per: 10, Offset: 0}
	total := int64(25)

	response := NewPaginatedResponse(data, params, total)

	if response.Data == nil {
		t.Error("Data is nil")
	}
	if response.Pagination.Page != 1 {
		t.Errorf("Pagination.Page = %d, want 1", response.Pagination.Page)
	}
	if response.Pagination.Total != 25 {
		t.Errorf("Pagination.Total = %d, want 25", response.Pagination.Total)
	}
	if response.Pagination.TotalPages != 3 {
		t.Errorf("Pagination.TotalPages = %d, want 3", response.Pagination.TotalPages)
	}
	if !response.Pagination.HasMore {
		t.Error("Pagination.HasMore should be true")
	}
}
