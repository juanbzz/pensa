package pyproject

import (
	"testing"
)

func TestPEP735_ParseGroups(t *testing.T) {
	data := []byte(`
[project]
name = "test"
version = "0.1.0"
dependencies = ["requests>=2.28"]

[dependency-groups]
dev = ["pytest>=7.0", "mypy"]
test = ["coverage[toml]"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`)
	proj, err := ParsePyProject(data)
	if err != nil {
		t.Fatal(err)
	}

	if proj.DependencyGroups == nil {
		t.Fatal("DependencyGroups should be parsed")
	}
	if len(proj.DependencyGroups["dev"]) != 2 {
		t.Errorf("dev group should have 2 entries, got %d", len(proj.DependencyGroups["dev"]))
	}
	if len(proj.DependencyGroups["test"]) != 1 {
		t.Errorf("test group should have 1 entry, got %d", len(proj.DependencyGroups["test"]))
	}
}

func TestPEP735_ResolveGroups(t *testing.T) {
	data := []byte(`
[project]
name = "test"
version = "0.1.0"
dependencies = ["requests>=2.28"]

[dependency-groups]
dev = ["pytest>=7.0", "mypy"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`)
	proj, err := ParsePyProject(data)
	if err != nil {
		t.Fatal(err)
	}

	deps, err := proj.ResolveDependenciesForGroups([]string{"main", "dev"})
	if err != nil {
		t.Fatal(err)
	}

	// Should have: requests (main) + pytest + mypy (dev) = 3
	if len(deps) != 3 {
		t.Fatalf("expected 3 deps, got %d", len(deps))
	}

	hasMain := false
	hasDev := false
	for _, d := range deps {
		if d.Group == "main" {
			hasMain = true
		}
		if d.Group == "dev" {
			hasDev = true
		}
	}
	if !hasMain {
		t.Error("should have main group deps")
	}
	if !hasDev {
		t.Error("should have dev group deps")
	}
}

func TestPEP735_IncludeGroup(t *testing.T) {
	data := []byte(`
[project]
name = "test"
version = "0.1.0"

[dependency-groups]
test = ["pytest>=7.0"]
dev = [{include-group = "test"}, "mypy"]
`)
	proj, err := ParsePyProject(data)
	if err != nil {
		t.Fatal(err)
	}

	deps, err := proj.ResolveDependenciesForGroups([]string{"dev"})
	if err != nil {
		t.Fatal(err)
	}

	// dev includes test group (pytest) + mypy = 2
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps from dev (with include), got %d", len(deps))
	}

	names := make(map[string]bool)
	for _, d := range deps {
		names[d.Dep.Name] = true
	}
	if !names["pytest"] {
		t.Error("should include pytest from test group")
	}
	if !names["mypy"] {
		t.Error("should include mypy from dev group")
	}
}

func TestPEP735_CyclicInclude(t *testing.T) {
	data := []byte(`
[project]
name = "test"
version = "0.1.0"

[dependency-groups]
a = [{include-group = "b"}]
b = [{include-group = "a"}]
`)
	proj, err := ParsePyProject(data)
	if err != nil {
		t.Fatal(err)
	}

	_, err = proj.ResolveDependenciesForGroups([]string{"a"})
	if err == nil {
		t.Fatal("expected error on cyclic include-group")
	}
}

func TestPEP735_TakesPrecedenceOverPoetry(t *testing.T) {
	data := []byte(`
[project]
name = "test"
version = "0.1.0"
dependencies = ["requests>=2.28"]

[dependency-groups]
dev = ["pytest>=7.0"]

[tool.poetry.group.dev.dependencies]
black = "*"

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`)
	proj, err := ParsePyProject(data)
	if err != nil {
		t.Fatal(err)
	}

	deps, err := proj.ResolveAllDependencies()
	if err != nil {
		t.Fatal(err)
	}

	// Should have requests (main) + pytest (dev from PEP 735) = 2
	// black from Poetry groups should be ignored since PEP 735 exists
	for _, d := range deps {
		if d.Dep.Name == "black" {
			t.Error("PEP 735 should take precedence, black from Poetry groups should be ignored")
		}
	}
}
