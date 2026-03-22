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
