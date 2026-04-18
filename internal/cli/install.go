package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	goRuntime "runtime"
	"strings"
	"time"

	"pensa.sh/pensa/internal/config"
	"pensa.sh/pensa/internal/installer"
	"pensa.sh/pensa/internal/lockfile"
	"pensa.sh/pensa/internal/python"
	"pensa.sh/pensa/internal/workspace"
	"pensa.sh/pensa/pkg/version"
	"github.com/spf13/cobra"
)

func newInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install dependencies from lock file",
		Long:  "Creates a virtual environment and installs all locked dependencies.",
		RunE:  runInstall,
	}
	cmd.Flags().Bool("no-root", false, "Do not install the project itself")
	cmd.Flags().Bool("no-dev", false, "Do not install dev dependencies")
	cmd.Flags().StringSlice("with", nil, "Include optional dependency groups")
	cmd.Flags().String("only", "", "Install only this dependency group")
	cmd.Flags().String("package", "", "Install only this workspace member's dependencies")
	return cmd
}

func runInstall(cmd *cobra.Command, args []string) error {
	noRoot, _ := cmd.Flags().GetBool("no-root")
	noDev, _ := cmd.Flags().GetBool("no-dev")
	withGroups, _ := cmd.Flags().GetStringSlice("with")
	onlyGroup, _ := cmd.Flags().GetString("only")

	groups := resolveInstallGroups(noDev, withGroups, onlyGroup)
	return installFromLock(cmd.OutOrStdout(), !noRoot, groups)
}

// resolveInstallGroups determines which groups to install based on flags.
func resolveInstallGroups(noDev bool, withGroups []string, onlyGroup string) []string {
	if onlyGroup != "" {
		return []string{onlyGroup}
	}
	groups := []string{"main"}
	if !noDev {
		groups = append(groups, "dev")
	}
	groups = append(groups, withGroups...)
	return groups
}

// installFromLock reads poetry.lock and installs packages into a venv.
// If installRoot is true, also installs the project itself in editable mode.
// groups controls which dependency groups to install (nil = all).
// Shared between `install`, `add`, `update`, and `remove` commands.
func installFromLock(w io.Writer, installRoot bool, groups []string) error {
	start := time.Now()

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Check for workspace — use workspace root for lock file and venv.
	ws, _ := workspace.Discover(dir)
	rootDir := dir
	if ws != nil {
		rootDir = ws.Root
	}

	lockPath, _ := lockfile.DetectLockFile(rootDir)
	if lockPath == "" {
		return fmt.Errorf("no lock file found (run 'pensa lock' first)")
	}
	lf, err := lockfile.ReadLockFile(lockPath)
	if err != nil {
		return fmt.Errorf("read lock file: %w", err)
	}

	if len(lf.Packages) == 0 {
		fmt.Fprintf(w, "No packages to install.\n")
		return nil
	}

	// Pick Python: prefer the existing venv's pyvenv.cfg (the source of truth
	// for what `pensa run python` will load from), fall back to host PATH when
	// we're creating a new venv.
	venvPath := filepath.Join(rootDir, ".venv")
	py, err := pickPython(w, venvPath)
	if err != nil {
		return err
	}

	// Create installer.
	cacheDir := defaultCacheDir()
	client, err := newPyPIClient()
	if err != nil {
		return err
	}

	ins := installer.NewInstaller(client, venvPath, py, cacheDir)

	// Check what's already installed.
	siteDir := py.SitePackagesDir(venvPath)
	installed, _ := installer.InstalledPackages(siteDir)

	var toInstall []lockfile.LockedPackage
	for _, pkg := range lf.Packages {
		// Filter by group if specified.
		if groups != nil && !packageInGroups(pkg, groups) {
			continue
		}
		if installed[normalizeName(pkg.Name)] == pkg.Version {
			continue
		}
		// Skip packages incompatible with current Python.
		if !compatibleWithPython(pkg, py) {
			continue
		}
		toInstall = append(toInstall, pkg)
	}

	if len(toInstall) == 0 {
		fmt.Fprintf(w, "%s\n", green("All packages up to date."))
		// Still install project itself if requested.
		if installRoot {
			if ws != nil {
				for _, m := range ws.Members {
					if err := installProject(w, m.Path, venvPath, py); err != nil {
						return fmt.Errorf("install member %s: %w", m.Name, err)
					}
				}
			} else {
				if err := installProject(w, dir, venvPath, py); err != nil {
					return fmt.Errorf("install project: %w", err)
				}
			}
		}
		return nil
	}

	results, err := downloadPackages(w, ins, toInstall)
	if err != nil {
		return err
	}

	// Phase 2: Install sequentially from cache.
	cfg, _ := config.New()
	verbose := cfg != nil && cfg.Verbose
	out := newUI(w, verbose, cfg != nil && cfg.Quiet)

	for _, res := range results {
		if err := ins.InstallFromCache(res.pkg, res.wheelPath); err != nil {
			return fmt.Errorf("install %s: %w", res.pkg.Name, err)
		}
	}

	elapsed := time.Since(start)
	out.Installed(len(results), elapsed)

	// Per-package diff only in verbose mode.
	if verbose {
		for _, res := range results {
			out.DiffAdd(res.pkg.Name, res.pkg.Version)
		}
	}

	// Install the project itself in editable mode.
	if installRoot {
		if ws != nil {
			// Workspace: install each member in editable mode.
			for _, m := range ws.Members {
				if err := installProject(w, m.Path, venvPath, py); err != nil {
					return fmt.Errorf("install member %s: %w", m.Name, err)
				}
			}
		} else {
			if err := installProject(w, dir, venvPath, py); err != nil {
				return fmt.Errorf("install project: %w", err)
			}
		}
	}

	return nil
}

