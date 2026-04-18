package python

import (
	"fmt"
	"os"
	"path/filepath"
)

// CreateVenv creates a virtual environment at the given path.
// Does not shell out to python — creates the structure directly.
func CreateVenv(path string, py *PythonInfo) error {
	binDir := filepath.Join(path, "bin")
	libDir := filepath.Join(path, "lib", fmt.Sprintf("python%d.%d", py.Major, py.Minor), "site-packages")
	includeDir := filepath.Join(path, "include")

	// Create directory structure.
	for _, dir := range []string{binDir, libDir, includeDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create venv directory %s: %w", dir, err)
		}
	}

	// Symlink python binary.
	pythonLink := filepath.Join(binDir, "python3")
	if err := os.Symlink(py.Path, pythonLink); err != nil && !os.IsExist(err) {
		return fmt.Errorf("symlink python3: %w", err)
	}

	pythonShort := filepath.Join(binDir, "python")
	if err := os.Symlink("python3", pythonShort); err != nil && !os.IsExist(err) {
		return fmt.Errorf("symlink python: %w", err)
	}

	// Versioned symlink (python3.11 → python3).
	versionedLink := filepath.Join(binDir, fmt.Sprintf("python%d.%d", py.Major, py.Minor))
	if err := os.Symlink("python3", versionedLink); err != nil && !os.IsExist(err) {
		// Not critical, ignore.
	}

	// Write pyvenv.cfg. Include both `version` and `version_info` so tools
	// reading either key (virtualenv uses `version`, uv/stdlib use
	// `version_info`) can detect the interpreter correctly.
	cfg := fmt.Sprintf("home = %s\ninclude-system-site-packages = false\nversion = %s\nversion_info = %s\n",
		filepath.Dir(py.Path), py.Version, py.Version)
	cfgPath := filepath.Join(path, "pyvenv.cfg")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		return fmt.Errorf("write pyvenv.cfg: %w", err)
	}

	return nil
}

// VenvExists checks if a virtual environment exists at the given path.
func VenvExists(path string) bool {
	_, err := os.Stat(filepath.Join(path, "pyvenv.cfg"))
	return err == nil
}
