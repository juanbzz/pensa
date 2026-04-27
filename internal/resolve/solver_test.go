package resolve

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/matryer/is"

	"pensa.sh/pensa/pkg/version"
)

// --- Mock Provider ---

type mockPackage struct {
	ver  version.Version
	deps []Dependency
}

type mockProvider struct {
	packages map[string][]mockPackage
	// preferred lets tests exercise the Preferred() hook: when set
	// for a package, the solver should try that version before the
	// default newest-first scan.
	preferred map[string]version.Version
}

func (m *mockProvider) Versions(_ context.Context, pkg string) ([]version.Version, error) {
	pkgs, ok := m.packages[pkg]
	if !ok {
		return nil, nil
	}
	var versions []version.Version
	for _, p := range pkgs {
		versions = append(versions, p.ver)
	}
	return versions, nil
}

func (m *mockProvider) Dependencies(_ context.Context, pkg string, ver version.Version) ([]Dependency, error) {
	pkgs, ok := m.packages[pkg]
	if !ok {
		return nil, fmt.Errorf("package %s not found", pkg)
	}
	for _, p := range pkgs {
		if version.Compare(p.ver, ver) == 0 {
			return p.deps, nil
		}
	}
	return nil, fmt.Errorf("version %s of %s not found", ver, pkg)
}

// DependenciesIfCached: mocks have no separate cache layer, so
// everything they know is "cached". Return deps as if all were
// already fetched.
func (m *mockProvider) DependenciesIfCached(ctx context.Context, pkg string, ver version.Version) ([]Dependency, bool) {
	deps, err := m.Dependencies(ctx, pkg, ver)
	if err != nil {
		return nil, false
	}
	return deps, true
}

// Preferred returns a version from the optional preferred map. Tests
// populate it to drive the lockfile-preference path.
func (m *mockProvider) Preferred(_ context.Context, pkg string) (version.Version, bool) {
	v, ok := m.preferred[pkg]
	return v, ok
}

func mustParseVersion(t *testing.T, s string) version.Version {
	t.Helper()
	v, err := version.Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", s, err)
	}
	return v
}

func mustParseConstraint(t *testing.T, s string) version.Constraint {
	t.Helper()
	c, err := version.ParseConstraint(s)
	if err != nil {
		t.Fatalf("ParseConstraint(%q) error: %v", s, err)
	}
	return c
}

// --- Tests ---

func TestSolver_SingleDependency(t *testing.T) {
	assert := is.New(t)

	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
				{ver: mustParseVersion(t, "1.5.0"), deps: nil},
				{ver: mustParseVersion(t, "2.0.0"), deps: nil},
			},
		},
	}

	solver := NewSolver(provider, "myproject", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, "^1.0")},
	})

	result, err := solver.Solve(context.Background())
	assert.NoErr(err)

	v, ok := result.Decisions["a"]
	assert.True(ok)
	assert.Equal(v.String(), "1.5.0")
}

func TestSolver_TwoDependencies(t *testing.T) {
	assert := is.New(t)

	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
			},
			"b": {
				{ver: mustParseVersion(t, "2.0.0"), deps: nil},
			},
		},
	}

	solver := NewSolver(provider, "myproject", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, "^1.0")},
		{Pkg: "b", Constraint: mustParseConstraint(t, "^2.0")},
	})

	result, err := solver.Solve(context.Background())
	assert.NoErr(err)
	assert.Equal(len(result.Decisions), 2)
	assert.Equal(result.Decisions["a"].String(), "1.0.0")
	assert.Equal(result.Decisions["b"].String(), "2.0.0")
}

