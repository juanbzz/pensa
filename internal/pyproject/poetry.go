package pyproject

import (
	"fmt"
	"strings"

	"pensa.sh/pensa/pkg/pep508"
	"pensa.sh/pensa/pkg/version"
)

// ParsePoetryDependency converts a Poetry-style dependency entry into a Dependency.
// name is the package name, value is the TOML value (string or map[string]any).
func ParsePoetryDependency(name string, value any) (pep508.Dependency, error) {
	dep := pep508.Dependency{
		Name: pep508.NormalizeName(name),
	}

	switch v := value.(type) {
	case string:
		c, err := version.ParseConstraint(v)
		if err != nil {
			return dep, fmt.Errorf("parse poetry dependency %q: %w", name, err)
		}
		dep.Constraint = c
		return dep, nil

	case map[string]any:
		return parsePoetryTable(dep, v)

	default:
		return dep, fmt.Errorf("parse poetry dependency %q: unexpected type %T", name, value)
	}
}

func parsePoetryTable(dep pep508.Dependency, table map[string]any) (pep508.Dependency, error) {
	if v, ok := table["version"]; ok {
		if vs, ok := v.(string); ok {
			c, err := version.ParseConstraint(vs)
			if err != nil {
				return dep, fmt.Errorf("parse poetry dependency %q version: %w", dep.Name, err)
			}
			dep.Constraint = c
		}
	}

	if v, ok := table["extras"]; ok {
		switch extras := v.(type) {
		case []any:
			for _, e := range extras {
				if s, ok := e.(string); ok {
					dep.Extras = append(dep.Extras, pep508.NormalizeName(s))
				}
			}
		}
	}

	if v, ok := table["url"]; ok {
		if s, ok := v.(string); ok {
			dep.URL = s
		}
	}

	if v, ok := table["path"]; ok {
		if s, ok := v.(string); ok {
			dep.URL = "file://" + s
		}
	}

	if v, ok := table["git"]; ok {
		if s, ok := v.(string); ok {
			gitURL := s
			if ref, ok := table["branch"]; ok {
				if rs, ok := ref.(string); ok {
					gitURL += "@" + rs
				}
			} else if ref, ok := table["tag"]; ok {
				if rs, ok := ref.(string); ok {
					gitURL += "@" + rs
				}
			} else if ref, ok := table["rev"]; ok {
				if rs, ok := ref.(string); ok {
					gitURL += "@" + rs
				}
			}
			dep.URL = "git+" + gitURL
		}
	}

	if v, ok := table["python"]; ok {
		if s, ok := v.(string); ok {
			marker, err := pythonConstraintToMarker(s)
			if err != nil {
				return dep, fmt.Errorf("parse poetry dependency %q python constraint: %w", dep.Name, err)
			}
			dep.Markers = marker
		}
	}

	if v, ok := table["markers"]; ok {
		if s, ok := v.(string); ok {
			m, err := pep508.ParseMarker(s)
			if err != nil {
				return dep, fmt.Errorf("parse poetry dependency %q markers: %w", dep.Name, err)
			}
			dep.Markers = m
		}
	}

	return dep, nil
}

func pythonConstraintToMarker(s string) (pep508.Marker, error) {
	c, err := version.ParseConstraint(s)
	if err != nil {
		return nil, err
	}

	markerStr := constraintToMarkerString(c)
	if markerStr == "" {
		return pep508.AnyMarker{}, nil
	}
	return pep508.ParseMarker(markerStr)
}

func constraintToMarkerString(c version.Constraint) string {
	switch ct := c.(type) {
	case *version.Range:
		var parts []string
		if ct.Min() != nil {
			op := ">="
			if !ct.IncludeMin() {
				op = ">"
			}
			parts = append(parts, fmt.Sprintf("python_version %s %q", op, formatPythonVersion(*ct.Min())))
		}
		if ct.Max() != nil {
			op := "<="
			if !ct.IncludeMax() {
				op = "<"
			}
			parts = append(parts, fmt.Sprintf("python_version %s %q", op, formatPythonVersion(*ct.Max())))
		}
		return strings.Join(parts, " and ")
	default:
		return fmt.Sprintf("python_version == %q", c.String())
	}
}

func formatPythonVersion(v version.Version) string {
	if len(v.Release()) >= 2 {
		return fmt.Sprintf("%d.%d", v.Major(), v.Minor())
	}
	return fmt.Sprintf("%d", v.Major())
}

// GroupedDependency is a dependency with its group label.
type GroupedDependency struct {
	Dep   pep508.Dependency
	Group string // "main", "dev", "test", etc.
}

