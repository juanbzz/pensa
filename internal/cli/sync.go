package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"pensa.sh/pensa/internal/config"
	"pensa.sh/pensa/internal/installer"
	"pensa.sh/pensa/internal/lockfile"
	"pensa.sh/pensa/internal/pyproject"
	"pensa.sh/pensa/internal/python"
	"pensa.sh/pensa/internal/workspace"
	"github.com/spf13/cobra"
)

// localProjectNames returns the set of normalized project names that
// pensa installs in editable mode (the cwd's project, or every
// workspace member). These names must be excluded from sync's
// remove list — otherwise sync would uninstall the editable
// project on every run only for installProject to recreate it,
// producing a misleading "Uninstalled 1 package" message and a
// venv state that never converges.
func localProjectNames(dir string) map[string]bool {
	out := map[string]bool{}
	if ws, _ := workspace.Discover(dir); ws != nil {
		for _, m := range ws.Members {
			if m.Name != "" {
				out[normalizeName(m.Name)] = true
			}
		}
		return out
	}
	if proj, err := pyproject.ReadPyProject(filepath.Join(dir, "pyproject.toml")); err == nil {
		if name := proj.Name(); name != "" {
			out[normalizeName(name)] = true
		}
	}
	return out
}

// venvChange describes a single uninstall action.
type venvChange struct {
	name    string
	version string
}

// planVenvChanges computes the install + remove deltas needed to
// converge a venv with the lockfile under the given group filter.
// Both `install` and `sync` go through this single planner so they
// can't drift in policy: a fix to one is automatically a fix to the
// other.
//
//   - desired   = lock packages matching the requested groups and
//                 installable on this platform
//   - toInstall = desired entries missing from the venv or at the
//                 wrong version
//   - toRemove  = installed entries outside desired, with the
//                 editable-installed project(s) and venv
//                 infrastructure (pip, setuptools, etc.) excluded
//
// A nil `groups` slice means "all groups" (no group filter applied);
// callers that want to enforce a filter must pass a non-nil slice.
func planVenvChanges(
	lf *lockfile.LockFile,
	installed map[string]string,
	groups []string,
	py *python.PythonInfo,
	projectNames map[string]bool,
) (toInstall []lockfile.LockedPackage, toRemove []venvChange) {
	desired := make(map[string]string, len(lf.Packages))
	for _, pkg := range lf.Packages {
		if groups != nil && !packageInGroups(pkg, groups) {
			continue
		}
		if !compatibleWithPython(pkg, py) {
			continue
		}
		desired[normalizeName(pkg.Name)] = pkg.Version
	}

	for _, pkg := range lf.Packages {
		norm := normalizeName(pkg.Name)
		want, ok := desired[norm]
		if !ok {
			continue
		}
		if installed[norm] == want {
			continue
		}
		toInstall = append(toInstall, pkg)
	}

	for name, ver := range installed {
		if _, want := desired[name]; want {
			continue
		}
		if projectNames[name] {
			continue
		}
		if venvSkipPackages[name] {
			continue
		}
		toRemove = append(toRemove, venvChange{name, ver})
	}

	return toInstall, toRemove
}

// installEditableProjects installs every workspace member (or, in
// single-project mode, the cwd's project) into the venv in editable
// mode. Used by both install and sync so members never drift out of
// link after a sync run from inside a member directory.
func installEditableProjects(w io.Writer, ws *workspace.Workspace, dir, venvPath string, py *python.PythonInfo) error {
	if ws != nil {
		for _, m := range ws.Members {
			if err := installProject(w, m.Path, venvPath, py); err != nil {
				return fmt.Errorf("install member %s: %w", m.Name, err)
			}
		}
		return nil
	}
	if err := installProject(w, dir, venvPath, py); err != nil {
		return fmt.Errorf("install project: %w", err)
	}
	return nil
}

// venvSkipPackages are infrastructure packages that should never be removed.
var venvSkipPackages = map[string]bool{
	"pip":             true,
	"setuptools":      true,
	"wheel":           true,
	"pkg-resources":   true,
	"distutils-hack":  true,
	"-distutils-hack": true,
}

func newSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync venv with lock file",
		Long:  "Makes the virtual environment match the lock file exactly. Installs missing packages and removes extras.",
		Example: `  pensa sync
  pensa sync --no-dev`,
		Args: cobra.NoArgs,
		RunE: runSync,
	}
	cmd.Flags().Bool("no-dev", false, "Do not install dev dependencies")
	cmd.Flags().StringSlice("with", nil, "Include optional dependency groups")
	cmd.Flags().String("only", "", "Install only this dependency group")
	return cmd
}

func runSync(cmd *cobra.Command, args []string) error {
	noDev, _ := cmd.Flags().GetBool("no-dev")
	withGroups, _ := cmd.Flags().GetStringSlice("with")
	onlyGroup, _ := cmd.Flags().GetString("only")
	groups := resolveInstallGroups(noDev, withGroups, onlyGroup)

	start := time.Now()
	w := cmd.OutOrStdout()

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Resolve workspace root so the lock file and venv are read from
	// the right place even when sync is invoked from inside a member.
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

	venvPath := filepath.Join(rootDir, ".venv")
	py, err := pickPython(w, venvPath)
	if err != nil {
		return err
	}

	siteDir := py.SitePackagesDir(venvPath)
	installed, _ := installer.InstalledPackages(siteDir)
	projectNames := localProjectNames(rootDir)

	cfg, _ := config.New()
	verbose := cfg != nil && cfg.Verbose
	out := newUI(w, verbose, cfg != nil && cfg.Quiet)

	toInstall, toRemove := planVenvChanges(lf, installed, groups, py, projectNames)

	if len(toInstall) == 0 && len(toRemove) == 0 {
		out.UpToDate("All packages up to date.")
		return nil
	}

	for _, pkg := range toRemove {
		if err := installer.UninstallPackage(siteDir, pkg.name, pkg.version); err != nil {
			return fmt.Errorf("uninstall %s: %w", pkg.name, err)
		}
	}

	if len(toInstall) > 0 {
		cacheDir := defaultCacheDir()
		client, err := newPyPIClient()
		if err != nil {
			return err
		}
		ins := installer.NewInstaller(client, venvPath, py, cacheDir)

		results, err := downloadPackages(w, ins, toInstall)
		if err != nil {
			return err
		}

		for _, res := range results {
			if err := ins.InstallFromCache(res.pkg, res.wheelPath); err != nil {
				return fmt.Errorf("install %s: %w", res.pkg.Name, err)
			}
		}
	}

	if err := installEditableProjects(w, ws, rootDir, venvPath, py); err != nil {
		return err
	}

	elapsed := time.Since(start)
	if len(toInstall) > 0 {
		out.Installed(len(toInstall), elapsed)
	}
	if len(toRemove) > 0 {
		out.Uninstalled(len(toRemove), elapsed)
	}

	if verbose {
		for _, pkg := range toRemove {
			out.DiffRemove(pkg.name, pkg.version)
		}
		for _, pkg := range toInstall {
			out.DiffAdd(pkg.Name, pkg.Version)
		}
	}

	return nil
}
