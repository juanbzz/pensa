package pyproject

import (
	"fmt"
	"strings"

	"github.com/juanbzz/pensa/pkg/pep508"
	"github.com/juanbzz/pensa/pkg/version"
)

// ParsePoetryDependency converts a Poetry-style dependency entry into a Dependency.
// name is the package name, value is the TOML value (string or map[string]interface{}).
func ParsePoetryDependency(name string, value interface{}) (pep508.Dependency, error) {
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

	case map[string]interface{}:
		return parsePoetryTable(dep, v)

	default:
		return dep, fmt.Errorf("parse poetry dependency %q: unexpected type %T", name, value)
	}
}

func parsePoetryTable(dep pep508.Dependency, table map[string]interface{}) (pep508.Dependency, error) {
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
		case []interface{}:
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

// ResolveDependencies returns a unified list of dependencies from the pyproject.
// If [project.dependencies] exists, those are the source of truth.
// [tool.poetry.dependencies] enriches or replaces if project deps are absent.
func (p *PyProject) ResolveDependencies() ([]pep508.Dependency, error) {
	if p.HasProjectSection() && len(p.Project.Dependencies) > 0 {
		var deps []pep508.Dependency
		for _, s := range p.Project.Dependencies {
			d, err := pep508.Parse(s)
			if err != nil {
				return nil, fmt.Errorf("parse project dependency: %w", err)
			}
			deps = append(deps, d)
		}
		return deps, nil
	}

	if p.HasPoetrySection() && len(p.Tool.Poetry.Dependencies) > 0 {
		var deps []pep508.Dependency
		for name, value := range p.Tool.Poetry.Dependencies {
			if strings.ToLower(name) == "python" {
				continue
			}
			d, err := ParsePoetryDependency(name, value)
			if err != nil {
				return nil, err
			}
			deps = append(deps, d)
		}
		return deps, nil
	}

	return nil, nil
}
