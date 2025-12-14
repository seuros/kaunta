package models

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/seuros/kaunta/internal/database"
)

// APIKey represents an API key for server-side event ingestion
type APIKey struct {
	KeyID              uuid.UUID  `json:"key_id"`
	WebsiteID          uuid.UUID  `json:"website_id"`
	CreatedBy          *uuid.UUID `json:"created_by,omitempty"`
	KeyHash            string     `json:"-"` // Never expose hash
	KeyPrefix          string     `json:"key_prefix"`
	Name               *string    `json:"name,omitempty"`
	Scopes             []string   `json:"scopes"`
	RateLimitPerMinute int        `json:"rate_limit_per_minute"`
	CreatedAt          time.Time  `json:"created_at"`
	LastUsedAt         *time.Time `json:"last_used_at,omitempty"`
	RevokedAt          *time.Time `json:"revoked_at,omitempty"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	WebsiteRateLimit   int        `json:"website_rate_limit,omitempty"` // From joined website
}

// APIKeyCreateResult contains the full key (shown once) and the stored record
type APIKeyCreateResult struct {
	FullKey string  `json:"api_key"` // Only returned on creation
	APIKey  *APIKey `json:"key"`
}

const (
	apiKeyPrefix = "kaunta_live_"
	keyByteLen   = 32 // 256-bit entropy
)

// GenerateAPIKey creates a new API key for a website with default scopes
func GenerateAPIKey(websiteID uuid.UUID, createdBy *uuid.UUID, name *string) (*APIKeyCreateResult, error) {
	return GenerateAPIKeyWithScopes(websiteID, createdBy, name, []string{"ingest"})
}

// GenerateAPIKeyWithScopes creates a new API key for a website with custom scopes
func GenerateAPIKeyWithScopes(websiteID uuid.UUID, createdBy *uuid.UUID, name *string, scopes []string) (*APIKeyCreateResult, error) {
	// Validate scopes
	validScopes := map[string]bool{"ingest": true, "stats": true}
	for _, scope := range scopes {
		if !validScopes[scope] {
			return nil, fmt.Errorf("invalid scope: %s (valid: ingest, stats)", scope)
		}
	}
	if len(scopes) == 0 {
		scopes = []string{"ingest"} // Default to ingest if none specified
	}
	// Generate 32 random bytes (256-bit entropy)
	randomBytes := make([]byte, keyByteLen)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, err
	}

	// Full key: kaunta_live_<64 hex chars>
	fullKey := apiKeyPrefix + hex.EncodeToString(randomBytes)

	// Hash for storage
	hash := sha256.Sum256([]byte(fullKey))
	keyHash := hex.EncodeToString(hash[:])

	// Prefix for display (first 16 chars including prefix)
	keyPrefixDisplay := fullKey[:16]

	keyID := uuid.New()
	now := time.Now()

	query := `
		INSERT INTO api_keys (key_id, website_id, created_by, key_hash, key_prefix, name, scopes, rate_limit_per_minute, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING key_id, website_id, created_by, key_prefix, name, scopes, rate_limit_per_minute, created_at
	`

	var apiKey APIKey
	var createdByNull sql.NullString
	if createdBy != nil {
		createdByNull = sql.NullString{String: createdBy.String(), Valid: true}
	}

	err := database.DB.QueryRow(
		query,
		keyID,
		websiteID,
		createdByNull,
		keyHash,
		keyPrefixDisplay,
		name,
		pq.Array(scopes),
		1000, // Default rate limit
		now,
	).Scan(
		&apiKey.KeyID,
		&apiKey.WebsiteID,
		&createdByNull,
		&apiKey.KeyPrefix,
		&apiKey.Name,
		pq.Array(&apiKey.Scopes),
		&apiKey.RateLimitPerMinute,
		&apiKey.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if createdByNull.Valid {
		id, _ := uuid.Parse(createdByNull.String)
		apiKey.CreatedBy = &id
	}

	return &APIKeyCreateResult{
		FullKey: fullKey,
		APIKey:  &apiKey,
	}, nil
}

// HashAPIKey creates SHA256 hash of an API key for lookup
func HashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// GetAPIKeyByHash retrieves an API key by its hash (for authentication)
func GetAPIKeyByHash(keyHash string) (*APIKey, error) {
	query := `
		SELECT
			ak.key_id,
			ak.website_id,
			ak.created_by,
			ak.key_prefix,
			ak.name,
			ak.scopes,
			ak.rate_limit_per_minute,
			ak.created_at,
			ak.last_used_at,
			ak.revoked_at,
			ak.expires_at,
			w.api_rate_limit_per_minute
		FROM api_keys ak
		JOIN website w ON ak.website_id = w.website_id
		WHERE ak.key_hash = $1
		  AND ak.revoked_at IS NULL
		  AND w.deleted_at IS NULL
		  AND (ak.expires_at IS NULL OR ak.expires_at > NOW())
	`

	var apiKey APIKey
	var createdByNull, nameNull sql.NullString
	var lastUsedAt, revokedAt, expiresAt sql.NullTime

	err := database.DB.QueryRow(query, keyHash).Scan(
		&apiKey.KeyID,
		&apiKey.WebsiteID,
		&createdByNull,
		&apiKey.KeyPrefix,
		&nameNull,
		pq.Array(&apiKey.Scopes),
		&apiKey.RateLimitPerMinute,
		&apiKey.CreatedAt,
		&lastUsedAt,
		&revokedAt,
		&expiresAt,
		&apiKey.WebsiteRateLimit,
	)
	if err != nil {
		return nil, err
	}

	if createdByNull.Valid {
		id, _ := uuid.Parse(createdByNull.String)
		apiKey.CreatedBy = &id
	}
	if nameNull.Valid {
		apiKey.Name = &nameNull.String
	}
	if lastUsedAt.Valid {
		apiKey.LastUsedAt = &lastUsedAt.Time
	}
	if revokedAt.Valid {
		apiKey.RevokedAt = &revokedAt.Time
	}
	if expiresAt.Valid {
		apiKey.ExpiresAt = &expiresAt.Time
	}

	return &apiKey, nil
}

// ListAPIKeys returns all API keys for a website
func ListAPIKeys(websiteID uuid.UUID) ([]*APIKey, error) {
	query := `
		SELECT
			key_id,
			website_id,
			created_by,
			key_prefix,
			name,
			scopes,
			rate_limit_per_minute,
			created_at,
			last_used_at,
			revoked_at,
			expires_at
		FROM api_keys
		WHERE website_id = $1
		ORDER BY created_at DESC
	`

	rows, err := database.DB.Query(query, websiteID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var keys []*APIKey
	for rows.Next() {
		var apiKey APIKey
		var createdByNull, nameNull sql.NullString
		var lastUsedAt, revokedAt, expiresAt sql.NullTime

		err := rows.Scan(
			&apiKey.KeyID,
			&apiKey.WebsiteID,
			&createdByNull,
			&apiKey.KeyPrefix,
			&nameNull,
			pq.Array(&apiKey.Scopes),
			&apiKey.RateLimitPerMinute,
			&apiKey.CreatedAt,
			&lastUsedAt,
			&revokedAt,
			&expiresAt,
		)
		if err != nil {
			return nil, err
		}

		if createdByNull.Valid {
			id, _ := uuid.Parse(createdByNull.String)
			apiKey.CreatedBy = &id
		}
		if nameNull.Valid {
			apiKey.Name = &nameNull.String
		}
		if lastUsedAt.Valid {
			apiKey.LastUsedAt = &lastUsedAt.Time
		}
		if revokedAt.Valid {
			apiKey.RevokedAt = &revokedAt.Time
		}
		if expiresAt.Valid {
			apiKey.ExpiresAt = &expiresAt.Time
		}

		keys = append(keys, &apiKey)
	}

	return keys, rows.Err()
}

// RevokeAPIKey marks an API key as revoked
func RevokeAPIKey(keyID uuid.UUID) error {
	query := `UPDATE api_keys SET revoked_at = NOW() WHERE key_id = $1 AND revoked_at IS NULL`
	result, err := database.DB.Exec(query, keyID)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// RevokeAPIKeyByPrefix revokes a key by its prefix
func RevokeAPIKeyByPrefix(prefix string) error {
	query := `UPDATE api_keys SET revoked_at = NOW() WHERE key_prefix = $1 AND revoked_at IS NULL`
	result, err := database.DB.Exec(query, prefix)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateAPIKeyLastUsed updates the last_used_at timestamp (fire and forget)
func UpdateAPIKeyLastUsed(keyID uuid.UUID) {
	if database.DB == nil {
		return // Skip if no database connection (e.g., during tests)
	}
	query := `UPDATE api_keys SET last_used_at = NOW() WHERE key_id = $1`
	_, _ = database.DB.Exec(query, keyID)
}

// GetAPIKeyByID retrieves an API key by its ID
func GetAPIKeyByID(keyID uuid.UUID) (*APIKey, error) {
	query := `
		SELECT
			key_id,
			website_id,
			created_by,
			key_prefix,
			name,
			scopes,
			rate_limit_per_minute,
			created_at,
			last_used_at,
			revoked_at,
			expires_at
		FROM api_keys
		WHERE key_id = $1
	`

	var apiKey APIKey
	var createdByNull, nameNull sql.NullString
	var lastUsedAt, revokedAt, expiresAt sql.NullTime

	err := database.DB.QueryRow(query, keyID).Scan(
		&apiKey.KeyID,
		&apiKey.WebsiteID,
		&createdByNull,
		&apiKey.KeyPrefix,
		&nameNull,
		pq.Array(&apiKey.Scopes),
		&apiKey.RateLimitPerMinute,
		&apiKey.CreatedAt,
		&lastUsedAt,
		&revokedAt,
		&expiresAt,
	)
	if err != nil {
		return nil, err
	}

	if createdByNull.Valid {
		id, _ := uuid.Parse(createdByNull.String)
		apiKey.CreatedBy = &id
	}
	if nameNull.Valid {
		apiKey.Name = &nameNull.String
	}
	if lastUsedAt.Valid {
		apiKey.LastUsedAt = &lastUsedAt.Time
	}
	if revokedAt.Valid {
		apiKey.RevokedAt = &revokedAt.Time
	}
	if expiresAt.Valid {
		apiKey.ExpiresAt = &expiresAt.Time
	}

	return &apiKey, nil
}

// CheckEventIDExists checks if an event ID already exists (idempotency)
func CheckEventIDExists(eventID uuid.UUID, websiteID uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM event_idempotency WHERE event_id = $1 AND website_id = $2)`
	var exists bool
	err := database.DB.QueryRow(query, eventID, websiteID).Scan(&exists)
	return exists, err
}

// InsertEventID records an event ID for idempotency checking
func InsertEventID(eventID uuid.UUID, websiteID uuid.UUID) error {
	query := `INSERT INTO event_idempotency (event_id, website_id, created_at) VALUES ($1, $2, NOW()) ON CONFLICT DO NOTHING`
	_, err := database.DB.Exec(query, eventID, websiteID)
	return err
}

// HasScope checks if the API key has a specific scope
func (k *APIKey) HasScope(scope string) bool {
	for _, s := range k.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// IsValid checks if the key is valid (not revoked, not expired)
func (k *APIKey) IsValid() bool {
	if k.RevokedAt != nil {
		return false
	}
	if k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now()) {
		return false
	}
	return true
}
