package handlers

import (
	"net"
	"net/http"
	"strings"
)

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
