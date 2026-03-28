package lockfile

import (
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// ReadLockFile parses a lock file in any supported format (pensa.lock, uv.lock, poetry.lock).
// Auto-detects format from the file path.
func ReadLockFile(path string) (*LockFile, error) {
	if strings.HasSuffix(path, "uv.lock") {
		return ReadUVLockFile(path)
	}
	// pensa.lock and poetry.lock use the same TOML format (for now).
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read lock file: %w", err)
	}
	return ParseLockFile(data)
}

// lockFileTOML is the TOML representation of poetry.lock.
type lockFileTOML struct {
	Package  []packageTOML `toml:"package"`
	Metadata metadataTOML  `toml:"metadata"`
}

type packageTOML struct {
	Name           string            `toml:"name"`
	Version        string            `toml:"version"`
	Description    string            `toml:"description"`
	Optional       bool              `toml:"optional"`
	PythonVersions string            `toml:"python-versions"`
	Groups         []string          `toml:"groups"`
	Files          []fileTOML        `toml:"files"`
	Dependencies   map[string]any `toml:"dependencies"`
	Extras         map[string][]string    `toml:"extras"`
}

type fileTOML struct {
	File string `toml:"file"`
	Hash string `toml:"hash"`
	URL  string `toml:"url"`
}

type metadataTOML struct {
	LockVersion    string `toml:"lock-version"`
	PythonVersions string `toml:"python-versions"`
	ContentHash    string `toml:"content-hash"`
}

// ParseLockFile parses poetry.lock content from bytes.
func ParseLockFile(data []byte) (*LockFile, error) {
	var raw lockFileTOML
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse lock file: %w", err)
	}

	lf := &LockFile{
		Metadata: LockMetadata{
			LockVersion:    raw.Metadata.LockVersion,
			PythonVersions: raw.Metadata.PythonVersions,
			ContentHash:    raw.Metadata.ContentHash,
		},
	}

	for _, pkg := range raw.Package {
		locked := LockedPackage{
			Name:           pkg.Name,
			Version:        pkg.Version,
			Description:    pkg.Description,
			Optional:       pkg.Optional,
			PythonVersions: pkg.PythonVersions,
			Groups:         pkg.Groups,
			Dependencies:   make(map[string]string),
			Extras:         pkg.Extras,
		}

		for _, f := range pkg.Files {
			locked.Files = append(locked.Files, PackageFile{
				File: f.File,
				Hash: f.Hash,
				URL:  f.URL,
			})
		}

		// Dependencies can be string or table in poetry.lock.
		for name, val := range pkg.Dependencies {
			switch v := val.(type) {
			case string:
				locked.Dependencies[name] = v
			case map[string]any:
				if ver, ok := v["version"]; ok {
					if vs, ok := ver.(string); ok {
						locked.Dependencies[name] = vs
					}
				}
			}
		}

		lf.Packages = append(lf.Packages, locked)
	}

	return lf, nil
}
