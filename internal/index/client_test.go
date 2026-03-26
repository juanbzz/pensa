package index

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	json "github.com/goccy/go-json"
	"github.com/matryer/is"

	"github.com/juanbzz/pensa/pkg/version"
)

func TestPyPIClient_GetPackageInfo(t *testing.T) {
	simpleResp := simpleJSON{
		Files: []simpleFileJSON{
			{
				Filename:       "requests-2.31.0-py3-none-any.whl",
				URL:            "https://files.example.com/requests-2.31.0-py3-none-any.whl",
				Hashes:         map[string]string{"sha256": "abc123"},
				RequiresPython: strPtr(">=3.7"),
				CoreMetadata:   true,
				Yanked:         false,
			},
			{
				Filename:       "requests-2.31.0.tar.gz",
				URL:            "https://files.example.com/requests-2.31.0.tar.gz",
				Hashes:         map[string]string{"sha256": "def456"},
				RequiresPython: strPtr(">=3.7"),
				CoreMetadata:   false,
				Yanked:         false,
			},
			{
				Filename:       "requests-2.30.0-py3-none-any.whl",
				URL:            "https://files.example.com/requests-2.30.0-py3-none-any.whl",
				Hashes:         map[string]string{"sha256": "ghi789"},
				RequiresPython: strPtr(">=3.7"),
				CoreMetadata:   false,
				Yanked:         "security issue",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.pypi.simple.v1+json")
		json.NewEncoder(w).Encode(simpleResp)
	}))
	defer server.Close()

	client := NewPyPIClient(
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)

	info, err := client.GetPackageInfo("requests")
	if err != nil {
		t.Fatal(err)
	}

	if info.Name != "requests" {
		t.Errorf("Name = %q", info.Name)
	}
	if len(info.Files) != 3 {
		t.Fatalf("Files count = %d, want 3", len(info.Files))
	}

	// Check first file.
	f := info.Files[0]
	if f.Filename != "requests-2.31.0-py3-none-any.whl" {
		t.Errorf("Filename = %q", f.Filename)
	}
	if !f.CoreMetadata {
		t.Error("expected CoreMetadata=true")
	}
	if f.RequiresPython != ">=3.7" {
		t.Errorf("RequiresPython = %q", f.RequiresPython)
	}

	// Check yanked file.
	yanked := info.Files[2]
	if !yanked.Yanked {
		t.Error("expected Yanked=true")
	}
	if yanked.YankedReason != "security issue" {
		t.Errorf("YankedReason = %q", yanked.YankedReason)
	}
}

func TestPyPIClient_GetPackageInfo_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewPyPIClient(
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)

	_, err := client.GetPackageInfo("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent package")
	}
}

func TestPyPIClient_GetVersionDetail_JSONAPI(t *testing.T) {
	// Simple API returns no wheel with core-metadata.
	simpleResp := simpleJSON{
		Files: []simpleFileJSON{
			{
				Filename:     "mypkg-1.0.0.tar.gz",
				URL:          "https://files.example.com/mypkg-1.0.0.tar.gz",
				Hashes:       map[string]string{"sha256": "abc"},
				CoreMetadata: false,
			},
		},
	}

	jsonAPIResp := jsonAPIResponse{}
	jsonAPIResp.Info.Name = "mypkg"
	jsonAPIResp.Info.Version = "1.0.0"
	jsonAPIResp.Info.RequiresPython = ">=3.8"
	jsonAPIResp.Info.RequiresDist = []string{
		"requests>=2.0",
		"flask>=2.0",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/simple/mypkg/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.pypi.simple.v1+json")
		json.NewEncoder(w).Encode(simpleResp)
	})
	mux.HandleFunc("/pypi/mypkg/1.0.0/json", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jsonAPIResp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := &PyPIClient{
		baseURL:    server.URL + "/simple/",
		jsonAPIURL: server.URL + "/pypi/",
		httpClient: server.Client(),
	}

	ver, _ := version.Parse("1.0.0")
	detail, err := client.GetVersionDetail("mypkg", ver)
	if err != nil {
		t.Fatal(err)
	}

	if detail.Name != "mypkg" {
		t.Errorf("Name = %q", detail.Name)
	}
	if detail.Version.String() != "1.0.0" {
		t.Errorf("Version = %q", detail.Version.String())
	}
	if detail.RequiresPython != ">=3.8" {
		t.Errorf("RequiresPython = %q", detail.RequiresPython)
	}
	if len(detail.Dependencies) != 2 {
		t.Fatalf("Dependencies count = %d, want 2", len(detail.Dependencies))
	}
	if detail.Dependencies[0].Name != "requests" {
		t.Errorf("first dep = %q", detail.Dependencies[0].Name)
	}
}

