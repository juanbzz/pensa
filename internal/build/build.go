package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/juanbzz/pensa/internal/python"
	"github.com/juanbzz/pensa/internal/pyproject"
)

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

	venvCmd := exec.Command(py.Path, "-m", "venv", buildVenv)
	venvCmd.Stderr = os.Stderr
	if err := venvCmd.Run(); err != nil {
		return nil, fmt.Errorf("create build venv: %w", err)
	}

	venvPython := filepath.Join(buildVenv, "bin", "python")

	// Install build dependencies (suppress pip output).
	if len(proj.BuildSystem.Requires) > 0 {
		args := append([]string{"-m", "pip", "install", "--quiet", "--disable-pip-version-check"}, proj.BuildSystem.Requires...)
		cmd := exec.Command(venvPython, args...)
		cmd.Dir = opts.ProjectDir
		if err := cmd.Run(); err != nil {
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
		file, err := invokeBuildHook(venvPython, opts.ProjectDir, opts.OutputDir, backendModule, backendObject, "build_wheel_for_editable")
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
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("invoke %s: %w", hook, err)
	}

	filename := strings.TrimSpace(string(out))
	if filename == "" {
		return "", fmt.Errorf("%s returned empty filename", hook)
	}

	return filepath.Join(absOutput, filename), nil
}
