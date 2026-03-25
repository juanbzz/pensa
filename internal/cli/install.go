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
	"github.com/juanbzz/pensa/internal/workspace"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
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
func installFromLock(w interface{ Write([]byte) (int, error) }, installRoot bool, groups []string) error {
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

	// Discover Python.
	py, err := python.Discover()
	if err != nil {
		return fmt.Errorf("find Python: %w", err)
	}

	// Create venv if it doesn't exist (at workspace root or project dir).
	venvPath := filepath.Join(rootDir, ".venv")
	if !python.VenvExists(venvPath) {
		fmt.Fprintf(w, "%s using Python %s\n", blue("Creating virtualenv"), bold(py.Version))
		if err := python.CreateVenv(venvPath, py); err != nil {
			return fmt.Errorf("create venv: %w", err)
		}
	}

	// Create installer.
	cacheDir := defaultCacheDir()
	if err != nil {
		return err
	}
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

	// Phase 1: Download all wheels in parallel.
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

	// Phase 2: Install sequentially from cache.
	fmt.Fprintf(w, "Installing %d packages...\n", len(results))
	for _, res := range results {
		fmt.Fprintf(w, "  %s %s %s\n", green("Installing"), bold(res.pkg.Name), dim("("+res.pkg.Version+")"))
		if err := ins.InstallFromCache(res.pkg, res.wheelPath); err != nil {
			return fmt.Errorf("install %s: %w", res.pkg.Name, err)
		}
	}

	elapsed := time.Since(start)
	fmt.Fprintf(w, "%s %d packages in %.1fs\n", green("Installed"), len(results), elapsed.Seconds())

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
