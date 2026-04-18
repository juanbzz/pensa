package index

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/matryer/is"

	"pensa.sh/pensa/pkg/version"
)

func TestCachedClient_SkipsPEP658_WhenUnsupported(t *testing.T) {
	is := is.New(t)

	var simpleHits, jsonAPIHits atomic.Int32

	// Simple API: no wheel with core-metadata.
	simpleResp := simpleJSON{
		Files: []simpleFileJSON{
			{
				Filename:     "mypkg-1.0.0.tar.gz",
				URL:          "https://files.example.com/mypkg-1.0.0.tar.gz",
				Hashes:       map[string]string{"sha256": "abc"},
				CoreMetadata: false,
			},
			{
				Filename:     "mypkg-2.0.0.tar.gz",
				URL:          "https://files.example.com/mypkg-2.0.0.tar.gz",
				Hashes:       map[string]string{"sha256": "def"},
				CoreMetadata: false,
			},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/simple/mypkg/", func(w http.ResponseWriter, r *http.Request) {
		simpleHits.Add(1)
		w.Header().Set("Content-Type", "application/vnd.pypi.simple.v1+json")
		json.NewEncoder(w).Encode(simpleResp)
	})
	mux.HandleFunc("/pypi/mypkg/", func(w http.ResponseWriter, r *http.Request) {
		jsonAPIHits.Add(1)
		resp := jsonAPIResponse{}
		resp.Info.Name = "mypkg"
		resp.Info.Version = "1.0.0"
		resp.Info.RequiresDist = []string{"requests>=2.0"}
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := &PyPIClient{
		baseURL:    server.URL + "/simple/",
		jsonAPIURL: server.URL + "/pypi/",
		httpClient: server.Client(),
	}
	cached := NewCachedClient(client, nil)

	// Populate PackageInfo in memory (as the resolver normally does).
	_, err := cached.GetPackageInfo("mypkg")
	is.NoErr(err)
	is.Equal(simpleHits.Load(), int32(1)) // one Simple API call

	// Now fetch version details — should go straight to JSON API, no extra Simple API call.
	v1, _ := version.Parse("1.0.0")
	_, err = cached.GetVersionDetail("mypkg", v1)
	is.NoErr(err)

	v2, _ := version.Parse("2.0.0")
	_, err = cached.GetVersionDetail("mypkg", v2)
	is.NoErr(err)

	// Simple API should NOT have been called again (no PEP 658 probing).
	is.Equal(simpleHits.Load(), int32(1))
	// JSON API should have been called twice (once per version).
	is.Equal(jsonAPIHits.Load(), int32(2))
}

func TestCachedClient_UsesPEP658_WhenAvailable(t *testing.T) {
	is := is.New(t)

	var metadataHits, jsonAPIHits atomic.Int32

	// We need the server URL in the file URL, so create mux first, start server,
	// then build the response referencing server.URL.
	mux := http.NewServeMux()

	server := httptest.NewServer(mux)
	defer server.Close()

	simpleResp := simpleJSON{
		Files: []simpleFileJSON{
			{
				Filename:     "mypkg-1.0.0-py3-none-any.whl",
				URL:          server.URL + "/files/mypkg-1.0.0-py3-none-any.whl",
				Hashes:       map[string]string{"sha256": "abc"},
				CoreMetadata: true,
			},
		},
	}

	mux.HandleFunc("/simple/mypkg/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.pypi.simple.v1+json")
		json.NewEncoder(w).Encode(simpleResp)
	})
	mux.HandleFunc("/files/mypkg-1.0.0-py3-none-any.whl.metadata", func(w http.ResponseWriter, r *http.Request) {
		metadataHits.Add(1)
		w.Write([]byte("Metadata-Version: 2.1\nName: mypkg\nVersion: 1.0.0\nRequires-Dist: flask>=2.0\n"))
	})
	mux.HandleFunc("/pypi/mypkg/", func(w http.ResponseWriter, r *http.Request) {
		jsonAPIHits.Add(1)
		t.Error("JSON API should not be called when PEP 658 is available")
	})

	client := &PyPIClient{
		baseURL:    server.URL + "/simple/",
		jsonAPIURL: server.URL + "/pypi/",
		httpClient: server.Client(),
	}
	cached := NewCachedClient(client, nil)

	// Populate PackageInfo.
	_, err := cached.GetPackageInfo("mypkg")
	is.NoErr(err)

	// Should use PEP 658 metadata URL directly.
	v1, _ := version.Parse("1.0.0")
	detail, err := cached.GetVersionDetail("mypkg", v1)
	is.NoErr(err)
	is.Equal(detail.Name, "mypkg")
	is.Equal(len(detail.Dependencies), 1)
	is.Equal(detail.Dependencies[0].Name, "flask")

	is.Equal(metadataHits.Load(), int32(1))
	is.Equal(jsonAPIHits.Load(), int32(0))
}