func TestPyPIClient_WithCache(t *testing.T) {
	callCount := 0
	simpleResp := simpleJSON{
		Files: []simpleFileJSON{
			{
				Filename: "pkg-1.0.0.tar.gz",
				URL:      "https://example.com/pkg-1.0.0.tar.gz",
				Hashes:   map[string]string{"sha256": "abc"},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/vnd.pypi.simple.v1+json")
		json.NewEncoder(w).Encode(simpleResp)
	}))
	defer server.Close()

	cache := NewCache(t.TempDir())
	client := NewPyPIClient(
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
		WithCache(cache),
	)

	// First call should hit the server.
	_, err := client.GetPackageInfo("pkg")
	if err != nil {
		t.Fatal(err)
	}
	if callCount != 1 {
		t.Errorf("call count = %d, want 1", callCount)
	}

	// Second call should use cache.
	_, err = client.GetPackageInfo("pkg")
	if err != nil {
		t.Fatal(err)
	}
	if callCount != 1 {
		t.Errorf("call count = %d after cache hit, want 1", callCount)
	}
}

func strPtr(s string) *string {
	return &s
}

func TestPyPIClient_GetPackageInfo_FreshCache(t *testing.T) {
	assert := is.New(t)

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		t.Error("server should not be called when cache is fresh")
	}))
	defer server.Close()

	cache := NewCache(t.TempDir())
	simpleResp := simpleJSON{
		Files: []simpleFileJSON{
			{Filename: "pkg-1.0.0.tar.gz", URL: "https://example.com/pkg-1.0.0.tar.gz"},
		},
	}
	data, _ := json.Marshal(simpleResp)

	// Pre-populate cache with fresh metadata.
	_ = cache.PutWithMeta("simple/pkg.json", data, CacheMeta{
		ETag:   `"fresh-etag"`,
		Date:   timeNowUnix(),
		MaxAge: 600,
	})

	client := NewPyPIClient(
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
		WithCache(cache),
	)

	info, err := client.GetPackageInfo("pkg")
	assert.NoErr(err)
	assert.Equal(info.Name, "pkg")
	assert.Equal(hits.Load(), int32(0))
}

func TestPyPIClient_GetPackageInfo_304(t *testing.T) {
	assert := is.New(t)

	var fullFetches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"test-etag"` {
			w.Header().Set("Cache-Control", "public, max-age=600")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		fullFetches.Add(1)
		w.Header().Set("Content-Type", "application/vnd.pypi.simple.v1+json")
		json.NewEncoder(w).Encode(simpleJSON{})
	}))
	defer server.Close()

	cache := NewCache(t.TempDir())
	data, _ := json.Marshal(simpleJSON{
		Files: []simpleFileJSON{
			{Filename: "pkg-1.0.0.tar.gz", URL: "https://example.com/pkg-1.0.0.tar.gz"},
		},
	})

	// Pre-populate cache with stale metadata (expired).
	_ = cache.PutWithMeta("simple/pkg.json", data, CacheMeta{
		ETag:   `"test-etag"`,
		Date:   timeNowUnix() - 700, // expired
		MaxAge: 600,
	})

	client := NewPyPIClient(
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
		WithCache(cache),
	)

	info, err := client.GetPackageInfo("pkg")
	assert.NoErr(err)
	assert.Equal(info.Name, "pkg")
	assert.Equal(fullFetches.Load(), int32(0)) // no full fetch — 304 reused cache

	// Meta should be updated with fresh timestamp.
	meta := cache.GetMeta("simple/pkg.json")
	assert.True(meta != nil)
	assert.True(meta.Fresh())
}

func TestPyPIClient_GetPackageInfo_200AfterStale(t *testing.T) {
	assert := is.New(t)

	newResp := simpleJSON{
		Files: []simpleFileJSON{
			{Filename: "pkg-2.0.0.tar.gz", URL: "https://example.com/pkg-2.0.0.tar.gz"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ETag changed — return full new response.
		w.Header().Set("Content-Type", "application/vnd.pypi.simple.v1+json")
		w.Header().Set("ETag", `"new-etag"`)
		w.Header().Set("Cache-Control", "public, max-age=600")
		json.NewEncoder(w).Encode(newResp)
	}))
	defer server.Close()

	cache := NewCache(t.TempDir())
	oldData, _ := json.Marshal(simpleJSON{
		Files: []simpleFileJSON{
			{Filename: "pkg-1.0.0.tar.gz", URL: "https://example.com/pkg-1.0.0.tar.gz"},
		},
	})

	_ = cache.PutWithMeta("simple/pkg.json", oldData, CacheMeta{
		ETag:   `"old-etag"`,
		Date:   timeNowUnix() - 700,
		MaxAge: 600,
	})

	client := NewPyPIClient(
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
		WithCache(cache),
	)

	info, err := client.GetPackageInfo("pkg")
	assert.NoErr(err)
	assert.Equal(info.Name, "pkg")
	assert.Equal(len(info.Files), 1)
	assert.Equal(info.Files[0].Filename, "pkg-2.0.0.tar.gz") // got new data

	meta := cache.GetMeta("simple/pkg.json")
	assert.True(meta != nil)
	assert.Equal(meta.ETag, `"new-etag"`)
}

func timeNowUnix() int64 {
	return time.Now().Unix()
}
