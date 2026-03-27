package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juanbzz/pensa/internal/index"
	"github.com/juanbzz/pensa/internal/lockfile"
	"github.com/juanbzz/pensa/internal/pyproject"
	"github.com/juanbzz/pensa/internal/python"
	"github.com/juanbzz/pensa/internal/workspace"
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
	cmd.Flags().String("package", "", "Target workspace member (by name)")
	return cmd
}

func runAdd(cmd *cobra.Command, args []string) error {
	out := uiFromCmd(cmd)
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	pkgFlag, _ := cmd.Flags().GetString("package")
	group, _ := cmd.Flags().GetString("group")

	// Resolve workspace + target member.
	ws, _ := workspace.Discover(dir)
	member, err := resolveTargetMember(ws, pkgFlag, dir)
	if err != nil {
		return err
	}

	// Determine which pyproject to modify.
	var pyprojectPath string
	var proj *pyproject.PyProject
	if member != nil {
		pyprojectPath = filepath.Join(member.Path, "pyproject.toml")
		proj = member.Project
	} else {
		pyprojectPath = filepath.Join(dir, "pyproject.toml")
		proj, err = pyproject.ReadPyProject(pyprojectPath)
		if err != nil {
			return fmt.Errorf("read pyproject.toml: %w", err)
		}
	}

	client, err := newPyPIClient()
	if err != nil {
		return err
	}

	lockDir := dir
	if ws != nil {
		lockDir = ws.Root
	}

	type addedPkg struct {
		name   string
		extras []string
	}
	var addedNames []addedPkg

	for _, arg := range args {
		name, constraintStr, extras, err := parseAddArg(arg)
		if err != nil {
			return err
		}
		name = pep508.NormalizeName(name)

		if constraintStr == "" {
			var requiresPython version.Constraint
			if proj.HasProjectSection() && proj.Project.RequiresPython != "" {
				requiresPython, _ = version.ParseConstraint(proj.Project.RequiresPython)
			}
			latest, err := getLatestCompatibleVersion(client, name, requiresPython)
			if err != nil {
				return fmt.Errorf("find latest version of %s: %w", name, err)
			}
			constraintStr = fmt.Sprintf("^%s", latest)
			// Version discovery is silent; resolved version shown after lock.
		}

		if group != "" {
			addToDependencyGroup(proj, name, constraintStr, extras, group)
		} else {
			addToProjectWithExtras(proj, name, constraintStr, extras)
		}

		addedNames = append(addedNames, addedPkg{name: name, extras: extras})
	}

	if err := pyproject.WritePyProject(pyprojectPath, proj); err != nil {
		return fmt.Errorf("write pyproject.toml: %w", err)
	}

	// Re-lock: entire workspace or single project.
	if ws != nil {
		if member != nil {
			member.Project, _ = pyproject.ReadPyProject(pyprojectPath)
		}
		if err := runLockWorkspace(os.Stderr, ws, lockOptions{}); err != nil {
			return err
		}
	} else {
		proj, err = pyproject.ReadPyProject(pyprojectPath)
		if err != nil {
			return fmt.Errorf("re-read pyproject.toml: %w", err)
		}
		if err := resolveAndLock(os.Stderr, proj, pyprojectPath, lockOptions{}); err != nil {
			return err
		}
	}

	if err := installFromLock(os.Stderr, true, nil); err != nil {
		return err
	}

	// Show what was added (resolved versions from lock file).
	lockDir = dir
	if ws != nil {
		lockDir = ws.Root
	}
	if lockPath, _ := lockfile.DetectLockFile(lockDir); lockPath != "" {
		if lf, err := lockfile.ReadLockFile(lockPath); err == nil {
			lockedPkgs := make(map[string]lockfile.LockedPackage, len(lf.Packages))
			for _, p := range lf.Packages {
				lockedPkgs[normalizeName(p.Name)] = p
			}
			// Only show diff for packages that would actually be installed
			// (compatible with current Python).
			py, _ := python.Discover()
			for _, pkg := range addedNames {
				lp, ok := lockedPkgs[normalizeName(pkg.name)]
				if !ok {
					continue
				}
				if py != nil && !compatibleWithPython(lp, py) {
					out.Warning(fmt.Sprintf("%s==%s requires Python %s (you have %s)",
						lp.Name, lp.Version, lp.PythonVersions, py.Version))
					continue
				}
				displayName := pkg.name
				if len(pkg.extras) > 0 {
					displayName += "[" + strings.Join(pkg.extras, ",") + "]"
				}
				out.DiffAdd(displayName, lp.Version)
			}
		}
	}

	return nil
}

// parseAddArg parses "name", "name@constraint", or "name[extras]@constraint".
func parseAddArg(s string) (name, constraint string, extras []string, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", nil, fmt.Errorf("empty package name")
	}

	// Split on @ for Poetry-style constraint (e.g. sqlalchemy@^2.0).
	if i := strings.Index(s, "@"); i >= 0 {
		name = s[:i]
		constraint = s[i+1:]
	} else if i := findPEP508Operator(s); i >= 0 {
		// Split on PEP 508 operator (e.g. sqlalchemy>=2.0).
		name = s[:i]
		constraint = s[i:]
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

// findPEP508Operator returns the index of the first PEP 508 version operator
// in s, or -1 if none found. Checks two-char operators before one-char.
func findPEP508Operator(s string) int {
	for _, op := range []string{">=", "<=", "!=", "~=", "=="} {
		if i := strings.Index(s, op); i > 0 {
			return i
		}
	}
	for _, op := range []string{">", "<"} {
		if i := strings.Index(s, op); i > 0 {
			return i
		}
	}
	return -1
}

// getLatestVersion queries PyPI for the latest stable version of a package.
// getLatestCompatibleVersion returns the latest stable version that's compatible
// with the project's requires-python. Falls back to latest stable if no filter.
func getLatestCompatibleVersion(client *index.PyPIClient, name string, requiresPython version.Constraint) (version.Version, error) {
	info, err := client.GetPackageInfo(name)
	if err != nil {
		return version.Version{}, err
	}
	versions := info.Versions()
	if len(versions) == 0 {
		return version.Version{}, fmt.Errorf("no versions found for %s", name)
	}

	sort.Slice(versions, func(i, j int) bool {
		return version.Compare(versions[i], versions[j]) > 0
	})

	for _, v := range versions {
		if !v.IsStable() {
			continue
		}
		if requiresPython != nil {
			if files := info.FilesForVersion(v); len(files) > 0 {
				if rp := files[0].RequiresPython; rp != "" {
					if !pythonRangesOverlap(requiresPython, rp) {
						continue
					}
				}
			}
		}
		return v, nil
	}

	// Fallback: return latest stable without filtering.
	for _, v := range versions {
		if v.IsStable() {
			return v, nil
		}
	}
	return versions[0], nil
}

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

func lockedVersion(lf *lockfile.LockFile, name string) string {
	if lf == nil {
		return ""
	}
	for _, pkg := range lf.Packages {
		if pep508.NormalizeName(pkg.Name) == name {
			return pkg.Version
		}
	}
	return ""
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
