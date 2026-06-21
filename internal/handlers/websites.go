package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/websites"
)

// WebsiteDetail is an alias for the shared websites.Detail type. The website
// data-access layer lives in internal/websites (shared with the CLI); the
// thin wrappers below adapt it to the handler call sites.
type WebsiteDetail = websites.Detail

func getWebsiteByID(ctx context.Context, websiteID string) (*WebsiteDetail, error) {
	return websites.GetByID(ctx, websiteID)
}

func listWebsites(ctx context.Context) ([]*WebsiteDetail, error) {
	return websites.List(ctx)
}

func createWebsite(ctx context.Context, domain, name string, allowedDomains []string) (*WebsiteDetail, error) {
	return websites.Create(ctx, domain, name, allowedDomains)
}

func updateWebsite(ctx context.Context, domain string, name *string) (*WebsiteDetail, error) {
	return websites.Update(ctx, domain, name, nil)
}

func addAllowedDomains(ctx context.Context, websiteDomain string, domains []string) (*WebsiteDetail, error) {
	return websites.AddAllowedDomains(ctx, websiteDomain, domains)
}

func removeAllowedDomain(ctx context.Context, websiteDomain, domainToRemove string) (*WebsiteDetail, error) {
	return websites.RemoveAllowedDomain(ctx, websiteDomain, domainToRemove)
}

// newWebsiteDetailResponse builds the API response shape from a WebsiteDetail.
func newWebsiteDetailResponse(website *WebsiteDetail) WebsiteDetailResponse {
	return WebsiteDetailResponse{
		ID:                 website.WebsiteID,
		Domain:             website.Domain,
		Name:               website.Name,
		AllowedDomains:     website.AllowedDomains,
		PublicStatsEnabled: website.PublicStatsEnabled,
		CreatedAt:          website.CreatedAt.Format(time.RFC3339),
	}
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
		respondError(w, r, http.StatusInternalServerError, "Failed to query websites")
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

	render.JSON(w, r, NewPaginatedResponse(websites, pagination, totalCount))
}

// HandleWebsiteShow returns a single website with its allowed domains
func HandleWebsiteShow(w http.ResponseWriter, r *http.Request) {
	websiteIDStr, ok := parseWebsiteID(w, r, chi.URLParam(r, "website_id"))
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	website, err := getWebsiteByID(ctx, websiteIDStr)
	if err != nil {
		respondError(w, r, http.StatusNotFound, err.Error())
		return
	}

	render.JSON(w, r, newWebsiteDetailResponse(website))
}

// HandleWebsiteList returns all websites with allowed domains
func HandleWebsiteList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	websites, err := listWebsites(ctx)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]WebsiteDetailResponse, 0, len(websites))
	for _, website := range websites {
		result = append(result, newWebsiteDetailResponse(website))
	}

	render.JSON(w, r, result)
}

// HandleWebsiteCreate creates a new website
func HandleWebsiteCreate(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	var req CreateWebsiteRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.Domain == "" {
		respondError(w, r, http.StatusBadRequest, "Domain is required")
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
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(w, r, newWebsiteDetailResponse(website))
}

// HandleWebsiteUpdate updates a website's name
func HandleWebsiteUpdate(w http.ResponseWriter, r *http.Request) {
	websiteIDStr, ok := parseWebsiteID(w, r, chi.URLParam(r, "website_id"))
	if !ok {
		return
	}

	defer func() { _ = r.Body.Close() }()
	var req UpdateWebsiteRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	existingWebsite, err := getWebsiteByID(ctx, websiteIDStr)
	if err != nil {
		respondError(w, r, http.StatusNotFound, err.Error())
		return
	}

	website, err := updateWebsite(ctx, existingWebsite.Domain, &req.Name)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	render.JSON(w, r, newWebsiteDetailResponse(website))
}

// HandleAddDomain adds an allowed domain to a website
func HandleAddDomain(w http.ResponseWriter, r *http.Request) {
	websiteIDStr, ok := parseWebsiteID(w, r, chi.URLParam(r, "website_id"))
	if !ok {
		return
	}

	defer func() { _ = r.Body.Close() }()
	var req DomainRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.Domain == "" {
		respondError(w, r, http.StatusBadRequest, "Domain is required")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	existingWebsite, err := getWebsiteByID(ctx, websiteIDStr)
	if err != nil {
		respondError(w, r, http.StatusNotFound, err.Error())
		return
	}

	website, err := addAllowedDomains(ctx, existingWebsite.Domain, []string{req.Domain})
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	render.JSON(w, r, newWebsiteDetailResponse(website))
}

// HandleRemoveDomain removes an allowed domain from a website
func HandleRemoveDomain(w http.ResponseWriter, r *http.Request) {
	websiteIDStr, ok := parseWebsiteID(w, r, chi.URLParam(r, "website_id"))
	if !ok {
		return
	}

	defer func() { _ = r.Body.Close() }()
	var req DomainRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.Domain == "" {
		respondError(w, r, http.StatusBadRequest, "Domain is required")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	existingWebsite, err := getWebsiteByID(ctx, websiteIDStr)
	if err != nil {
		respondError(w, r, http.StatusNotFound, err.Error())
		return
	}

	website, err := removeAllowedDomain(ctx, existingWebsite.Domain, req.Domain)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	render.JSON(w, r, newWebsiteDetailResponse(website))
}

// HandleSetPublicStats enables or disables public stats for a website
// PATCH /api/websites/:website_id/public-stats
func HandleSetPublicStats(w http.ResponseWriter, r *http.Request) {
	websiteIDStr, ok := parseWebsiteID(w, r, chi.URLParam(r, "website_id"))
	if !ok {
		return
	}

	defer func() { _ = r.Body.Close() }()
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	existingWebsite, err := getWebsiteByID(ctx, websiteIDStr)
	if err != nil {
		respondError(w, r, http.StatusNotFound, err.Error())
		return
	}

	website, err := websites.SetPublicStatsEnabled(ctx, existingWebsite.Domain, req.Enabled)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "Failed to update website")
		return
	}

	render.JSON(w, r, newWebsiteDetailResponse(website))
}