// ResolveDependencies returns main group dependencies only.
// Backwards-compatible wrapper around ResolveDependenciesForGroups.
func (p *PyProject) ResolveDependencies() ([]pep508.Dependency, error) {
	grouped, err := p.ResolveDependenciesForGroups([]string{"main"})
	if err != nil {
		return nil, err
	}
	deps := make([]pep508.Dependency, len(grouped))
	for i, g := range grouped {
		deps[i] = g.Dep
	}
	return deps, nil
}

// ResolveAllDependencies returns dependencies from all groups (main + all named groups).
// PEP 735 [dependency-groups] takes precedence over [tool.poetry.group.X].
func (p *PyProject) ResolveAllDependencies() ([]GroupedDependency, error) {
	groups := []string{"main"}
	if p.DependencyGroups != nil {
		for name := range p.DependencyGroups {
			groups = append(groups, name)
		}
	} else if p.HasPoetrySection() && p.Tool.Poetry.Groups != nil {
		for name := range p.Tool.Poetry.Groups {
			groups = append(groups, name)
		}
	}
	return p.ResolveDependenciesForGroups(groups)
}

// ResolveDependenciesForGroups returns dependencies for the specified groups.
func (p *PyProject) ResolveDependenciesForGroups(groups []string) ([]GroupedDependency, error) {
	var result []GroupedDependency

	groupSet := make(map[string]bool, len(groups))
	for _, g := range groups {
		groupSet[g] = true
	}

	// Main group.
	if groupSet["main"] {
		if p.HasProjectSection() && len(p.Project.Dependencies) > 0 {
			for _, s := range p.Project.Dependencies {
				d, err := pep508.Parse(s)
				if err != nil {
					return nil, fmt.Errorf("parse project dependency: %w", err)
				}
				result = append(result, GroupedDependency{Dep: d, Group: "main"})
			}
		} else if p.HasPoetrySection() && len(p.Tool.Poetry.Dependencies) > 0 {
			for name, value := range p.Tool.Poetry.Dependencies {
				if strings.ToLower(name) == "python" {
					continue
				}
				d, err := ParsePoetryDependency(name, value)
				if err != nil {
					return nil, err
				}
				result = append(result, GroupedDependency{Dep: d, Group: "main"})
			}
		}
	}

	// Named groups — PEP 735 takes precedence over Poetry groups.
	if p.DependencyGroups != nil {
		for groupName := range groupSet {
			if groupName == "main" {
				continue
			}
			if _, ok := p.DependencyGroups[groupName]; !ok {
				continue
			}
			deps, err := p.resolvePEP735Group(groupName, groupName, make(map[string]bool))
			if err != nil {
				return nil, err
			}
			result = append(result, deps...)
		}
	} else if p.HasPoetrySection() && p.Tool.Poetry.Groups != nil {
		for groupName, group := range p.Tool.Poetry.Groups {
			if !groupSet[groupName] {
				continue
			}
			for name, value := range group.Dependencies {
				if strings.ToLower(name) == "python" {
					continue
				}
				d, err := ParsePoetryDependency(name, value)
				if err != nil {
					return nil, err
				}
				result = append(result, GroupedDependency{Dep: d, Group: groupName})
			}
		}
	}

	return result, nil
}

// resolvePEP735Group resolves a PEP 735 dependency group, expanding include-group references.
// originGroup is the top-level group name (for labeling). seen tracks visited groups for cycle detection.
func (p *PyProject) resolvePEP735Group(groupName, originGroup string, seen map[string]bool) ([]GroupedDependency, error) {
	if seen[groupName] {
		return nil, fmt.Errorf("circular dependency group reference: %s", groupName)
	}
	seen[groupName] = true
	defer delete(seen, groupName)

	entries, ok := p.DependencyGroups[groupName]
	if !ok {
		return nil, fmt.Errorf("dependency group %q not found", groupName)
	}

	var result []GroupedDependency
	for _, entry := range entries {
		switch v := entry.(type) {
		case string:
			d, err := pep508.Parse(v)
			if err != nil {
				return nil, fmt.Errorf("parse dependency in group %q: %w", groupName, err)
			}
			result = append(result, GroupedDependency{Dep: d, Group: originGroup})
		case map[string]any:
			if includeGroup, ok := v["include-group"]; ok {
				if name, ok := includeGroup.(string); ok {
					deps, err := p.resolvePEP735Group(name, originGroup, seen)
					if err != nil {
						return nil, err
					}
					result = append(result, deps...)
				}
			}
		}
	}
	return result, nil
}
