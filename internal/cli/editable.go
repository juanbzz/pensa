package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/juanbzz/pensa/internal/build"
	"github.com/juanbzz/pensa/internal/installer"
	"github.com/juanbzz/pensa/internal/pyproject"
	"github.com/juanbzz/pensa/internal/python"
)

// installProject installs the current project in editable mode into the venv.
// Uses PEP 660 build_wheel_for_editable to produce an editable wheel,
// then unpacks it into site-packages.
// Skips silently if no [build-system] is defined (project is not a package).
func installProject(w io.Writer, projectDir, venvPath string, py *python.PythonInfo) error {
	pyprojectPath := filepath.Join(projectDir, "pyproject.toml")
	proj, err := pyproject.ReadPyProject(pyprojectPath)
	if err != nil {
		return nil // no pyproject.toml — skip
	}

	// Skip if no build system (not a package).
	if proj.BuildSystem == nil || proj.BuildSystem.BuildBackend == "" {
		return nil
	}

	name := proj.Name()
	if name == "" {
		name = filepath.Base(projectDir)
	}

	// Build editable wheel into a temp dir.
	tmpDir, err := os.MkdirTemp("", "pensa-editable-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	result, err := build.Build(build.Options{
		ProjectDir: projectDir,
		OutputDir:  tmpDir,
		Editable:   true,
	})
	if err != nil {
		// Warn but don't fail — deps are installed, just no editable project.
		out := newUI(w, false, false)
		out.Warning(fmt.Sprintf("editable install failed: %s", err))
		return nil
	}

	if len(result.Files) == 0 {
		return fmt.Errorf("editable build produced no files")
	}

	// Install the editable wheel into site-packages.
	wheelPath := result.Files[0]
	sitePackages := py.SitePackagesDir(venvPath)

	if err := installer.UnpackWheel(wheelPath, sitePackages); err != nil {
		return fmt.Errorf("unpack editable wheel: %w", err)
	}

	// Install entry points (scripts).
	ver := proj.Version()
	if ver == "" {
		ver = "0.0.0"
	}
	distInfo, err := installer.FindDistInfo(sitePackages, name, ver)
	if err == nil {
		binDir := filepath.Join(venvPath, "bin")
		pythonPath := filepath.Join(binDir, "python")
		installer.InstallEntryPoints(distInfo, binDir, pythonPath)
	}

	out := newUI(w, false, false)
	out.Infof("%s %s in editable mode", green("Installed"), bold(name))

	return nil
}
