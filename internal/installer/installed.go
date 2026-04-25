package installer

import (
	"os"
	"regexp"
	"strings"
)

// distNameNormalizer collapses runs of [-_.] to a single hyphen (PEP 503).
// Required because dist-info dirs use either `-`, `_`, or `.` as separators
// depending on the project (e.g. `zope.interface-7.2.dist-info`,
// `charset_normalizer-3.4.0.dist-info`), while lock files use hyphens.
var distNameNormalizer = regexp.MustCompile(`[-_.]+`)

// InstalledPackages scans site-packages for *.dist-info directories and returns
// a map of normalized package name → version string.
func InstalledPackages(sitePackagesDir string) (map[string]string, error) {
	entries, err := os.ReadDir(sitePackagesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	installed := make(map[string]string)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".dist-info") {
			continue
		}

		// Parse "{name}-{version}.dist-info"
		trimmed := strings.TrimSuffix(name, ".dist-info")
		// Find last hyphen — version is everything after it.
		// Package names can contain hyphens, but versions never start with a letter
		// after a hyphen in the dist-info convention, so we find the last hyphen
		// where the remainder starts with a digit.
		pkgName, pkgVersion := splitDistInfo(trimmed)
		if pkgName == "" || pkgVersion == "" {
			continue
		}

		installed[normalizeDistName(pkgName)] = pkgVersion
	}

	return installed, nil
}

// splitDistInfo splits "package_name-1.2.3" into ("package_name", "1.2.3").
// Finds the rightmost hyphen where the right side starts with a digit.
func splitDistInfo(s string) (string, string) {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '-' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
			return s[:i], s[i+1:]
		}
	}
	return "", ""
}

// normalizeDistName returns the PEP 503 canonical form of a dist-info
// package name: lowercase, with any run of `-`, `_`, or `.` collapsed to
// a single hyphen. Matches the normalization lock files and resolvers use,
// so comparisons across the two stay sound (e.g. `zope.interface` on disk
// compares equal to `zope-interface` in the lock).
func normalizeDistName(name string) string {
	return distNameNormalizer.ReplaceAllString(strings.ToLower(name), "-")
}