func TestSolver_TransitiveDependency(t *testing.T) {
	assert := is.New(t)

	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "b", Constraint: mustParseConstraint(t, "^2.0")},
				}},
			},
			"b": {
				{ver: mustParseVersion(t, "2.0.0"), deps: nil},
				{ver: mustParseVersion(t, "2.5.0"), deps: nil},
			},
		},
	}

	solver := NewSolver(provider, "myproject", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, "^1.0")},
	})

	result, err := solver.Solve(context.Background())
	assert.NoErr(err)
	assert.Equal(len(result.Decisions), 2)
	assert.Equal(result.Decisions["a"].String(), "1.0.0")
	_, ok := result.Decisions["b"]
	assert.True(ok)
}

func TestSolver_NoMatchingVersions(t *testing.T) {
	assert := is.New(t)

	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
			},
		},
	}

	solver := NewSolver(provider, "myproject", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, ">=99.0")},
	})

	_, err := solver.Solve(context.Background())
	assert.True(err != nil)
}

func TestSolver_PrefersNewestVersion(t *testing.T) {
	assert := is.New(t)

	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
				{ver: mustParseVersion(t, "1.1.0"), deps: nil},
				{ver: mustParseVersion(t, "1.2.0"), deps: nil},
			},
		},
	}

	solver := NewSolver(provider, "myproject", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, ">=1.0,<2.0")},
	})

	result, err := solver.Solve(context.Background())
	assert.NoErr(err)
	assert.Equal(result.Decisions["a"].String(), "1.2.0")
}

func TestSolver_Backtracking(t *testing.T) {
	assert := is.New(t)

	// root → a ^1.0, b ^1.0
	// a 1.5 → c ^2.0
	// a 1.0 → c ^1.0
	// b 1.0 → c ^1.0
	// Must backtrack a from 1.5 to 1.0 to satisfy both c constraints.
	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "c", Constraint: mustParseConstraint(t, "^1.0")},
				}},
				{ver: mustParseVersion(t, "1.5.0"), deps: []Dependency{
					{Pkg: "c", Constraint: mustParseConstraint(t, "^2.0")},
				}},
			},
			"b": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "c", Constraint: mustParseConstraint(t, "^1.0")},
				}},
			},
			"c": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
				{ver: mustParseVersion(t, "1.5.0"), deps: nil},
				{ver: mustParseVersion(t, "2.0.0"), deps: nil},
			},
		},
	}

	solver := NewSolver(provider, "myproject", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, "^1.0")},
		{Pkg: "b", Constraint: mustParseConstraint(t, "^1.0")},
	})

	result, err := solver.Solve(context.Background())
	assert.NoErr(err)
	assert.Equal(result.Decisions["a"].String(), "1.0.0")
	assert.Equal(result.Decisions["c"].Major(), 1)
}

func TestSolver_Conflict(t *testing.T) {
	// root → a ^1.0, b ^1.0
	// a 1.0 → c >=2.0
	// b 1.0 → c <2.0
	// Impossible to satisfy.
	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "c", Constraint: mustParseConstraint(t, ">=2.0")},
				}},
			},
			"b": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "c", Constraint: mustParseConstraint(t, "<2.0")},
				}},
			},
			"c": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
				{ver: mustParseVersion(t, "2.0.0"), deps: nil},
				{ver: mustParseVersion(t, "3.0.0"), deps: nil},
			},
		},
	}

	solver := NewSolver(provider, "myproject", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, "^1.0")},
		{Pkg: "b", Constraint: mustParseConstraint(t, "^1.0")},
	})

	assert := is.New(t)

	_, err := solver.Solve(context.Background())
	assert.True(err != nil)

	msg := err.Error()
	assert.True(strings.Contains(msg, "version solving failed:"))
	assert.True(strings.Contains(msg, "depends on c"))
	assert.True(!strings.Contains(msg, "$root"))
	assert.True(!strings.Contains(msg, "{"))
	// Actionable footer is appended verbatim — assert against the
	// helper's output so a future rephrasing doesn't slip past us.
	assert.True(strings.HasSuffix(strings.TrimRight(msg, "\n"), strings.TrimRight(solveErrorHint(), "\n")))
}

