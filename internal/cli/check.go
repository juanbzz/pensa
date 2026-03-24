package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juanbzz/pensa/internal/lockfile"
	"github.com/juanbzz/pensa/internal/pyproject"
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
	pyprojectPath := filepath.Join(dir, "pyproject.toml")
	// Read pyproject.toml.
	proj, err := pyproject.ReadPyProject(pyprojectPath)
	if err != nil {
		return fmt.Errorf("read pyproject.toml: %w", err)
	}

	// Read lock file.
	lockPath, _ := lockfile.DetectLockFile(dir)
	if lockPath == "" {
		return fmt.Errorf("no lock file found (run 'pensa lock' first)")
	}
	lf, err := lockfile.ReadLockFile(lockPath)
	if err != nil {
		return fmt.Errorf("read lock file: %w", err)
	}

	var issues []string

	// Check content hash.
	currentHash := computeContentHash(pyprojectPath)
	if currentHash != "" && lf.Metadata.ContentHash != "" && currentHash != lf.Metadata.ContentHash {
		issues = append(issues, "poetry.lock is out of date (content hash mismatch). Run \"pensa lock\" to update.")
	}

	// Check all direct deps (all groups) are present in lock.
	allDeps, err := proj.ResolveAllDependencies()
	if err != nil {
		return fmt.Errorf("resolve dependencies: %w", err)
	}

	lockedNames := make(map[string]bool, len(lf.Packages))
	for _, p := range lf.Packages {
		lockedNames[normalizeName(p.Name)] = true
	}

	for _, gd := range allDeps {
		if !lockedNames[normalizeName(gd.Dep.Name)] {
			issues = append(issues, fmt.Sprintf("dependency %q (%s group) is in pyproject.toml but missing from poetry.lock.", gd.Dep.Name, gd.Group))
		}
	}

	if len(issues) > 0 {
		fmt.Fprintf(w, "%s\n", red("check failed:"))
		for _, issue := range issues {
			fmt.Fprintf(w, "  %s %s\n", red("-"), issue)
		}
		return fmt.Errorf("check failed with %d %s", len(issues), pluralize(len(issues), "issue", "issues"))
	}

	fmt.Fprintf(w, "%s\n", green("All checks passed."))
	return nil
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// depNameFromPEP508 extracts the package name from a PEP 508 dependency string.
// This is a simple extraction — just the name before any version specifier.
func depNameFromPEP508(s string) string {
	// Find the first non-name character.
	for i, c := range s {
		if c == '>' || c == '<' || c == '=' || c == '!' || c == '~' || c == '^' || c == '[' || c == ';' || c == ' ' {
			return strings.TrimSpace(s[:i])
		}
	}
	return strings.TrimSpace(s)
}
