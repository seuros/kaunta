package config

import (
	"fmt"
	"net/url"
	"strings"
)

// SanitizeTrustedDomain validates and normalizes a trusted domain value.
// It returns the host (optionally with port) in lowercase, with any scheme removed.
// Paths, queries, fragments, wildcards, and empty values are rejected.
func SanitizeTrustedDomain(raw string) (string, error) {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return "", fmt.Errorf("domain cannot be empty")
	}

	cleaned = strings.ToLower(cleaned)

	// Remove optional scheme prefix
	cleaned = strings.TrimPrefix(cleaned, "http://")
	cleaned = strings.TrimPrefix(cleaned, "https://")

	// Remove a single trailing slash (root path)
	cleaned = strings.TrimSuffix(cleaned, "/")

	if strings.ContainsAny(cleaned, " \t\r\n") {
		return "", fmt.Errorf("domain cannot contain whitespace")
	}
	if strings.Contains(cleaned, "*") {
		return "", fmt.Errorf("wildcards are not allowed in trusted origins")
	}

	// Use url.Parse to validate host[:port] without allowing paths or queries.
	u, err := url.Parse("http://" + cleaned)
	if err != nil {
		return "", fmt.Errorf("invalid domain format")
	}

	if u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("domain must not include path, query, or fragment")
	}

	return u.Host, nil
}