func TestSolver_ConflictShowsProjectName(t *testing.T) {
	assert := is.New(t)

	// root → a >=2.0, b ^1.0
	// b 1.0 → a <2.0
	// Root directly conflicts with b's dependency on a.
	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
				{ver: mustParseVersion(t, "2.0.0"), deps: nil},
			},
			"b": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "a", Constraint: mustParseConstraint(t, "<2.0")},
				}},
			},
		},
	}

	solver := NewSolver(provider, "myproject", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, ">=2.0")},
		{Pkg: "b", Constraint: mustParseConstraint(t, "^1.0")},
	})

	_, err := solver.Solve(context.Background())
	assert.True(err != nil)

	msg := err.Error()
	assert.True(strings.Contains(msg, "myproject"))
	assert.True(!strings.Contains(msg, "$root"))
}

func TestSolver_NoDependencies(t *testing.T) {
	assert := is.New(t)

	provider := &mockProvider{
		packages: map[string][]mockPackage{},
	}

	solver := NewSolver(provider, "myproject", nil)

	result, err := solver.Solve(context.Background())
	assert.NoErr(err)
	assert.Equal(len(result.Decisions), 0)
}

func TestSolver_ConflictingTransitiveDepsNoPanic(t *testing.T) {
	assert := is.New(t)

	// Reproduces a panic in Satisfier() when Intersect returns nil.
	// root → a ^1.0, b ^1.0, c ^1.0
	// a 1.0 → d >=2.0,<3.0
	// b 1.0 → d >=1.0,<2.0
	// c 1.0 → d >=1.5,<2.5
	// Multiple overlapping constraints on d should not panic.
	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "d", Constraint: mustParseConstraint(t, ">=2.0,<3.0")},
				}},
			},
			"b": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "d", Constraint: mustParseConstraint(t, ">=1.0,<2.0")},
				}},
			},
			"c": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "d", Constraint: mustParseConstraint(t, ">=1.5,<2.5")},
				}},
			},
			"d": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
				{ver: mustParseVersion(t, "1.5.0"), deps: nil},
				{ver: mustParseVersion(t, "2.0.0"), deps: nil},
				{ver: mustParseVersion(t, "2.5.0"), deps: nil},
			},
		},
	}

	solver := NewSolver(provider, "myproject", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, "^1.0")},
		{Pkg: "b", Constraint: mustParseConstraint(t, "^1.0")},
		{Pkg: "c", Constraint: mustParseConstraint(t, "^1.0")},
	})

	// Should not panic — either resolves or returns an error.
	_, err := solver.Solve(context.Background())
	if err != nil {
		msg := err.Error()
		assert.True(strings.Contains(msg, "version solving failed"))
	}
}

// TestSolver_ManyVersionsSharedConflict reproduces a 10k-iteration
// thrash: a shared-conflict pattern where the old conflict-resolution
// couldn't generalize across versions and re-explored each one.
//
// Shape: package "a" has 50 versions, all of which depend on "conflict ^2.0".
// Package "b" requires "conflict ^1.0", making every version of "a"
// incompatible with b.
//
// A well-behaved solver should learn ONE range-scoped clause ("no a version
// is compatible") and fail (or succeed by dropping a) in a handful of
// iterations. A solver with unit-exclusion behavior will learn one
// per-version clause per iteration and grind through all 50 (or hit the
// iteration cap if we had more versions).
func TestSolver_ManyVersionsSharedConflict(t *testing.T) {
	assert := is.New(t)

	const numVersions = 50

	aPackages := make([]mockPackage, 0, numVersions)
	for i := 0; i < numVersions; i++ {
		aPackages = append(aPackages, mockPackage{
			ver: mustParseVersion(t, fmt.Sprintf("1.%d.0", i)),
			deps: []Dependency{
				{Pkg: "conflict", Constraint: mustParseConstraint(t, "^2.0")},
			},
		})
	}

	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": aPackages,
			"b": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "conflict", Constraint: mustParseConstraint(t, "^1.0")},
				}},
			},
			"conflict": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
				{ver: mustParseVersion(t, "2.0.0"), deps: nil},
			},
		},
	}

	solver := NewSolver(provider, "myproject", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, ">=1.0,<2.0")},
		{Pkg: "b", Constraint: mustParseConstraint(t, "^1.0")},
	})

	_, err := solver.Solve(context.Background())
	// Unsolvable — every version of a conflicts with b. Guard nil before
	// calling Error() so a regression returning nil fails with a clear
	// message instead of a nil-pointer panic.
	assert.True(err != nil)

	// The solver must NOT blow through the iteration cap. If the fix is
	// in place, conflict resolution generalizes over a's versions and this
	// fails cleanly. If broken, we hit "exceeded 10000 iterations".
	assert.True(!strings.Contains(err.Error(), "exceeded 10000 iterations"))
}

