package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/juanbzz/pensa/internal/installer"
	"github.com/juanbzz/pensa/internal/lockfile"
	"github.com/juanbzz/pensa/internal/python"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
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

	if len(toInstall) == 0 && len(toRemove) == 0 {
		fmt.Fprintf(w, "%s\n", green("All packages up to date."))
		return nil
	}

	// Remove extras first.
	if len(toRemove) > 0 {
		fmt.Fprintf(w, "Removing %d packages...\n", len(toRemove))
		for _, pkg := range toRemove {
			fmt.Fprintf(w, "  %s %s %s\n", yellow("Removing"), bold(pkg.name), dim("("+pkg.version+")"))
			if err := installer.UninstallPackage(siteDir, pkg.name, pkg.version); err != nil {
				return fmt.Errorf("uninstall %s: %w", pkg.name, err)
			}
		}
	}

	// Install missing.
	if len(toInstall) > 0 {
		cacheDir := defaultCacheDir()
		if err != nil {
			return err
		}
		client, err := newPyPIClient()
		if err != nil {
			return err
		}
		ins := installer.NewInstaller(client, venvPath, py, cacheDir)

		// Phase 1: Download in parallel.
		type downloadResult struct {
			pkg       lockfile.LockedPackage
			wheelPath string
		}

		stop := downloadSpinner(w, len(toInstall))

		var mu sync.Mutex
		var results []downloadResult

		g := new(errgroup.Group)
		g.SetLimit(4)

		for _, pkg := range toInstall {
			pkg := pkg
			g.Go(func() error {
				path, err := ins.ResolvePackage(pkg)
				if err != nil {
					return fmt.Errorf("download %s: %w", pkg.Name, err)
				}
				mu.Lock()
				results = append(results, downloadResult{pkg, path})
				mu.Unlock()
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			stop()
			return err
		}
		stop()

		// Phase 2: Install from cache.
		fmt.Fprintf(w, "Installing %d packages...\n", len(results))
		for _, res := range results {
			fmt.Fprintf(w, "  %s %s %s\n", green("Installing"), bold(res.pkg.Name), dim("("+res.pkg.Version+")"))
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
	fmt.Fprintf(w, "%s in %.1fs\n", green("Synced"), elapsed.Seconds())

	return nil
}
