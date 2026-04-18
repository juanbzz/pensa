package cli

import (
	"testing"

	"github.com/matryer/is"

	"pensa.sh/pensa/internal/pyproject"
)

func TestParseAddArg_NameOnly(t *testing.T) {
	assert := is.New(t)
	name, constraint, extras, err := parseAddArg("requests")
	assert.NoErr(err)
	assert.Equal(name, "requests")
	assert.Equal(constraint, "")
	assert.Equal(len(extras), 0)
}

func TestParseAddArg_WithConstraint(t *testing.T) {
	assert := is.New(t)
	name, constraint, _, err := parseAddArg("requests@^2.28")
	assert.NoErr(err)
	assert.Equal(name, "requests")
	assert.Equal(constraint, "^2.28")
}

func TestParseAddArg_WithVersionRange(t *testing.T) {
	assert := is.New(t)
	name, constraint, _, err := parseAddArg("flask@>=2.0,<3.0")
	assert.NoErr(err)
	assert.Equal(name, "flask")
	assert.Equal(constraint, ">=2.0,<3.0")
}

func TestParseAddArg_WithExtras(t *testing.T) {
	assert := is.New(t)
	name, constraint, extras, err := parseAddArg("requests[security]@^2.28")
	assert.NoErr(err)
	assert.Equal(name, "requests")
	assert.Equal(constraint, "^2.28")
	assert.Equal(len(extras), 1)
	assert.Equal(extras[0], "security")
}

func TestParseAddArg_MultipleExtras(t *testing.T) {
	assert := is.New(t)
	name, _, extras, err := parseAddArg("black[d,jupyter]")
	assert.NoErr(err)
	assert.Equal(name, "black")
	assert.Equal(len(extras), 2)
	assert.Equal(extras[0], "d")
	assert.Equal(extras[1], "jupyter")
}

func TestParseAddArg_Empty(t *testing.T) {
	assert := is.New(t)
	_, _, _, err := parseAddArg("")
	assert.True(err != nil) // expected error for empty arg
}

func TestAddToProject_PEP621(t *testing.T) {
	assert := is.New(t)
	proj := &pyproject.PyProject{
		Project: &pyproject.ProjectTable{
			Name:         "test",
			Dependencies: []string{"existing>=1.0"},
		},
	}

	addToProject(proj, "requests", "^2.28")

	assert.Equal(len(proj.Project.Dependencies), 2)
	assert.Equal(proj.Project.Dependencies[1], "requests>=2.28,<3.0")
}

func TestAddToProject_PEP621_UpdateExisting(t *testing.T) {
	assert := is.New(t)
	proj := &pyproject.PyProject{
		Project: &pyproject.ProjectTable{
			Name:         "test",
			Dependencies: []string{"requests>=1.0"},
		},
	}

	addToProject(proj, "requests", "^2.28")

	assert.Equal(len(proj.Project.Dependencies), 1)
	assert.Equal(proj.Project.Dependencies[0], "requests>=2.28,<3.0")
}

func TestAddToProject_Poetry(t *testing.T) {
	assert := is.New(t)
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
	assert.True(ok) // requests should be added to poetry dependencies
	assert.Equal(val, "^2.28")
}

func TestAddToProject_NoSection(t *testing.T) {
	assert := is.New(t)
	proj := &pyproject.PyProject{}

	addToProject(proj, "requests", "^2.28")

	assert.True(proj.Project != nil) // expected project section to be created
	assert.Equal(len(proj.Project.Dependencies), 1)
}

func TestParseAddArg_PEP508_GreaterEqual(t *testing.T) {
	assert := is.New(t)
	name, constraint, _, err := parseAddArg("sqlalchemy>=2.0")
	assert.NoErr(err)
	assert.Equal(name, "sqlalchemy")
	assert.Equal(constraint, ">=2.0")
}

func TestParseAddArg_PEP508_Exact(t *testing.T) {
	assert := is.New(t)
	name, constraint, _, err := parseAddArg("requests==2.31.0")
	assert.NoErr(err)
	assert.Equal(name, "requests")
	assert.Equal(constraint, "==2.31.0")
}

func TestParseAddArg_PEP508_Compatible(t *testing.T) {
	assert := is.New(t)
	name, constraint, _, err := parseAddArg("flask~=2.0")
	assert.NoErr(err)
	assert.Equal(name, "flask")
	assert.Equal(constraint, "~=2.0")
}

func TestParseAddArg_PEP508_Range(t *testing.T) {
	assert := is.New(t)
	name, constraint, _, err := parseAddArg("numpy>=1.21,<2.0")
	assert.NoErr(err)
	assert.Equal(name, "numpy")
	assert.Equal(constraint, ">=1.21,<2.0")
}

func TestParseAddArg_PEP508_WithExtras(t *testing.T) {
	assert := is.New(t)
	name, constraint, extras, err := parseAddArg("requests[security]>=2.28")
	assert.NoErr(err)
	assert.Equal(name, "requests")
	assert.Equal(constraint, ">=2.28")
	assert.Equal(len(extras), 1)
	assert.Equal(extras[0], "security")
}

func TestParseAddArg_PEP508_NotEqual(t *testing.T) {
	assert := is.New(t)
	name, constraint, _, err := parseAddArg("setuptools!=50.0")
	assert.NoErr(err)
	assert.Equal(name, "setuptools")
	assert.Equal(constraint, "!=50.0")
}

func TestParseAddArg_PEP508_LessThan(t *testing.T) {
	assert := is.New(t)
	name, constraint, _, err := parseAddArg("protobuf<4.0")
	assert.NoErr(err)
	assert.Equal(name, "protobuf")
	assert.Equal(constraint, "<4.0")
}

func TestParseAddArg_AtStylePreferred(t *testing.T) {
	assert := is.New(t)
	// @ style should still work and take precedence.
	name, constraint, _, err := parseAddArg("requests@^2.28")
	assert.NoErr(err)
	assert.Equal(name, "requests")
	assert.Equal(constraint, "^2.28")
}
