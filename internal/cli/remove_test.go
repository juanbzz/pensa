package cli

import (
	"testing"

	"github.com/juanbzz/pensa/internal/pyproject"
)

func TestRemoveFromProject_PEP621(t *testing.T) {
	proj := &pyproject.PyProject{
		Project: &pyproject.ProjectTable{
			Name:         "test",
			Dependencies: []string{"requests>=2.28", "flask>=2.0"},
		},
	}

	if err := removeFromProject(proj, "requests"); err != nil {
		t.Fatal(err)
	}

	if len(proj.Project.Dependencies) != 1 {
		t.Fatalf("deps count = %d, want 1", len(proj.Project.Dependencies))
	}
	if proj.Project.Dependencies[0] != "flask>=2.0" {
		t.Errorf("remaining dep = %q, want flask>=2.0", proj.Project.Dependencies[0])
	}
}

func TestRemoveFromProject_Poetry(t *testing.T) {
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

	if err := removeFromProject(proj, "requests"); err != nil {
		t.Fatal(err)
	}

	if _, ok := proj.Tool.Poetry.Dependencies["requests"]; ok {
		t.Error("requests should have been removed")
	}
	if _, ok := proj.Tool.Poetry.Dependencies["flask"]; !ok {
		t.Error("flask should still be present")
	}
	if _, ok := proj.Tool.Poetry.Dependencies["python"]; !ok {
		t.Error("python should still be present")
	}
}

func TestRemoveFromProject_NotFound(t *testing.T) {
	proj := &pyproject.PyProject{
		Project: &pyproject.ProjectTable{
			Name:         "test",
			Dependencies: []string{"flask>=2.0"},
		},
	}

	err := removeFromProject(proj, "requests")
	if err == nil {
		t.Fatal("expected error for nonexistent dep")
	}
}

func TestRemoveFromProject_NormalizedName(t *testing.T) {
	proj := &pyproject.PyProject{
		Project: &pyproject.ProjectTable{
			Name:         "test",
			Dependencies: []string{"charset-normalizer>=2.0"},
		},
	}

	// Remove with underscore — should match hyphenated name.
	if err := removeFromProject(proj, "charset_normalizer"); err != nil {
		t.Fatalf("should match normalized name: %v", err)
	}

	if len(proj.Project.Dependencies) != 0 {
		t.Error("dep should have been removed")
	}
}

func TestRemoveFromProject_CannotRemovePython(t *testing.T) {
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
	if err == nil {
		t.Fatal("should not be able to remove python")
	}
}

func TestRemoveFromProject_NoSection(t *testing.T) {
	proj := &pyproject.PyProject{}

	err := removeFromProject(proj, "requests")
	if err == nil {
		t.Fatal("expected error when no dependency section exists")
	}
}
