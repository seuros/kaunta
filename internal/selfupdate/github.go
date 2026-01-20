// Package selfupdate provides self-updating functionality for ore CLI.
// It fetches releases from GitHub and uses contriboss/go-update to apply them.
package selfupdate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/blang/semver"
)

const (
	githubAPIURL = "https://api.github.com"
	userAgent    = "kaunta-selfupdate"
)

// Release represents a GitHub release with its assets.
type Release struct {
	TagName    string    `json:"tag_name"`
	Name       string    `json:"name"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	CreatedAt  time.Time `json:"created_at"`
	Assets     []Asset   `json:"assets"`
	Version    semver.Version
}

// Asset represents a release asset (downloadable file).
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	ContentType        string `json:"content_type"`
}

// Client handles GitHub API requests for release detection.
type Client struct {
	httpClient *http.Client
	owner      string
	repo       string
}

// NewClient creates a new GitHub client for the specified repository.
func NewClient(owner, repo string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		owner:      owner,
		repo:       repo,
	}
}

// DetectLatest fetches the latest non-draft, non-prerelease version from GitHub.
func (c *Client) DetectLatest() (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", githubAPIURL, c.owner, c.repo)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no releases found for %s/%s", c.owner, c.repo)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API error: %s", resp.Status)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Parse version from tag
	versionStr := strings.TrimPrefix(release.TagName, "v")
	version, err := semver.Parse(versionStr)
	if err != nil {
		return nil, fmt.Errorf("invalid version tag %q: %w", release.TagName, err)
	}
	release.Version = version

	return &release, nil
}

// FindAsset finds the appropriate asset for the current platform.
// Asset naming convention: kaunta_{os}_{arch}.tar.gz (with ore_* fallbacks)
func (r *Release) FindAsset() (*Asset, error) {
	expectedName := fmt.Sprintf("kaunta_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)

	for i := range r.Assets {
		if r.Assets[i].Name == expectedName {
			return &r.Assets[i], nil
		}
	}

	// Try alternative naming patterns (prefer kaunta*, then fallback to ore*)
	alternatives := []string{
		// kaunta primary variants
		fmt.Sprintf("kaunta-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH),
		fmt.Sprintf("kaunta_v%s_%s_%s.tar.gz", r.Version.String(), runtime.GOOS, runtime.GOARCH),

		// ore fallbacks (for compatibility with ore-light naming)
		fmt.Sprintf("ore_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH),
		fmt.Sprintf("ore-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH),
		fmt.Sprintf("ore_v%s_%s_%s.tar.gz", r.Version.String(), runtime.GOOS, runtime.GOARCH),
	}

	for _, alt := range alternatives {
		for i := range r.Assets {
			if r.Assets[i].Name == alt {
				return &r.Assets[i], nil
			}
		}
	}

	return nil, fmt.Errorf("no asset found for %s/%s (expected %s)", runtime.GOOS, runtime.GOARCH, expectedName)
}
