package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pensa.sh/pensa/internal/lockfile"
	"pensa.sh/pensa/internal/pyproject"
	"pensa.sh/pensa/internal/workspace"
)

// normalizeName lowercases and replaces underscores with hyphens for comparison.
func normalizeName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

// readLockFileFromCwd reads the lock file from the current or workspace root directory.
// Auto-detects format: pensa.lock, uv.lock, or poetry.lock.
func readLockFileFromCwd() (*lockfile.LockFile, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	// Check workspace root first.
	if ws, _ := workspace.Discover(dir); ws != nil {
		dir = ws.Root
	}

	lockPath, _ := lockfile.DetectLockFile(dir)
	if lockPath == "" {
		return nil, fmt.Errorf("no lock file found (run 'pensa lock' first)")
	}
	lf, err := lockfile.ReadLockFile(lockPath)
	if err != nil {
		return nil, fmt.Errorf("read lock file: %w", err)
	}
	return lf, nil
}

// sortPackages sorts packages alphabetically by name (case-insensitive).
func sortPackages(pkgs []lockfile.LockedPackage) {
	sort.Slice(pkgs, func(i, j int) bool {
		return normalizeName(pkgs[i].Name) < normalizeName(pkgs[j].Name)
	})
}

// filterTopLevel returns only packages that are direct dependencies in pyproject.toml.
func filterTopLevel(pkgs []lockfile.LockedPackage) ([]lockfile.LockedPackage, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	pp, err := pyproject.ReadPyProject(filepath.Join(dir, "pyproject.toml"))
	if err != nil {
		return nil, fmt.Errorf("read pyproject.toml: %w", err)
	}

	deps, err := pp.ResolveDependencies()
	if err != nil {
		return nil, fmt.Errorf("resolve dependencies: %w", err)
	}

	directNames := make(map[string]bool, len(deps))
	for _, d := range deps {
		directNames[normalizeName(d.Name)] = true
	}

	var filtered []lockfile.LockedPackage
	for _, p := range pkgs {
		if directNames[normalizeName(p.Name)] {
			filtered = append(filtered, p)
		}
	}
	return filtered, nil
}

// packageInGroups checks if a locked package belongs to any of the specified groups.
func packageInGroups(pkg lockfile.LockedPackage, groups []string) bool {
	for _, pkgGroup := range pkg.Groups {
		for _, g := range groups {
			if pkgGroup == g {
				return true
			}
		}
	}
	return false
}

// buildPackageIndex creates a map from normalized name to package for lookups.
func buildPackageIndex(pkgs []lockfile.LockedPackage) map[string]lockfile.LockedPackage {
	idx := make(map[string]lockfile.LockedPackage, len(pkgs))
	for _, p := range pkgs {
		idx[normalizeName(p.Name)] = p
	}
	return idx
}

// depNameFromPEP508 extracts the package name from a PEP 508 dependency string.
// This is a simple extraction — just the name before any version specifier.
func depNameFromPEP508(s string) string {
	for i, c := range s {
		if c == '>' || c == '<' || c == '=' || c == '!' || c == '~' || c == '^' || c == '[' || c == ';' || c == ' ' {
			return strings.TrimSpace(s[:i])
		}
	}
	return strings.TrimSpace(s)
}
