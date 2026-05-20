package bakery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/castrojo/knuckle/internal/model"
)

const (
	// DefaultCatalogURL is the GitHub Releases API for the sysext-bakery repo
	DefaultCatalogURL = "https://api.github.com/repos/flatcar/sysext-bakery/releases?per_page=100"
	defaultTimeout    = 30 * time.Second
)

// Client is the interface for fetching the sysext catalog
type Client interface {
	FetchCatalog(ctx context.Context) ([]model.SysextEntry, error)
	// FetchCatalogArch fetches the catalog and selects assets for the given arch.
	// arch must be "amd64" or "arm64". Falls back to x86-64 assets for amd64.
	FetchCatalogArch(ctx context.Context, arch string) ([]model.SysextEntry, error)
}

// HTTPClient fetches the catalog from the GitHub Releases API
type HTTPClient struct {
	CatalogURL string
	HTTP       *http.Client
}

// NewHTTPClient creates a new bakery HTTP client
func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		CatalogURL: DefaultCatalogURL,
		HTTP: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// NewHTTPClientWithURL creates a client pointing at a custom catalog URL (for testing)
func NewHTTPClientWithURL(url string) *HTTPClient {
	return &HTTPClient{
		CatalogURL: url,
		HTTP: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// githubRelease represents a single release from the GitHub Releases API.
type githubRelease struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func (c *HTTPClient) FetchCatalog(ctx context.Context) ([]model.SysextEntry, error) {
	return c.FetchCatalogArch(ctx, "amd64")
}

// FetchCatalogArch fetches the catalog and selects assets for the given arch.
// Flatcar Bakery asset naming: "<name>-<ver>-x86-64.raw" (amd64) and "<name>-<ver>-arm64.raw" (arm64).
func (c *HTTPClient) FetchCatalogArch(ctx context.Context, arch string) ([]model.SysextEntry, error) {
	// Map Go arch name to the suffix used in Flatcar Bakery asset filenames.
	var assetSuffix string
	switch arch {
	case "amd64":
		assetSuffix = "x86-64"
	case "arm64":
		assetSuffix = "arm64"
	default:
		return nil, fmt.Errorf("unsupported architecture %q: must be amd64 or arm64", arch)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.CatalogURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "knuckle/1.0")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching catalog: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog returned status %d", resp.StatusCode)
	}

	// Limit response body to 5MB to prevent OOM from malicious/broken responses.
	// If we read exactly maxResponseSize bytes, the response was truncated.
	const maxResponseSize = 5 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if int64(len(body)) >= maxResponseSize {
		return nil, fmt.Errorf("catalog response exceeds 5MB size limit")
	}

	var releases []githubRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, fmt.Errorf("parsing catalog JSON: %w", err)
	}

	seen := make(map[string]bool)
	sysexts := make([]model.SysextEntry, 0, len(releases))

	for _, rel := range releases {
		name, version := ParseTagName(rel.TagName)
		if name == "" {
			continue
		}

		// Deduplicate by name — keep first (latest) since API returns newest first
		if seen[name] {
			continue
		}

		// Find the asset matching the requested arch suffix
		var downloadURL string
		for _, asset := range rel.Assets {
			if strings.Contains(asset.Name, assetSuffix) && strings.HasSuffix(asset.Name, ".raw") {
				downloadURL = asset.BrowserDownloadURL
				break
			}
		}
		if downloadURL == "" {
			continue
		}

		seen[name] = true

		desc := truncateDescription(rel.Body, 80)
		sysexts = append(sysexts, model.SysextEntry{
			Name:        name,
			Description: desc,
			Version:     version,
			URL:         downloadURL,
			Selected:    false,
		})
	}

	return sysexts, nil
}

// ParseTagName extracts sysext name and version from a release tag.
// Formats: "<name>-v<version>" or "<name>-<version>"
func ParseTagName(tag string) (name, version string) {
	// Try splitting on "-v" followed by a digit (find last occurrence)
	for i := len(tag) - 1; i >= 1; i-- {
		if tag[i-1] == '-' && tag[i] == 'v' && i+1 < len(tag) && unicode.IsDigit(rune(tag[i+1])) {
			return tag[:i-1], tag[i+1:]
		}
	}

	// Fallback: find first segment that starts with a digit after a '-'
	for i := 1; i < len(tag); i++ {
		if tag[i-1] == '-' && unicode.IsDigit(rune(tag[i])) {
			return tag[:i-1], tag[i:]
		}
	}

	return "", ""
}

// truncateDescription trims and truncates a description to maxLen characters.
func truncateDescription(s string, maxLen int) string {
	// Take only first line
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		s = s[:maxLen-3] + "..."
	}
	return s
}

// MockClient is a test double that returns preconfigured results
type MockClient struct {
	Entries []model.SysextEntry
	Err     error
}

func (m *MockClient) FetchCatalog(ctx context.Context) ([]model.SysextEntry, error) {
	return m.Entries, m.Err
}

func (m *MockClient) FetchCatalogArch(ctx context.Context, arch string) ([]model.SysextEntry, error) {
	return m.Entries, m.Err
}
