package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/seuros/kaunta/internal/httpx"
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
func HandlePixelTracking(w http.ResponseWriter, r *http.Request) {
	websiteID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(websiteID); err != nil {
		logging.L().Warn("pixel tracking: invalid website ID",
			zap.String("id", websiteID),
			zap.String("ip", httpx.ClientIP(r)),
		)
		servePixel(w)
		return
	}

	payload := buildPixelPayload(r, websiteID)
	req := withPixelPayload(r, payload)

	recorder := httptest.NewRecorder()
	HandleTracking(recorder, req)

	if recorder.Code >= 400 {
		logging.L().Debug("pixel tracking failed",
			zap.String("website_id", websiteID),
			zap.Int("status", recorder.Code),
		)
	}

	servePixel(w)
}

// buildPixelPayload constructs TrackingPayload from path parameter and query params
func buildPixelPayload(r *http.Request, websiteID string) TrackingPayload {
	payload := TrackingPayload{
		Type: "event",
		Payload: PayloadData{
			Website: websiteID,
		},
	}

	// Cache Referer header (used for both URL and Referrer fallbacks)
	refererHeader := r.Header.Get("Referer")

	// Extract URL (query param or Referer header)
	query := r.URL.Query()

	if urlParam := query.Get("url"); urlParam != "" {
		payload.Payload.URL = &urlParam
	} else if refererHeader != "" {
		payload.Payload.URL = &refererHeader
	}

	// Extract title
	if title := query.Get("title"); title != "" {
		payload.Payload.Title = &title
	}

	// Extract referrer (query param or Referer header)
	if referrer := query.Get("referrer"); referrer != "" {
		payload.Payload.Referrer = &referrer
	} else if refererHeader != "" {
		payload.Payload.Referrer = &refererHeader
	}

	// Extract or derive hostname
	if hostname := query.Get("hostname"); hostname != "" {
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
	if name := query.Get("name"); name != "" {
		payload.Payload.Name = &name
	}

	// Extract event tag
	if tag := query.Get("tag"); tag != "" {
		payload.Payload.Tag = &tag
	}

	// Extract UTM campaign parameters
	if utmSource := query.Get("utm_source"); utmSource != "" {
		payload.Payload.UTMSource = &utmSource
	}
	if utmMedium := query.Get("utm_medium"); utmMedium != "" {
		payload.Payload.UTMMedium = &utmMedium
	}
	if utmCampaign := query.Get("utm_campaign"); utmCampaign != "" {
		payload.Payload.UTMCampaign = &utmCampaign
	}
	if utmTerm := query.Get("utm_term"); utmTerm != "" {
		payload.Payload.UTMTerm = &utmTerm
	}
	if utmContent := query.Get("utm_content"); utmContent != "" {
		payload.Payload.UTMContent = &utmContent
	}

	// Extract language from Accept-Language header
	if lang := r.Header.Get("Accept-Language"); lang != "" {
		payload.Payload.Language = &lang
	}

	return payload
}

// servePixel returns a 1x1 transparent GIF with appropriate headers
func servePixel(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Content-Length", strconv.Itoa(len(pixelGIF)))
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	_, _ = w.Write(pixelGIF)
}
