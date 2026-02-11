package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/httpx"
)

// WebsiteDetail holds complete website information for API operations
type WebsiteDetail struct {
	WebsiteID          string    `json:"website_id"`
	Domain             string    `json:"domain"`
	Name               string    `json:"name"`
	AllowedDomains     []string  `json:"allowed_domains"`
	ShareID            *string   `json:"share_id,omitempty"`
	PublicStatsEnabled bool      `json:"public_stats_enabled"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// getWebsiteByID retrieves a website by website_id
func getWebsiteByID(ctx context.Context, websiteID string) (*WebsiteDetail, error) {
	query := `
		SELECT website_id, domain, name, allowed_domains, share_id, public_stats_enabled, created_at, updated_at
		FROM website
		WHERE deleted_at IS NULL AND website_id = $1
		LIMIT 1
	`

	var website WebsiteDetail
	var allowedDomainsJSON []byte
	var shareID *string

	err := database.DB.QueryRowContext(ctx, query, websiteID).Scan(
		&website.WebsiteID,
		&website.Domain,
		&website.Name,
		&allowedDomainsJSON,
		&shareID,
		&website.PublicStatsEnabled,
		&website.CreatedAt,
		&website.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("website with ID '%s' not found", websiteID)
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	website.ShareID = shareID
	website.AllowedDomains = []string{}
	if len(allowedDomainsJSON) > 0 {
		_ = json.Unmarshal(allowedDomainsJSON, &website.AllowedDomains)
	}

	return &website, nil
}

// listWebsites retrieves all non-deleted websites ordered by domain
func listWebsites(ctx context.Context) ([]*WebsiteDetail, error) {
	query := `
		SELECT website_id, domain, name, allowed_domains, share_id, public_stats_enabled, created_at, updated_at
		FROM website
		WHERE deleted_at IS NULL
		ORDER BY LOWER(domain)
	`

	rows, err := database.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var websites []*WebsiteDetail
	for rows.Next() {
		var website WebsiteDetail
		var allowedDomainsJSON []byte
		var shareID *string

		err := rows.Scan(
			&website.WebsiteID,
			&website.Domain,
			&website.Name,
			&allowedDomainsJSON,
			&shareID,
			&website.PublicStatsEnabled,
			&website.CreatedAt,
			&website.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("database error: %w", err)
		}

		website.ShareID = shareID
		website.AllowedDomains = []string{}
		if len(allowedDomainsJSON) > 0 {
			_ = json.Unmarshal(allowedDomainsJSON, &website.AllowedDomains)
		}

		websites = append(websites, &website)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	return websites, nil
}

// createWebsite creates a new website with the provided details
func createWebsite(ctx context.Context, domain, name string, allowedDomains []string) (*WebsiteDetail, error) {
	if domain == "" {
		return nil, fmt.Errorf("domain cannot be empty")
	}
	if len(domain) > 253 {
		return nil, fmt.Errorf("domain cannot exceed 253 characters")
	}

	if name == "" {
		name = domain
	}

	// Check if domain already exists
	checkQuery := `SELECT COUNT(*) FROM website WHERE LOWER(domain) = LOWER($1) AND deleted_at IS NULL`
	var count int
	err := database.DB.QueryRowContext(ctx, checkQuery, domain).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("website with domain '%s' already exists", domain)
	}

	allowedDomainsJSON := "[]"
	if len(allowedDomains) > 0 {
		data, _ := json.Marshal(allowedDomains)
		allowedDomainsJSON = string(data)
	}

	websiteID := uuid.New().String()

	query := `
		INSERT INTO website (website_id, domain, name, allowed_domains, created_at, updated_at)
		VALUES ($1, $2, $3, $4::jsonb, NOW(), NOW())
		RETURNING website_id, domain, name, allowed_domains, share_id, public_stats_enabled, created_at, updated_at
	`

	var website WebsiteDetail
	var allowedDomainsResult []byte
	var shareID *string

	err = database.DB.QueryRowContext(ctx, query, websiteID, domain, name, allowedDomainsJSON).Scan(
		&website.WebsiteID,
		&website.Domain,
		&website.Name,
		&allowedDomainsResult,
		&shareID,
		&website.PublicStatsEnabled,
		&website.CreatedAt,
		&website.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create website: %w", err)
	}

	website.ShareID = shareID
	website.AllowedDomains = []string{}
	if len(allowedDomainsResult) > 0 {
		_ = json.Unmarshal(allowedDomainsResult, &website.AllowedDomains)
	}

	return &website, nil
}

// updateWebsite updates an existing website by domain
func updateWebsite(ctx context.Context, domain string, name *string) (*WebsiteDetail, error) {
	website, err := getWebsiteByDomain(ctx, domain)
	if err != nil {
		return nil, err
	}

	updates := []string{"updated_at = NOW()"}
	args := []interface{}{website.WebsiteID}
	argIndex := 2

	if name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argIndex))
		args = append(args, *name)
	}

	query := fmt.Sprintf(`
		UPDATE website
		SET %s
		WHERE website_id = $1 AND deleted_at IS NULL
		RETURNING website_id, domain, name, allowed_domains, share_id, public_stats_enabled, created_at, updated_at
	`, strings.Join(updates, ", "))

	var updatedWebsite WebsiteDetail
	var allowedDomainsResult []byte
	var shareID *string

	err = database.DB.QueryRowContext(ctx, query, args...).Scan(
		&updatedWebsite.WebsiteID,
		&updatedWebsite.Domain,
		&updatedWebsite.Name,
		&allowedDomainsResult,
		&shareID,
		&updatedWebsite.PublicStatsEnabled,
		&updatedWebsite.CreatedAt,
		&updatedWebsite.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("website '%s' not found", domain)
		}
		return nil, fmt.Errorf("failed to update website: %w", err)
	}

	updatedWebsite.ShareID = shareID
	updatedWebsite.AllowedDomains = []string{}
	if len(allowedDomainsResult) > 0 {
		_ = json.Unmarshal(allowedDomainsResult, &updatedWebsite.AllowedDomains)
	}

	return &updatedWebsite, nil
}

// getWebsiteByDomain retrieves a website by domain (case-insensitive)
func getWebsiteByDomain(ctx context.Context, domain string) (*WebsiteDetail, error) {
	query := `
		SELECT website_id, domain, name, allowed_domains, share_id, public_stats_enabled, created_at, updated_at
		FROM website
		WHERE deleted_at IS NULL AND LOWER(domain) = LOWER($1)
		LIMIT 1
	`

	var website WebsiteDetail
	var allowedDomainsJSON []byte
	var shareID *string

	err := database.DB.QueryRowContext(ctx, query, domain).Scan(
		&website.WebsiteID,
		&website.Domain,
		&website.Name,
		&allowedDomainsJSON,
		&shareID,
		&website.PublicStatsEnabled,
		&website.CreatedAt,
		&website.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("website '%s' not found", domain)
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	website.ShareID = shareID
	website.AllowedDomains = []string{}
	if len(allowedDomainsJSON) > 0 {
		_ = json.Unmarshal(allowedDomainsJSON, &website.AllowedDomains)
	}

	return &website, nil
}

// addAllowedDomains adds domains to website's allowed_domains JSONB array
func addAllowedDomains(ctx context.Context, websiteDomain string, domains []string) (*WebsiteDetail, error) {
	website, err := getWebsiteByDomain(ctx, websiteDomain)
	if err != nil {
		return nil, err
	}

	existingMap := make(map[string]bool)
	for _, d := range website.AllowedDomains {
		existingMap[strings.ToLower(d)] = true
	}

	mergedDomains := website.AllowedDomains
	for _, d := range domains {
		if !existingMap[strings.ToLower(d)] {
			mergedDomains = append(mergedDomains, d)
			existingMap[strings.ToLower(d)] = true
		}
	}

	domainsJSON, _ := json.Marshal(mergedDomains)

	query := `
		UPDATE website
		SET allowed_domains = $1::jsonb, updated_at = NOW()
		WHERE website_id = $2 AND deleted_at IS NULL
		RETURNING website_id, domain, name, allowed_domains, share_id, public_stats_enabled, created_at, updated_at
	`

	var updatedWebsite WebsiteDetail
	var allowedDomainsResult []byte
	var shareID *string

	err = database.DB.QueryRowContext(ctx, query, string(domainsJSON), website.WebsiteID).Scan(
		&updatedWebsite.WebsiteID,
		&updatedWebsite.Domain,
		&updatedWebsite.Name,
		&allowedDomainsResult,
		&shareID,
		&updatedWebsite.PublicStatsEnabled,
		&updatedWebsite.CreatedAt,
		&updatedWebsite.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("website '%s' not found", websiteDomain)
		}
		return nil, fmt.Errorf("failed to update website: %w", err)
	}

	updatedWebsite.ShareID = shareID
	updatedWebsite.AllowedDomains = []string{}
	if len(allowedDomainsResult) > 0 {
		_ = json.Unmarshal(allowedDomainsResult, &updatedWebsite.AllowedDomains)
	}

	return &updatedWebsite, nil
}

// removeAllowedDomain removes a domain from allowed_domains array
func removeAllowedDomain(ctx context.Context, websiteDomain, domainToRemove string) (*WebsiteDetail, error) {
	website, err := getWebsiteByDomain(ctx, websiteDomain)
	if err != nil {
		return nil, err
	}

	found := false
	newDomains := []string{}
	for _, d := range website.AllowedDomains {
		if !strings.EqualFold(d, domainToRemove) {
			newDomains = append(newDomains, d)
		} else {
			found = true
		}
	}

	if !found {
		return nil, fmt.Errorf("domain '%s' not found in allowed list", domainToRemove)
	}

	if len(newDomains) == 0 {
		return nil, fmt.Errorf("cannot remove the last allowed domain")
	}

	domainsJSON, _ := json.Marshal(newDomains)

	query := `
		UPDATE website
		SET allowed_domains = $1::jsonb, updated_at = NOW()
		WHERE website_id = $2 AND deleted_at IS NULL
		RETURNING website_id, domain, name, allowed_domains, share_id, public_stats_enabled, created_at, updated_at
	`

	var updatedWebsite WebsiteDetail
	var allowedDomainsResult []byte
	var shareID *string

	err = database.DB.QueryRowContext(ctx, query, string(domainsJSON), website.WebsiteID).Scan(
		&updatedWebsite.WebsiteID,
		&updatedWebsite.Domain,
		&updatedWebsite.Name,
		&allowedDomainsResult,
		&shareID,
		&updatedWebsite.PublicStatsEnabled,
		&updatedWebsite.CreatedAt,
		&updatedWebsite.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("website '%s' not found", websiteDomain)
		}
		return nil, fmt.Errorf("failed to update website: %w", err)
	}

	updatedWebsite.ShareID = shareID
	updatedWebsite.AllowedDomains = []string{}
	if len(allowedDomainsResult) > 0 {
		_ = json.Unmarshal(allowedDomainsResult, &updatedWebsite.AllowedDomains)
	}

	return &updatedWebsite, nil
}

// HandleWebsites returns list of all websites with pagination
func HandleWebsites(w http.ResponseWriter, r *http.Request) {
	pagination := ParsePaginationParams(r)

	// Query with COUNT and pagination
	rows, err := database.DB.Query(`
		WITH total AS (
			SELECT COUNT(*)::BIGINT as count FROM website
		)
		SELECT w.website_id, w.domain, w.name, t.count as total_count
		FROM website w
		CROSS JOIN total t
		ORDER BY w.name, w.domain
		LIMIT $1 OFFSET $2
	`, pagination.Per, pagination.Offset)

	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "Failed to query websites")
		return
	}
	defer func() { _ = rows.Close() }()

	var websites []Website
	var totalCount int64
	for rows.Next() {
		var website Website
		var name *string
		var rowTotal int64
		if err := rows.Scan(&website.ID, &website.Domain, &name, &rowTotal); err != nil {
			continue
		}
		totalCount = rowTotal // Capture total count
		if name != nil {
			website.Name = *name
		} else {
			website.Name = website.Domain
		}
		websites = append(websites, website)
	}

	httpx.WriteJSON(w, http.StatusOK, NewPaginatedResponse(websites, pagination, totalCount))
}

// HandleWebsiteShow returns a single website with its allowed domains
func HandleWebsiteShow(w http.ResponseWriter, r *http.Request) {
	websiteIDStr := chi.URLParam(r, "website_id")
	if _, err := uuid.Parse(websiteIDStr); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid website ID")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	website, err := getWebsiteByID(ctx, websiteIDStr)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, WebsiteDetailResponse{
		ID:                 website.WebsiteID,
		Domain:             website.Domain,
		Name:               website.Name,
		AllowedDomains:     website.AllowedDomains,
		PublicStatsEnabled: website.PublicStatsEnabled,
		CreatedAt:          website.CreatedAt.Format(time.RFC3339),
	})
}

// HandleWebsiteList returns all websites with allowed domains
func HandleWebsiteList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	websites, err := listWebsites(ctx)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]WebsiteDetailResponse, 0, len(websites))
	for _, w := range websites {
		result = append(result, WebsiteDetailResponse{
			ID:                 w.WebsiteID,
			Domain:             w.Domain,
			Name:               w.Name,
			AllowedDomains:     w.AllowedDomains,
			PublicStatsEnabled: w.PublicStatsEnabled,
			CreatedAt:          w.CreatedAt.Format(time.RFC3339),
		})
	}

	httpx.WriteJSON(w, http.StatusOK, result)
}

// HandleWebsiteCreate creates a new website
func HandleWebsiteCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateWebsiteRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Domain == "" {
		httpx.Error(w, http.StatusBadRequest, "Domain is required")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Auto-add common domain variations
	allowedDomains := []string{
		req.Domain,
		"www." + req.Domain,
		"https://" + req.Domain,
		"http://" + req.Domain,
		"https://www." + req.Domain,
		"http://www." + req.Domain,
	}

	website, err := createWebsite(ctx, req.Domain, req.Name, allowedDomains)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, WebsiteDetailResponse{
		ID:                 website.WebsiteID,
		Domain:             website.Domain,
		Name:               website.Name,
		AllowedDomains:     website.AllowedDomains,
		PublicStatsEnabled: website.PublicStatsEnabled,
		CreatedAt:          website.CreatedAt.Format(time.RFC3339),
	})
}

// HandleWebsiteUpdate updates a website's name
func HandleWebsiteUpdate(w http.ResponseWriter, r *http.Request) {
	websiteIDStr := chi.URLParam(r, "website_id")
	if _, err := uuid.Parse(websiteIDStr); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid website ID")
		return
	}

	var req UpdateWebsiteRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	existingWebsite, err := getWebsiteByID(ctx, websiteIDStr)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, err.Error())
		return
	}

	website, err := updateWebsite(ctx, existingWebsite.Domain, &req.Name)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, WebsiteDetailResponse{
		ID:                 website.WebsiteID,
		Domain:             website.Domain,
		Name:               website.Name,
		AllowedDomains:     website.AllowedDomains,
		PublicStatsEnabled: website.PublicStatsEnabled,
		CreatedAt:          website.CreatedAt.Format(time.RFC3339),
	})
}

// HandleAddDomain adds an allowed domain to a website
func HandleAddDomain(w http.ResponseWriter, r *http.Request) {
	websiteIDStr := chi.URLParam(r, "website_id")
	if _, err := uuid.Parse(websiteIDStr); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid website ID")
		return
	}

	var req DomainRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Domain == "" {
		httpx.Error(w, http.StatusBadRequest, "Domain is required")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	existingWebsite, err := getWebsiteByID(ctx, websiteIDStr)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, err.Error())
		return
	}

	website, err := addAllowedDomains(ctx, existingWebsite.Domain, []string{req.Domain})
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, WebsiteDetailResponse{
		ID:                 website.WebsiteID,
		Domain:             website.Domain,
		Name:               website.Name,
		AllowedDomains:     website.AllowedDomains,
		PublicStatsEnabled: website.PublicStatsEnabled,
		CreatedAt:          website.CreatedAt.Format(time.RFC3339),
	})
}

// HandleRemoveDomain removes an allowed domain from a website
func HandleRemoveDomain(w http.ResponseWriter, r *http.Request) {
	websiteIDStr := chi.URLParam(r, "website_id")
	if _, err := uuid.Parse(websiteIDStr); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid website ID")
		return
	}

	var req DomainRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Domain == "" {
		httpx.Error(w, http.StatusBadRequest, "Domain is required")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	existingWebsite, err := getWebsiteByID(ctx, websiteIDStr)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, err.Error())
		return
	}

	website, err := removeAllowedDomain(ctx, existingWebsite.Domain, req.Domain)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, WebsiteDetailResponse{
		ID:                 website.WebsiteID,
		Domain:             website.Domain,
		Name:               website.Name,
		AllowedDomains:     website.AllowedDomains,
		PublicStatsEnabled: website.PublicStatsEnabled,
		CreatedAt:          website.CreatedAt.Format(time.RFC3339),
	})
}

// HandleSetPublicStats enables or disables public stats for a website
// PATCH /api/websites/:website_id/public-stats
func HandleSetPublicStats(w http.ResponseWriter, r *http.Request) {
	websiteIDStr := chi.URLParam(r, "website_id")
	if _, err := uuid.Parse(websiteIDStr); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid website ID")
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	existingWebsite, err := getWebsiteByID(ctx, websiteIDStr)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, err.Error())
		return
	}

	query := `
		UPDATE website
		SET public_stats_enabled = $1, updated_at = NOW()
		WHERE website_id = $2 AND deleted_at IS NULL
		RETURNING website_id, domain, name, allowed_domains, share_id, public_stats_enabled, created_at, updated_at
	`

	var website WebsiteDetail
	var allowedDomainsResult []byte
	var shareID *string

	err = database.DB.QueryRowContext(ctx, query, req.Enabled, existingWebsite.WebsiteID).Scan(
		&website.WebsiteID,
		&website.Domain,
		&website.Name,
		&allowedDomainsResult,
		&shareID,
		&website.PublicStatsEnabled,
		&website.CreatedAt,
		&website.UpdatedAt,
	)

	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "Failed to update website")
		return
	}

	website.ShareID = shareID
	website.AllowedDomains = []string{}
	if len(allowedDomainsResult) > 0 {
		_ = json.Unmarshal(allowedDomainsResult, &website.AllowedDomains)
	}

	httpx.WriteJSON(w, http.StatusOK, WebsiteDetailResponse{
		ID:                 website.WebsiteID,
		Domain:             website.Domain,
		Name:               website.Name,
		AllowedDomains:     website.AllowedDomains,
		PublicStatsEnabled: website.PublicStatsEnabled,
		CreatedAt:          website.CreatedAt.Format(time.RFC3339),
	})
}
