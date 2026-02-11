package handlers

import (
	"net/http"
	"strings"

	"github.com/seuros/kaunta/internal/httpx"
)

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
	return httpx.ClientIP(r)
}
