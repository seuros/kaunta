package cli

import (
	"encoding/json"
	"strings"

	"github.com/seuros/kaunta/internal/websites"
)

// The website data-access layer lives in internal/websites (shared with the
// HTTP handlers). These aliases keep the historical cli.* names and signatures
// so existing commands and tests compile unchanged.

// WebsiteDetail is an alias for the shared websites.Detail type.
type WebsiteDetail = websites.Detail

var (
	GetWebsiteByDomain    = websites.GetByDomain
	GetWebsiteByID        = websites.GetByID
	ListWebsites          = websites.List
	CreateWebsite         = websites.Create
	UpdateWebsite         = websites.Update
	DeleteWebsite         = websites.Delete
	AddAllowedDomains     = websites.AddAllowedDomains
	RemoveAllowedDomain   = websites.RemoveAllowedDomain
	GetAllowedDomains     = websites.GetAllowedDomains
	SetPublicStatsEnabled = websites.SetPublicStatsEnabled
	validateDomain        = websites.ValidateDomain
)

// ParseAllowedDomains parses a comma-separated string of allowed domains.
func ParseAllowedDomains(csvString string) []string {
	if csvString == "" {
		return []string{}
	}

	domains := strings.Split(csvString, ",")
	var result []string
	for _, d := range domains {
		trimmed := strings.TrimSpace(d)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// AllowedDomainsToJSON converts []string to JSON array string.
func AllowedDomainsToJSON(domains []string) string {
	if len(domains) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(domains)
	return string(data)
}
