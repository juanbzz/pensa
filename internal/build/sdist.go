package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pensa.sh/pensa/internal/pyproject"
	"pensa.sh/pensa/internal/python"
)

type SdistBuildOptions struct {
	Name      string // package name
	Version   string // package version
	SdistPath string // path to sdist archive
	OutputDir string // directory to write built wheel to
	Python    *python.PythonInfo
}

func BuildFromSdist(opts SdistBuildOptions) (string, error) {
	tmpDir, err := os.MkdirTemp("", "pensa-sdist-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := ExtractSdist(opts.SdistPath, tmpDir); err != nil {
		return "", err
	}

	// find root, sdist have one top level dir named {name}-{version}
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return "", fmt.Errorf("read extracted sdist: %w", err)
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		return "", fmt.Errorf("unexpected sdist structure: expected one directory, got %d", len(entries))
	}
	sdistRoot := filepath.Join(tmpDir, entries[0].Name())

	// read pyproject.toml from extracted dir
	requires := []string{"setuptools >= 40.8.0", "wheel"}
	backend := "setuptools.build_meta:__legacy__"

	pyprojPath := filepath.Join(sdistRoot, "pyproject.toml")

	// check if pyproject.toml exists
	if _, err := os.Stat(pyprojPath); err == nil {
		proj, err := pyproject.ReadPyProject(pyprojPath)
		if err != nil {
			return "", fmt.Errorf("read pyproject.toml: %w", err)
		}

		// otherwise, fallback to default build system (setuptools)
		if proj.BuildSystem != nil {
			requires = proj.BuildSystem.Requires
			backend = proj.BuildSystem.BuildBackend
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check pyproject.toml: %w", err)
	}

	buildVenv, err := os.MkdirTemp("", "pensa-build-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(buildVenv)

	venvPython, err := createVenv(opts.Python.Path, buildVenv)
	if err != nil {
		return "", fmt.Errorf("create build venv: %w", err)
	}

	effective := effectiveBuildRequires(requires, backend)
	if len(effective) > 0 {
		if err := installDeps(venvPython, effective); err != nil {
			return "", fmt.Errorf("install build dependencies: %w", err)
		}
	}

	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	module, object := parseBackend(backend)
	wheelPath, err := invokeBuildHook(venvPython, sdistRoot, opts.OutputDir, module, object, "build_wheel")
	if err != nil {
		return "", fmt.Errorf("build wheel from sdist: %w", err)
	}

	return wheelPath, nil
}

// effectiveBuildRequires returns the install list for the isolated
// build venv. It starts from the sdist's declared
// build-system.requires (or our default for legacy setup.py sdists
// without pyproject.toml). When the backend is setuptools'
// build_meta and 'wheel' isn't already declared, we add it: setuptools'
// build_wheel invokes bdist_wheel from the wheel package and many
// projects (psutil, others) don't list it explicitly, expecting the
// frontend to provide it — pip, build, uv all do.
func effectiveBuildRequires(declared []string, backend string) []string {
	out := append([]string(nil), declared...)
	if strings.HasPrefix(backend, "setuptools.build_meta") && !requiresHas(out, "wheel") {
		out = append(out, "wheel")
	}
	return out
}

// requiresHas reports whether reqs declares a dependency on name.
// Comparison is case-insensitive on the leading PEP 508 distribution
// name only (extras, version specifiers, and markers are ignored).
func requiresHas(reqs []string, name string) bool {
	for _, r := range reqs {
		head := strings.TrimSpace(r)
		end := len(head)
		for i := 0; i < len(head); i++ {
			c := head[i]
			if c == ' ' || c == '\t' || c == '<' || c == '>' || c == '=' ||
				c == '!' || c == '~' || c == '[' || c == ';' || c == '(' {
				end = i
				break
			}
		}
		if strings.EqualFold(head[:end], name) {
			return true
		}
	}
	return false
}
