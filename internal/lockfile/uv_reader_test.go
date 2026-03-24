package lockfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadUVLockFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "uv.lock")

	content := `version = 1
requires-python = ">=3.10"

[[package]]
name = "requests"
version = "2.32.5"
source = { registry = "https://pypi.org/simple" }
dependencies = [
    { name = "certifi" },
    { name = "urllib3" },
]
sdist = { url = "https://files.pythonhosted.org/packages/requests-2.32.5.tar.gz", hash = "sha256:abc123", size = 12345 }
wheels = [
    { url = "https://files.pythonhosted.org/packages/requests-2.32.5-py3-none-any.whl", hash = "sha256:def456", size = 6789 },
]

[[package]]
name = "certifi"
version = "2024.2.2"
source = { registry = "https://pypi.org/simple" }
wheels = [
    { url = "https://files.pythonhosted.org/packages/certifi-2024.2.2-py3-none-any.whl", hash = "sha256:ghi789", size = 1234 },
]
`
	os.WriteFile(path, []byte(content), 0644)

	lf, err := ReadUVLockFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(lf.Packages) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(lf.Packages))
	}

	// Check requests package.
	var req *LockedPackage
	for i, p := range lf.Packages {
		if p.Name == "requests" {
			req = &lf.Packages[i]
		}
	}
	if req == nil {
		t.Fatal("missing requests package")
	}

	if req.Version != "2.32.5" {
		t.Errorf("version = %q", req.Version)
	}

	// Should have wheel + sdist = 2 files.
	if len(req.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(req.Files))
	}

	// Files should have URLs.
	for _, f := range req.Files {
		if f.URL == "" {
			t.Errorf("file %q should have URL", f.File)
		}
	}

	// Dependencies.
	if len(req.Dependencies) != 2 {
		t.Errorf("expected 2 deps, got %d", len(req.Dependencies))
	}

	// Metadata.
	if lf.Metadata.PythonVersions != ">=3.10" {
		t.Errorf("python versions = %q", lf.Metadata.PythonVersions)
	}
}

func TestReadUVLockFile_SkipsVirtual(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "uv.lock")

	content := `version = 1
requires-python = ">=3.10"

[[package]]
name = "myproject"
version = "0.1.0"
source = { virtual = "." }

[[package]]
name = "requests"
version = "2.32.5"
source = { registry = "https://pypi.org/simple" }
wheels = [
    { url = "https://example.com/requests-2.32.5-py3-none-any.whl", hash = "sha256:abc", size = 100 },
]
`
	os.WriteFile(path, []byte(content), 0644)

	lf, err := ReadUVLockFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Virtual workspace member should be skipped.
	if len(lf.Packages) != 1 {
		t.Fatalf("expected 1 package (virtual skipped), got %d", len(lf.Packages))
	}
	if lf.Packages[0].Name != "requests" {
		t.Errorf("expected requests, got %q", lf.Packages[0].Name)
	}
}

func TestDetectLockFile(t *testing.T) {
	dir := t.TempDir()

	// No lock file.
	path, format := DetectLockFile(dir)
	if path != "" || format != "" {
		t.Error("should return empty when no lock file")
	}

	// poetry.lock only.
	os.WriteFile(filepath.Join(dir, "poetry.lock"), []byte(""), 0644)
	path, format = DetectLockFile(dir)
	if format != FormatPoetry {
		t.Errorf("format = %q, want poetry", format)
	}

	// uv.lock takes precedence over poetry.lock.
	os.WriteFile(filepath.Join(dir, "uv.lock"), []byte(""), 0644)
	path, format = DetectLockFile(dir)
	if format != FormatUV {
		t.Errorf("format = %q, want uv", format)
	}

	// pensa.lock takes highest precedence.
	os.WriteFile(filepath.Join(dir, "pensa.lock"), []byte(""), 0644)
	path, format = DetectLockFile(dir)
	if format != FormatPensa {
		t.Errorf("format = %q, want pensa", format)
	}
	_ = path
}

func TestFilenameFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://files.pythonhosted.org/packages/abc/requests-2.32.5-py3-none-any.whl", "requests-2.32.5-py3-none-any.whl"},
		{"https://example.com/certifi-2024.2.2.tar.gz", "certifi-2024.2.2.tar.gz"},
		{"https://example.com/pkg.whl?auth=token", "pkg.whl"},
	}
	for _, tt := range tests {
		got := filenameFromURL(tt.url)
		if got != tt.want {
			t.Errorf("filenameFromURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}
