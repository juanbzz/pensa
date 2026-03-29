package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matryer/is"

	"github.com/juanbzz/pensa/internal/lockfile"
)

// testLockFile returns a lock file with requests + its transitive deps.
func testLockFile() *lockfile.LockFile {
	return &lockfile.LockFile{
		Packages: []lockfile.LockedPackage{
			{
				Name:           "requests",
				Version:        "2.31.0",
				Description:    "Python HTTP for Humans.",
				PythonVersions: ">=3.7",
				Groups:         []string{"main"},
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
				PythonVersions: ">=3.6",
				Groups:         []string{"main"},
			},
			{
				Name:           "charset-normalizer",
				Version:        "3.3.2",
				Description:    "The Real First Universal Charset Detector.",
				PythonVersions: ">=3.7",
				Groups:         []string{"main"},
			},
			{
				Name:           "idna",
				Version:        "3.7",
				Description:    "Internationalized Domain Names in Applications.",
				PythonVersions: ">=3.5",
				Groups:         []string{"main"},
			},
			{
				Name:           "urllib3",
				Version:        "2.2.1",
				Description:    "HTTP library with thread-safe connection pooling.",
				PythonVersions: ">=3.8",
				Groups:         []string{"main"},
			},
		},
		Metadata: lockfile.LockMetadata{
			LockVersion:    "2.1",
			PythonVersions: ">=3.8",
			ContentHash:    "testhash",
		},
	}
}

// setupTestDir creates a temp dir with a poetry.lock and chdirs into it.
// Returns the dir path. Cleanup restores the original directory.
func setupTestDir(t *testing.T, lf *lockfile.LockFile) string {
	t.Helper()
	assert := is.New(t)
	dir := t.TempDir()

	orig, err := os.Getwd()
	assert.NoErr(err)
	t.Cleanup(func() { os.Chdir(orig) })

	assert.NoErr(lockfile.WriteLockFile(filepath.Join(dir, "poetry.lock"), lf))
	assert.NoErr(os.Chdir(dir))
	return dir
}

// writePyprojectWithDeps writes a pyproject.toml with the given direct deps.
func writePyprojectWithDeps(t *testing.T, dir string, deps ...string) {
	t.Helper()
	assert := is.New(t)
	content := `[tool.poetry]
name = "test-project"
version = "0.1.0"
description = ""

[tool.poetry.dependencies]
python = "^3.8"
`
	for _, d := range deps {
		content += d + "\n"
	}
	assert.NoErr(os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(content), 0644))
}
