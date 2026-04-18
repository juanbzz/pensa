package cli

import (
	"testing"

	"github.com/matryer/is"

	"pensa.sh/pensa/internal/pyproject"
)

func TestRemoveFromProject_PEP621(t *testing.T) {
	assert := is.New(t)
	proj := &pyproject.PyProject{
		Project: &pyproject.ProjectTable{
			Name:         "test",
			Dependencies: []string{"requests>=2.28", "flask>=2.0"},
		},
	}

	assert.NoErr(removeFromProject(proj, "requests"))
	assert.Equal(len(proj.Project.Dependencies), 1)
	assert.Equal(proj.Project.Dependencies[0], "flask>=2.0")
}

func TestRemoveFromProject_Poetry(t *testing.T) {
	assert := is.New(t)
	proj := &pyproject.PyProject{
		Tool: &pyproject.ToolTable{
			Poetry: &pyproject.PoetryTable{
				Name: "test",
				Dependencies: map[string]interface{}{
					"python":   "^3.8",
					"requests": "^2.28",
					"flask":    "^2.0",
				},
			},
		},
	}

	assert.NoErr(removeFromProject(proj, "requests"))

	_, ok := proj.Tool.Poetry.Dependencies["requests"]
	assert.True(!ok) // requests should have been removed
	_, ok = proj.Tool.Poetry.Dependencies["flask"]
	assert.True(ok) // flask should still be present
	_, ok = proj.Tool.Poetry.Dependencies["python"]
	assert.True(ok) // python should still be present
}

func TestRemoveFromProject_NotFound(t *testing.T) {
	assert := is.New(t)
	proj := &pyproject.PyProject{
		Project: &pyproject.ProjectTable{
			Name:         "test",
			Dependencies: []string{"flask>=2.0"},
		},
	}

	err := removeFromProject(proj, "requests")
	assert.True(err != nil) // expected error for nonexistent dep
}

func TestRemoveFromProject_NormalizedName(t *testing.T) {
	assert := is.New(t)
	proj := &pyproject.PyProject{
		Project: &pyproject.ProjectTable{
			Name:         "test",
			Dependencies: []string{"charset-normalizer>=2.0"},
		},
	}

	// Remove with underscore — should match hyphenated name.
	assert.NoErr(removeFromProject(proj, "charset_normalizer"))
	assert.Equal(len(proj.Project.Dependencies), 0)
}

func TestRemoveFromProject_CannotRemovePython(t *testing.T) {
	assert := is.New(t)
	proj := &pyproject.PyProject{
		Tool: &pyproject.ToolTable{
			Poetry: &pyproject.PoetryTable{
				Name: "test",
				Dependencies: map[string]interface{}{
					"python":   "^3.8",
					"requests": "^2.28",
				},
			},
		},
	}

	err := removeFromProject(proj, "python")
	assert.True(err != nil) // should not be able to remove python
}

func TestRemoveFromProject_NoSection(t *testing.T) {
	assert := is.New(t)
	proj := &pyproject.PyProject{}

	err := removeFromProject(proj, "requests")
	assert.True(err != nil) // expected error when no dependency section exists
}
