package pyproject

import (
	"testing"

	"github.com/matryer/is"
)

// pgm/startapp shape: PEP 621 project with [project.optional-dependencies].
// uv treats these as lockable; pensa must too.
const pgmShapedPyProject = `
[project]
name = "pgmarketing-backend"
version = "0.1.0"
requires-python = ">=3.10,<3.13"
dependencies = [
    "fastapi>=0.115.6",
    "sqlalchemy>=2.0.36",
]

[project.optional-dependencies]
dev = [
    "pytest>=8.3.3",
    "black>=24.10.0",
]
infra = [
    "pulumi>=3.181.0",
    "pulumi-aws>=6.83.0",
]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`

func TestResolveAllDependencies_PEP621OptionalDependencies(t *testing.T) {
	is := is.New(t)

	proj, err := ParsePyProject([]byte(pgmShapedPyProject))
	is.NoErr(err)

	deps, err := proj.ResolveAllDependencies()
	is.NoErr(err)

	byGroup := groupedByName(deps)
	is.True(byGroup["fastapi"] == "main")
	is.True(byGroup["sqlalchemy"] == "main")
	is.True(byGroup["pytest"] == "dev")
	is.True(byGroup["black"] == "dev")
	is.True(byGroup["pulumi"] == "infra")
	is.True(byGroup["pulumi-aws"] == "infra")
}

func TestResolveDependenciesForGroups_SelectsOptionalGroup(t *testing.T) {
	is := is.New(t)

	proj, err := ParsePyProject([]byte(pgmShapedPyProject))
	is.NoErr(err)

	deps, err := proj.ResolveDependenciesForGroups([]string{"main", "dev"})
	is.NoErr(err)

	names := nameSet(deps)
	is.True(names["fastapi"])
	is.True(names["pytest"])
	is.True(!names["pulumi"]) // infra not requested
}

func TestResolveDependenciesForGroups_OmitsUnrequestedOptionalGroup(t *testing.T) {
	is := is.New(t)

	proj, err := ParsePyProject([]byte(pgmShapedPyProject))
	is.NoErr(err)

	deps, err := proj.ResolveDependenciesForGroups([]string{"main"})
	is.NoErr(err)

	names := nameSet(deps)
	is.True(names["fastapi"])
	is.True(!names["pytest"]) // dev not requested
	is.True(!names["pulumi"]) // infra not requested
}

// PEP 735 [dependency-groups] takes precedence over PEP 621
// [project.optional-dependencies] when the group name appears in both.
func TestResolveDependenciesForGroups_PEP735BeatsOptionalDeps(t *testing.T) {
	is := is.New(t)

	const collisionPyProject = `
[project]
name = "collision"
version = "0.1.0"
dependencies = ["requests>=2.28"]

[project.optional-dependencies]
dev = ["black>=24.10.0"]

[dependency-groups]
dev = ["pytest>=8.3.3"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`
	proj, err := ParsePyProject([]byte(collisionPyProject))
	is.NoErr(err)

	deps, err := proj.ResolveDependenciesForGroups([]string{"dev"})
	is.NoErr(err)

	names := nameSet(deps)
	is.True(names["pytest"])  // PEP 735 wins
	is.True(!names["black"])  // PEP 621 optional-deps loses
}

func groupedByName(deps []GroupedDependency) map[string]string {
	m := make(map[string]string, len(deps))
	for _, d := range deps {
		m[d.Dep.Name] = d.Group
	}
	return m
}

func nameSet(deps []GroupedDependency) map[string]bool {
	s := make(map[string]bool, len(deps))
	for _, d := range deps {
		s[d.Dep.Name] = true
	}
	return s
}
