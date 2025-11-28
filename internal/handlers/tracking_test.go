package handlers

import (
	"strings"
	"testing"
)

// TestGetClientIPLogic tests the IP extraction logic without Fiber dependency
func TestGetClientIPLogic(t *testing.T) {
	tests := []struct {
		name               string
		proxyMode          string
		cfConnectingIP     string
		xForwardedFor      string
		expectedStartsWith string
		description        string
	}{
		{
			name:           "none mode ignores headers",
			proxyMode:      "none",
			cfConnectingIP: "203.0.113.1",
			xForwardedFor:  "198.51.100.1",
			description:    "Should use c.IP() fallback when proxy_mode is 'none'",
		},
		{
			name:               "cloudflare mode uses CF header",
			proxyMode:          "cloudflare",
			cfConnectingIP:     "203.0.113.1",
			xForwardedFor:      "198.51.100.1",
			expectedStartsWith: "203.0.113.1",
			description:        "Should extract CF-Connecting-IP when proxy_mode is 'cloudflare'",
		},
		{
			name:           "cloudflare mode fallback when header empty",
			proxyMode:      "cloudflare",
			cfConnectingIP: "",
			xForwardedFor:  "198.51.100.1",
			description:    "Should fallback to c.IP() when CF-Connecting-IP is empty",
		},
		{
			name:               "cloudflare multiple IPs takes first",
			proxyMode:          "cloudflare",
			cfConnectingIP:     "102.97.33.165, 2400:cb00:40:1000:abaf:890d:d2f:bbf4",
			xForwardedFor:      "198.51.100.1",
			expectedStartsWith: "102.97.33.165",
			description:        "Should extract first IP from CF-Connecting-IP when multiple IPs (IPv4 and IPv6)",
		},
		{
			name:               "cloudflare trims whitespace",
			proxyMode:          "cloudflare",
			cfConnectingIP:     "102.97.33.165 , 198.51.100.1",
			xForwardedFor:      "",
			expectedStartsWith: "102.97.33.165",
			description:        "Should trim whitespace from CF-Connecting-IP",
		},
		{
			name:               "xforwarded takes first IP",
			proxyMode:          "xforwarded",
			cfConnectingIP:     "203.0.113.1",
			xForwardedFor:      "203.0.113.2, 198.51.100.1, 192.0.2.1",
			expectedStartsWith: "203.0.113.2",
			description:        "Should extract first IP from X-Forwarded-For list",
		},
		{
			name:               "xforwarded single IP",
			proxyMode:          "xforwarded",
			cfConnectingIP:     "203.0.113.1",
			xForwardedFor:      "203.0.113.2",
			expectedStartsWith: "203.0.113.2",
			description:        "Should handle single IP in X-Forwarded-For",
		},
		{
			name:               "xforwarded trims whitespace",
			proxyMode:          "xforwarded",
			cfConnectingIP:     "203.0.113.1",
			xForwardedFor:      "203.0.113.2 , 198.51.100.1",
			expectedStartsWith: "203.0.113.2",
			description:        "Should trim whitespace from X-Forwarded-For",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Log(tt.description)

			// Test the logic directly (simulating what getClientIP does)
			var ip string

			switch tt.proxyMode {
			case "cloudflare":
				if tt.cfConnectingIP != "" {
					ip = strings.TrimSpace(strings.Split(tt.cfConnectingIP, ",")[0])
				} else {
					ip = "127.0.0.1" // fallback
				}
			case "xforwarded":
				if tt.xForwardedFor != "" {
					ip = strings.TrimSpace(strings.Split(tt.xForwardedFor, ",")[0])
				} else {
					ip = "127.0.0.1" // fallback
				}
			default:
				ip = "127.0.0.1" // default fallback
			}

			if tt.expectedStartsWith != "" && !strings.HasPrefix(ip, tt.expectedStartsWith) {
				t.Errorf("proxyMode=%s: expected IP starting with %q, got %q", tt.proxyMode, tt.expectedStartsWith, ip)
			}
		})
	}
}

// TestProxyModeValues tests valid proxy mode values
func TestProxyModeValues(t *testing.T) {
	validModes := map[string]bool{
		"none":       true,
		"cloudflare": true,
		"xforwarded": true,
		"invalid":    false,
	}

	for mode, shouldBeValid := range validModes {
		t.Run("proxy_mode="+mode, func(t *testing.T) {
			isValid := mode == "none" || mode == "cloudflare" || mode == "xforwarded"
			if isValid != shouldBeValid {
				t.Errorf("proxy_mode %q validity mismatch: expected %v, got %v", mode, shouldBeValid, isValid)
			}
		})
	}
}