func TestCachedClient_FallsBackWithoutPackageInfo(t *testing.T) {
	is := is.New(t)

	var simpleHits atomic.Int32

	// Simple API returns no PEP 658 — will trigger the old full path.
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
	jsonAPIResp.Info.RequiresDist = []string{"requests>=2.0"}

	mux := http.NewServeMux()
	mux.HandleFunc("/simple/mypkg/", func(w http.ResponseWriter, r *http.Request) {
		simpleHits.Add(1)
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
	cached := NewCachedClient(client, nil)

	// DON'T call GetPackageInfo first — simulate the rare edge case.
	v1, _ := version.Parse("1.0.0")
	detail, err := cached.GetVersionDetail("mypkg", v1)
	is.NoErr(err)
	is.Equal(detail.Name, "mypkg")

	// Falls back to old path: fetchPEP658Metadata calls GetPackageInfo internally.
	is.Equal(simpleHits.Load(), int32(1))
}

func TestCachedClient_GetPackageInfo_FromResolutionCache(t *testing.T) {
	is := is.New(t)

	var simpleHits atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/simple/mypkg/", func(w http.ResponseWriter, r *http.Request) {
		simpleHits.Add(1)
		t.Error("Simple API should not be called when resolution cache has versions")
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := &PyPIClient{
		baseURL:    server.URL + "/simple/",
		jsonAPIURL: server.URL + "/pypi/",
		httpClient: server.Client(),
	}

	// Pre-populate resolution cache.
	resCache, _ := NewResolutionCache(t.TempDir())
	resCache.Put(&ResolutionPackage{
		Name:     "mypkg",
		Versions: []string{"2.0.0", "1.0.0"},
		Deps:     map[string]ResolutionEntry{},
		PEP658:   false,
	})

	cached := NewCachedClient(client, resCache)

	// GetPackageInfo should use resolution cache, not hit the server.
	info, err := cached.GetPackageInfo("mypkg")
	is.NoErr(err)
	is.Equal(info.Name, "mypkg")

	versions := info.Versions()
	is.Equal(len(versions), 2)
	is.Equal(simpleHits.Load(), int32(0))
}

func TestCachedClient_GetPackageInfo_FromResolutionCache_PEP658(t *testing.T) {
	is := is.New(t)

	var metadataHits atomic.Int32

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/files/mypkg-1.0.0-py3-none-any.whl.metadata", func(w http.ResponseWriter, r *http.Request) {
		metadataHits.Add(1)
		w.Write([]byte("Metadata-Version: 2.1\nName: mypkg\nVersion: 1.0.0\nRequires-Dist: flask>=2.0\n"))
	})

	client := &PyPIClient{
		baseURL:    server.URL + "/simple/",
		jsonAPIURL: server.URL + "/pypi/",
		httpClient: server.Client(),
	}

	// Pre-populate resolution cache with PEP 658 + wheel URL.
	resCache, _ := NewResolutionCache(t.TempDir())
	resCache.Put(&ResolutionPackage{
		Name:     "mypkg",
		Versions: []string{"1.0.0"},
		Deps:     map[string]ResolutionEntry{},
		PEP658:   true,
		WheelURLs: map[string]string{
			"1.0.0": server.URL + "/files/mypkg-1.0.0-py3-none-any.whl",
		},
	})

	cached := NewCachedClient(client, resCache)

	// GetPackageInfo from resolution cache.
	info, err := cached.GetPackageInfo("mypkg")
	is.NoErr(err)

	// BestWheel should have CoreMetadata + URL from cached data.
	v1, _ := version.Parse("1.0.0")
	wheel := info.BestWheel(v1)
	is.True(wheel != nil)
	is.True(wheel.CoreMetadata)

	// fetchVersionDetail should use PEP 658 via the cached wheel URL.
	detail, err := cached.GetVersionDetail("mypkg", v1)
	is.NoErr(err)
	is.Equal(detail.Name, "mypkg")
	is.Equal(len(detail.Dependencies), 1)
	is.Equal(detail.Dependencies[0].Name, "flask")
	is.Equal(metadataHits.Load(), int32(1))
}

func TestResolutionPackage_ToPackageInfo_RoundTrip(t *testing.T) {
	is := is.New(t)

	// Create a PackageInfo with PEP 658 wheels.
	original := &PackageInfo{
		Name: "my-pkg",
		Files: []FileInfo{
			{
				Filename:     "my_pkg-1.0.0-py3-none-any.whl",
				URL:          "https://example.com/my_pkg-1.0.0-py3-none-any.whl",
				CoreMetadata: true,
			},
			{
				Filename:     "my_pkg-2.0.0-py3-none-any.whl",
				URL:          "https://example.com/my_pkg-2.0.0-py3-none-any.whl",
				CoreMetadata: true,
			},
			{
				Filename: "my_pkg-1.0.0.tar.gz",
				URL:      "https://example.com/my_pkg-1.0.0.tar.gz",
			},
		},
	}

	// Convert to ResolutionPackage and back.
	rp := FromPackageInfo(original)
	is.Equal(rp.PEP658, true)
	is.Equal(len(rp.WheelURLs), 2)

	reconstructed := rp.ToPackageInfo()
	is.Equal(reconstructed.Name, "my-pkg")

	// Versions should match.
	origVersions := original.Versions()
	reconVersions := reconstructed.Versions()
	is.Equal(len(reconVersions), len(origVersions))

	// BestWheel should preserve CoreMetadata + URL.
	v1, _ := version.Parse("1.0.0")
	origWheel := original.BestWheel(v1)
	reconWheel := reconstructed.BestWheel(v1)
	is.True(reconWheel != nil)
	is.Equal(reconWheel.CoreMetadata, origWheel.CoreMetadata)
	is.Equal(reconWheel.URL, origWheel.URL)
}
