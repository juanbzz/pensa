package lockfile

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// uv.lock TOML structures.

type uvLockFile struct {
	Version        int         `toml:"version"`
	RequiresPython string      `toml:"requires-python"`
	Packages       []uvPackage `toml:"package"`
}

type uvPackage struct {
	Name         string         `toml:"name"`
	Version      string         `toml:"version"`
	Source       uvSource       `toml:"source"`
	Dependencies []uvDependency `toml:"dependencies"`
	Sdist        *uvDistFile    `toml:"sdist"`
	Wheels       []uvDistFile   `toml:"wheels"`
}

type uvSource struct {
	Registry string `toml:"registry"`
	Virtual  string `toml:"virtual"`
	Editable string `toml:"editable"`
}

type uvDependency struct {
	Name    string `toml:"name"`
	Marker  string `toml:"marker"`
	Version string `toml:"version"`
}

type uvDistFile struct {
	URL  string `toml:"url"`
	Hash string `toml:"hash"`
	Size int    `toml:"size"`
}

// ReadUVLockFile parses a uv.lock file and converts it to our LockFile format.
func ReadUVLockFile(filePath string) (*LockFile, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read uv.lock: %w", err)
	}

	var uv uvLockFile
	if err := toml.Unmarshal(data, &uv); err != nil {
		return nil, fmt.Errorf("parse uv.lock: %w", err)
	}

	lf := &LockFile{
		Metadata: LockMetadata{
			LockVersion:    fmt.Sprintf("%d", uv.Version),
			PythonVersions: uv.RequiresPython,
		},
	}

	for _, pkg := range uv.Packages {
		// Skip virtual/editable workspace members — they're not installable from PyPI.
		if pkg.Source.Virtual != "" || pkg.Source.Editable != "" {
			continue
		}

		locked := LockedPackage{
			Name:         pkg.Name,
			Version:      pkg.Version,
			Groups:       []string{"main"}, // uv.lock doesn't track groups
			Dependencies: make(map[string]string),
			Extras:       make(map[string][]string),
		}

		// Convert wheels to Files.
		for _, w := range pkg.Wheels {
			locked.Files = append(locked.Files, PackageFile{
				File: filenameFromURL(w.URL),
				Hash: w.Hash,
				URL:  w.URL,
			})
		}

		// Convert sdist to Files.
		if pkg.Sdist != nil {
			locked.Files = append(locked.Files, PackageFile{
				File: filenameFromURL(pkg.Sdist.URL),
				Hash: pkg.Sdist.Hash,
				URL:  pkg.Sdist.URL,
			})
		}

		// Convert dependencies.
		for _, dep := range pkg.Dependencies {
			constraint := "*"
			if dep.Version != "" {
				constraint = dep.Version
			}
			locked.Dependencies[dep.Name] = constraint
		}

		lf.Packages = append(lf.Packages, locked)
	}

	return lf, nil
}

// filenameFromURL extracts the filename from a URL path.
func filenameFromURL(url string) string {
	// URL like: https://files.pythonhosted.org/packages/.../requests-2.32.5-py3-none-any.whl
	base := path.Base(url)
	// Remove any query params.
	if i := strings.Index(base, "?"); i >= 0 {
		base = base[:i]
	}
	return base
}
