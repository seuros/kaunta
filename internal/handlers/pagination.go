package handlers

import (
	"net/http"
	"slices"
	"strings"

	"github.com/seuros/kaunta/internal/httpx"
)

// SortDirection represents sort order
type SortDirection string

const (
	SortAsc  SortDirection = "asc"
	SortDesc SortDirection = "desc"
)

// PaginationParams holds pagination and sorting query parameters
type PaginationParams struct {
	Page      int           `json:"page"`       // 1-indexed page number (default: 1)
	Per       int           `json:"per"`        // Items per page (default: 10, max: 100)
	Offset    int           `json:"-"`          // Calculated offset for SQL (not exposed in JSON)
	SortBy    string        `json:"sort_by"`    // Column to sort by (default: "count")
	SortOrder SortDirection `json:"sort_order"` // Sort direction: "asc" or "desc" (default: "desc")
}

// PaginationMeta contains pagination metadata
type PaginationMeta struct {
	Page       int   `json:"page"`
	Per        int   `json:"per"`
	Total      int64 `json:"total"`       // Total items across all pages
	TotalPages int   `json:"total_pages"` // Calculated total pages
	HasMore    bool  `json:"has_more"`    // Whether more pages exist
}

// PaginatedResponse wraps any list response with pagination metadata
type PaginatedResponse struct {
	Data       interface{}    `json:"data"`
	Pagination PaginationMeta `json:"pagination"`
}

// ValidSortColumns defines allowed sort columns per endpoint type
var ValidSortColumns = map[string][]string{
	"breakdown": {"count", "name"},
	"pages":     {"views", "path", "unique_visitors", "avg_engagement_time"},
	"map":       {"visitors", "country", "percentage"},
}

// ParsePaginationParams extracts and validates pagination from request
func ParsePaginationParams(r *http.Request) PaginationParams {
	page := max(httpx.QueryInt(r, "page", 1), 1)
	per := min(max(httpx.QueryInt(r, "per", 10), 1), 100)
	offset := (page - 1) * per

	// Parse sort parameters
	sortBy := strings.ToLower(httpx.QueryString(r, "sort_by", "count"))
	sortOrder := SortDirection(strings.ToLower(httpx.QueryString(r, "sort_order", "desc")))

	// Validate sort order
	if sortOrder != SortAsc && sortOrder != SortDesc {
		sortOrder = SortDesc
	}

	return PaginationParams{
		Page:      page,
		Per:       per,
		Offset:    offset,
		SortBy:    sortBy,
		SortOrder: sortOrder,
	}
}

// ParsePaginationParamsWithValidation extracts pagination with column validation
func ParsePaginationParamsWithValidation(r *http.Request, endpointType string) PaginationParams {
	params := ParsePaginationParams(r)

	// Validate sort column against allowed list
	validColumns, ok := ValidSortColumns[endpointType]
	if ok && !slices.Contains(validColumns, params.SortBy) {
		// Default to first valid column if invalid
		params.SortBy = validColumns[0]
	}

	return params
}

// BuildPaginationMeta creates pagination metadata from query results
func BuildPaginationMeta(params PaginationParams, total int64) PaginationMeta {
	var totalPages int
	if total > 0 && params.Per > 0 {
		totalPages = int((total + int64(params.Per) - 1) / int64(params.Per))
	}
	hasMore := params.Page < totalPages

	return PaginationMeta{
		Page:       params.Page,
		Per:        params.Per,
		Total:      total,
		TotalPages: totalPages,
		HasMore:    hasMore,
	}
}

// NewPaginatedResponse wraps data with pagination metadata
func NewPaginatedResponse(data interface{}, params PaginationParams, total int64) PaginatedResponse {
	return PaginatedResponse{
		Data:       data,
		Pagination: BuildPaginationMeta(params, total),
	}
}
