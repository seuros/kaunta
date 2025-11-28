package handlers

import (
	"net/url"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/seuros/kaunta/internal/logging"
	"go.uber.org/zap"
)

// pixelGIF is a minimal 1x1 transparent GIF (42 bytes) - GIF89a format
var pixelGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00,
	0x01, 0x00, 0x80, 0x00, 0x00, 0xFF, 0xFF, 0xFF,
	0x00, 0x00, 0x00, 0x21, 0xF9, 0x04, 0x01, 0x00,
	0x00, 0x00, 0x00, 0x2C, 0x00, 0x00, 0x00, 0x00,
	0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44,
	0x01, 0x00,
}

// HandlePixelTracking serves a 1x1 transparent GIF and tracks the pageview/event
// Endpoint: GET /p/:id.gif?url=...&title=...
// Used for email campaigns, RSS feeds, and no-JS environments
func HandlePixelTracking(c fiber.Ctx) error {
	// Validate website UUID from path parameter
	websiteID := c.Params("id")
	if _, err := uuid.Parse(websiteID); err != nil {
		logging.L().Warn("pixel tracking: invalid website ID",
			zap.String("id", websiteID),
			zap.String("ip", c.IP()),
		)
		// Still return GIF to avoid breaking email rendering
		return servePixel(c)
	}

	// Build payload from query parameters
	payload := buildPixelPayload(c, websiteID)

	// Store in context for HandleTracking to consume
	c.Locals("pixel_payload", payload)

	// Call existing tracking handler (reuse all validation, bot detection, GeoIP, etc.)
	if err := HandleTracking(c); err != nil {
		// Log errors but still return GIF (silent tracking for emails/RSS)
		logging.L().Debug("pixel tracking failed",
			zap.String("website_id", websiteID),
			zap.Error(err),
		)
	}

	// Always return pixel image
	return servePixel(c)
}

// buildPixelPayload constructs TrackingPayload from path parameter and query params
func buildPixelPayload(c fiber.Ctx, websiteID string) TrackingPayload {
	payload := TrackingPayload{
		Type: "event",
		Payload: PayloadData{
			Website: websiteID,
		},
	}

	// Cache Referer header (used for both URL and Referrer fallbacks)
	refererHeader := c.Get("Referer")

	// Extract URL (query param or Referer header)
	if urlParam := c.Query("url"); urlParam != "" {
		payload.Payload.URL = &urlParam
	} else if refererHeader != "" {
		payload.Payload.URL = &refererHeader
	}

	// Extract title
	if title := c.Query("title"); title != "" {
		payload.Payload.Title = &title
	}

	// Extract referrer (query param or Referer header)
	if referrer := c.Query("referrer"); referrer != "" {
		payload.Payload.Referrer = &referrer
	} else if refererHeader != "" {
		payload.Payload.Referrer = &refererHeader
	}

	// Extract or derive hostname
	if hostname := c.Query("hostname"); hostname != "" {
		payload.Payload.Hostname = &hostname
	} else if payload.Payload.URL != nil {
		// Extract hostname from URL
		if u, err := url.Parse(*payload.Payload.URL); err == nil {
			h := u.Hostname()
			payload.Payload.Hostname = &h
		} else {
			logging.L().Debug("pixel tracking: failed to parse URL for hostname",
				zap.String("url", *payload.Payload.URL),
			)
		}
	}

	// Extract custom event name
	if name := c.Query("name"); name != "" {
		payload.Payload.Name = &name
	}

	// Extract event tag
	if tag := c.Query("tag"); tag != "" {
		payload.Payload.Tag = &tag
	}

	// Extract UTM campaign parameters
	if utmSource := c.Query("utm_source"); utmSource != "" {
		payload.Payload.UTMSource = &utmSource
	}
	if utmMedium := c.Query("utm_medium"); utmMedium != "" {
		payload.Payload.UTMMedium = &utmMedium
	}
	if utmCampaign := c.Query("utm_campaign"); utmCampaign != "" {
		payload.Payload.UTMCampaign = &utmCampaign
	}
	if utmTerm := c.Query("utm_term"); utmTerm != "" {
		payload.Payload.UTMTerm = &utmTerm
	}
	if utmContent := c.Query("utm_content"); utmContent != "" {
		payload.Payload.UTMContent = &utmContent
	}

	// Extract language from Accept-Language header
	if lang := c.Get("Accept-Language"); lang != "" {
		payload.Payload.Language = &lang
	}

	return payload
}

// servePixel returns a 1x1 transparent GIF with appropriate headers
func servePixel(c fiber.Ctx) error {
	// Prevent caching (every pixel request is unique)
	c.Set("Content-Type", "image/gif")
	c.Set("Content-Length", strconv.Itoa(len(pixelGIF)))
	c.Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")

	// CORS headers (allow embedding from anywhere)
	c.Set("Access-Control-Allow-Origin", "*")
	c.Set("Access-Control-Allow-Methods", "GET, OPTIONS")

	// Return GIF
	return c.Send(pixelGIF)
}
