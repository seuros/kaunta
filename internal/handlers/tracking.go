package handlers

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/seuros/kaunta/internal/config"
	"github.com/seuros/kaunta/internal/database"
	"github.com/seuros/kaunta/internal/geoip"
	"github.com/seuros/kaunta/internal/logging"
	"github.com/seuros/kaunta/internal/middleware"
	"github.com/seuros/kaunta/internal/realtime"
	"go.uber.org/zap"
)

const MaxURLSize = 2000 // Max URL length (Plausible standard)

// Spam referrer domains (from Plausible patterns)
var spamReferrers = []string{
	"semalt.com",
	"buttons-for-website.com",
	"darodar.com",
	"best-seo-offer.com",
	"free-share-buttons.com",
	"blackhatworth.com",
	"hulfingtonpost.com",
	"o-o-6-o-o.com",
	"priceg.com",
	"make-money-online",
	"simple-share-buttons.com",
	"kambasoft.com",
}

// TrackingPayload matches Umami's /api/send payload
type TrackingPayload struct {
	Type    string      `json:"type"` // "event" or "identify"
	Payload PayloadData `json:"payload"`
}

type PayloadData struct {
	Website   string                 `json:"website"` // website UUID
	Hostname  *string                `json:"hostname,omitempty"`
	Language  *string                `json:"language,omitempty"`
	Referrer  *string                `json:"referrer,omitempty"`
	Screen    *string                `json:"screen,omitempty"`
	Title     *string                `json:"title,omitempty"`
	URL       *string                `json:"url,omitempty"`
	Name      *string                `json:"name,omitempty"` // event name
	Tag       *string                `json:"tag,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	IP        *string                `json:"ip,omitempty"`
	UserAgent *string                `json:"userAgent,omitempty"`
	Timestamp *int64                 `json:"timestamp,omitempty"`
	ID        *string                `json:"id,omitempty"` // distinct_id

	// Enhanced tracking (Phase 2)
	ScrollDepth    *int                   `json:"scroll_depth,omitempty"`    // 0-100 percentage
	EngagementTime *int                   `json:"engagement_time,omitempty"` // milliseconds
	Props          map[string]interface{} `json:"props,omitempty"`           // custom properties

	// UTM Campaign Parameters
	UTMSource   *string `json:"utm_source,omitempty"`   // e.g., google, newsletter
	UTMMedium   *string `json:"utm_medium,omitempty"`   // e.g., cpc, email
	UTMCampaign *string `json:"utm_campaign,omitempty"` // e.g., spring_sale
	UTMTerm     *string `json:"utm_term,omitempty"`     // paid search keywords
	UTMContent  *string `json:"utm_content,omitempty"`  // ad variant identifier
}

// HandleTracking is the /api/send endpoint - compatible with Umami
func HandleTracking(c fiber.Ctx) error {
	var payload TrackingPayload
	if err := c.Bind().Body(&payload); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid JSON payload",
		})
	}

	// Validate website UUID
	websiteID, err := uuid.Parse(payload.Payload.Website)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid website ID",
		})
	}

	// Verify website exists and fetch proxy_mode
	var proxyMode string
	err = database.DB.QueryRow(
		"SELECT COALESCE(proxy_mode, 'none') FROM website WHERE website_id = $1",
		websiteID,
	).Scan(&proxyMode)

	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Website not found",
		})
	}

	// Self website (dogfooding) requires authenticated session to prevent abuse
	// Since the UUID is well-known, we verify the request comes from a logged-in user
	if websiteID.String() == config.SelfWebsiteID {
		sessionToken := c.Cookies("kaunta_session")
		if sessionToken == "" {
			return c.Status(403).JSON(fiber.Map{
				"error": "Self-tracking requires authentication",
			})
		}
		// Verify session is valid
		tokenHash := middleware.HashToken(sessionToken)
		var sessionValid bool
		err := database.DB.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM user_sessions WHERE token_hash = $1 AND expires_at > NOW())",
			tokenHash,
		).Scan(&sessionValid)
		if err != nil || !sessionValid {
			return c.Status(403).JSON(fiber.Map{
				"error": "Invalid session for self-tracking",
			})
		}
	}

	// Origin validation (CORS security)
	origin := c.Get("Origin")
	if origin == "" {
		origin = c.Get("Referer") // Fallback to Referer header
	}

	var originAllowed bool
	err = database.DB.QueryRow(
		"SELECT validate_origin($1, $2)",
		websiteID, origin,
	).Scan(&originAllowed)

	if err != nil {
		logging.L().Warn("origin validation error", zap.String("website_id", websiteID.String()), zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"error": "Origin validation failed",
		})
	}

	if !originAllowed {
		logging.L().Warn("origin blocked", zap.String("origin", origin), zap.String("website_id", websiteID.String()))
		return c.Status(403).JSON(fiber.Map{
			"error":  "Origin not allowed",
			"origin": origin,
			"hint":   "Add this domain to the allowed list using: kaunta website add-domain",
		})
	}

	// Set proper CORS header for allowed origin
	if origin != "" && origin != "null" {
		c.Set("Access-Control-Allow-Origin", origin)
	} else {
		c.Set("Access-Control-Allow-Origin", "*")
	}

	// Get client info
	ip := getClientIP(c, proxyMode)
	userAgent := c.Get("User-Agent")

	// Override with payload if provided
	if payload.Payload.IP != nil {
		ip = *payload.Payload.IP
	}
	if payload.Payload.UserAgent != nil {
		userAgent = *payload.Payload.UserAgent
	}

	// Bot detection using PostgreSQL (dictatorship approach - all logic in DB)
	// This updates IP metadata and returns bot status in one call
	var isBot *bool // Use pointer to handle NULL values
	err = database.DB.QueryRow(`
		SELECT update_ip_metadata($1::inet, $2, NULL)
	`, ip, userAgent).Scan(&isBot)

	if err != nil {
		// Log error but don't block traffic on bot detection failure
		logging.L().Warn("bot detection error", zap.String("ip", ip), zap.Error(err))
		// Default to not a bot if detection fails
		isBotVal := false
		isBot = &isBotVal
	}

	// Check if it's a bot (handle nil gracefully)
	if isBot != nil && *isBot {
		// Return 202 for bots (acknowledged but not processed)
		return c.Status(202).JSON(fiber.Map{"beep": "boop", "bot_detected": true})
	}

	// Validate URL length
	if payload.Payload.URL != nil && len(*payload.Payload.URL) > MaxURLSize {
		return c.Status(400).JSON(fiber.Map{
			"error": "URL too long (max 2000 characters)",
		})
	}

	// Check spam referrer
	if payload.Payload.Referrer != nil && isSpamReferrer(*payload.Payload.Referrer) {
		return c.Status(202).JSON(fiber.Map{"dropped": "spam_referrer"})
	}

	// Parse client info
	browser, os, device := parseUserAgent(userAgent)

	// GeoIP lookup from IP address
	countryStr, cityStr, regionStr := geoIPLookup(ip)
	country := &countryStr
	region := &regionStr
	city := &cityStr

	// Generate session ID (deterministic based on IP + UA + date)
	createdAt := time.Now()
	if payload.Payload.Timestamp != nil {
		createdAt = time.Unix(*payload.Payload.Timestamp, 0)
	}

	sessionSalt := hashDate(createdAt, "month")
	sessionID := generateUUID(websiteID.String(), ip, userAgent, sessionSalt)

	// Parse URL path for entry/exit page tracking
	var urlPath *string
	if payload.Payload.URL != nil {
		if u, err := url.Parse(*payload.Payload.URL); err == nil {
			path := u.Path
			urlPath = &path
		}
	}

	// Create or update session (also tracks entry/exit pages)
	distinctID := payload.Payload.ID
	err = upsertSession(sessionID, websiteID, browser, os, device,
		payload.Payload.Screen, payload.Payload.Language,
		country, region, city, distinctID, urlPath)

	if err != nil {
		logging.L().Error("session creation error",
			zap.String("website_id", websiteID.String()),
			zap.String("session_id", sessionID.String()),
			zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to create session: " + err.Error(),
		})
	}

	// Handle event type
	if payload.Type == "event" {
		visitSalt := hashDate(createdAt, "hour")
		visitID := generateUUID(sessionID.String(), visitSalt)

		err = saveEvent(websiteID, sessionID, visitID, createdAt, payload.Payload,
			browser, os, device, country, region, city)

		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to save event: " + err.Error(),
			})
		}

		eventPath := ""
		if payload.Payload.URL != nil {
			eventPath = *payload.Payload.URL
		}
		eventTitle := ""
		if payload.Payload.Title != nil {
			eventTitle = *payload.Payload.Title
		}
		realtime.NotifyEvent(
			context.Background(),
			realtime.NewEventPayload(
				payload.Type,
				websiteID,
				sessionID,
				visitID,
				eventPath,
				eventTitle,
				createdAt,
			),
		)

		// Return 202 Accepted (acknowledges receipt, not completion)
		return c.Status(202).JSON(fiber.Map{
			"sessionId": sessionID.String(),
			"visitId":   visitID.String(),
		})
	}

	// Handle identify type
	if payload.Type == "identify" && payload.Payload.Data != nil {
		// TODO: Save session_data
		return c.Status(202).JSON(fiber.Map{
			"sessionId": sessionID.String(),
		})
	}

	return c.Status(400).JSON(fiber.Map{
		"error": "Invalid type",
	})
}

// upsertSession creates or updates a session
// On INSERT: sets entry_page and exit_page to the first page visited
// On UPDATE: only updates exit_page (entry_page remains the original landing page)
func upsertSession(sessionID, websiteID uuid.UUID, browser, os, device, screen, language, country, region, city *string, distinctID *string, urlPath *string) error {
	query := `
		INSERT INTO session (
			session_id, website_id, browser, os, device, screen, language,
			country, region, city, created_at, distinct_id, entry_page, exit_page
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), $11, $12, $12)
		ON CONFLICT (session_id) DO UPDATE SET exit_page = EXCLUDED.entry_page
	`
	_, err := database.DB.Exec(query, sessionID, websiteID, browser, os, device,
		screen, language, country, region, city, distinctID, urlPath)
	return err
}

// saveEvent saves a pageview or custom event
func saveEvent(websiteID, sessionID, visitID uuid.UUID, createdAt time.Time,
	payload PayloadData, browser, os, device, country, region, city *string) error {

	eventID := uuid.New()
	eventType := 1
	if payload.Name != nil && strings.TrimSpace(*payload.Name) != "" {
		eventType = 2
	}

	// Parse URL
	var urlPath, urlQuery, hostname, referrerPath, referrerQuery, referrerDomain *string
	if payload.URL != nil {
		if u, err := url.Parse(*payload.URL); err == nil {
			path := u.Path
			urlPath = &path
			query := u.RawQuery
			if query != "" {
				urlQuery = &query
			}
			if payload.Hostname != nil {
				hostname = payload.Hostname
			} else {
				h := u.Hostname()
				hostname = &h
			}
		}
	}

	// Parse referrer
	if payload.Referrer != nil {
		if u, err := url.Parse(*payload.Referrer); err == nil {
			path := u.Path
			referrerPath = &path
			query := u.RawQuery
			if query != "" {
				referrerQuery = &query
			}
			domain := strings.TrimPrefix(u.Hostname(), "www.")
			if domain != "localhost" && domain != "" {
				referrerDomain = &domain
			}
		}
	}

	// Convert props/data to JSON (Phase 2)
	var propsJSON interface{}
	if payload.Props != nil || payload.Data != nil {
		combined := make(map[string]interface{})
		if payload.Props != nil {
			for key, value := range payload.Props {
				combined[key] = value
			}
		}
		if payload.Data != nil {
			for key, value := range payload.Data {
				combined[key] = value
			}
		}
		if len(combined) > 0 {
			jsonBytes, _ := json.Marshal(combined)
			propsJSON = jsonBytes
		}
	}

	// Enhanced tracking: scroll_depth and engagement_time (Phase 2)
	var scrollDepth *int
	var engagementTime *int

	if payload.ScrollDepth != nil {
		// Validate scroll depth (0-100)
		if *payload.ScrollDepth >= 0 && *payload.ScrollDepth <= 100 {
			scrollDepth = payload.ScrollDepth
		}
	}

	if payload.EngagementTime != nil {
		// Validate engagement time (positive milliseconds)
		if *payload.EngagementTime >= 0 {
			engagementTime = payload.EngagementTime
		}
	}

	// Enhanced schema: includes Phase 2 fields + UTM tracking
	query := `
		INSERT INTO website_event (
			event_id, website_id, session_id, visit_id, created_at,
			page_title, hostname, url_path, url_query,
			referrer_path, referrer_query, referrer_domain,
			event_name, tag, event_type,
			scroll_depth, engagement_time, props,
			utm_source, utm_medium, utm_campaign, utm_term, utm_content
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11, $12,
			$13, $14, $15,
			$16, $17, $18,
			$19, $20, $21, $22, $23
		)
	`

	logging.L().Debug("inserting event",
		zap.Int("event_type", eventType),
		zap.String("event_id", eventID.String()),
		zap.String("website_id", websiteID.String()),
		zap.String("session_id", sessionID.String()),
		zap.String("visit_id", visitID.String()),
	)

	_, err := database.DB.Exec(query,
		eventID, websiteID, sessionID, visitID, createdAt,
		payload.Title, hostname, urlPath, urlQuery,
		referrerPath, referrerQuery, referrerDomain,
		payload.Name, payload.Tag, eventType,
		scrollDepth, engagementTime, propsJSON,
		payload.UTMSource, payload.UTMMedium, payload.UTMCampaign, payload.UTMTerm, payload.UTMContent,
	)

	if err != nil {
		logging.L().Error("failed to insert event", zap.Error(err))
	}

	return err
}

// generateUUID creates a deterministic UUID from components
func generateUUID(parts ...string) uuid.UUID {
	combined := strings.Join(parts, "|")
	hash := md5.Sum([]byte(combined))
	id, _ := uuid.FromBytes(hash[:])
	return id
}

// hashDate creates a salt from a date (for session/visit IDs)
func hashDate(t time.Time, period string) string {
	var key string
	switch period {
	case "month":
		key = t.Format("2006-01")
	case "hour":
		key = t.Format("2006-01-02T15")
	default:
		key = t.Format("2006-01-02")
	}
	hash := md5.Sum([]byte(key))
	return hex.EncodeToString(hash[:])
}

// isBot is now handled by PostgreSQL function: update_ip_metadata()
// See database/migrations/000005_add_bot_detection.up.sql
// Kept as comment for reference - DO NOT USE, call update_ip_metadata() instead

// isSpamReferrer checks if referrer is from known spam domain
func isSpamReferrer(referrer string) bool {
	if referrer == "" {
		return false
	}

	// Parse referrer URL to get domain
	u, err := url.Parse(referrer)
	if err != nil {
		return false
	}

	domain := strings.ToLower(u.Hostname())
	domain = strings.TrimPrefix(domain, "www.")

	// Check against spam list
	for _, spam := range spamReferrers {
		if strings.Contains(domain, spam) {
			return true
		}
	}
	return false
}

// parseUserAgent extracts browser, OS, device from UA string
func parseUserAgent(ua string) (browser, os, device *string) {
	// Simple parsing (TODO: use proper UA parser library)
	ua = strings.ToLower(ua)

	// Browser
	var b string
	switch {
	case strings.Contains(ua, "edg"):
		b = "Edge"
	case strings.Contains(ua, "chrome"):
		b = "Chrome"
	case strings.Contains(ua, "firefox"):
		b = "Firefox"
	case strings.Contains(ua, "safari"):
		b = "Safari"
	default:
		b = "Unknown"
	}
	browser = &b

	// OS
	var o string
	switch {
	case strings.Contains(ua, "android"):
		o = "Android"
	case strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad") || strings.Contains(ua, "ios"):
		o = "iOS"
	case strings.Contains(ua, "windows"):
		o = "Windows"
	case strings.Contains(ua, "mac os x") || strings.Contains(ua, "macintosh"):
		o = "macOS"
	case strings.Contains(ua, "linux"):
		o = "Linux"
	default:
		o = "Unknown"
	}
	os = &o

	// Device
	var d string
	if strings.Contains(ua, "mobile") || strings.Contains(ua, "iphone") || strings.Contains(ua, "android") || strings.Contains(ua, "ipad") {
		d = "mobile"
	} else {
		d = "desktop"
	}
	device = &d

	return
}

// geoIPLookup performs country/city/region lookup for an IP address
func geoIPLookup(ip string) (country, city, region string) {
	country, city, region = geoip.LookupIP(ip)
	return
}

// getClientIP extracts client IP based on proxy_mode configuration
// Supports:
// - "none": direct connection IP (default)
// - "xforwarded": X-Forwarded-For header (first IP from comma-separated list)
// - "cloudflare": CF-Connecting-IP header (Cloudflare)
func getClientIP(c fiber.Ctx, proxyMode string) string {
	switch proxyMode {
	case "cloudflare":
		if cfIP := c.Get("CF-Connecting-IP"); cfIP != "" {
			return cfIP
		}
	case "xforwarded":
		if xff := c.Get("X-Forwarded-For"); xff != "" {
			// Take first IP from comma-separated list
			return strings.Split(xff, ",")[0]
		}
	}
	// Default: use direct connection IP
	return c.IP()
}
