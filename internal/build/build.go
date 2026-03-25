package build

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/juanbzz/pensa/internal/pyproject"
	"github.com/juanbzz/pensa/internal/python"
)

// lastLine returns the last non-empty line from a string (for clean error messages).
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

// Options controls what to build.
type Options struct {
	ProjectDir string
	OutputDir  string
	Wheel      bool
	Sdist      bool
	Editable   bool // PEP 660 editable wheel
}

// Result contains the paths of built artifacts.
type Result struct {
	Files []string
}

// Build creates distributable archives using PEP 517.
// It creates an isolated build venv, installs the build backend,
// and invokes build_wheel/build_sdist.
func Build(opts Options) (*Result, error) {
	proj, err := pyproject.ReadPyProject(filepath.Join(opts.ProjectDir, "pyproject.toml"))
	if err != nil {
		return nil, fmt.Errorf("read pyproject.toml: %w", err)
	}

	if proj.BuildSystem == nil || proj.BuildSystem.BuildBackend == "" {
		return nil, fmt.Errorf("pyproject.toml missing [build-system] with build-backend")
	}

	// Discover Python.
	py, err := python.Discover()
	if err != nil {
		return nil, fmt.Errorf("find Python: %w", err)
	}

	// Create isolated build venv with pip (using python -m venv, not our
	// lightweight CreateVenv, because we need pip to install build deps).
	buildVenv, err := os.MkdirTemp("", "pensa-build-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(buildVenv)

	venvPython, err := createVenv(py.Path, buildVenv)
	if err != nil {
		return nil, fmt.Errorf("create build venv: %w", err)
	}

	// Install build dependencies (suppress pip output).
	if len(proj.BuildSystem.Requires) > 0 {
		if err := installDeps(venvPython, proj.BuildSystem.Requires); err != nil {
			return nil, fmt.Errorf("install build dependencies: %w", err)
		}
	}

	// Ensure output directory exists.
	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	// Parse build-backend string: "module.path" or "module.path:object"
	backendModule, backendObject := parseBackend(proj.BuildSystem.BuildBackend)

	var result Result

	// Editable wheel (PEP 660).
	if opts.Editable {
		// Get extra build deps for editable installs (e.g., hatchling needs 'editables').
		if extraDeps, err := getEditableBuildDeps(venvPython, opts.ProjectDir, backendModule, backendObject); err == nil && len(extraDeps) > 0 {
			installDeps(venvPython, extraDeps) // best-effort, don't fail if this doesn't work
		}

		file, err := invokeBuildHook(venvPython, opts.ProjectDir, opts.OutputDir, backendModule, backendObject, "build_editable")
		if err != nil {
			return nil, fmt.Errorf("build editable: %w", err)
		}
		result.Files = append(result.Files, file)
		return &result, nil
	}

	// Build sdist.
	if opts.Sdist {
		file, err := invokeBuildHook(venvPython, opts.ProjectDir, opts.OutputDir, backendModule, backendObject, "build_sdist")
		if err != nil {
			return nil, fmt.Errorf("build sdist: %w", err)
		}
		result.Files = append(result.Files, file)
	}

	// Build wheel.
	if opts.Wheel {
		file, err := invokeBuildHook(venvPython, opts.ProjectDir, opts.OutputDir, backendModule, backendObject, "build_wheel")
		if err != nil {
			return nil, fmt.Errorf("build wheel: %w", err)
		}
		result.Files = append(result.Files, file)
	}

	return &result, nil
}

// parseBackend splits "module.path:object" into module and object parts.
// If no ":" is present, object is empty (the module itself has the hooks).
func parseBackend(backend string) (module, object string) {
	if i := strings.Index(backend, ":"); i >= 0 {
		return backend[:i], backend[i+1:]
	}
	return backend, ""
}

// invokeBuildHook calls a PEP 517 build hook (build_wheel or build_sdist)
// via a Python subprocess and returns the filename of the built artifact.
func invokeBuildHook(pythonPath, projectDir, outputDir, module, object, hook string) (string, error) {
	absOutput, err := filepath.Abs(outputDir)
	if err != nil {
		return "", err
	}

	// Build the Python script to invoke the hook.
	var script string
	if object == "" {
		script = fmt.Sprintf(`
import importlib, os
os.chdir(%q)
mod = importlib.import_module(%q)
result = mod.%s(%q)
print(result)
`, projectDir, module, hook, absOutput)
	} else {
		script = fmt.Sprintf(`
import importlib, os
os.chdir(%q)
mod = importlib.import_module(%q)
obj = getattr(mod, %q)
result = obj.%s(%q)
print(result)
`, projectDir, module, object, hook, absOutput)
	}

	cmd := exec.Command(pythonPath, "-c", script)
	cmd.Dir = projectDir

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		// Include stderr in error for debugging, but don't print it raw.
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("invoke %s: %s", hook, lastLine(errMsg))
		}
		return "", fmt.Errorf("invoke %s: %w", hook, err)
	}

	filename := strings.TrimSpace(string(out))
	if filename == "" {
		return "", fmt.Errorf("%s returned empty filename", hook)
	}

	return filepath.Join(absOutput, filename), nil
}

// getEditableBuildDeps calls get_requires_for_build_editable on the backend
// to discover extra dependencies needed for editable builds.
func getEditableBuildDeps(pythonPath, projectDir, module, object string) ([]string, error) {
	var script string
	if object == "" {
		script = fmt.Sprintf(`
import importlib, json
mod = importlib.import_module(%q)
if hasattr(mod, 'get_requires_for_build_editable'):
    print(json.dumps(mod.get_requires_for_build_editable()))
else:
    print('[]')
`, module)
	} else {
		script = fmt.Sprintf(`
import importlib, json
mod = importlib.import_module(%q)
obj = getattr(mod, %q)
if hasattr(obj, 'get_requires_for_build_editable'):
    print(json.dumps(obj.get_requires_for_build_editable()))
else:
    print('[]')
`, module, object)
	}

	cmd := exec.Command(pythonPath, "-c", script)
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Parse JSON list of strings.
	result := strings.TrimSpace(string(out))
	if result == "[]" || result == "" {
		return nil, nil
	}

	// Simple JSON array parse — strip brackets, split by comma, trim quotes.
	result = strings.Trim(result, "[]")
	var deps []string
	for _, s := range strings.Split(result, ",") {
		s = strings.TrimSpace(s)
		s = strings.Trim(s, `"`)
		if s != "" {
			deps = append(deps, s)
		}
	}
	return deps, nil
}

func createVenv(pythonPath, venvPath string) (string, error) {
	cmd := exec.Command(pythonPath, "-m", "venv", venvPath)
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("create venv: %w", err)
	}

	return filepath.Join(venvPath, "bin", "python"), nil
}

func installDeps(pythonPath string, deps []string) error {
	args := append([]string{"-m", "pip", "install", "--quiet", "--disable-pip-version-check"}, deps...)
	cmd := exec.Command(pythonPath, args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
