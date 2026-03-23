package cli

import (
	"testing"

	"github.com/juanbzz/pensa/internal/pyproject"
)

func TestParseAddArg_NameOnly(t *testing.T) {
	name, constraint, err := parseAddArg("requests")
	if err != nil {
		t.Fatal(err)
	}
	if name != "requests" {
		t.Errorf("name = %q, want %q", name, "requests")
	}
	if constraint != "" {
		t.Errorf("constraint = %q, want empty", constraint)
	}
}

func TestParseAddArg_WithConstraint(t *testing.T) {
	name, constraint, err := parseAddArg("requests@^2.28")
	if err != nil {
		t.Fatal(err)
	}
	if name != "requests" {
		t.Errorf("name = %q", name)
	}
	if constraint != "^2.28" {
		t.Errorf("constraint = %q, want %q", constraint, "^2.28")
	}
}

func TestParseAddArg_WithVersionRange(t *testing.T) {
	name, constraint, err := parseAddArg("flask@>=2.0,<3.0")
	if err != nil {
		t.Fatal(err)
	}
	if name != "flask" {
		t.Errorf("name = %q", name)
	}
	if constraint != ">=2.0,<3.0" {
		t.Errorf("constraint = %q", constraint)
	}
}

func TestParseAddArg_Empty(t *testing.T) {
	_, _, err := parseAddArg("")
	if err == nil {
		t.Error("expected error for empty arg")
	}
}

func TestAddToProject_PEP621(t *testing.T) {
	proj := &pyproject.PyProject{
		Project: &pyproject.ProjectTable{
			Name:         "test",
			Dependencies: []string{"existing>=1.0"},
		},
	}

	addToProject(proj, "requests", "^2.28")

	if len(proj.Project.Dependencies) != 2 {
		t.Fatalf("deps count = %d, want 2", len(proj.Project.Dependencies))
	}
	if proj.Project.Dependencies[1] != "requests^2.28" {
		t.Errorf("dep = %q", proj.Project.Dependencies[1])
	}
}

func TestAddToProject_PEP621_UpdateExisting(t *testing.T) {
	proj := &pyproject.PyProject{
		Project: &pyproject.ProjectTable{
			Name:         "test",
			Dependencies: []string{"requests>=1.0"},
		},
	}

	addToProject(proj, "requests", "^2.28")

	if len(proj.Project.Dependencies) != 1 {
		t.Fatalf("deps count = %d, want 1 (updated in place)", len(proj.Project.Dependencies))
	}
	if proj.Project.Dependencies[0] != "requests^2.28" {
		t.Errorf("dep = %q", proj.Project.Dependencies[0])
	}
}

func TestAddToProject_Poetry(t *testing.T) {
	proj := &pyproject.PyProject{
		Tool: &pyproject.ToolTable{
			Poetry: &pyproject.PoetryTable{
				Name:         "test",
				Dependencies: map[string]interface{}{"python": "^3.8"},
			},
		},
	}

	addToProject(proj, "requests", "^2.28")

	val, ok := proj.Tool.Poetry.Dependencies["requests"]
	if !ok {
		t.Fatal("requests not added to poetry dependencies")
	}
	if val != "^2.28" {
		t.Errorf("requests = %v, want %q", val, "^2.28")
	}
}

func TestAddToProject_NoSection(t *testing.T) {
	proj := &pyproject.PyProject{}

	addToProject(proj, "requests", "^2.28")

	if proj.Project == nil {
		t.Fatal("expected project section to be created")
	}
	if len(proj.Project.Dependencies) != 1 {
		t.Fatalf("deps count = %d, want 1", len(proj.Project.Dependencies))
	}
}
