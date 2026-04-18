package build

import (
	"fmt"
	"pensa.sh/pensa/internal/pyproject"
	"pensa.sh/pensa/internal/python"
	"os"
	"path/filepath"
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

	if len(requires) > 0 {
		if err := installDeps(venvPython, requires); err != nil {
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
