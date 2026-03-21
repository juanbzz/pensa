package pyproject

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePyProject_PEP621(t *testing.T) {
	data := []byte(`
[project]
name = "my-package"
version = "1.0.0"
description = "A test package"
requires-python = ">=3.8"
license = "MIT"
authors = [
    { name = "Test Author", email = "test@example.com" },
]
keywords = ["test", "package"]
dependencies = [
    "requests>=2.13.0",
    "flask>=2.0; python_version>='3.8'",
]

[project.optional-dependencies]
dev = ["pytest>=6.0"]

[project.scripts]
my-cli = "my_package.cli:main"

[project.urls]
homepage = "https://example.com"
`)
	p, err := ParsePyProject(data)
	if err != nil {
		t.Fatal(err)
	}

	if p.Name() != "my-package" {
		t.Errorf("Name() = %q, want %q", p.Name(), "my-package")
	}
	if p.Version() != "1.0.0" {
		t.Errorf("Version() = %q", p.Version())
	}
	if p.Project.Description != "A test package" {
		t.Errorf("Description = %q", p.Project.Description)
	}
	if p.Project.RequiresPython != ">=3.8" {
		t.Errorf("RequiresPython = %q", p.Project.RequiresPython)
	}
	if len(p.Project.Authors) != 1 || p.Project.Authors[0].Name != "Test Author" {
		t.Errorf("Authors = %v", p.Project.Authors)
	}
	if len(p.Project.Dependencies) != 2 {
		t.Errorf("Dependencies = %v", p.Project.Dependencies)
	}
	if len(p.Project.OptionalDependencies["dev"]) != 1 {
		t.Errorf("OptionalDependencies = %v", p.Project.OptionalDependencies)
	}
	if p.Project.Scripts["my-cli"] != "my_package.cli:main" {
		t.Errorf("Scripts = %v", p.Project.Scripts)
	}
	if p.Project.URLs["homepage"] != "https://example.com" {
		t.Errorf("URLs = %v", p.Project.URLs)
	}
}

func TestParsePyProject_PoetryOnly(t *testing.T) {
	data := []byte(`
[tool.poetry]
name = "my-poetry-package"
version = "0.1.0"
description = "A Poetry package"
authors = ["Test Author <test@example.com>"]

[tool.poetry.dependencies]
python = "^3.8"
requests = "^2.13.0"

[tool.poetry.group.test.dependencies]
pytest = "*"

[[tool.poetry.source]]
name = "private"
url = "https://private.example.com/simple"
priority = "supplemental"
`)
	p, err := ParsePyProject(data)
	if err != nil {
		t.Fatal(err)
	}

	if p.Name() != "my-poetry-package" {
		t.Errorf("Name() = %q", p.Name())
	}
	if p.Version() != "0.1.0" {
		t.Errorf("Version() = %q", p.Version())
	}
	if !p.HasPoetrySection() {
		t.Error("expected HasPoetrySection")
	}
	if p.HasProjectSection() {
		t.Error("expected no project section")
	}
	if len(p.Tool.Poetry.Dependencies) != 2 {
		t.Errorf("Dependencies count = %d, want 2", len(p.Tool.Poetry.Dependencies))
	}
	if len(p.Tool.Poetry.Groups) != 1 {
		t.Errorf("Groups count = %d, want 1", len(p.Tool.Poetry.Groups))
	}
	if len(p.Tool.Poetry.Source) != 1 {
		t.Errorf("Source count = %d, want 1", len(p.Tool.Poetry.Source))
	}
	if p.Tool.Poetry.Source[0].Name != "private" {
		t.Errorf("Source name = %q", p.Tool.Poetry.Source[0].Name)
	}
}

