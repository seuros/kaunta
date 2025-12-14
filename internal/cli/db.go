package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/seuros/kaunta/internal/database"
)

// WebsiteDetail holds complete website information for CLI operations
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

// GetWebsiteByDomain retrieves a website by domain (case-insensitive lookup)
// Falls back to website_id lookup if domain not found
func GetWebsiteByDomain(ctx context.Context, domain string, websiteID *string) (*WebsiteDetail, error) {
	query := `
		SELECT website_id, domain, name, allowed_domains, share_id, public_stats_enabled, created_at, updated_at
		FROM website
		WHERE deleted_at IS NULL AND (LOWER(domain) = LOWER($1) OR website_id = $2)
		LIMIT 1
	`

	var website WebsiteDetail
	var allowedDomainsJSON []byte
	var shareID *string

	err := database.DB.QueryRowContext(ctx, query, domain, websiteID).Scan(
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

	// Parse JSONB array into []string
	website.AllowedDomains = []string{}
	if len(allowedDomainsJSON) > 0 {
		if err := json.Unmarshal(allowedDomainsJSON, &website.AllowedDomains); err != nil {
			// If parsing fails, just leave as empty array
			website.AllowedDomains = []string{}
		}
	}

	return &website, nil
}

// GetWebsiteByID retrieves a website by website_id
func GetWebsiteByID(ctx context.Context, websiteID string) (*WebsiteDetail, error) {
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

	// Parse JSONB array into []string
	website.AllowedDomains = []string{}
	if len(allowedDomainsJSON) > 0 {
		if err := json.Unmarshal(allowedDomainsJSON, &website.AllowedDomains); err != nil {
			// If parsing fails, just leave as empty array
			website.AllowedDomains = []string{}
		}
	}

	return &website, nil
}

// ListWebsites retrieves all non-deleted websites ordered by domain
func ListWebsites(ctx context.Context) ([]*WebsiteDetail, error) {
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

		// Parse JSONB array into []string
		website.AllowedDomains = []string{}
		if len(allowedDomainsJSON) > 0 {
			if err := json.Unmarshal(allowedDomainsJSON, &website.AllowedDomains); err != nil {
				// If parsing fails, just leave as empty array
				website.AllowedDomains = []string{}
			}
		}

		websites = append(websites, &website)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	return websites, nil
}

// CreateWebsite creates a new website with the provided details
func CreateWebsite(ctx context.Context, domain, name string, allowedDomains []string) (*WebsiteDetail, error) {
	// Validate domain format
	if err := validateDomain(domain); err != nil {
		return nil, err
	}

	// Use name as domain if name is empty
	if name == "" {
		name = domain
	}

	// Check if domain already exists (case-insensitive)
	checkQuery := `SELECT COUNT(*) FROM website WHERE LOWER(domain) = LOWER($1) AND deleted_at IS NULL`
	var count int
	err := database.DB.QueryRowContext(ctx, checkQuery, domain).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("website with domain '%s' already exists", domain)
	}

	// Convert allowed domains to JSON string for JSONB column
	allowedDomainsJSON := "[]"
	if len(allowedDomains) > 0 {
		data, _ := json.Marshal(allowedDomains)
		allowedDomainsJSON = string(data)
	}

	// Generate UUID
	websiteID := uuid.New().String()

	// Insert website
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

	// Parse JSONB array into []string
	website.AllowedDomains = []string{}
	if len(allowedDomainsResult) > 0 {
		if err := json.Unmarshal(allowedDomainsResult, &website.AllowedDomains); err != nil {
			// If parsing fails, just leave as empty array
			website.AllowedDomains = []string{}
		}
	}

	return &website, nil
}

// UpdateWebsite updates an existing website by domain
func UpdateWebsite(ctx context.Context, domain string, name *string, allowedDomains []string) (*WebsiteDetail, error) {
	// Get website first
	website, err := GetWebsiteByDomain(ctx, domain, nil)
	if err != nil {
		return nil, err
	}

	// Prepare update query
	updates := []string{"updated_at = NOW()"}
	args := []interface{}{website.WebsiteID}
	argIndex := 2

	if name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argIndex))
		args = append(args, *name)
		argIndex++
	}

	if len(allowedDomains) > 0 {
		// Convert allowed domains to JSON string for JSONB column
		data, _ := json.Marshal(allowedDomains)
		allowedDomainsJSON := string(data)
		updates = append(updates, fmt.Sprintf("allowed_domains = $%d::jsonb", argIndex))
		args = append(args, allowedDomainsJSON)
	}

	// Build update query
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

	// Parse JSONB array into []string
	updatedWebsite.AllowedDomains = []string{}
	if len(allowedDomainsResult) > 0 {
		if err := json.Unmarshal(allowedDomainsResult, &updatedWebsite.AllowedDomains); err != nil {
			// If parsing fails, just leave as empty array
			updatedWebsite.AllowedDomains = []string{}
		}
	}

	return &updatedWebsite, nil
}

