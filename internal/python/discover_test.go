package python

import (
	"testing"
)

func TestDiscover_FindsPython3(t *testing.T) {
	info, err := Discover()
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if info.Major < 3 {
		t.Errorf("expected Python 3+, got %d.%d", info.Major, info.Minor)
	}
	if info.Path == "" {
		t.Error("Path is empty")
	}
	if info.Version == "" {
		t.Error("Version is empty")
	}
	t.Logf("Found Python %s at %s", info.Version, info.Path)
}

func TestPythonInfo_SitePackagesDir(t *testing.T) {
	info := &PythonInfo{Major: 3, Minor: 11}
	got := info.SitePackagesDir("/tmp/venv")
	want := "/tmp/venv/lib/python3.11/site-packages"
	if got != want {
		t.Errorf("SitePackagesDir = %q, want %q", got, want)
	}
}
