package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juanbzz/goetry/internal/lockfile"
	"github.com/juanbzz/goetry/internal/pyproject"
	"github.com/spf13/cobra"
)

func newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Validate project consistency",
		Long:  "Checks that pyproject.toml and poetry.lock are consistent.",
		Example: `  goetry check`,
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
	lockPath := filepath.Join(dir, "poetry.lock")

	// Read pyproject.toml.
	proj, err := pyproject.ReadPyProject(pyprojectPath)
	if err != nil {
		return fmt.Errorf("read pyproject.toml: %w", err)
	}

	// Read poetry.lock.
	lf, err := lockfile.ReadLockFile(lockPath)
	if err != nil {
		return fmt.Errorf("read poetry.lock: %w (run 'goetry lock' first)", err)
	}

	var issues []string

	// Check content hash.
	currentHash := computeContentHash(pyprojectPath)
	if currentHash != "" && lf.Metadata.ContentHash != "" && currentHash != lf.Metadata.ContentHash {
		issues = append(issues, "poetry.lock is out of date (content hash mismatch). Run \"goetry lock\" to update.")
	}

	// Check all direct deps are present in lock.
	deps, err := proj.ResolveDependencies()
	if err != nil {
		return fmt.Errorf("resolve dependencies: %w", err)
	}

	lockedNames := make(map[string]bool, len(lf.Packages))
	for _, p := range lf.Packages {
		lockedNames[normalizeName(p.Name)] = true
	}

	for _, dep := range deps {
		if !lockedNames[normalizeName(dep.Name)] {
			issues = append(issues, fmt.Sprintf("dependency %q is in pyproject.toml but missing from poetry.lock.", dep.Name))
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
