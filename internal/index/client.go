package index

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	json "github.com/goccy/go-json"

	"github.com/juanbzz/pensa/pkg/pep508"
	"github.com/juanbzz/pensa/pkg/version"
)

// Compile-time interface check.
var _ Repository = (*PyPIClient)(nil)

// Option configures a PyPIClient.
type Option func(*PyPIClient)

// WithBaseURL sets the Simple API base URL.
func WithBaseURL(url string) Option {
	return func(c *PyPIClient) {
		c.baseURL = strings.TrimRight(url, "/") + "/"
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *PyPIClient) {
		c.httpClient = client
	}
}

// WithCache sets the on-disk cache.
func WithCache(cache *Cache) Option {
	return func(c *PyPIClient) {
		c.cache = cache
	}
}

// PyPIClient fetches package metadata from a PEP 503/691 Simple Repository.
type PyPIClient struct {
	baseURL    string
	jsonAPIURL string // for fallback JSON API (e.g. https://pypi.org/pypi/)
	httpClient *http.Client
	cache      *Cache
}

// NewPyPIClient creates a new client with the given options.
func NewPyPIClient(opts ...Option) *PyPIClient {
	c := &PyPIClient{
		baseURL:    "https://pypi.org/simple/",
		jsonAPIURL: "https://pypi.org/pypi/",
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// HTTPClient returns the underlying HTTP client for direct use.
func (c *PyPIClient) HTTPClient() *http.Client { return c.httpClient }

// GetPackageInfo fetches the Simple API listing for a package.
// Uses HTTP conditional requests (ETag/304) to avoid re-downloading unchanged data.
func (c *PyPIClient) GetPackageInfo(name string) (*PackageInfo, error) {
	normalized := pep508.NormalizeName(name)
	cacheKey := "simple/" + normalized + ".json"

	if c.cache != nil {
		if data, err := c.cache.Get(cacheKey); err == nil && data != nil {
			meta := c.cache.GetMeta(cacheKey)

			// Fresh cache — skip network entirely.
			if meta != nil && meta.Fresh() {
				return c.parseSimpleJSON(normalized, data)
			}

			// Stale cache with ETag — try conditional request.
			if meta != nil && meta.ETag != "" {
				info, err := c.conditionalFetch(normalized, cacheKey, data, meta)
				if err == nil {
					return info, nil
				}
				// Fall through to full fetch on error.
			}

			// Stale cache without metadata — use cached data but refresh in background.
			// For now, just use cached data to avoid blocking.
			return c.parseSimpleJSON(normalized, data)
		}
	}

	return c.fullFetchPackageInfo(normalized, cacheKey)
}

// conditionalFetch sends an If-None-Match request. On 304, returns cached data.
// On 200, updates cache and returns new data.
func (c *PyPIClient) conditionalFetch(name, cacheKey string, cachedData []byte, meta *CacheMeta) (*PackageInfo, error) {
	url := c.baseURL + name + "/"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.pypi.simple.v1+json")
	req.Header.Set("If-None-Match", meta.ETag)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		// Data unchanged — update freshness timestamp and reuse cached data.
		newMeta := parseCacheMeta(resp)
		newMeta.ETag = meta.ETag // preserve ETag (304 may not echo it)
		_ = c.cache.UpdateMeta(cacheKey, newMeta)
		return c.parseSimpleJSON(name, cachedData)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("index: unexpected status %d for %s", resp.StatusCode, url)
	}

	// Full new response — cache it.
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("index: read response: %w", err)
	}
	_ = c.cache.PutWithMeta(cacheKey, data, parseCacheMeta(resp))
	return c.parseSimpleJSON(name, data)
}

