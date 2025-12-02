package handlers

import (
	"database/sql"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/logging"
	"go.uber.org/zap"
)

// goalCacheEntry represents cached goals for a website
type goalCacheEntry struct {
	goals     []cachedGoal
	lastFetch time.Time
}

// cachedGoal is an optimized in-memory goal representation
type cachedGoal struct {
	ID          uuid.UUID
	Type        string // "page_view" or "custom_event"
	TargetValue string // URL path or event name
}

// GoalCache manages per-website goal caching with TTL
type GoalCache struct {
	cache map[uuid.UUID]*goalCacheEntry
	mu    sync.RWMutex
	ttl   time.Duration
}

var (
	goalCache = &GoalCache{
		cache: make(map[uuid.UUID]*goalCacheEntry),
		ttl:   5 * time.Minute, // Same as TrustedOriginsCache
	}
)

// GetGoalsForWebsite returns cached goals, refreshing if needed
func (gc *GoalCache) GetGoalsForWebsite(websiteID uuid.UUID) ([]cachedGoal, error) {
	gc.mu.RLock()
	entry, exists := gc.cache[websiteID]

	// Check if cache is valid
	if exists && time.Since(entry.lastFetch) < gc.ttl {
		goals := entry.goals
		gc.mu.RUnlock()
		return goals, nil
	}
	gc.mu.RUnlock()

	// Cache miss or expired - refresh
	return gc.refreshGoalsForWebsite(websiteID)
}

// refreshGoalsForWebsite fetches goals from database and updates cache
func (gc *GoalCache) refreshGoalsForWebsite(websiteID uuid.UUID) ([]cachedGoal, error) {
	rows, err := database.DB.Query(`
        SELECT id, target_url, target_event
        FROM goals
        WHERE website_id = $1
    `, websiteID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var goals []cachedGoal
	for rows.Next() {
		var id uuid.UUID
		var targetURL, targetEvent sql.NullString

		if err := rows.Scan(&id, &targetURL, &targetEvent); err != nil {
			return nil, err
		}

		// Determine goal type and value
		var goalType, targetValue string
		if targetEvent.Valid && targetEvent.String != "" {
			goalType = "custom_event"
			targetValue = targetEvent.String
		} else if targetURL.Valid {
			goalType = "page_view"
			targetValue = targetURL.String
		} else {
			// Skip malformed goals
			continue
		}

		goals = append(goals, cachedGoal{
			ID:          id,
			Type:        goalType,
			TargetValue: targetValue,
		})
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	// Update cache
	gc.mu.Lock()
	gc.cache[websiteID] = &goalCacheEntry{
		goals:     goals,
		lastFetch: time.Now(),
	}
	gc.mu.Unlock()

	logging.L().Debug("goal cache refreshed",
		zap.String("website_id", websiteID.String()),
		zap.Int("count", len(goals)))

	return goals, nil
}

// InvalidateWebsite removes a website's goals from cache (call on goal CRUD)
func (gc *GoalCache) InvalidateWebsite(websiteID uuid.UUID) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	delete(gc.cache, websiteID)
	logging.L().Debug("goal cache invalidated", zap.String("website_id", websiteID.String()))
}

// GetGoalsForWebsite is a package-level function for easy access
func GetGoalsForWebsite(websiteID uuid.UUID) ([]cachedGoal, error) {
	return goalCache.GetGoalsForWebsite(websiteID)
}

// InvalidateGoalCache invalidates cache for a website (call from goal handlers)
func InvalidateGoalCache(websiteID uuid.UUID) {
	goalCache.InvalidateWebsite(websiteID)
}
