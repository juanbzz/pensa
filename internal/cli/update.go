package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juanbzz/pensa/internal/pyproject"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update [packages...]",
		Short: "Update dependencies to latest compatible versions",
		Long:  "Re-resolves dependencies to their latest compatible versions, updates the lock file, and installs.",
		Example: `  pensa update
  pensa update requests
  pensa update requests flask`,
		RunE: runUpdate,
	}
}

func runUpdate(cmd *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	pyprojectPath := filepath.Join(dir, "pyproject.toml")
	proj, err := pyproject.ReadPyProject(pyprojectPath)
	if err != nil {
		return fmt.Errorf("read pyproject.toml: %w", err)
	}

	deps, err := proj.ResolveDependencies()
	if err != nil {
		return fmt.Errorf("resolve dependencies: %w", err)
	}

	if len(deps) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), yellow("No dependencies to update."))
		return nil
	}

	w := cmd.OutOrStdout()

	opts := lockOptions{}
	if len(args) == 0 {
		// Update all — ignore all pinned versions.
		opts.upgrade = true
		fmt.Fprintf(w, "%s\n", blue("Updating all dependencies..."))
	} else {
		// Update specific packages only.
		opts.upgradePackages = args
		for _, pkg := range args {
			fmt.Fprintf(w, "%s %s\n", blue("Updating"), bold(pkg))
		}
	}

	if err := resolveAndLock(w, proj, pyprojectPath, opts); err != nil {
		return err
	}

	return installFromLock(w, true)
}
