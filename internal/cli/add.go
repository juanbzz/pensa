package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juanbzz/pensa/internal/index"
	"github.com/juanbzz/pensa/internal/pyproject"
	"github.com/juanbzz/pensa/pkg/pep508"
	"github.com/juanbzz/pensa/pkg/version"
	"github.com/spf13/cobra"
)

func newAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [packages...]",
		Short: "Add dependencies to pyproject.toml and lock them",
		Long: `Adds one or more packages to pyproject.toml and resolves dependencies.

Specify packages as:
  pensa add requests              # latest version, caret constraint
  pensa add requests@^2.28       # explicit constraint
  pensa add requests flask        # multiple packages
  pensa add pytest -G dev         # add to dev group`,
		Args: cobra.MinimumNArgs(1),
		RunE: runAdd,
	}
	cmd.Flags().StringP("group", "G", "", "Dependency group (e.g., dev, test)")
	return cmd
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

	group, _ := cmd.Flags().GetString("group")

	for _, arg := range args {
		name, constraintStr, extras, err := parseAddArg(arg)
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
			fmt.Fprintf(cmd.OutOrStdout(), "Using version %s for %s\n", bold(constraintStr), bold(name))
		}

		if group != "" {
			addToDependencyGroup(proj, name, constraintStr, extras, group)
		} else {
			addToProjectWithExtras(proj, name, constraintStr, extras)
		}

		displayName := name
		if len(extras) > 0 {
			displayName = name + "[" + strings.Join(extras, ",") + "]"
		}
		groupLabel := ""
		if group != "" {
			groupLabel = " " + dim("("+group+" group)")
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s%s\n", green("Adding"), bold(displayName), dim("("+constraintStr+")"), groupLabel)
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
	if err := resolveAndLock(cmd.OutOrStdout(), proj, pyprojectPath, lockOptions{}); err != nil {
		return err
	}

	// Install packages.
	return installFromLock(cmd.OutOrStdout(), true, nil)
}

// parseAddArg parses "name", "name@constraint", or "name[extras]@constraint".
func parseAddArg(s string) (name, constraint string, extras []string, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", nil, fmt.Errorf("empty package name")
	}

	// Split on @ for constraint.
	if i := strings.Index(s, "@"); i >= 0 {
		name = s[:i]
		constraint = s[i+1:]
	} else {
		name = s
	}

	// Extract extras from name: requests[security,tests] → requests, [security, tests]
	if bracketStart := strings.Index(name, "["); bracketStart >= 0 {
		bracketEnd := strings.Index(name, "]")
		if bracketEnd < 0 {
			return "", "", nil, fmt.Errorf("unclosed bracket in %q", s)
		}
		extrasStr := name[bracketStart+1 : bracketEnd]
		name = name[:bracketStart]
		for _, e := range strings.Split(extrasStr, ",") {
			e = strings.TrimSpace(e)
			if e != "" {
				extras = append(extras, e)
			}
		}
	}

	return name, constraint, extras, nil
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
		// PEP 621: convert Poetry constraint (^, ~) to PEP 508 range.
		depStr := name + toPEP508(constraint)

		// Sanitize all existing deps to PEP 508 (fix any previously written caret/tilde syntax).
		sanitizePEP621Deps(proj)

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
		proj.Project.Dependencies = append(proj.Project.Dependencies, name+toPEP508(constraint))
	}
}

// sanitizePEP621Deps converts any Poetry-style constraints in [project.dependencies]
// to valid PEP 508 format. Fixes deps that were written with ^/~ syntax.
func sanitizePEP621Deps(proj *pyproject.PyProject) {
	if proj.Project == nil {
		return
	}
	for i, dep := range proj.Project.Dependencies {
		// Try to parse as PEP 508. If it fails, it might have caret/tilde syntax.
		if _, err := pep508.Parse(dep); err != nil {
			// Extract name and constraint, convert constraint.
			name := depNameFromPEP508(dep)
			constraint := strings.TrimSpace(dep[len(name):])
			if constraint != "" {
				proj.Project.Dependencies[i] = name + toPEP508(constraint)
			}
		}
	}
}

