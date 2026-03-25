package pyproject

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// PyProject represents a parsed pyproject.toml.
type PyProject struct {
	Project          *ProjectTable            `toml:"project,omitempty"`
	Tool             *ToolTable               `toml:"tool,omitempty"`
	BuildSystem      *BuildSystem             `toml:"build-system,omitempty"`
	DependencyGroups map[string][]interface{} `toml:"dependency-groups,omitempty"`
}

// ProjectTable represents the [project] section (PEP 621).
type ProjectTable struct {
	Name                 string                        `toml:"name"`
	Version              string                        `toml:"version,omitempty"`
	Description          string                        `toml:"description,omitempty"`
	RequiresPython       string                        `toml:"requires-python,omitempty"`
	License              string                        `toml:"license,omitempty"`
	Authors              []Author                      `toml:"authors,omitempty"`
	Maintainers          []Author                      `toml:"maintainers,omitempty"`
	Keywords             []string                      `toml:"keywords,omitempty"`
	Classifiers          []string                      `toml:"classifiers,omitempty"`
	URLs                 map[string]string             `toml:"urls,omitempty"`
	Dependencies         []string                      `toml:"dependencies,omitempty"`
	OptionalDependencies map[string][]string           `toml:"optional-dependencies,omitempty"`
	Scripts              map[string]string             `toml:"scripts,omitempty"`
	GUIScripts           map[string]string             `toml:"gui-scripts,omitempty"`
	EntryPoints          map[string]map[string]string  `toml:"entry-points,omitempty"`
	Dynamic              []string                      `toml:"dynamic,omitempty"`
}

// Author represents an author or maintainer entry.
type Author struct {
	Name  string `toml:"name,omitempty"`
	Email string `toml:"email,omitempty"`
}

// ToolTable represents the [tool] section.
type ToolTable struct {
	Poetry *PoetryTable `toml:"poetry,omitempty"`
	Pensa  *PensaTable  `toml:"pensa,omitempty"`
	UV     *UVTable     `toml:"uv,omitempty"`
}

// PensaTable represents [tool.pensa].
type PensaTable struct {
	Workspace *WorkspaceConfig       `toml:"workspace,omitempty"`
	Sources   map[string]SourceEntry `toml:"sources,omitempty"`
}

// UVTable represents [tool.uv].
type UVTable struct {
	Workspace *WorkspaceConfig       `toml:"workspace,omitempty"`
	Sources   map[string]SourceEntry `toml:"sources,omitempty"`
}

// SourceEntry represents a dependency source override.
type SourceEntry struct {
	Workspace bool `toml:"workspace,omitempty"`
}

// WorkspaceConfig represents workspace member configuration.
type WorkspaceConfig struct {
	Members []string `toml:"members"`
	Exclude []string `toml:"exclude,omitempty"`
}

// PoetryTable represents [tool.poetry].
type PoetryTable struct {
	Name         string                        `toml:"name,omitempty"`
	Version      string                        `toml:"version,omitempty"`
	Description  string                        `toml:"description,omitempty"`
	License      string                        `toml:"license,omitempty"`
	Authors      []string                      `toml:"authors,omitempty"`
	Readme       interface{}                   `toml:"readme,omitempty"`
	PackageMode  *bool                         `toml:"package-mode,omitempty"`
	Packages     []PoetryPackage               `toml:"packages,omitempty"`
	Dependencies map[string]interface{}         `toml:"dependencies,omitempty"`
	Groups       map[string]PoetryGroup         `toml:"group,omitempty"`
	Scripts      map[string]interface{}         `toml:"scripts,omitempty"`
	Extras       map[string][]string           `toml:"extras,omitempty"`
	Source       []PoetrySource                `toml:"source,omitempty"`
}

// PoetryGroup represents a [tool.poetry.group.NAME] section.
type PoetryGroup struct {
	Dependencies map[string]interface{} `toml:"dependencies,omitempty"`
}

