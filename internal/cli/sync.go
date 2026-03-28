package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/juanbzz/pensa/internal/config"
	"github.com/juanbzz/pensa/internal/installer"
	"github.com/juanbzz/pensa/internal/lockfile"
	"github.com/juanbzz/pensa/internal/python"
	"github.com/spf13/cobra"
)

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

	lockPath, _ := lockfile.DetectLockFile(dir)
	if lockPath == "" {
		return fmt.Errorf("no lock file found (run 'pensa lock' first)")
	}
	lf, err := lockfile.ReadLockFile(lockPath)
	if err != nil {
		return fmt.Errorf("read lock file: %w", err)
	}

	// Discover Python.
	py, err := python.Discover()
	if err != nil {
		return fmt.Errorf("find Python: %w", err)
	}

	// Create venv if it doesn't exist.
	venvPath := filepath.Join(dir, ".venv")
	if !python.VenvExists(venvPath) {
		fmt.Fprintf(w, "%s using Python %s\n", blue("Creating virtualenv"), bold(py.Version))
		if err := python.CreateVenv(venvPath, py); err != nil {
			return fmt.Errorf("create venv: %w", err)
		}
	}

	siteDir := py.SitePackagesDir(venvPath)
	installed, _ := installer.InstalledPackages(siteDir)

	// Build set of locked package names.
	lockedNames := make(map[string]string, len(lf.Packages))
	for _, pkg := range lf.Packages {
		lockedNames[normalizeName(pkg.Name)] = pkg.Version
	}

	// Compute what to install (missing or wrong version, filtered by group).
	var toInstall []lockfile.LockedPackage
	for _, pkg := range lf.Packages {
		if !packageInGroups(pkg, groups) {
			continue
		}
		if installed[normalizeName(pkg.Name)] == pkg.Version {
			continue
		}
		toInstall = append(toInstall, pkg)
	}

	// Compute what to remove (installed but not in lock).
	type removeEntry struct {
		name    string
		version string
	}
	var toRemove []removeEntry
	for name, ver := range installed {
		if _, inLock := lockedNames[name]; inLock {
			continue
		}
		if venvSkipPackages[name] {
			continue
		}
		toRemove = append(toRemove, removeEntry{name, ver})
	}

	cfg, _ := config.New()
	verbose := cfg != nil && cfg.Verbose
	out := newUI(w, verbose, cfg != nil && cfg.Quiet)

	if len(toInstall) == 0 && len(toRemove) == 0 {
		out.UpToDate("All packages up to date.")
		return nil
	}

	// Remove extras first.
	if len(toRemove) > 0 {
		for _, pkg := range toRemove {
			if err := installer.UninstallPackage(siteDir, pkg.name, pkg.version); err != nil {
				return fmt.Errorf("uninstall %s: %w", pkg.name, err)
			}
		}
	}

	// Install missing.
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

	// Install the project itself in editable mode.
	if err := installProject(w, dir, venvPath, py); err != nil {
		return fmt.Errorf("install project: %w", err)
	}

	elapsed := time.Since(start)
	if len(toInstall) > 0 {
		out.Installed(len(toInstall), elapsed)
	}
	if len(toRemove) > 0 {
		out.Uninstalled(len(toRemove), elapsed)
	}

	// Per-package diff only in verbose mode.
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