// TestSolver_PreferredPicksPinnedOverNewest confirms: when the
// provider advertises a preferred version that still satisfies the
// current constraints, the solver picks it ahead of the newest-first
// default. Models warm re-lock stability against an existing lockfile.
func TestSolver_PreferredPicksPinnedOverNewest(t *testing.T) {
	assert := is.New(t)

	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
				{ver: mustParseVersion(t, "1.1.0"), deps: nil},
				{ver: mustParseVersion(t, "1.2.0"), deps: nil},
			},
		},
		preferred: map[string]version.Version{
			"a": mustParseVersion(t, "1.1.0"),
		},
	}

	solver := NewSolver(provider, "myproject", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, "^1.0")},
	})

	result, err := solver.Solve(context.Background())
	assert.NoErr(err)
	assert.Equal(result.Decisions["a"].String(), "1.1.0")
}

// TestSolver_PreferredIgnoredWhenUnsatisfiable confirms the solver
// falls through to newest-first when the pinned version no longer
// satisfies current constraints (e.g., user tightened the range
// in pyproject).
func TestSolver_PreferredIgnoredWhenUnsatisfiable(t *testing.T) {
	assert := is.New(t)

	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
				{ver: mustParseVersion(t, "2.0.0"), deps: nil},
				{ver: mustParseVersion(t, "2.5.0"), deps: nil},
			},
		},
		preferred: map[string]version.Version{
			"a": mustParseVersion(t, "1.0.0"), // outside ^2.0
		},
	}

	solver := NewSolver(provider, "myproject", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, "^2.0")},
	})

	result, err := solver.Solve(context.Background())
	assert.NoErr(err)
	assert.Equal(result.Decisions["a"].String(), "2.5.0")
}

// TestSolver_PreferredMissingFromIndex confirms the solver falls
// through when the pinned version was yanked or is otherwise absent
// from the Versions() list. The solver must not crash or stall; it
// picks the newest satisfying version instead.
func TestSolver_PreferredMissingFromIndex(t *testing.T) {
	assert := is.New(t)

	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
				{ver: mustParseVersion(t, "1.2.0"), deps: nil},
			},
		},
		preferred: map[string]version.Version{
			"a": mustParseVersion(t, "1.1.0"), // not in index
		},
	}

	solver := NewSolver(provider, "myproject", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, "^1.0")},
	})

	result, err := solver.Solve(context.Background())
	assert.NoErr(err)
	assert.Equal(result.Decisions["a"].String(), "1.2.0")
}

// TestSolver_ContextCancelled confirms Solve honors ctx cancellation:
// on a canceled context the main loop returns promptly with ctx.Err()
// rather than completing the solve.
func TestSolver_ContextCancelled(t *testing.T) {
	assert := is.New(t)

	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
			},
		},
	}

	solver := NewSolver(provider, "myproject", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, "^1.0")},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := solver.Solve(ctx)
	assert.True(errors.Is(err, context.Canceled))
}