// DeleteWebsite soft-deletes a website by setting deleted_at
func DeleteWebsite(ctx context.Context, domain string) (*time.Time, error) {
	// Get website first to verify it exists
	website, err := GetWebsiteByDomain(ctx, domain, nil)
	if err != nil {
		return nil, err
	}

	// Soft delete
	query := `
		UPDATE website
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE website_id = $1
		RETURNING deleted_at
	`

	var deletedAt time.Time
	err = database.DB.QueryRowContext(ctx, query, website.WebsiteID).Scan(&deletedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to delete website: %w", err)
	}

	return &deletedAt, nil
}

// validateDomain validates a domain string format
func validateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("invalid domain format: domain cannot be empty")
	}

	if len(domain) > 253 {
		return fmt.Errorf("invalid domain format: domain cannot exceed 253 characters (DNS standard)")
	}

	// Basic domain validation (alphanumeric, dots, hyphens)
	// Allow localhost for testing
	if domain == "localhost" {
		return nil
	}

	for _, ch := range domain {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') &&
			ch != '.' && ch != '-' && ch != ':' {
			return fmt.Errorf("invalid domain format: contains invalid characters")
		}
	}

	return nil
}

// ParseAllowedDomains parses a comma-separated string of allowed domains
func ParseAllowedDomains(csvString string) []string {
	if csvString == "" {
		return []string{}
	}

	domains := strings.Split(csvString, ",")
	var result []string
	for _, d := range domains {
		trimmed := strings.TrimSpace(d)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// AllowedDomainsToJSON converts []string to JSON array string
func AllowedDomainsToJSON(domains []string) string {
	if len(domains) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(domains)
	return string(data)
}

// AddAllowedDomains adds domains to website's allowed_domains JSONB array
func AddAllowedDomains(ctx context.Context, websiteDomain string, domains []string) (*WebsiteDetail, error) {
	// Get website first to ensure it exists
	website, err := GetWebsiteByDomain(ctx, websiteDomain, nil)
	if err != nil {
		return nil, err
	}

	// Merge existing domains with new ones (avoid duplicates)
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

	// Convert to JSON
	domainsJSON, _ := json.Marshal(mergedDomains)

	// Update website
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

	// Parse JSONB array into []string
	updatedWebsite.AllowedDomains = []string{}
	if len(allowedDomainsResult) > 0 {
		if err := json.Unmarshal(allowedDomainsResult, &updatedWebsite.AllowedDomains); err != nil {
			updatedWebsite.AllowedDomains = []string{}
		}
	}

	return &updatedWebsite, nil
}

// RemoveAllowedDomain removes a domain from allowed_domains array
func RemoveAllowedDomain(ctx context.Context, websiteDomain, domainToRemove string) (*WebsiteDetail, error) {
	// Get website first
	website, err := GetWebsiteByDomain(ctx, websiteDomain, nil)
	if err != nil {
		return nil, err
	}

	// Check if domain exists (case-insensitive)
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

	// Check if we're trying to remove the last domain
	if len(newDomains) == 0 {
		return nil, fmt.Errorf("cannot remove the last allowed domain (security: at least one domain must remain)")
	}

	// Convert to JSON
	domainsJSON, _ := json.Marshal(newDomains)

	// Update website
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

	// Parse JSONB array into []string
	updatedWebsite.AllowedDomains = []string{}
	if len(allowedDomainsResult) > 0 {
		if err := json.Unmarshal(allowedDomainsResult, &updatedWebsite.AllowedDomains); err != nil {
			updatedWebsite.AllowedDomains = []string{}
		}
	}

	return &updatedWebsite, nil
}

// GetAllowedDomains returns the allowed_domains array for a website
func GetAllowedDomains(ctx context.Context, websiteDomain string) ([]string, *WebsiteDetail, error) {
	website, err := GetWebsiteByDomain(ctx, websiteDomain, nil)
	if err != nil {
		return nil, nil, err
	}

	return website.AllowedDomains, website, nil
}

// SetPublicStatsEnabled enables or disables public stats for a website
func SetPublicStatsEnabled(ctx context.Context, websiteDomain string, enabled bool) (*WebsiteDetail, error) {
	// Get website first to ensure it exists
	website, err := GetWebsiteByDomain(ctx, websiteDomain, nil)
	if err != nil {
		return nil, err
	}

	query := `
		UPDATE website
		SET public_stats_enabled = $1, updated_at = NOW()
		WHERE website_id = $2 AND deleted_at IS NULL
		RETURNING website_id, domain, name, allowed_domains, share_id, public_stats_enabled, created_at, updated_at
	`

	var updatedWebsite WebsiteDetail
	var allowedDomainsResult []byte
	var shareID *string

	err = database.DB.QueryRowContext(ctx, query, enabled, website.WebsiteID).Scan(
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

	// Parse JSONB array into []string
	updatedWebsite.AllowedDomains = []string{}
	if len(allowedDomainsResult) > 0 {
		if err := json.Unmarshal(allowedDomainsResult, &updatedWebsite.AllowedDomains); err != nil {
			updatedWebsite.AllowedDomains = []string{}
		}
	}

	return &updatedWebsite, nil
}
