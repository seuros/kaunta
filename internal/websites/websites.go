// Package websites is the single source of truth for website records: the
// shared data-access layer used by both the CLI (internal/cli) and the HTTP
// handlers (internal/handlers).
package websites

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/seuros/kaunta/internal/database"
)

// selectCols is the canonical column list returned by every website query so
// that rows can be consumed by scanDetail.
const selectCols = `website_id, domain, name, allowed_domains, share_id, public_stats_enabled, created_at, updated_at`

// Detail holds complete website information shared across CLI and API operations.
type Detail struct {
	WebsiteID          string    `json:"website_id"`
	Domain             string    `json:"domain"`
	Name               string    `json:"name"`
	AllowedDomains     []string  `json:"allowed_domains"`
	ShareID            *string   `json:"share_id,omitempty"`
	PublicStatsEnabled bool      `json:"public_stats_enabled"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanDetail reads one website row (in selectCols order) into a Detail.
func scanDetail(s rowScanner) (*Detail, error) {
	var d Detail
	var allowedDomainsJSON []byte
	var shareID *string

	if err := s.Scan(
		&d.WebsiteID,
		&d.Domain,
		&d.Name,
		&allowedDomainsJSON,
		&shareID,
		&d.PublicStatsEnabled,
		&d.CreatedAt,
		&d.UpdatedAt,
	); err != nil {
		return nil, err
	}

	d.ShareID = shareID
	d.AllowedDomains = ParseJSONDomains(allowedDomainsJSON)
	return &d, nil
}

// ParseJSONDomains decodes the JSON-encoded allowed_domains column,
// returning an empty slice if the data is empty or malformed.
func ParseJSONDomains(data []byte) []string {
	domains := []string{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &domains); err != nil {
			// If parsing fails, just leave as empty array
			return []string{}
		}
	}
	return domains
}

// GetByDomain retrieves a website by domain (case-insensitive lookup),
// falling back to a website_id match when websiteID is provided.
func GetByDomain(ctx context.Context, domain string, websiteID *string) (*Detail, error) {
	query := `SELECT ` + selectCols + `
		FROM website
		WHERE deleted_at IS NULL AND (LOWER(domain) = LOWER($1) OR website_id = $2)
		LIMIT 1`

	d, err := scanDetail(database.DB.QueryRowContext(ctx, query, domain, websiteID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("website '%s' not found", domain)
		}
		return nil, fmt.Errorf("database error: %w", err)
	}
	return d, nil
}

// GetByID retrieves a website by website_id.
func GetByID(ctx context.Context, websiteID string) (*Detail, error) {
	query := `SELECT ` + selectCols + `
		FROM website
		WHERE deleted_at IS NULL AND website_id = $1
		LIMIT 1`

	d, err := scanDetail(database.DB.QueryRowContext(ctx, query, websiteID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("website with ID '%s' not found", websiteID)
		}
		return nil, fmt.Errorf("database error: %w", err)
	}
	return d, nil
}

// List retrieves all non-deleted websites ordered by domain.
func List(ctx context.Context) ([]*Detail, error) {
	query := `SELECT ` + selectCols + `
		FROM website
		WHERE deleted_at IS NULL
		ORDER BY LOWER(domain)`

	rows, err := database.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var websites []*Detail
	for rows.Next() {
		d, err := scanDetail(rows)
		if err != nil {
			return nil, fmt.Errorf("database error: %w", err)
		}
		websites = append(websites, d)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	return websites, nil
}

// Create creates a new website with the provided details.
func Create(ctx context.Context, domain, name string, allowedDomains []string) (*Detail, error) {
	if err := ValidateDomain(domain); err != nil {
		return nil, err
	}

	// Use domain as name if name is empty
	if name == "" {
		name = domain
	}

	// Check if domain already exists (case-insensitive)
	checkQuery := `SELECT COUNT(*) FROM website WHERE LOWER(domain) = LOWER($1) AND deleted_at IS NULL`
	var count int
	if err := database.DB.QueryRowContext(ctx, checkQuery, domain).Scan(&count); err != nil {
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

	websiteID := uuid.New().String()

	query := `INSERT INTO website (website_id, domain, name, allowed_domains, created_at, updated_at)
		VALUES ($1, $2, $3, $4::jsonb, NOW(), NOW())
		RETURNING ` + selectCols

	d, err := scanDetail(database.DB.QueryRowContext(ctx, query, websiteID, domain, name, allowedDomainsJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create website: %w", err)
	}
	return d, nil
}

// Update updates an existing website by domain. The name is updated when
// non-nil and allowedDomains replaces the stored list when non-empty.
func Update(ctx context.Context, domain string, name *string, allowedDomains []string) (*Detail, error) {
	website, err := GetByDomain(ctx, domain, nil)
	if err != nil {
		return nil, err
	}

	updates := []string{"updated_at = NOW()"}
	args := []any{website.WebsiteID}
	argIndex := 2

	if name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argIndex))
		args = append(args, *name)
		argIndex++
	}

	if len(allowedDomains) > 0 {
		data, _ := json.Marshal(allowedDomains)
		updates = append(updates, fmt.Sprintf("allowed_domains = $%d::jsonb", argIndex))
		args = append(args, string(data))
	}

	query := fmt.Sprintf(`UPDATE website
		SET %s
		WHERE website_id = $1 AND deleted_at IS NULL
		RETURNING `+selectCols, strings.Join(updates, ", "))

	return updateAndScan(ctx, domain, query, args...)
}

// Delete soft-deletes a website by setting deleted_at, returning the timestamp.
func Delete(ctx context.Context, domain string) (*time.Time, error) {
	website, err := GetByDomain(ctx, domain, nil)
	if err != nil {
		return nil, err
	}

	query := `UPDATE website
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE website_id = $1
		RETURNING deleted_at`

	var deletedAt time.Time
	if err := database.DB.QueryRowContext(ctx, query, website.WebsiteID).Scan(&deletedAt); err != nil {
		return nil, fmt.Errorf("failed to delete website: %w", err)
	}
	return &deletedAt, nil
}

// AddAllowedDomains merges domains into a website's allowed_domains JSONB array.
func AddAllowedDomains(ctx context.Context, websiteDomain string, domains []string) (*Detail, error) {
	website, err := GetByDomain(ctx, websiteDomain, nil)
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

	return setAllowedDomains(ctx, websiteDomain, website.WebsiteID, mergedDomains)
}

// RemoveAllowedDomain removes a domain from a website's allowed_domains array.
func RemoveAllowedDomain(ctx context.Context, websiteDomain, domainToRemove string) (*Detail, error) {
	website, err := GetByDomain(ctx, websiteDomain, nil)
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
		return nil, fmt.Errorf("cannot remove the last allowed domain (security: at least one domain must remain)")
	}

	return setAllowedDomains(ctx, websiteDomain, website.WebsiteID, newDomains)
}

// GetAllowedDomains returns the allowed_domains array for a website along with
// the full Detail it was read from.
func GetAllowedDomains(ctx context.Context, websiteDomain string) ([]string, *Detail, error) {
	website, err := GetByDomain(ctx, websiteDomain, nil)
	if err != nil {
		return nil, nil, err
	}
	return website.AllowedDomains, website, nil
}

// SetPublicStatsEnabled enables or disables public stats for a website.
func SetPublicStatsEnabled(ctx context.Context, websiteDomain string, enabled bool) (*Detail, error) {
	website, err := GetByDomain(ctx, websiteDomain, nil)
	if err != nil {
		return nil, err
	}

	query := `UPDATE website
		SET public_stats_enabled = $1, updated_at = NOW()
		WHERE website_id = $2 AND deleted_at IS NULL
		RETURNING ` + selectCols

	return updateAndScan(ctx, websiteDomain, query, enabled, website.WebsiteID)
}

// setAllowedDomains persists a replacement allowed_domains array.
func setAllowedDomains(ctx context.Context, websiteDomain, websiteID string, domains []string) (*Detail, error) {
	domainsJSON, _ := json.Marshal(domains)

	query := `UPDATE website
		SET allowed_domains = $1::jsonb, updated_at = NOW()
		WHERE website_id = $2 AND deleted_at IS NULL
		RETURNING ` + selectCols

	return updateAndScan(ctx, websiteDomain, query, string(domainsJSON), websiteID)
}

// updateAndScan runs a RETURNING update and scans the resulting row, mapping
// a missing row to a not-found error.
func updateAndScan(ctx context.Context, websiteDomain, query string, args ...any) (*Detail, error) {
	d, err := scanDetail(database.DB.QueryRowContext(ctx, query, args...))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("website '%s' not found", websiteDomain)
		}
		return nil, fmt.Errorf("failed to update website: %w", err)
	}
	return d, nil
}

// ValidateDomain validates a domain string format.
func ValidateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("invalid domain format: domain cannot be empty")
	}

	if len(domain) > 253 {
		return fmt.Errorf("invalid domain format: domain cannot exceed 253 characters (DNS standard)")
	}

	// Basic domain validation (alphanumeric, dots, hyphens).
	// Allow localhost for testing.
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
