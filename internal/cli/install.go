package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/juanbzz/pensa/internal/installer"
	"github.com/juanbzz/pensa/internal/lockfile"
	"github.com/juanbzz/pensa/internal/python"
	"github.com/spf13/cobra"
)

func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install dependencies from poetry.lock",
		Long:  "Creates a virtual environment and installs all locked dependencies.",
		RunE:  runInstall,
	}
}

func runInstall(cmd *cobra.Command, args []string) error {
	return installFromLock(cmd.OutOrStdout())
}

// installFromLock reads poetry.lock and installs packages into a venv.
// Shared between `install` and `add` commands.
func installFromLock(w interface{ Write([]byte) (int, error) }) error {
	start := time.Now()

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	lockPath := filepath.Join(dir, "poetry.lock")
	lf, err := lockfile.ReadLockFile(lockPath)
	if err != nil {
		return fmt.Errorf("read poetry.lock: %w (run 'pensa lock' first)", err)
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

	// Create venv if it doesn't exist.
	venvPath := filepath.Join(dir, ".venv")
	if !python.VenvExists(venvPath) {
		fmt.Fprintf(w, "%s using Python %s\n", blue("Creating virtualenv"), bold(py.Version))
		if err := python.CreateVenv(venvPath, py); err != nil {
			return fmt.Errorf("create venv: %w", err)
		}
	}

	// Create installer.
	cacheDir, err := defaultCacheDir()
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
		if installed[normalizeName(pkg.Name)] == pkg.Version {
			continue
		}
		toInstall = append(toInstall, pkg)
	}

	if len(toInstall) == 0 {
		fmt.Fprintf(w, "%s\n", green("All packages up to date."))
		return nil
	}

	// Install only what's needed.
	fmt.Fprintf(w, "Installing %d packages...\n", len(toInstall))
	for _, pkg := range toInstall {
		fmt.Fprintf(w, "  %s %s %s\n", green("Installing"), bold(pkg.Name), dim("("+pkg.Version+")"))
		if err := ins.InstallPackage(pkg); err != nil {
			return fmt.Errorf("install %s: %w", pkg.Name, err)
		}
	}

	elapsed := time.Since(start)
	fmt.Fprintf(w, "%s %d packages in %.1fs\n", green("Installed"), len(toInstall), elapsed.Seconds())

	return nil
}
