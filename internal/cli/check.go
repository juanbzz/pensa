package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juanbzz/pensa/internal/lockfile"
	"github.com/juanbzz/pensa/internal/pyproject"
	"github.com/juanbzz/pensa/internal/workspace"
	"github.com/spf13/cobra"
)

func newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Validate project consistency",
		Long:  "Checks that pyproject.toml and poetry.lock are consistent.",
		Example: `  pensa check`,
		Args: cobra.NoArgs,
		RunE: runCheck,
	}
}

func runCheck(cmd *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	w := cmd.OutOrStdout()

	// Workspace-aware: lock lives at the workspace root when in a workspace,
	// and its content hash combines the root + every member's dep hash.
	ws, _ := workspace.Discover(dir)
	rootDir := dir
	if ws != nil {
		rootDir = ws.Root
	}
	pyprojectPath := filepath.Join(rootDir, "pyproject.toml")

	lockPath, _ := lockfile.DetectLockFile(rootDir)
	if lockPath == "" {
		return fmt.Errorf("no lock file found (run 'pensa lock' first)")
	}
	lockName := filepath.Base(lockPath)
	lf, err := lockfile.ReadLockFile(lockPath)
	if err != nil {
		return fmt.Errorf("read lock file: %w", err)
	}

	var issues []string

	// Check content hash. In a workspace this combines the root + all members;
	// single-project mode just hashes the one pyproject.
	var currentHash string
	if ws != nil {
		currentHash = computeWorkspaceHash(ws)
	} else {
		currentHash = computeContentHash(pyprojectPath)
	}
	if currentHash != "" && lf.Metadata.ContentHash != "" && currentHash != lf.Metadata.ContentHash {
		issues = append(issues, fmt.Sprintf(
			"%s is out of date (content hash mismatch). Run \"pensa lock\" to update.", lockName))
	}

	// Check all direct deps (all groups) are present in lock — across every
	// pyproject that contributes to the lock (root + workspace members).
	var projects []*pyproject.PyProject
	if ws != nil {
		for _, m := range ws.Members {
			projects = append(projects, m.Project)
		}
	} else {
		proj, err := pyproject.ReadPyProject(pyprojectPath)
		if err != nil {
			return fmt.Errorf("read pyproject.toml: %w", err)
		}
		projects = append(projects, proj)
	}

	lockedNames := make(map[string]bool, len(lf.Packages))
	for _, p := range lf.Packages {
		lockedNames[normalizeName(p.Name)] = true
	}

	for _, proj := range projects {
		allDeps, err := proj.ResolveAllDependencies()
		if err != nil {
			return fmt.Errorf("resolve dependencies: %w", err)
		}
		for _, gd := range allDeps {
			if !lockedNames[normalizeName(gd.Dep.Name)] {
				issues = append(issues, fmt.Sprintf(
					"dependency %q (%s group) is in pyproject.toml but missing from %s.",
					gd.Dep.Name, gd.Group, lockName))
			}
		}
	}

	out := newUI(w, false, false)
	if len(issues) > 0 {
		out.Error("check failed")
		for _, issue := range issues {
			fmt.Fprintf(w, "  %s %s\n", red("-"), issue)
		}
		return fmt.Errorf("check failed with %d %s", len(issues), pluralize(len(issues), "issue", "issues"))
	}

	out.UpToDate("All checks passed.")
	return nil
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