// PoetryPackage represents a package entry in [tool.poetry.packages].
type PoetryPackage struct {
	Include string      `toml:"include"`
	From    string      `toml:"from,omitempty"`
	To      string      `toml:"to,omitempty"`
	Format  interface{} `toml:"format,omitempty"`
}

// PoetrySource represents a [[tool.poetry.source]] entry.
type PoetrySource struct {
	Name     string `toml:"name"`
	URL      string `toml:"url"`
	Priority string `toml:"priority,omitempty"`
}

// BuildSystem represents the [build-system] section.
type BuildSystem struct {
	Requires     []string `toml:"requires,omitempty"`
	BuildBackend string   `toml:"build-backend,omitempty"`
}

// ReadPyProject reads and parses a pyproject.toml file.
func ReadPyProject(path string) (*PyProject, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pyproject.toml: %w", err)
	}
	return ParsePyProject(data)
}

// ParsePyProject parses pyproject.toml content from bytes.
func ParsePyProject(data []byte) (*PyProject, error) {
	var p PyProject
	if err := toml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse pyproject.toml: %w", err)
	}
	return &p, nil
}

// WritePyProject writes a PyProject to a file.
func WritePyProject(path string, p *PyProject) error {
	data, err := toml.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal pyproject.toml: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// Name returns the project name from either [project] or [tool.poetry].
func (p *PyProject) Name() string {
	if p.Project != nil && p.Project.Name != "" {
		return p.Project.Name
	}
	if p.Tool != nil && p.Tool.Poetry != nil {
		return p.Tool.Poetry.Name
	}
	return ""
}

// Version returns the project version from either [project] or [tool.poetry].
func (p *PyProject) Version() string {
	if p.Project != nil && p.Project.Version != "" {
		return p.Project.Version
	}
	if p.Tool != nil && p.Tool.Poetry != nil {
		return p.Tool.Poetry.Version
	}
	return ""
}

// HasProjectSection returns true if [project] is defined.
func (p *PyProject) HasProjectSection() bool {
	return p.Project != nil
}

// HasPoetrySection returns true if [tool.poetry] is defined.
func (p *PyProject) HasPoetrySection() bool {
	return p.Tool != nil && p.Tool.Poetry != nil
}

// WorkspaceMembers returns the workspace member paths if this is a workspace root.
// Checks [tool.pensa.workspace] first, then [tool.uv.workspace].
// Returns nil if not a workspace.
func (p *PyProject) WorkspaceMembers() []string {
	if p.Tool != nil {
		if p.Tool.Pensa != nil && p.Tool.Pensa.Workspace != nil && len(p.Tool.Pensa.Workspace.Members) > 0 {
			return p.Tool.Pensa.Workspace.Members
		}
		if p.Tool.UV != nil && p.Tool.UV.Workspace != nil && len(p.Tool.UV.Workspace.Members) > 0 {
			return p.Tool.UV.Workspace.Members
		}
	}
	return nil
}

// IsWorkspaceRoot returns true if this pyproject defines a workspace.
func (p *PyProject) IsWorkspaceRoot() bool {
	return len(p.WorkspaceMembers()) > 0
}

// WorkspaceSources returns dep names that are workspace members (not PyPI packages).
// Checks [tool.pensa.sources] first, then [tool.uv.sources].
func (p *PyProject) WorkspaceSources() map[string]bool {
	result := make(map[string]bool)
	var sources map[string]SourceEntry

	if p.Tool != nil {
		if p.Tool.Pensa != nil && len(p.Tool.Pensa.Sources) > 0 {
			sources = p.Tool.Pensa.Sources
		} else if p.Tool.UV != nil && len(p.Tool.UV.Sources) > 0 {
			sources = p.Tool.UV.Sources
		}
	}

	for name, entry := range sources {
		if entry.Workspace {
			result[name] = true
		}
	}
	return result
}
