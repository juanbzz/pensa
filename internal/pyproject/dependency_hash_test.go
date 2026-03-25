package pyproject

import (
	"testing"

	"github.com/matryer/is"
)

func TestDependencyHash_StableAcrossCalls(t *testing.T) {
	is := is.New(t)

	p := &PyProject{
		Project: &ProjectTable{
			Name:           "mypackage",
			Version:        "1.0.0",
			RequiresPython: ">=3.8",
			Dependencies:   []string{"requests>=2.0", "flask>=2.0"},
		},
	}

	h1 := p.DependencyHash()
	h2 := p.DependencyHash()
	is.Equal(h1, h2)
	is.True(len(h1) == 64) // SHA-256 hex
}

func TestDependencyHash_IgnoresNonDepFields(t *testing.T) {
	is := is.New(t)

	base := &PyProject{
		Project: &ProjectTable{
			Name:           "mypackage",
			Version:        "1.0.0",
			Description:    "Original description",
			RequiresPython: ">=3.8",
			Dependencies:   []string{"requests>=2.0"},
		},
	}

	changed := &PyProject{
		Project: &ProjectTable{
			Name:           "mypackage",
			Version:        "2.0.0",           // changed
			Description:    "New description",  // changed
			License:        "MIT",              // added
			RequiresPython: ">=3.8",
			Dependencies:   []string{"requests>=2.0"},
			Authors:        []Author{{Name: "Someone"}}, // added
			Keywords:       []string{"new"},              // added
		},
	}

	is.Equal(base.DependencyHash(), changed.DependencyHash())
}

func TestDependencyHash_ChangesOnDepChange(t *testing.T) {
	is := is.New(t)

	base := &PyProject{
		Project: &ProjectTable{
			RequiresPython: ">=3.8",
			Dependencies:   []string{"requests>=2.0"},
		},
	}

	withExtraDep := &PyProject{
		Project: &ProjectTable{
			RequiresPython: ">=3.8",
			Dependencies:   []string{"requests>=2.0", "flask>=2.0"},
		},
	}

	withPythonChange := &PyProject{
		Project: &ProjectTable{
			RequiresPython: ">=3.10",
			Dependencies:   []string{"requests>=2.0"},
		},
	}

	withOptionalDep := &PyProject{
		Project: &ProjectTable{
			RequiresPython:       ">=3.8",
			Dependencies:         []string{"requests>=2.0"},
			OptionalDependencies: map[string][]string{"dev": {"pytest"}},
		},
	}

	h := base.DependencyHash()
	is.True(h != withExtraDep.DependencyHash())
	is.True(h != withPythonChange.DependencyHash())
	is.True(h != withOptionalDep.DependencyHash())
}

func TestDependencyHash_ChangesOnPoetryDepChange(t *testing.T) {
	is := is.New(t)

	base := &PyProject{
		Tool: &ToolTable{
			Poetry: &PoetryTable{
				Name:    "mypackage",
				Version: "1.0.0",
				Dependencies: map[string]interface{}{
					"python":   "^3.8",
					"requests": "^2.0",
				},
			},
		},
	}

	changed := &PyProject{
		Tool: &ToolTable{
			Poetry: &PoetryTable{
				Name:    "mypackage",
				Version: "2.0.0", // non-dep change
				Dependencies: map[string]interface{}{
					"python":   "^3.8",
					"requests": "^2.0",
				},
			},
		},
	}

	depChanged := &PyProject{
		Tool: &ToolTable{
			Poetry: &PoetryTable{
				Name:    "mypackage",
				Version: "1.0.0",
				Dependencies: map[string]interface{}{
					"python":   "^3.8",
					"requests": "^3.0", // dep change
				},
			},
		},
	}

	is.Equal(base.DependencyHash(), changed.DependencyHash())    // version change ignored
	is.True(base.DependencyHash() != depChanged.DependencyHash()) // dep change detected
}

func TestDependencyHash_WorkspaceChange(t *testing.T) {
	is := is.New(t)

	base := &PyProject{
		Tool: &ToolTable{
			UV: &UVTable{
				Workspace: &WorkspaceConfig{Members: []string{"apps/backend"}},
			},
		},
	}

	changed := &PyProject{
		Tool: &ToolTable{
			UV: &UVTable{
				Workspace: &WorkspaceConfig{Members: []string{"apps/backend", "apps/frontend"}},
			},
		},
	}

	is.True(base.DependencyHash() != changed.DependencyHash())
}
