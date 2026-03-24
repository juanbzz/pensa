package lockfile

import (
	"os"
	"path/filepath"
)

// Lock file format identifiers.
const (
	FormatPensa  = "pensa"
	FormatUV     = "uv"
	FormatPoetry = "poetry"
)

// DetectLockFile finds the lock file in the given directory.
// Returns the path and format, or empty strings if no lock file found.
// Priority: pensa.lock > uv.lock > poetry.lock
func DetectLockFile(dir string) (path, format string) {
	candidates := []struct {
		name   string
		format string
	}{
		{"pensa.lock", FormatPensa},
		{"uv.lock", FormatUV},
		{"poetry.lock", FormatPoetry},
	}

	for _, c := range candidates {
		p := filepath.Join(dir, c.name)
		if _, err := os.Stat(p); err == nil {
			return p, c.format
		}
	}
	return "", ""
}
