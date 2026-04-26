package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"pensa.sh/pensa/internal/pyproject"
	"pensa.sh/pensa/internal/workspace"
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

	out := uiFromCmd(cmd)

	opts := lockOptions{}
	if len(args) == 0 {
		opts.upgrade = true
	} else {
		opts.upgradePackages = args
	}

	announce := func() {
		if len(args) == 0 {
			out.Info(blue("Updating all dependencies..."))
			return
		}
		for _, pkg := range args {
			out.Infof("%s %s", blue("Updating"), bold(pkg))
		}
	}

	// Workspace path: aggregate member deps via runLockWorkspace.
	// Reading only the root pyproject misses every member's
	// [project].dependencies, so a workspace update used to bail
	// with "no dependencies to update" even when packages clearly
	// were updatable.
	if ws, _ := workspace.Discover(dir); ws != nil {
		if !workspaceHasDependencies(ws) {
			out.Warning("no dependencies to update")
			return nil
		}
		announce()
		if err := runLockWorkspace(cmd.Context(), os.Stderr, ws, opts); err != nil {
			return err
		}
		return installFromLock(os.Stderr, true, nil, "")
	}

	pyprojectPath := filepath.Join(dir, "pyproject.toml")
	proj, err := pyproject.ReadPyProject(pyprojectPath)
	if err != nil {
		return fmt.Errorf("read pyproject.toml: %w", err)
	}

	deps, err := proj.ResolveAllDependencies()
	if err != nil {
		return fmt.Errorf("resolve dependencies: %w", err)
	}

	if len(deps) == 0 {
		out.Warning("no dependencies to update")
		return nil
	}

	announce()
	if err := resolveAndLock(cmd.Context(), os.Stderr, proj, pyprojectPath, opts); err != nil {
		return err
	}

	return installFromLock(os.Stderr, true, nil, "")
}

// workspaceHasDependencies returns true when any workspace member
// declares at least one dependency in any group. Mirrors
// runLockWorkspace's own dep collection (ResolveAllDependencies) so
// dev-only workspaces aren't falsely reported as empty.
//
// Errors reading a member's pyproject are intentionally swallowed
// here: a malformed member would still fail downstream in
// runLockWorkspace with a clearer error context, and pre-checking
// shouldn't be the place that surfaces parse failures.
func workspaceHasDependencies(ws *workspace.Workspace) bool {
	for _, m := range ws.Members {
		deps, err := m.Project.ResolveAllDependencies()
		if err == nil && len(deps) > 0 {
			return true
		}
	}
	return false
}
