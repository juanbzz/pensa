package installer

import (
	"os"
	"strings"
)

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

// normalizeDistName converts dist-info naming convention to comparison form.
// Dist-info uses underscores (charset_normalizer), lock files use hyphens (charset-normalizer).
func normalizeDistName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}
