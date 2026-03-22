package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juanbzz/goetry/internal/index"
	"github.com/juanbzz/goetry/internal/pyproject"
	"github.com/juanbzz/goetry/pkg/pep508"
	"github.com/juanbzz/goetry/pkg/version"
	"github.com/spf13/cobra"
)

func newAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add [packages...]",
		Short: "Add dependencies to pyproject.toml and lock them",
		Long: `Adds one or more packages to pyproject.toml and resolves dependencies.

Specify packages as:
  goetry add requests              # latest version, caret constraint
  goetry add requests@^2.28       # explicit constraint
  goetry add requests flask        # multiple packages`,
		Args: cobra.MinimumNArgs(1),
		RunE: runAdd,
	}
}

func runAdd(cmd *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	pyprojectPath := filepath.Join(dir, "pyproject.toml")
	proj, err := pyproject.ReadPyProject(pyprojectPath)
	if err != nil {
		return fmt.Errorf("read pyproject.toml: %w", err)
	}

	client, err := newPyPIClient()
	if err != nil {
		return err
	}

	for _, arg := range args {
		name, constraintStr, err := parseAddArg(arg)
		if err != nil {
			return err
		}
		name = pep508.NormalizeName(name)

		// If no constraint specified, find latest version and use caret.
		if constraintStr == "" {
			latest, err := getLatestVersion(client, name)
			if err != nil {
				return fmt.Errorf("find latest version of %s: %w", name, err)
			}
			constraintStr = fmt.Sprintf("^%s", latest)
			fmt.Fprintf(cmd.OutOrStdout(), "Using version %s for %s\n", constraintStr, name)
		}

		addToProject(proj, name, constraintStr)
		fmt.Fprintf(cmd.OutOrStdout(), "Adding %s (%s)\n", name, constraintStr)
	}

	// Write updated pyproject.toml.
	if err := pyproject.WritePyProject(pyprojectPath, proj); err != nil {
		return fmt.Errorf("write pyproject.toml: %w", err)
	}

	// Re-read to pick up the changes (WritePyProject may normalize).
	proj, err = pyproject.ReadPyProject(pyprojectPath)
	if err != nil {
		return fmt.Errorf("re-read pyproject.toml: %w", err)
	}

	// Resolve and lock.
	return resolveAndLock(cmd.OutOrStdout(), proj, pyprojectPath)
}

// parseAddArg parses "name" or "name@constraint" into components.
func parseAddArg(s string) (name, constraint string, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", fmt.Errorf("empty package name")
	}
	if i := strings.Index(s, "@"); i >= 0 {
		return s[:i], s[i+1:], nil
	}
	return s, "", nil
}

// getLatestVersion queries PyPI for the latest stable version of a package.
func getLatestVersion(client *index.PyPIClient, name string) (version.Version, error) {
	info, err := client.GetPackageInfo(name)
	if err != nil {
		return version.Version{}, err
	}
	versions := info.Versions()
	if len(versions) == 0 {
		return version.Version{}, fmt.Errorf("no versions found for %s", name)
	}

	// Sort newest first.
	sort.Slice(versions, func(i, j int) bool {
		return version.Compare(versions[i], versions[j]) > 0
	})

	// Pick the latest stable version.
	for _, v := range versions {
		if v.IsStable() {
			return v, nil
		}
	}
	// Fall back to latest (even if pre-release).
	return versions[0], nil
}

// addToProject adds a dependency to the appropriate section of pyproject.toml.
func addToProject(proj *pyproject.PyProject, name, constraint string) {
	if proj.HasProjectSection() {
		// PEP 621: append PEP 508 string.
		depStr := name + constraint
		// Check if already exists — update if so.
		for i, existing := range proj.Project.Dependencies {
			dep, err := pep508.Parse(existing)
			if err == nil && dep.Name == name {
				proj.Project.Dependencies[i] = depStr
				return
			}
		}
		proj.Project.Dependencies = append(proj.Project.Dependencies, depStr)
	} else if proj.HasPoetrySection() {
		// Poetry format: add to [tool.poetry.dependencies].
		if proj.Tool.Poetry.Dependencies == nil {
			proj.Tool.Poetry.Dependencies = make(map[string]interface{})
		}
		proj.Tool.Poetry.Dependencies[name] = constraint
	} else {
		// No section exists — create [project.dependencies].
		if proj.Project == nil {
			proj.Project = &pyproject.ProjectTable{
				Name: proj.Name(),
			}
		}
		proj.Project.Dependencies = append(proj.Project.Dependencies, name+constraint)
	}
}
