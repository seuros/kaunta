package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/render"
	"github.com/google/uuid"
)

// respondError writes a JSON error response with the given status code.
func respondError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	render.Status(r, status)
	render.JSON(w, r, map[string]any{"error": msg})
}

// parseAllowedDomains decodes the JSON-encoded allowed_domains column,
// returning an empty slice if the data is empty or malformed.
func parseAllowedDomains(data []byte) []string {
	domains := []string{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &domains); err != nil {
			return []string{}
		}
	}
	return domains
}

// decodeJSONBody decodes the request body into dst, writing a 400 error and
// returning false if decoding fails.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := render.DecodeJSON(r.Body, dst); err != nil {
		respondError(w, r, http.StatusBadRequest, "Invalid request body")
		return false
	}
	return true
}

// parseWebsiteID validates the website_id URL param, returning it on success.
// On failure it writes a 400 error response and returns ok=false.
func parseWebsiteID(w http.ResponseWriter, r *http.Request, websiteIDStr string) (string, bool) {
	if _, err := uuid.Parse(websiteIDStr); err != nil {
		respondError(w, r, http.StatusBadRequest, "Invalid website ID")
		return "", false
	}
	return websiteIDStr, true
}

// clientIP returns the client IP for display/logging purposes, respecting common proxy headers.
// Only use this for display/logging — not for security decisions like rate limiting,
// since X-Forwarded-For can be spoofed by any client.
func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// clientIPFromRequest derives the client IP according to proxy mode configuration.
func clientIPFromRequest(r *http.Request, proxyMode string) string {
	switch proxyMode {
	case "cloudflare":
		if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
			return strings.TrimSpace(strings.Split(cfIP, ",")[0])
		}
	case "xforwarded":
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	return clientIP(r)
}
