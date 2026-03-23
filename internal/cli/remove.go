package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juanbzz/goetry/internal/pyproject"
	"github.com/juanbzz/goetry/pkg/pep508"
	"github.com/spf13/cobra"
)

func newRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove [packages...]",
		Short: "Remove dependencies from the project",
		Long:  "Removes one or more packages from pyproject.toml, re-locks, and re-installs.",
		Example: `  goetry remove requests
  goetry remove requests flask`,
		Args: cobra.MinimumNArgs(1),
		RunE: runRemove,
	}
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

	for _, arg := range args {
		name := pep508.NormalizeName(arg)
		if err := removeFromProject(proj, name); err != nil {
			return err
		}
		fmt.Fprintf(w, "Removing %s\n", name)
	}

	if err := pyproject.WritePyProject(pyprojectPath, proj); err != nil {
		return fmt.Errorf("write pyproject.toml: %w", err)
	}

	// Re-read to pick up normalized changes.
	proj, err = pyproject.ReadPyProject(pyprojectPath)
	if err != nil {
		return fmt.Errorf("re-read pyproject.toml: %w", err)
	}

	// Check if any deps remain.
	deps, err := proj.ResolveDependencies()
	if err != nil {
		return fmt.Errorf("resolve dependencies: %w", err)
	}

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
	if err := resolveAndLock(w, proj, pyprojectPath); err != nil {
		return err
	}
	return installFromLock(w)
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