// fullFetchPackageInfo fetches the Simple API with no cache.
func (c *PyPIClient) fullFetchPackageInfo(name, cacheKey string) (*PackageInfo, error) {
	url := c.baseURL + name + "/"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("index: create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.pypi.simple.v1+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("index: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("index: package %q not found", name)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("index: unexpected status %d for %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("index: read response: %w", err)
	}

	if c.cache != nil {
		_ = c.cache.PutWithMeta(cacheKey, data, parseCacheMeta(resp))
	}

	return c.parseSimpleJSON(name, data)
}

// parseCacheMeta extracts HTTP cache policy from response headers.
func parseCacheMeta(resp *http.Response) CacheMeta {
	meta := CacheMeta{
		ETag: resp.Header.Get("ETag"),
		Date: time.Now().Unix(),
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "" {
		for _, directive := range strings.Split(cc, ",") {
			d := strings.TrimSpace(directive)
			if strings.HasPrefix(d, "max-age=") {
				if v, err := strconv.Atoi(d[8:]); err == nil {
					meta.MaxAge = v
				}
			}
		}
	}
	return meta
}

// simpleJSON matches the PEP 691 JSON response format.
type simpleJSON struct {
	Files []simpleFileJSON `json:"files"`
}

type simpleFileJSON struct {
	Filename       string            `json:"filename"`
	URL            string            `json:"url"`
	Hashes         map[string]string `json:"hashes"`
	RequiresPython *string           `json:"requires-python"`
	CoreMetadata   interface{}       `json:"core-metadata"`
	Yanked         interface{}       `json:"yanked"`
}

func (c *PyPIClient) parseSimpleJSON(name string, data []byte) (*PackageInfo, error) {
	var resp simpleJSON
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("index: parse simple JSON for %s: %w", name, err)
	}

	info := &PackageInfo{Name: name}
	for _, f := range resp.Files {
		fi := FileInfo{
			Filename: f.Filename,
			URL:      f.URL,
			Hashes:   f.Hashes,
		}

		if f.RequiresPython != nil {
			fi.RequiresPython = *f.RequiresPython
		}

		// core-metadata can be bool or dict.
		switch cm := f.CoreMetadata.(type) {
		case bool:
			fi.CoreMetadata = cm
		case map[string]interface{}:
			fi.CoreMetadata = len(cm) > 0
		}

		// yanked can be bool or string.
		switch y := f.Yanked.(type) {
		case bool:
			fi.Yanked = y
		case string:
			fi.Yanked = true
			fi.YankedReason = y
		}

		info.Files = append(info.Files, fi)
	}

	return info, nil
}

// GetVersionDetail fetches dependency metadata for a specific version.
func (c *PyPIClient) GetVersionDetail(name string, ver version.Version) (*VersionDetail, error) {
	normalized := pep508.NormalizeName(name)
	cacheKey := "metadata/" + normalized + "/" + ver.String() + ".json"

	// Check cache.
	if c.cache != nil {
		if data, err := c.cache.Get(cacheKey); err == nil && data != nil {
			var detail VersionDetail
			if err := json.Unmarshal(data, &detail); err == nil {
				return &detail, nil
			}
		}
	}

	// Try PEP 658 metadata first.
	detail, err := c.fetchPEP658Metadata(normalized, ver)
	if err == nil && detail != nil {
		c.cacheVersionDetail(cacheKey, detail)
		return detail, nil
	}

	// Fall back to JSON API.
	detail, err = c.FetchJSONAPI(normalized, ver)
	if err != nil {
		return nil, err
	}

	c.cacheVersionDetail(cacheKey, detail)
	return detail, nil
}

func (c *PyPIClient) fetchPEP658Metadata(name string, ver version.Version) (*VersionDetail, error) {
	// First get the package info to find a wheel with core-metadata.
	info, err := c.GetPackageInfo(name)
	if err != nil {
		return nil, err
	}

	wheel := info.BestWheel(ver)
	if wheel == nil || !wheel.CoreMetadata {
		return nil, fmt.Errorf("no PEP 658 metadata available")
	}

	metadataURL := wheel.URL + ".metadata"
	resp, err := c.httpClient.Get(metadataURL)
	if err != nil {
		return nil, fmt.Errorf("index: fetch metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("index: metadata status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("index: read metadata: %w", err)
	}

	return ParseMetadata(data)
}

// FetchPEP658Metadata fetches PEP 658 metadata from a .metadata URL.
func (c *PyPIClient) FetchPEP658Metadata(metadataURL string) (*VersionDetail, error) {
	resp, err := c.httpClient.Get(metadataURL)
	if err != nil {
		return nil, fmt.Errorf("index: fetch metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("index: metadata status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("index: read metadata: %w", err)
	}

	return ParseMetadata(data)
}

// jsonAPIResponse matches the PyPI JSON API response.
type jsonAPIResponse struct {
	Info struct {
		Name           string   `json:"name"`
		Version        string   `json:"version"`
		RequiresPython string   `json:"requires_python"`
		RequiresDist   []string `json:"requires_dist"`
	} `json:"info"`
}

// FetchJSONAPI fetches version metadata from the PyPI JSON API.
func (c *PyPIClient) FetchJSONAPI(name string, ver version.Version) (*VersionDetail, error) {
	url := c.jsonAPIURL + name + "/" + ver.String() + "/json"
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("index: fetch JSON API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("index: JSON API status %d for %s/%s", resp.StatusCode, name, ver)
	}

	var apiResp jsonAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("index: decode JSON API: %w", err)
	}

	detail := &VersionDetail{
		Name:           pep508.NormalizeName(apiResp.Info.Name),
		RequiresPython: apiResp.Info.RequiresPython,
	}

	v, err := version.Parse(apiResp.Info.Version)
	if err != nil {
		return nil, fmt.Errorf("index: parse version %q: %w", apiResp.Info.Version, err)
	}
	detail.Version = v

	for _, reqStr := range apiResp.Info.RequiresDist {
		dep, err := pep508.Parse(reqStr)
		if err != nil {
			continue // skip unparseable deps
		}
		detail.Dependencies = append(detail.Dependencies, dep)
	}

	return detail, nil
}

func (c *PyPIClient) cacheVersionDetail(key string, detail *VersionDetail) {
	if c.cache == nil {
		return
	}
	data, err := json.Marshal(detail)
	if err != nil {
		return
	}
	_ = c.cache.Put(key, data)
}
