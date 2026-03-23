package installer

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UninstallPackage removes a package from site-packages by reading its
// RECORD file and deleting all listed files, then the dist-info directory.
func UninstallPackage(sitePackagesDir, pkgName, pkgVersion string) error {
	distInfo, err := FindDistInfo(sitePackagesDir, pkgName, pkgVersion)
	if err != nil {
		return fmt.Errorf("find dist-info for %s: %w", pkgName, err)
	}

	// Read RECORD to get list of installed files.
	recordPath := filepath.Join(distInfo, "RECORD")
	files, err := readRecord(recordPath)
	if err != nil {
		// If no RECORD, fall back to removing just the dist-info and package dir.
		removeDir(filepath.Join(sitePackagesDir, normalizeDistName(pkgName)))
		removeDir(distInfo)
		return nil
	}

	// Delete each recorded file.
	removedDirs := make(map[string]bool)
	for _, relPath := range files {
		absPath := filepath.Join(sitePackagesDir, relPath)
		os.Remove(absPath)
		removedDirs[filepath.Dir(absPath)] = true
	}

	// Clean up empty directories (deepest first).
	for dir := range removedDirs {
		cleanEmptyDirs(dir, sitePackagesDir)
	}

	// Remove the dist-info directory itself.
	removeDir(distInfo)

	return nil
}

// readRecord parses a RECORD file and returns the list of relative file paths.
// RECORD format: path,hash,size (CSV, one per line).
func readRecord(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var files []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// First field is the relative path.
		parts := strings.SplitN(line, ",", 2)
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		files = append(files, parts[0])
	}
	return files, scanner.Err()
}

// cleanEmptyDirs removes empty directories up to (but not including) stopAt.
func cleanEmptyDirs(dir, stopAt string) {
	for dir != stopAt && dir != "." && dir != "/" {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}

func removeDir(path string) {
	os.RemoveAll(path)
}
