package resolve

import (
	"fmt"
	"testing"

	"github.com/juanbzz/pensa/pkg/version"
)

// --- Mock Provider ---

type mockPackage struct {
	ver  version.Version
	deps []Dependency
}

type mockProvider struct {
	packages map[string][]mockPackage
}

func (m *mockProvider) Versions(pkg string) ([]version.Version, error) {
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

func (m *mockProvider) Dependencies(pkg string, ver version.Version) ([]Dependency, error) {
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

	result, err := solver.Solve()
	if err != nil {
		t.Fatal(err)
	}

	if v, ok := result.Decisions["a"]; !ok {
		t.Error("expected decision for 'a'")
	} else if v.String() != "1.5.0" {
		t.Errorf("a = %s, want 1.5.0", v)
	}
}

func TestSolver_TwoDependencies(t *testing.T) {
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

	result, err := solver.Solve()
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Decisions) != 2 {
		t.Fatalf("decisions = %d, want 2", len(result.Decisions))
	}
	if result.Decisions["a"].String() != "1.0.0" {
		t.Errorf("a = %s", result.Decisions["a"])
	}
	if result.Decisions["b"].String() != "2.0.0" {
		t.Errorf("b = %s", result.Decisions["b"])
	}
}

func TestSolver_TransitiveDependency(t *testing.T) {
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

	result, err := solver.Solve()
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Decisions) != 2 {
		t.Fatalf("decisions = %d, want 2", len(result.Decisions))
	}
	if result.Decisions["a"].String() != "1.0.0" {
		t.Errorf("a = %s", result.Decisions["a"])
	}
	if _, ok := result.Decisions["b"]; !ok {
		t.Error("expected decision for 'b'")
	}
}

func TestSolver_NoMatchingVersions(t *testing.T) {
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

	_, err := solver.Solve()
	if err == nil {
		t.Error("expected error for no matching versions")
	}
}

func TestSolver_PrefersNewestVersion(t *testing.T) {
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

	result, err := solver.Solve()
	if err != nil {
		t.Fatal(err)
	}

	if result.Decisions["a"].String() != "1.2.0" {
		t.Errorf("a = %s, want 1.2.0 (newest)", result.Decisions["a"])
	}
}

func TestSolver_Backtracking(t *testing.T) {
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

	result, err := solver.Solve()
	if err != nil {
		t.Fatal(err)
	}

	if result.Decisions["a"].String() != "1.0.0" {
		t.Errorf("a = %s, want 1.0.0 (backtracked)", result.Decisions["a"])
	}
	if v := result.Decisions["c"]; v.Major() != 1 {
		t.Errorf("c = %s, want 1.x", v)
	}
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

	_, err := solver.Solve()
	if err == nil {
		t.Error("expected conflict error")
	}
}

func TestSolver_NoDependencies(t *testing.T) {
	provider := &mockProvider{
		packages: map[string][]mockPackage{},
	}

	solver := NewSolver(provider, "myproject", nil)

	result, err := solver.Solve()
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Decisions) != 0 {
		t.Errorf("decisions = %d, want 0", len(result.Decisions))
	}
}
