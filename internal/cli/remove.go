package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juanbzz/pensa/internal/pyproject"
	"github.com/juanbzz/pensa/pkg/pep508"
	"github.com/spf13/cobra"
)

func newRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove [packages...]",
		Short: "Remove dependencies from the project",
		Long:  "Removes one or more packages from pyproject.toml, re-locks, and re-installs.",
		Example: `  pensa remove requests
  pensa remove requests flask
  pensa remove pytest -G dev`,
		Args: cobra.MinimumNArgs(1),
		RunE: runRemove,
	}
	cmd.Flags().StringP("group", "G", "", "Dependency group (e.g., dev, test)")
	return cmd
}

func runRemove(cmd *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	pyprojectPath := filepath.Join(dir, "pyproject.toml")
	proj, err := pyproject.ReadPyProject(pyprojectPath)
	if err != nil {
		return fmt.Errorf("read pyproject.toml: %w", err)
	}

	w := cmd.OutOrStdout()

	group, _ := cmd.Flags().GetString("group")

	for _, arg := range args {
		name := pep508.NormalizeName(arg)
		if group != "" {
			if err := removeFromDependencyGroup(proj, name, group); err != nil {
				return err
			}
		} else {
			if err := removeFromProject(proj, name); err != nil {
				return err
			}
		}
		fmt.Fprintf(w, "%s %s\n", yellow("Removing"), bold(name))
	}

	if err := pyproject.WritePyProject(pyprojectPath, proj); err != nil {
		return fmt.Errorf("write pyproject.toml: %w", err)
	}

	// Re-read to pick up normalized changes.
	proj, err = pyproject.ReadPyProject(pyprojectPath)
	if err != nil {
		return fmt.Errorf("re-read pyproject.toml: %w", err)
	}

	// Check if any deps remain (across all groups).
	allDeps, err := proj.ResolveAllDependencies()
	if err != nil {
		return fmt.Errorf("resolve dependencies: %w", err)
	}
	deps := allDeps

	lockPath := filepath.Join(dir, "poetry.lock")

	if len(deps) == 0 {
		// No deps left — remove lock file if it exists.
		if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove poetry.lock: %w", err)
		}
		fmt.Fprintf(w, "No dependencies remaining.\n")
		return nil
	}

	// Re-lock and re-install.
	if err := resolveAndLock(w, proj, pyprojectPath, lockOptions{}); err != nil {
		return err
	}
	return installFromLock(w, true, nil)
}

// removeFromProject removes a dependency from the appropriate section of pyproject.toml.
func removeFromProject(proj *pyproject.PyProject, name string) error {
	normalized := normalizeName(name)

	if proj.HasProjectSection() && len(proj.Project.Dependencies) > 0 {
		for i, existing := range proj.Project.Dependencies {
			dep, err := pep508.Parse(existing)
			if err != nil {
				continue
			}
			if normalizeName(dep.Name) == normalized {
				proj.Project.Dependencies = append(proj.Project.Dependencies[:i], proj.Project.Dependencies[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("%q is not a dependency in [project.dependencies]", name)
	}

	if proj.HasPoetrySection() && len(proj.Tool.Poetry.Dependencies) > 0 {
		for key := range proj.Tool.Poetry.Dependencies {
			if normalizeName(key) == normalized && normalizeName(key) != "python" {
				delete(proj.Tool.Poetry.Dependencies, key)
				return nil
			}
		}
		return fmt.Errorf("%q is not a dependency in [tool.poetry.dependencies]", name)
	}

	return fmt.Errorf("%q is not a dependency", name)
}

// removeFromDependencyGroup removes a dependency from a PEP 735 or Poetry group.
// Tries PEP 735 [dependency-groups] first, falls back to Poetry groups.
func removeFromDependencyGroup(proj *pyproject.PyProject, name, group string) error {
	normalized := normalizeName(name)

	// Try PEP 735 first.
	if proj.DependencyGroups != nil {
		entries, ok := proj.DependencyGroups[group]
		if ok {
			for i, entry := range entries {
				if s, ok := entry.(string); ok {
					d, err := pep508.Parse(s)
					if err == nil && normalizeName(d.Name) == normalized {
						proj.DependencyGroups[group] = append(entries[:i], entries[i+1:]...)
						return nil
					}
				}
			}
			return fmt.Errorf("%q is not a dependency in group %q", name, group)
		}
	}

	// Fall back to Poetry groups.
	return removeFromGroup(proj, name, group)
}

// removeFromGroup removes a dependency from a Poetry named group.
func removeFromGroup(proj *pyproject.PyProject, name, group string) error {
	normalized := normalizeName(name)

	if !proj.HasPoetrySection() || proj.Tool.Poetry.Groups == nil {
		return fmt.Errorf("%q is not a dependency in group %q", name, group)
	}

	g, ok := proj.Tool.Poetry.Groups[group]
	if !ok || len(g.Dependencies) == 0 {
		return fmt.Errorf("%q is not a dependency in group %q", name, group)
	}

	for key := range g.Dependencies {
		if normalizeName(key) == normalized {
			delete(g.Dependencies, key)
			proj.Tool.Poetry.Groups[group] = g
			return nil
		}
	}

	return fmt.Errorf("%q is not a dependency in group %q", name, group)
}
