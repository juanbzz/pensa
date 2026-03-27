package installer

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// UnpackWheel extracts a wheel zip into the target directory (site-packages).
func UnpackWheel(wheelPath, targetDir string) error {
	r, err := zip.OpenReader(wheelPath)
	if err != nil {
		return fmt.Errorf("open wheel %s: %w", wheelPath, err)
	}
	defer r.Close()

	for _, f := range r.File {
		destPath := filepath.Join(targetDir, f.Name)

		// Prevent zip slip.
		if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(targetDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path in wheel: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(destPath, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

// extractToCache extracts a wheel ZIP to the global cache if not already extracted.
// Returns the path to the extracted directory.
func extractToCache(wheelPath, cacheDir string) (string, error) {
	base := filepath.Base(wheelPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	// Strip platform tags: "flask-3.1.3-py3-none-any" → "flask-3.1.3"
	parts := strings.SplitN(name, "-", 3)
	if len(parts) >= 2 {
		name = parts[0] + "-" + parts[1]
	}

	extractDir := filepath.Join(cacheDir, "extracted", name)

	// Already extracted? Check for marker file.
	if _, err := os.Stat(filepath.Join(extractDir, ".extracted")); err == nil {
		return extractDir, nil
	}

	// Extract to temp dir, then rename atomically.
	os.MkdirAll(filepath.Join(cacheDir, "extracted"), 0755)
	tmpDir, err := os.MkdirTemp(filepath.Join(cacheDir, "extracted"), ".extract-*")
	if err != nil {
		return "", err
	}

	if err := UnpackWheel(wheelPath, tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		return "", err
	}

	// Write marker.
	os.WriteFile(filepath.Join(tmpDir, ".extracted"), nil, 0644)

	// Atomic rename.
	os.RemoveAll(extractDir)
	if err := os.Rename(tmpDir, extractDir); err != nil {
		os.RemoveAll(tmpDir)
		return "", err
	}

	return extractDir, nil
}

// linkToSitePackages hard-links all files from extractedDir to sitePackages.
// Falls back to copy when hard-links fail (cross-device, permissions).
func linkToSitePackages(extractedDir, sitePackages string) error {
	return filepath.Walk(extractedDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || info.Name() == ".extracted" {
			return nil
		}
		rel, err := filepath.Rel(extractedDir, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(sitePackages, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		os.Remove(dst) // remove existing before linking
		return linkOrCopy(path, dst)
	})
}

func linkOrCopy(src, dst string) error {
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// FindDistInfo finds the .dist-info directory after unpacking a wheel.
func FindDistInfo(sitePackages, pkgName, pkgVersion string) (string, error) {
	// Wheel dist-info dirs use underscores: requests-2.32.5.dist-info
	normalized := strings.ReplaceAll(pkgName, "-", "_")
	pattern := filepath.Join(sitePackages, fmt.Sprintf("%s-%s.dist-info", normalized, pkgVersion))
	if _, err := os.Stat(pattern); err == nil {
		return pattern, nil
	}

	// Try glob as fallback.
	matches, _ := filepath.Glob(filepath.Join(sitePackages, normalized+"-*.dist-info"))
	if len(matches) > 0 {
		return matches[0], nil
	}

	return "", fmt.Errorf("dist-info not found for %s %s", pkgName, pkgVersion)
}

// InstallEntryPoints reads console_scripts from the dist-info and creates
// wrapper scripts in binDir.
func InstallEntryPoints(distInfoDir, binDir, pythonPath string) error {
	entryPointsPath := filepath.Join(distInfoDir, "entry_points.txt")
	data, err := os.ReadFile(entryPointsPath)
	if err != nil {
		return nil // No entry points — not an error.
	}

	inConsoleScripts := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "[console_scripts]" {
			inConsoleScripts = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inConsoleScripts = false
			continue
		}
		if !inConsoleScripts || line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse: name = module:function
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		scriptName := strings.TrimSpace(parts[0])
		entryPoint := strings.TrimSpace(parts[1])

		modFunc := strings.SplitN(entryPoint, ":", 2)
		if len(modFunc) != 2 {
			continue
		}
		module := modFunc[0]
		function := modFunc[1]

		script := fmt.Sprintf(`#!/bin/sh
exec "%s" -c "import sys; from %s import %s; sys.exit(%s())" "$@"
`, pythonPath, module, function, function)

		scriptPath := filepath.Join(binDir, scriptName)
		if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
			return fmt.Errorf("write entry point script %s: %w", scriptName, err)
		}
	}

	return nil
}