func TestParsePyProject_Both(t *testing.T) {
	data := []byte(`
[project]
name = "combined-package"
version = "2.0.0"
dependencies = [
    "requests>=2.13.0",
]

[tool.poetry.dependencies]
requests = { version = ">=2.13.0", source = "private" }
`)
	p, err := ParsePyProject(data)
	if err != nil {
		t.Fatal(err)
	}

	if !p.HasProjectSection() {
		t.Error("expected project section")
	}
	if !p.HasPoetrySection() {
		t.Error("expected poetry section")
	}

	deps, err := p.ResolveDependencies()
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 {
		t.Fatalf("deps count = %d, want 1", len(deps))
	}
	if deps[0].Name != "requests" {
		t.Errorf("dep name = %q", deps[0].Name)
	}
}

func TestParsePyProject_BuildSystem(t *testing.T) {
	data := []byte(`
[build-system]
requires = ["poetry-core>=1.0.0"]
build-backend = "poetry.core.masonry.api"
`)
	p, err := ParsePyProject(data)
	if err != nil {
		t.Fatal(err)
	}

	if p.BuildSystem == nil {
		t.Fatal("expected build-system")
	}
	if p.BuildSystem.BuildBackend != "poetry.core.masonry.api" {
		t.Errorf("BuildBackend = %q", p.BuildSystem.BuildBackend)
	}
	if len(p.BuildSystem.Requires) != 1 {
		t.Errorf("Requires = %v", p.BuildSystem.Requires)
	}
}

func TestParsePyProject_MissingSections(t *testing.T) {
	data := []byte(`
[build-system]
requires = ["setuptools"]
`)
	p, err := ParsePyProject(data)
	if err != nil {
		t.Fatal(err)
	}

	if p.HasProjectSection() {
		t.Error("expected no project section")
	}
	if p.HasPoetrySection() {
		t.Error("expected no poetry section")
	}
	if p.Name() != "" {
		t.Errorf("Name() = %q, want empty", p.Name())
	}
}

func TestWriteAndReadRoundTrip(t *testing.T) {
	p := &PyProject{
		Project: &ProjectTable{
			Name:         "roundtrip-test",
			Version:      "1.0.0",
			Dependencies: []string{"requests>=2.0"},
		},
		BuildSystem: &BuildSystem{
			Requires:     []string{"poetry-core>=1.0.0"},
			BuildBackend: "poetry.core.masonry.api",
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "pyproject.toml")

	if err := WritePyProject(path, p); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}

	p2, err := ReadPyProject(path)
	if err != nil {
		t.Fatal(err)
	}

	if p2.Name() != "roundtrip-test" {
		t.Errorf("Name() = %q after roundtrip", p2.Name())
	}
	if p2.Version() != "1.0.0" {
		t.Errorf("Version() = %q after roundtrip", p2.Version())
	}
	if len(p2.Project.Dependencies) != 1 {
		t.Errorf("Dependencies = %v after roundtrip", p2.Project.Dependencies)
	}
}

func TestResolveDependencies_Poetry(t *testing.T) {
	data := []byte(`
[tool.poetry]
name = "test"
version = "0.1.0"

[tool.poetry.dependencies]
python = "^3.8"
requests = "^2.13.0"
flask = { version = "^2.0", extras = ["async"] }
`)
	p, err := ParsePyProject(data)
	if err != nil {
		t.Fatal(err)
	}

	deps, err := p.ResolveDependencies()
	if err != nil {
		t.Fatal(err)
	}

	if len(deps) != 2 {
		t.Fatalf("deps count = %d, want 2", len(deps))
	}

	depMap := make(map[string]int)
	for i, d := range deps {
		depMap[d.Name] = i
	}

	if _, ok := depMap["requests"]; !ok {
		t.Error("expected requests dependency")
	}
	if idx, ok := depMap["flask"]; ok {
		if len(deps[idx].Extras) != 1 || deps[idx].Extras[0] != "async" {
			t.Errorf("flask extras = %v", deps[idx].Extras)
		}
	} else {
		t.Error("expected flask dependency")
	}
}
