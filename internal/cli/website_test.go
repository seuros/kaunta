package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stubAddAllowedDomains(t *testing.T, fn func(ctx context.Context, websiteDomain string, domains []string) (*WebsiteDetail, error)) {
	original := addAllowedDomainsFunc
	addAllowedDomainsFunc = fn
	t.Cleanup(func() {
		addAllowedDomainsFunc = original
	})
}

func stubRemoveAllowedDomain(t *testing.T, fn func(ctx context.Context, websiteDomain, domainToRemove string) (*WebsiteDetail, error)) {
	original := removeAllowedDomainFn
	removeAllowedDomainFn = fn
	t.Cleanup(func() {
		removeAllowedDomainFn = original
	})
}

func TestParseAllowedDomains(t *testing.T) {
	assert.Empty(t, ParseAllowedDomains(""))
	assert.Equal(t, []string{"example.com"}, ParseAllowedDomains("example.com"))
	assert.Equal(
		t,
		[]string{"a.com", "b.com", "c.com"},
		ParseAllowedDomains(" a.com, b.com , , c.com "),
	)
}

func TestAllowedDomainsToJSON(t *testing.T) {
	assert.Equal(t, "[]", AllowedDomainsToJSON(nil))
	assert.Equal(t, `["example.com"]`, AllowedDomainsToJSON([]string{"example.com"}))
}

