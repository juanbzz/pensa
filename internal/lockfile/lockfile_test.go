package lockfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteLockFile(t *testing.T) {
	lf := &LockFile{
		Packages: []LockedPackage{
			{
				Name:           "requests",
				Version:        "2.31.0",
				Description:    "Python HTTP for Humans.",
				Optional:       false,
				PythonVersions: ">=3.7",
				Groups:         []string{"main"},
				Files: []PackageFile{
					{File: "requests-2.31.0-py3-none-any.whl", Hash: "sha256:abc123"},
					{File: "requests-2.31.0.tar.gz", Hash: "sha256:def456"},
				},
				Dependencies: map[string]string{
					"certifi":            ">=2017.4.17",
					"charset-normalizer": ">=2,<4",
					"idna":               ">=2.5,<4",
					"urllib3":            ">=1.21.1,<3",
				},
			},
			{
				Name:           "certifi",
				Version:        "2023.7.22",
				Description:    "Python package for providing Mozilla's CA Bundle.",
				Optional:       false,
				PythonVersions: ">=3.6",
				Groups:         []string{"main"},
				Files: []PackageFile{
					{File: "certifi-2023.7.22-py3-none-any.whl", Hash: "sha256:ghi789"},
				},
			},
		},
		Metadata: LockMetadata{
			LockVersion:    "2.1",
			PythonVersions: ">=3.8",
			ContentHash:    "abc123hash",
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "poetry.lock")

	err := WriteLockFile(path, lf)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Verify structure.
	if !strings.Contains(content, "[[package]]") {
		t.Error("missing [[package]]")
	}
	if !strings.Contains(content, `name = "certifi"`) {
		t.Error("missing certifi package")
	}
	if !strings.Contains(content, `name = "requests"`) {
		t.Error("missing requests package")
	}
	// Certifi should come before requests (alphabetical).
	certifiIdx := strings.Index(content, `name = "certifi"`)
	requestsIdx := strings.Index(content, `name = "requests"`)
	if certifiIdx > requestsIdx {
		t.Error("packages not sorted alphabetically")
	}
	// Dependencies section.
	if !strings.Contains(content, "[package.dependencies]") {
		t.Error("missing [package.dependencies]")
	}
	if !strings.Contains(content, `certifi = ">=2017.4.17"`) {
		t.Error("missing certifi dependency in requests")
	}
	// Metadata.
	if !strings.Contains(content, "[metadata]") {
		t.Error("missing [metadata]")
	}
	if !strings.Contains(content, `lock-version = "2.1"`) {
		t.Error("missing lock-version")
	}
	if !strings.Contains(content, `python-versions = ">=3.8"`) {
		t.Error("missing python-versions")
	}
	// Files.
	if !strings.Contains(content, `file = "requests-2.31.0-py3-none-any.whl"`) {
		t.Error("missing wheel file entry")
	}
	// Generated header.
	if !strings.Contains(content, "automatically @generated") {
		t.Error("missing generated header")
	}
}

func TestWriteLockFile_Empty(t *testing.T) {
	lf := &LockFile{
		Metadata: LockMetadata{
			LockVersion:    "2.1",
			PythonVersions: ">=3.8",
			ContentHash:    "empty",
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "poetry.lock")

	err := WriteLockFile(path, lf)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !strings.Contains(content, "[metadata]") {
		t.Error("missing metadata in empty lock file")
	}
	if strings.Contains(content, "[[package]]") {
		t.Error("empty lock file should not have packages")
	}
}
