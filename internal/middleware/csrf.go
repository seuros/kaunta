package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/logging"
	"go.uber.org/zap"
)

// TrustedOriginsCache manages cached trusted origins with TTL
type TrustedOriginsCache struct {
	origins   []string
	lastFetch time.Time
	mu        sync.RWMutex
	ttl       time.Duration
}

var (
	originsCache = &TrustedOriginsCache{
		ttl: 5 * time.Minute, // Cache for 5 minutes
	}
)

// loadTrustedOrigins fetches trusted origins from database
func (c *TrustedOriginsCache) loadTrustedOrigins() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if cache is still valid
	if time.Since(c.lastFetch) < c.ttl && len(c.origins) > 0 {
		return nil
	}

	// Fetch from database using PostgreSQL function
	rows, err := database.DB.Query("SELECT unnest(get_trusted_origins())")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	var origins []string
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			return err
		}
		origins = append(origins, domain)
	}

	if err = rows.Err(); err != nil {
		return err
	}

	c.origins = origins
	c.lastFetch = time.Now()

	logging.L().Info("trusted origins cache refreshed", zap.Int("count", len(origins)))
	return nil
}

// GetTrustedOrigins returns cached trusted origins, refreshing if needed
func (c *TrustedOriginsCache) GetTrustedOrigins() ([]string, error) {
	// Try to load/refresh cache
	if err := c.loadTrustedOrigins(); err != nil {
		// If database fails, try to use stale cache
		c.mu.RLock()
		defer c.mu.RUnlock()

		if len(c.origins) > 0 {
			logging.L().Warn("using stale trusted origins cache", zap.Error(err))
			return c.origins, nil
		}
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.origins, nil
}

// GetTrustedOrigins is a package-level function that returns cached trusted origins
func GetTrustedOrigins() ([]string, error) {
	return originsCache.GetTrustedOrigins()
}

// ForceRefresh immediately refreshes the cache from database
func (c *TrustedOriginsCache) ForceRefresh() error {
	c.mu.Lock()
	c.lastFetch = time.Time{} // Reset last fetch time
	c.mu.Unlock()

	return c.loadTrustedOrigins()
}

// RefreshTrustedOrigins is a middleware that can be used to force cache refresh
// Useful for admin endpoints that modify trusted origins
func RefreshTrustedOrigins(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := originsCache.ForceRefresh(); err != nil {
			logging.L().Warn("failed to refresh trusted origins cache", zap.Error(err))
		}
		next.ServeHTTP(w, r)
	})
}

// InitTrustedOriginsCache initializes the cache at startup
func InitTrustedOriginsCache() error {
	logging.L().Info("initializing trusted origins cache")
	if err := originsCache.ForceRefresh(); err != nil {
		logging.L().Warn("failed to initialize trusted origins cache", zap.Error(err))
		// Don't fail startup if no trusted origins exist yet
		return nil
	}
	return nil
}
