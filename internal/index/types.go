package index

import (
	"fmt"
	"regexp"
	"strings"

	"pensa.sh/pensa/pkg/pep508"
	"pensa.sh/pensa/pkg/version"
)

// Repository is the interface the resolver uses to fetch package info.
type Repository interface {
	GetPackageInfo(name string) (*PackageInfo, error)
	GetVersionDetail(name string, ver version.Version) (*VersionDetail, error)
}

// PackageInfo holds all known files for a package from the Simple API.
type PackageInfo struct {
	Name  string
	Files []FileInfo
}

// FileInfo represents a single distribution file.
type FileInfo struct {
	Filename       string
	URL            string
	Hashes         map[string]string
	RequiresPython string
	CoreMetadata   bool
	Yanked         bool
	YankedReason   string
}

// VersionDetail holds resolved metadata for a specific version.
type VersionDetail struct {
	Name           string
	Version        version.Version
	RequiresPython string
	Dependencies   []pep508.Dependency
}

// Versions extracts and deduplicates all versions from file listings.
func (p *PackageInfo) Versions() []version.Version {
	seen := make(map[string]bool)
	var versions []version.Version
	for _, f := range p.Files {
		v, err := VersionFromFilename(f.Filename)
		if err != nil {
			continue
		}
		key := v.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		versions = append(versions, v)
	}
	return versions
}

// FilesForVersion returns all files matching a specific version.
func (p *PackageInfo) FilesForVersion(ver version.Version) []FileInfo {
	var files []FileInfo
	for _, f := range p.Files {
		v, err := VersionFromFilename(f.Filename)
		if err != nil {
			continue
		}
		if version.Compare(v, ver) == 0 {
			files = append(files, f)
		}
	}
	return files
}

// BestWheel returns the best wheel file for a version, preferring
// pure-Python universal wheels.
func (p *PackageInfo) BestWheel(ver version.Version) *FileInfo {
	files := p.FilesForVersion(ver)
	var best *FileInfo
	for i, f := range files {
		if !strings.HasSuffix(f.Filename, ".whl") {
			continue
		}
		if best == nil {
			best = &files[i]
			continue
		}
		// Prefer py3-none-any wheels.
		if strings.Contains(f.Filename, "-py3-none-any") {
			best = &files[i]
		}
	}
	return best
}

// --- Filename parsing ---

var wheelRe = regexp.MustCompile(`^(?P<name>[A-Za-z0-9]([A-Za-z0-9._]*[A-Za-z0-9])?)-(?P<version>[^-]+)(-[^-]+)?-[^-]+-[^-]+-[^-]+\.whl$`)
// Sdist filenames are ambiguous because names can contain hyphens.
// We split on the last hyphen before a digit to find the version.
var sdistRe = regexp.MustCompile(`^(?P<name>.+)-(?P<version>[0-9][^-]*)\.(tar\.gz|zip|tar\.bz2)$`)

// VersionFromFilename extracts the version from a wheel or sdist filename.
func VersionFromFilename(filename string) (version.Version, error) {
	if strings.HasSuffix(filename, ".whl") {
		return parseWheelVersion(filename)
	}
	return parseSdistVersion(filename)
}

func parseWheelVersion(filename string) (version.Version, error) {
	m := wheelRe.FindStringSubmatch(filename)
	if m == nil {
		return version.Version{}, fmt.Errorf("invalid wheel filename: %s", filename)
	}
	verStr := m[wheelRe.SubexpIndex("version")]
	return version.Parse(verStr)
}

func parseSdistVersion(filename string) (version.Version, error) {
	m := sdistRe.FindStringSubmatch(filename)
	if m == nil {
		return version.Version{}, fmt.Errorf("invalid sdist filename: %s", filename)
	}
	verStr := m[sdistRe.SubexpIndex("version")]
	return version.Parse(verStr)
}

// ParseMetadata parses a Python METADATA file (RFC 822-like format)
// and extracts dependency information.
func ParseMetadata(data []byte) (*VersionDetail, error) {
	var detail VersionDetail
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")

		// Stop at the body separator.
		if line == "" {
			break
		}

		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}

		switch strings.ToLower(key) {
		case "name":
			detail.Name = pep508.NormalizeName(value)
		case "version":
			v, err := version.Parse(value)
			if err == nil {
				detail.Version = v
			}
		case "requires-python":
			detail.RequiresPython = value
		case "requires-dist":
			dep, err := pep508.Parse(value)
			if err == nil {
				detail.Dependencies = append(detail.Dependencies, dep)
			}
		}
	}

	return &detail, nil
}
