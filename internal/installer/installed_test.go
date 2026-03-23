package installer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstalledPackages_Empty(t *testing.T) {
	dir := t.TempDir()
	installed, err := InstalledPackages(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 0 {
		t.Errorf("expected empty map, got %d entries", len(installed))
	}
}

func TestInstalledPackages_NonExistent(t *testing.T) {
	installed, err := InstalledPackages("/nonexistent/path")
	if err != nil {
		t.Fatal("should not error on nonexistent dir")
	}
	if installed != nil {
		t.Error("expected nil map")
	}
}

func TestInstalledPackages_ScansDistInfo(t *testing.T) {
	dir := t.TempDir()

	// Create fake dist-info directories.
	dirs := []string{
		"requests-2.31.0.dist-info",
		"certifi-2023.7.22.dist-info",
		"charset_normalizer-3.3.2.dist-info",
		"urllib3-2.2.1.dist-info",
	}
	for _, d := range dirs {
		os.MkdirAll(filepath.Join(dir, d), 0755)
	}

	installed, err := InstalledPackages(dir)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		version string
	}{
		{"requests", "2.31.0"},
		{"certifi", "2023.7.22"},
		{"charset-normalizer", "3.3.2"}, // underscore → hyphen normalization
		{"urllib3", "2.2.1"},
	}

	for _, tt := range tests {
		v, ok := installed[tt.name]
		if !ok {
			t.Errorf("missing package %q", tt.name)
			continue
		}
		if v != tt.version {
			t.Errorf("%s version = %q, want %q", tt.name, v, tt.version)
		}
	}
}

func TestInstalledPackages_IgnoresNonDistInfo(t *testing.T) {
	dir := t.TempDir()

	// Create a mix of dist-info and non-dist-info entries.
	os.MkdirAll(filepath.Join(dir, "requests-2.31.0.dist-info"), 0755)
	os.MkdirAll(filepath.Join(dir, "requests"), 0755)           // package dir
	os.MkdirAll(filepath.Join(dir, "__pycache__"), 0755)        // cache
	os.WriteFile(filepath.Join(dir, "somefile.py"), []byte{}, 0644) // file

	installed, err := InstalledPackages(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(installed) != 1 {
		t.Errorf("expected 1 entry, got %d", len(installed))
	}
	if installed["requests"] != "2.31.0" {
		t.Error("missing requests")
	}
}

func TestSplitDistInfo(t *testing.T) {
	tests := []struct {
		input   string
		name    string
		version string
	}{
		{"requests-2.31.0", "requests", "2.31.0"},
		{"charset_normalizer-3.3.2", "charset_normalizer", "3.3.2"},
		{"my-package-1.0.0", "my-package", "1.0.0"},
		{"certifi-2023.7.22", "certifi", "2023.7.22"},
		{"no-version", "", ""},
	}

	for _, tt := range tests {
		name, version := splitDistInfo(tt.input)
		if name != tt.name || version != tt.version {
			t.Errorf("splitDistInfo(%q) = (%q, %q), want (%q, %q)",
				tt.input, name, version, tt.name, tt.version)
		}
	}
}