// addToGroup adds a dependency to a named group in [tool.poetry.group.{group}.dependencies].
func addToGroup(proj *pyproject.PyProject, name, constraint, group string) {
	// Ensure poetry section exists.
	if proj.Tool == nil {
		proj.Tool = &pyproject.ToolTable{}
	}
	if proj.Tool.Poetry == nil {
		proj.Tool.Poetry = &pyproject.PoetryTable{}
	}
	if proj.Tool.Poetry.Groups == nil {
		proj.Tool.Poetry.Groups = make(map[string]pyproject.PoetryGroup)
	}
	g := proj.Tool.Poetry.Groups[group]
	if g.Dependencies == nil {
		g.Dependencies = make(map[string]interface{})
	}
	g.Dependencies[name] = constraint
	proj.Tool.Poetry.Groups[group] = g
}

// addToProjectWithExtras adds a dependency with optional extras.
func addToProjectWithExtras(proj *pyproject.PyProject, name, constraint string, extras []string) {
	if len(extras) == 0 {
		addToProject(proj, name, constraint)
		return
	}

	if proj.HasProjectSection() {
		// PEP 621: include extras in dep string: requests[security]>=2.28,<3
		extrasStr := "[" + strings.Join(extras, ",") + "]"
		depStr := name + extrasStr + toPEP508(constraint)
		sanitizePEP621Deps(proj)
		for i, existing := range proj.Project.Dependencies {
			dep, err := pep508.Parse(existing)
			if err == nil && dep.Name == name {
				proj.Project.Dependencies[i] = depStr
				return
			}
		}
		proj.Project.Dependencies = append(proj.Project.Dependencies, depStr)
	} else if proj.HasPoetrySection() {
		// Poetry table format: {version = "^2.28", extras = ["security"]}
		if proj.Tool.Poetry.Dependencies == nil {
			proj.Tool.Poetry.Dependencies = make(map[string]interface{})
		}
		proj.Tool.Poetry.Dependencies[name] = map[string]interface{}{
			"version": constraint,
			"extras":  extras,
		}
	} else {
		if proj.Project == nil {
			proj.Project = &pyproject.ProjectTable{Name: proj.Name()}
		}
		extrasStr := "[" + strings.Join(extras, ",") + "]"
		proj.Project.Dependencies = append(proj.Project.Dependencies, name+extrasStr+toPEP508(constraint))
	}
}

// addToGroupWithExtras adds a dependency with optional extras to a named group.
func addToGroupWithExtras(proj *pyproject.PyProject, name, constraint, group string, extras []string) {
	if len(extras) == 0 {
		addToGroup(proj, name, constraint, group)
		return
	}
	if proj.Tool == nil {
		proj.Tool = &pyproject.ToolTable{}
	}
	if proj.Tool.Poetry == nil {
		proj.Tool.Poetry = &pyproject.PoetryTable{}
	}
	if proj.Tool.Poetry.Groups == nil {
		proj.Tool.Poetry.Groups = make(map[string]pyproject.PoetryGroup)
	}
	g := proj.Tool.Poetry.Groups[group]
	if g.Dependencies == nil {
		g.Dependencies = make(map[string]interface{})
	}
	g.Dependencies[name] = map[string]interface{}{
		"version": constraint,
		"extras":  extras,
	}
	proj.Tool.Poetry.Groups[group] = g
}

// addToDependencyGroup adds a dep to [dependency-groups] (PEP 735 format).
func addToDependencyGroup(proj *pyproject.PyProject, name, constraint string, extras []string, group string) {
	if proj.DependencyGroups == nil {
		proj.DependencyGroups = make(map[string][]interface{})
	}

	depStr := name
	if len(extras) > 0 {
		depStr += "[" + strings.Join(extras, ",") + "]"
	}
	depStr += toPEP508(constraint)

	// Check if already exists — update in place.
	normalized := pep508.NormalizeName(name)
	entries := proj.DependencyGroups[group]
	for i, entry := range entries {
		if s, ok := entry.(string); ok {
			d, err := pep508.Parse(s)
			if err == nil && pep508.NormalizeName(d.Name) == normalized {
				entries[i] = depStr
				proj.DependencyGroups[group] = entries
				return
			}
		}
	}

	proj.DependencyGroups[group] = append(entries, depStr)
}

// toPEP508 converts a Poetry-style constraint (^2.32.5, ~1.0) to PEP 508 format (>=2.32.5,<3).
// If the constraint is already PEP 508 compatible, it's returned as-is.
func toPEP508(constraint string) string {
	if constraint == "" {
		return ""
	}
	c, err := version.ParseConstraint(constraint)
	if err != nil {
		return constraint // can't parse, return as-is
	}
	return c.String()
}