// compatibleWithPython checks whether a locked package is compatible with
// the current Python interpreter. Skips packages whose python-versions
// constraint excludes the current Python, or that have wheels but none
// matching the current CPython version.
func compatibleWithPython(pkg lockfile.LockedPackage, py *python.PythonInfo) bool {
	pyVer, err := version.Parse(fmt.Sprintf("%d.%d.%d", py.Major, py.Minor, py.Patch))
	if err != nil {
		return true // can't parse, don't skip
	}

	// Check python-versions constraint.
	if pkg.PythonVersions != "" {
		constraint, err := version.ParseConstraint(pkg.PythonVersions)
		if err == nil && !constraint.Allows(pyVer) {
			return false
		}
	}

	// Check if any wheel is compatible with current CPython.
	if !hasCompatibleWheel(pkg.Files, py) {
		return false
	}

	return true
}

// hasCompatibleWheel checks if at least one wheel in the file list matches
// the current Python version and platform. Returns true if no wheels exist (sdist-only).
func hasCompatibleWheel(files []lockfile.PackageFile, py *python.PythonInfo) bool {
	cpTag := fmt.Sprintf("cp%d%d", py.Major, py.Minor)
	hasWheel := false

	for _, f := range files {
		if !strings.HasSuffix(f.File, ".whl") {
			continue
		}
		hasWheel = true

		// Universal wheels always match.
		if strings.Contains(f.File, "-py3-none-any") || strings.Contains(f.File, "-py2.py3-none-any") {
			return true
		}

		// Check Python version tag.
		if !strings.Contains(f.File, cpTag) && !strings.Contains(f.File, "-py3-") {
			continue
		}

		// Check platform tag — skip wheels for other platforms.
		if !wheelMatchesPlatform(f.File) {
			continue
		}

		return true
	}

	// No wheels at all → sdist-only package, allow it.
	return !hasWheel
}

// wheelMatchesPlatform checks if a wheel filename is compatible with the
// current OS. Wheel filenames end with {python}-{abi}-{platform}.whl.
func wheelMatchesPlatform(filename string) bool {
	// Platform-independent.
	if strings.Contains(filename, "-any.whl") {
		return true
	}

	switch {
	case strings.Contains(filename, "macosx") || strings.Contains(filename, "darwin"):
		return isDarwin()
	case strings.Contains(filename, "manylinux") || strings.Contains(filename, "musllinux") || strings.Contains(filename, "linux"):
		return isLinux()
	case strings.Contains(filename, "win32") || strings.Contains(filename, "win_amd64") || strings.Contains(filename, "win_arm64"):
		return isWindows()
	}

	return true // unknown platform tag, don't skip
}

func isDarwin() bool  { return goRuntime.GOOS == "darwin" }
func isLinux() bool   { return goRuntime.GOOS == "linux" }
func isWindows() bool { return goRuntime.GOOS == "windows" }
