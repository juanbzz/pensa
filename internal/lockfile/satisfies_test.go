package lockfile

import (
	"testing"

	"github.com/matryer/is"

	"github.com/juanbzz/pensa/pkg/pep508"
	"github.com/juanbzz/pensa/pkg/version"
)

func testLockFileWith(packages []LockedPackage, pythonVersions string) *LockFile {
	return &LockFile{
		Packages: packages,
		Metadata: LockMetadata{
			LockVersion:    "2.1",
			PythonVersions: pythonVersions,
			ContentHash:    "test",
		},
	}
}

func dep(name, constraint string) pep508.Dependency {
	d := pep508.Dependency{Name: name}
	if constraint != "" {
		c, _ := version.ParseConstraint(constraint)
		d.Constraint = c
	}
	return d
}

func TestSatisfies_AllDepsMatch(t *testing.T) {
	assert := is.New(t)

	lf := testLockFileWith([]LockedPackage{
		{Name: "requests", Version: "2.31.0"},
		{Name: "flask", Version: "3.0.0"},
	}, ">=3.8")

	reqs := []pep508.Dependency{
		dep("requests", ">=2.0"),
		dep("flask", ">=2.0,<4.0"),
	}

	result := Satisfies(lf, reqs, ">=3.8")
	assert.True(result.Satisfied)
	assert.Equal(result.Reason, "")
}

func TestSatisfies_NewDependency(t *testing.T) {
	assert := is.New(t)

	lf := testLockFileWith([]LockedPackage{
		{Name: "requests", Version: "2.31.0"},
	}, ">=3.8")

	reqs := []pep508.Dependency{
		dep("requests", ">=2.0"),
		dep("click", ">=8.0"),
	}

	result := Satisfies(lf, reqs, ">=3.8")
	assert.True(!result.Satisfied)
	assert.Equal(result.Reason, "new dependency: click")
}

func TestSatisfies_ConstraintNotMet(t *testing.T) {
	assert := is.New(t)

	lf := testLockFileWith([]LockedPackage{
		{Name: "requests", Version: "2.31.0"},
	}, ">=3.8")

	reqs := []pep508.Dependency{
		dep("requests", ">=3.0"),
	}

	result := Satisfies(lf, reqs, ">=3.8")
	assert.True(!result.Satisfied)
	assert.True(result.Reason != "")
}

func TestSatisfies_ConstraintRelaxed(t *testing.T) {
	assert := is.New(t)

	lf := testLockFileWith([]LockedPackage{
		{Name: "requests", Version: "2.31.0"},
	}, ">=3.8")

	// Widened from ">=2.0,<3.0" to ">=2.0" — locked 2.31.0 still valid.
	reqs := []pep508.Dependency{
		dep("requests", ">=2.0"),
	}

	result := Satisfies(lf, reqs, ">=3.8")
	assert.True(result.Satisfied)
}

func TestSatisfies_ExtraConstraintAdded(t *testing.T) {
	assert := is.New(t)

	lf := testLockFileWith([]LockedPackage{
		{Name: "requests", Version: "2.31.0"},
	}, ">=3.8")

	// Added upper bound that still includes locked version.
	reqs := []pep508.Dependency{
		dep("requests", ">=2.0,<3.0"),
	}

	result := Satisfies(lf, reqs, ">=3.8")
	assert.True(result.Satisfied)
}

func TestSatisfies_RequiresPythonChanged(t *testing.T) {
	assert := is.New(t)

	lf := testLockFileWith([]LockedPackage{
		{Name: "requests", Version: "2.31.0"},
	}, ">=3.8")

	reqs := []pep508.Dependency{
		dep("requests", ">=2.0"),
	}

	result := Satisfies(lf, reqs, ">=3.10")
	assert.True(!result.Satisfied)
	assert.Equal(result.Reason, "requires-python changed")
}

func TestSatisfies_EmptyRequirements(t *testing.T) {
	assert := is.New(t)

	lf := testLockFileWith([]LockedPackage{
		{Name: "requests", Version: "2.31.0"},
	}, ">=3.8")

	result := Satisfies(lf, nil, ">=3.8")
	assert.True(result.Satisfied)
}

func TestSatisfies_NoConstraint(t *testing.T) {
	assert := is.New(t)

	lf := testLockFileWith([]LockedPackage{
		{Name: "requests", Version: "2.31.0"},
	}, ">=3.8")

	// No constraint — any version is fine.
	reqs := []pep508.Dependency{
		dep("requests", ""),
	}

	result := Satisfies(lf, reqs, ">=3.8")
	assert.True(result.Satisfied)
}

func TestSatisfies_NormalizedNames(t *testing.T) {
	assert := is.New(t)

	lf := testLockFileWith([]LockedPackage{
		{Name: "my-package", Version: "1.0.0"},
	}, ">=3.8")

	// Pyproject uses underscore, lock uses hyphen.
	reqs := []pep508.Dependency{
		dep("my_package", ">=1.0"),
	}

	result := Satisfies(lf, reqs, ">=3.8")
	assert.True(result.Satisfied)
}