func TestValidateDomain(t *testing.T) {
	testCases := []struct {
		name      string
		domain    string
		shouldErr bool
	}{
		{"empty", "", true},
		{"too-long", strings.Repeat("a", 254), true},
		{"invalid chars", "exa$mple.com", true},
		{"localhost", "localhost", false},
		{"valid", "analytics.example.com", false},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDomain(tt.domain)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRunWebsiteTrackingCodeFormats(t *testing.T) {
	stubDB(t)
	stubConnectClose(t)

	website := &WebsiteDetail{
		WebsiteID: "site-123",
	}

	originalFetcher := fetchWebsiteByDomain
	fetchWebsiteByDomain = func(ctx context.Context, domain string, websiteID *string) (*WebsiteDetail, error) {
		assert.Equal(t, "example.com", domain)
		return website, nil
	}
	defer func() { fetchWebsiteByDomain = originalFetcher }()

	output, err := captureOutput(t, func() error {
		return runWebsiteTrackingCode("example.com")
	})
	require.NoError(t, err)
	assert.Contains(t, output, `<script async src="/k.js" data-website-id="site-123"></script>`)
}

func TestRunListDomainsFormats(t *testing.T) {
	stubDB(t)
	stubConnectClose(t)

	detail := &WebsiteDetail{
		Domain:         "example.com",
		AllowedDomains: []string{"a.com", "b.com"},
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	originalGetter := getAllowedDomainsFunc
	getAllowedDomainsFunc = func(ctx context.Context, websiteDomain string) ([]string, *WebsiteDetail, error) {
		assert.Equal(t, "example.com", websiteDomain)
		return detail.AllowedDomains, detail, nil
	}
	defer func() { getAllowedDomainsFunc = originalGetter }()

	t.Run("text", func(t *testing.T) {
		output, err := captureOutput(t, func() error {
			return runListDomains("example.com", "text")
		})
		require.NoError(t, err)
		assert.Contains(t, output, "a.com")
		assert.Contains(t, output, "b.com")
	})

	t.Run("json", func(t *testing.T) {
		output, err := captureOutput(t, func() error {
			return runListDomains("example.com", "json")
		})
		require.NoError(t, err)
		assert.Contains(t, output, "[")
		assert.Contains(t, output, "a.com")
	})

	t.Run("table", func(t *testing.T) {
		output, err := captureOutput(t, func() error {
			return runListDomains("example.com", "table")
		})
		require.NoError(t, err)
		assert.Contains(t, output, "DOMAIN")
		assert.Contains(t, output, "1")
	})
}

func TestRunListDomainsInvalidFormat(t *testing.T) {
	stubDB(t)
	stubConnectClose(t)

	originalGetter := getAllowedDomainsFunc
	getAllowedDomainsFunc = func(ctx context.Context, websiteDomain string) ([]string, *WebsiteDetail, error) {
		return []string{"a.com"}, &WebsiteDetail{Domain: websiteDomain}, nil
	}
	defer func() { getAllowedDomainsFunc = originalGetter }()

	_, err := captureOutput(t, func() error {
		return runListDomains("example.com", "yaml")
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid format")
}

func TestRunAddDomainSuccess(t *testing.T) {
	stubDB(t)
	stubConnectClose(t)

	stubAddAllowedDomains(t, func(ctx context.Context, websiteDomain string, domains []string) (*WebsiteDetail, error) {
		assert.Equal(t, "example.com", websiteDomain)
		assert.Equal(t, []string{"allow.com", "extra.com"}, domains)
		return &WebsiteDetail{
			Domain:         "example.com",
			AllowedDomains: []string{"allow.com", "extra.com"},
		}, nil
	})

	output, err := captureOutput(t, func() error {
		return runAddDomain("example.com", "allow.com", "extra.com")
	})
	require.NoError(t, err)
	assert.Contains(t, output, "Allowed domains updated successfully")
	assert.Contains(t, output, "Total allowed domains: 2")
}

func TestRunAddDomainValidationError(t *testing.T) {
	stubDB(t)
	stubConnectClose(t)

	_, err := captureOutput(t, func() error {
		return runAddDomain("example.com", "invalid domain", "")
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid domain format")
}

func TestRunRemoveDomainSuccess(t *testing.T) {
	stubDB(t)
	stubConnectClose(t)

	stubRemoveAllowedDomain(t, func(ctx context.Context, websiteDomain, domainToRemove string) (*WebsiteDetail, error) {
		assert.Equal(t, "example.com", websiteDomain)
		assert.Equal(t, "allow.com", domainToRemove)
		return &WebsiteDetail{
			Domain:         "example.com",
			AllowedDomains: []string{"remaining.com"},
		}, nil
	})

	output, err := captureOutput(t, func() error {
		return runRemoveDomain("example.com", "allow.com")
	})
	require.NoError(t, err)
	assert.Contains(t, output, "Domain removed successfully")
	assert.Contains(t, output, "Remaining allowed domains: 1")
}

func TestRunRemoveDomainError(t *testing.T) {
	stubDB(t)
	stubConnectClose(t)

	stubRemoveAllowedDomain(t, func(ctx context.Context, websiteDomain, domainToRemove string) (*WebsiteDetail, error) {
		return nil, errors.New("no such domain")
	})

	_, err := captureOutput(t, func() error {
		return runRemoveDomain("example.com", "missing.com")
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such domain")
}

func sampleWebsite() *WebsiteDetail {
	share := "public"
	return &WebsiteDetail{
		WebsiteID:      "site-123",
		Domain:         "example.com",
		Name:           "Example",
		AllowedDomains: []string{"a.com", "b.com"},
		ShareID:        &share,
		CreatedAt:      time.Unix(0, 0),
		UpdatedAt:      time.Unix(0, 0),
	}
}

func TestOutputJSONHelpers(t *testing.T) {
	site := sampleWebsite()

	output, err := captureOutput(t, func() error {
		return outputJSON([]*WebsiteDetail{site})
	})
	require.NoError(t, err)
	assert.Contains(t, output, `"domain": "example.com"`)

	output, err = captureOutput(t, func() error {
		return outputSingleJSON(site)
	})
	require.NoError(t, err)
	assert.Contains(t, output, `"website_id": "site-123"`)
}

func TestOutputCSV(t *testing.T) {
	site := sampleWebsite()
	output, err := captureOutput(t, func() error {
		return outputCSV([]*WebsiteDetail{site})
	})
	require.NoError(t, err)
	assert.Contains(t, output, "domain,name,website_id,created_at")
	assert.Contains(t, output, "example.com,Example,site-123")
}

func TestOutputTables(t *testing.T) {
	site := sampleWebsite()

	tableOutput, err := captureOutput(t, func() error {
		return outputTable([]*WebsiteDetail{site})
	})
	require.NoError(t, err)
	assert.Contains(t, tableOutput, "DOMAIN")
	assert.Contains(t, tableOutput, "example.com")

	singleOutput, err := captureOutput(t, func() error {
		return outputSingleTable(site)
	})
	require.NoError(t, err)
	assert.Contains(t, singleOutput, "Domain:")
	assert.Contains(t, singleOutput, "Allowed Domains:")
	assert.Contains(t, singleOutput, "a.com, b.com")
}
