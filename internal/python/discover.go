package python

import (
	"bufio"
	"fmt"
	"os"
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

// FromVenv returns PythonInfo for an existing venv by reading its pyvenv.cfg.
// This is the source of truth for which Python the venv will actually run —
// host-PATH discovery may point at a different interpreter when the venv was
// built by uv/pyenv/poetry/etc. Returns an error if pyvenv.cfg is missing or
// doesn't contain a parseable version_info.
func FromVenv(venvPath string) (*PythonInfo, error) {
	cfgPath := filepath.Join(venvPath, "pyvenv.cfg")
	f, err := os.Open(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("read venv metadata at %s (rebuild with 'rm -rf %s && pensa install'): %w", cfgPath, venvPath, err)
	}
	defer f.Close()

	// Read both `version_info` (uv, Python's stdlib venv) and `version`
	// (virtualenv, pensa's own CreateVenv). Prefer `version_info` when both
	// are present — stdlib writes both and they're identical.
	var version, versionFallback string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "version_info":
			version = strings.TrimSpace(v)
		case "version":
			versionFallback = strings.TrimSpace(v)
		}
	}
	if version == "" {
		version = versionFallback
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read pyvenv.cfg: %w", err)
	}
	if version == "" {
		return nil, fmt.Errorf("%s missing version_info (rebuild with 'rm -rf %s && pensa install')", cfgPath, venvPath)
	}

	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("unparseable version_info %q", version)
	}
	major, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, fmt.Errorf("version_info major: %w", err)
	}
	minor, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return nil, fmt.Errorf("version_info minor: %w", err)
	}
	patch := 0
	if len(parts) >= 3 {
		patch, _ = strconv.Atoi(strings.TrimSpace(parts[2]))
	}

	return &PythonInfo{
		Path:    filepath.Join(venvPath, "bin", "python"),
		Version: fmt.Sprintf("%d.%d.%d", major, minor, patch),
		Major:   major,
		Minor:   minor,
		Patch:   patch,
	}, nil
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
