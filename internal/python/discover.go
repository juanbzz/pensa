package python

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// PythonInfo holds information about a discovered Python interpreter.
type PythonInfo struct {
	Path    string // absolute path to the interpreter
	Version string // full version string, e.g. "3.11.4"
	Major   int
	Minor   int
	Patch   int
}

// SitePackagesDir returns the site-packages path for a venv using this Python.
func (p *PythonInfo) SitePackagesDir(venvPath string) string {
	return filepath.Join(venvPath, "lib", fmt.Sprintf("python%d.%d", p.Major, p.Minor), "site-packages")
}

// Discover finds a suitable Python 3 interpreter on the system.
func Discover() (*PythonInfo, error) {
	candidates := []string{
		"python3",
		"python",
	}

	for _, name := range candidates {
		info, err := probe(name)
		if err == nil && info.Major >= 3 {
			return info, nil
		}
	}

	return nil, fmt.Errorf("no Python 3 interpreter found on PATH")
}

var versionRe = regexp.MustCompile(`Python (\d+)\.(\d+)\.(\d+)`)

func probe(name string) (*PythonInfo, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, err
	}

	// Ask Python for its real executable path (handles pyenv shims, etc.).
	realPath, err := exec.Command(path, "-c", "import sys; print(sys.executable)").Output()
	if err != nil {
		// Fall back to symlink resolution.
		resolved, err2 := filepath.EvalSymlinks(path)
		if err2 != nil {
			return nil, fmt.Errorf("resolve %s: %w", path, err)
		}
		path = resolved
	} else {
		path = strings.TrimSpace(string(realPath))
	}

	out, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("run %s --version: %w", path, err)
	}

	m := versionRe.FindStringSubmatch(strings.TrimSpace(string(out)))
	if m == nil {
		return nil, fmt.Errorf("unexpected version output: %s", out)
	}

	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])

	return &PythonInfo{
		Path:    path,
		Version: fmt.Sprintf("%d.%d.%d", major, minor, patch),
		Major:   major,
		Minor:   minor,
		Patch:   patch,
	}, nil
}
