package resolve

import (
	"fmt"
	"testing"

	"pensa.sh/pensa/pkg/version"
)

// The solvability property: given a graph that has at least one valid
// assignment, Solve must return a non-error result AND the decisions
// must satisfy every root-level constraint. Failure of this property
// is a search-order / priority-heuristic bug — not a correctness bug
// in primitives (Layer 0 / 1 covers those).
//
// These tests construct graphs shaped like real-world Python patterns
// that historically strained pensa: large version sets with narrow
// transitive constraints, diamonds with
// overlapping-but-not-identical requirements, chains where a leaf
// dominates upstream choice.
//
// Each test asserts Solve returns a valid solution, not a specific
// one — so heuristic churn doesn't cause churn in these tests.

// assertSolved runs Solve, fails if err != nil, and verifies every
// root dep's constraint is satisfied by the chosen version.
func assertSolved(t *testing.T, provider *mockProvider, roots []Dependency) map[string]version.Version {
	t.Helper()
	solver := NewSolver(provider, "proj", roots)
	result, err := solver.Solve()
	if err != nil {
		t.Fatalf("expected solution, got error: %v", err)
	}
	for _, root := range roots {
		v, ok := result.Decisions[root.Pkg]
		if !ok {
			t.Errorf("root dep %q missing from decisions", root.Pkg)
			continue
		}
		if !root.Constraint.Allows(v) {
			t.Errorf("root dep %q=%s violates root constraint %s",
				root.Pkg, v, root.Constraint)
		}
	}
	return result.Decisions
}

// --- Shared transitive (diamond) ---

// a and b both depend on c with overlapping ranges. Solver picks c
// in the intersection.
func TestSolvability_DiamondOverlapping(t *testing.T) {
	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "c", Constraint: mustParseConstraint(t, ">=1.0,<=3.0")},
				}},
			},
			"b": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "c", Constraint: mustParseConstraint(t, ">=2.0,<=4.0")},
				}},
			},
			"c": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
				{ver: mustParseVersion(t, "2.0.0"), deps: nil},
				{ver: mustParseVersion(t, "3.0.0"), deps: nil},
				{ver: mustParseVersion(t, "4.0.0"), deps: nil},
			},
		},
	}
	decisions := assertSolved(t, provider, []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, "^1.0")},
		{Pkg: "b", Constraint: mustParseConstraint(t, "^1.0")},
	})
	c := decisions["c"]
	lo := mustParseVersion(t, "2.0.0")
	hi := mustParseVersion(t, "3.0.0")
	if version.Compare(c, lo) < 0 || version.Compare(c, hi) > 0 {
		t.Errorf("c=%s; not in intersection [2.0, 3.0]", c)
	}
}

// --- Many-versioned dominant pkg + narrow transitive ---

// This is the boto3/urllib3-style shape from Poetry's comparator
// inversion (commit 36332d25). Pkg A has 30 versions. Each A version
// requires t in a narrow range. B constrains t tightly, forcing a
// particular A. The solver must not thrash through all 30 A versions.
func TestSolvability_BotoStylePackage(t *testing.T) {
	aPackages := make([]mockPackage, 0, 30)
	for i := 1; i <= 30; i++ {
		aPackages = append(aPackages, mockPackage{
			ver: mustParseVersion(t, fmt.Sprintf("1.%d.0", i)),
			deps: []Dependency{
				{Pkg: "t", Constraint: mustParseConstraint(t, fmt.Sprintf(">=%d.0,<%d.0", i, i+1))},
			},
		})
	}
	tVersions := make([]mockPackage, 0, 30)
	for i := 1; i <= 30; i++ {
		tVersions = append(tVersions, mockPackage{
			ver: mustParseVersion(t, fmt.Sprintf("%d.0.0", i)),
		})
	}
	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": aPackages,
			"b": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "t", Constraint: mustParseConstraint(t, ">=15.0,<16.0")}, // only a v15 works
				}},
			},
			"t": tVersions,
		},
	}
	decisions := assertSolved(t, provider, []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, "^1.0")},
		{Pkg: "b", Constraint: mustParseConstraint(t, "^1.0")},
	})
	a := decisions["a"]
	if a.String() != "1.15.0" {
		t.Errorf("a=%s; only 1.15.0 works with b's t constraint", a)
	}
}

// --- Chain with leaf-dominant constraint ---

// a → b → c → d where d has a narrow valid range. Chain leaves
// (d) dominate upstream choices despite root only specifying a.
func TestSolvability_LeafDominantChain(t *testing.T) {
	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "b", Constraint: mustParseConstraint(t, "^1.0")},
				}},
				{ver: mustParseVersion(t, "2.0.0"), deps: []Dependency{
					{Pkg: "b", Constraint: mustParseConstraint(t, "^2.0")},
				}},
			},
			"b": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "c", Constraint: mustParseConstraint(t, "^1.0")},
				}},
				{ver: mustParseVersion(t, "2.0.0"), deps: []Dependency{
					{Pkg: "c", Constraint: mustParseConstraint(t, "^2.0")},
				}},
			},
			"c": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "d", Constraint: mustParseConstraint(t, "^1.0")},
				}},
				{ver: mustParseVersion(t, "2.0.0"), deps: []Dependency{
					{Pkg: "d", Constraint: mustParseConstraint(t, "^5.0")}, // impossible
				}},
			},
			"d": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
			},
		},
	}
	decisions := assertSolved(t, provider, []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, ">=1.0")},
	})
	// Only a=1.0 allows reaching d=1.0 via c=1.0 → b=1.0.
	if decisions["a"].String() != "1.0.0" {
		t.Errorf("a=%s; want 1.0.0 (leaf d forces it)", decisions["a"])
	}
}

// --- Multiple narrow transitive constraints on a single pkg ---

// Mimics lifeandhomes's typing-extensions situation in miniature:
// pkg t is constrained by multiple root-level packages with different
// ranges that must all overlap at a single valid t version.
func TestSolvability_MultipleTransitiveConstraints(t *testing.T) {
	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"p1": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "t", Constraint: mustParseConstraint(t, ">=4.5.0")},
				}},
			},
			"p2": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "t", Constraint: mustParseConstraint(t, "<5.0.0")},
				}},
			},
			"p3": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "t", Constraint: mustParseConstraint(t, "^4.0")},
				}},
			},
			"t": {
				{ver: mustParseVersion(t, "3.0.0"), deps: nil},
				{ver: mustParseVersion(t, "4.0.0"), deps: nil},
				{ver: mustParseVersion(t, "4.5.0"), deps: nil},
				{ver: mustParseVersion(t, "4.9.0"), deps: nil},
				{ver: mustParseVersion(t, "5.0.0"), deps: nil},
				{ver: mustParseVersion(t, "6.0.0"), deps: nil},
			},
		},
	}
	decisions := assertSolved(t, provider, []Dependency{
		{Pkg: "p1", Constraint: mustParseConstraint(t, "^1.0")},
		{Pkg: "p2", Constraint: mustParseConstraint(t, "^1.0")},
		{Pkg: "p3", Constraint: mustParseConstraint(t, "^1.0")},
	})
	tv := decisions["t"]
	// Valid t ∈ [4.5, 5.0) — overlap of all three constraints.
	lo := mustParseVersion(t, "4.5.0")
	hi := mustParseVersion(t, "4.9.0")
	if version.Compare(tv, lo) < 0 || version.Compare(tv, hi) > 0 {
		t.Errorf("t=%s; not in overlap [4.5, 4.9]", tv)
	}
}

// --- Packages with version-specific transitive deps (lifeandhomes pattern) ---

// pkg P has many versions; each version's dep on transitive T is a
// slightly different narrow range. B's constraint on T forces a
// specific P version. This shape caused the Stage 2 regressions.
func TestSolvability_VersionSpecificTransitiveDeps(t *testing.T) {
	// P versions 1..20; each requires T = v (an exact singleton,
	// varying by P version).
	pPackages := make([]mockPackage, 0, 20)
	for i := 1; i <= 20; i++ {
		pPackages = append(pPackages, mockPackage{
			ver: mustParseVersion(t, fmt.Sprintf("%d.0.0", i)),
			deps: []Dependency{
				{Pkg: "t", Constraint: mustParseConstraint(t, fmt.Sprintf("==%d.0.0", i))},
			},
		})
	}
	// B locks T to exactly 7.0.0.
	bPackage := mockPackage{
		ver: mustParseVersion(t, "1.0.0"),
		deps: []Dependency{
			{Pkg: "t", Constraint: mustParseConstraint(t, "==7.0.0")},
		},
	}
	tVersions := make([]mockPackage, 0, 20)
	for i := 1; i <= 20; i++ {
		tVersions = append(tVersions, mockPackage{
			ver: mustParseVersion(t, fmt.Sprintf("%d.0.0", i)),
		})
	}
	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"p": pPackages,
			"b": {bPackage},
			"t": tVersions,
		},
	}
	decisions := assertSolved(t, provider, []Dependency{
		{Pkg: "p", Constraint: mustParseConstraint(t, ">=1.0,<=20.0")},
		{Pkg: "b", Constraint: mustParseConstraint(t, "^1.0")},
	})
	if decisions["p"].String() != "7.0.0" || decisions["t"].String() != "7.0.0" {
		t.Errorf("p=%s t=%s; want both 7.0.0 (only compatible with b's t constraint)",
			decisions["p"], decisions["t"])
	}
}

// --- Wide graph, no conflicts ---

// 20 independent direct deps, no transitives, no constraints — should
// resolve cleanly in linear time with zero backtracks.
func TestSolvability_WideIndependentGraph(t *testing.T) {
	pkgs := map[string][]mockPackage{}
	roots := []Dependency{}
	for i := 1; i <= 20; i++ {
		name := fmt.Sprintf("p%d", i)
		pkgs[name] = []mockPackage{
			{ver: mustParseVersion(t, "1.0.0"), deps: nil},
		}
		roots = append(roots, Dependency{
			Pkg:        name,
			Constraint: mustParseConstraint(t, "^1.0"),
		})
	}
	provider := &mockProvider{packages: pkgs}
	assertSolved(t, provider, roots)
}

// --- Shared dep reachable via two distinct paths ---

// root → a → shared; root → b → shared, with shared reached twice.
// Both paths must pick the same shared version.
func TestSolvability_SharedDepViaTwoPaths(t *testing.T) {
	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "shared", Constraint: mustParseConstraint(t, "^1.0")},
				}},
			},
			"b": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "shared", Constraint: mustParseConstraint(t, "^1.0")},
				}},
			},
			"shared": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
				{ver: mustParseVersion(t, "1.5.0"), deps: nil},
			},
		},
	}
	decisions := assertSolved(t, provider, []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, "^1.0")},
		{Pkg: "b", Constraint: mustParseConstraint(t, "^1.0")},
	})
	if _, ok := decisions["shared"]; !ok {
		t.Error("shared package missing from decisions")
	}
}

// --- Backtrack-required with valid alternative ---

// Solver's first pick (a@latest) leads to a dead-end; a second pick
// (a@earlier) works.
func TestSolvability_BacktrackFindsAlternative(t *testing.T) {
	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "b", Constraint: mustParseConstraint(t, "^1.0")},
				}},
				{ver: mustParseVersion(t, "2.0.0"), deps: []Dependency{
					{Pkg: "b", Constraint: mustParseConstraint(t, "^3.0")}, // no b 3.x exists
				}},
			},
			"b": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
				{ver: mustParseVersion(t, "2.0.0"), deps: nil},
			},
		},
	}
	decisions := assertSolved(t, provider, []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, ">=1.0")},
	})
	if decisions["a"].String() != "1.0.0" {
		t.Errorf("a=%s; want 1.0.0 (a@2 leads to no valid b)", decisions["a"])
	}
}

// --- Scale-up: lifeandhomes-like complexity ---

// Mimics a real-world workspace pattern: many direct deps, one
// heavy-versioned pkg, version-specific
// transitive constraints on a shared low-level pkg, and an overlap
// that pins the heavy pkg to a specific version.
//
// Shape:
//   - `heavy` has 50 versions. Each heavy@v requires `t` in a narrow
//     range that walks across t's version space (v=1→t 1.x, v=2→t 2.x,
//     ..., v=50→t 50.x).
//   - `leaf1` requires `t ^15.0` (only heavy@15 fits).
//   - `leaf2` requires `t ^15.0` (reinforcing, same range).
//   - Additional 10 independent root deps that don't interact with
//     the heavy/t subgraph (width noise).
//
// A valid solution exists: heavy=1.15.0, t=15.x.y, leaf1=leaf2=1.0.0,
// plus the 10 independent pkgs at their sole version. If the solver
// can't find it quickly, the eos symptom reproduces here.
func TestSolvability_LifeandhomesScale(t *testing.T) {
	if testing.Short() {
		t.Skip("scale test; skip in -short mode")
	}

	pkgs := map[string][]mockPackage{}

	// heavy: 50 versions; heavy@i requires t in [i.0, i+1).
	heavyVersions := make([]mockPackage, 0, 50)
	for i := 1; i <= 50; i++ {
		heavyVersions = append(heavyVersions, mockPackage{
			ver: mustParseVersion(t, fmt.Sprintf("1.%d.0", i)),
			deps: []Dependency{
				{Pkg: "t", Constraint: mustParseConstraint(t,
					fmt.Sprintf(">=%d.0,<%d.0", i, i+1))},
			},
		})
	}
	pkgs["heavy"] = heavyVersions

	// t: 50 versions, three sub-revisions each (so heavy@i has multiple
	// compatible picks in [i.0, i+1)).
	tVersions := make([]mockPackage, 0, 150)
	for i := 1; i <= 50; i++ {
		for j := 0; j < 3; j++ {
			tVersions = append(tVersions, mockPackage{
				ver: mustParseVersion(t, fmt.Sprintf("%d.%d.0", i, j)),
			})
		}
	}
	pkgs["t"] = tVersions

	// leaf1 and leaf2 pin t to ^15.0 — only heavy@15 is compatible.
	pkgs["leaf1"] = []mockPackage{
		{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
			{Pkg: "t", Constraint: mustParseConstraint(t, "^15.0")},
		}},
	}
	pkgs["leaf2"] = []mockPackage{
		{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
			{Pkg: "t", Constraint: mustParseConstraint(t, "^15.0")},
		}},
	}

	// 10 noise deps: isolated, single version, no interaction.
	roots := []Dependency{
		{Pkg: "heavy", Constraint: mustParseConstraint(t, "^1.0")},
		{Pkg: "leaf1", Constraint: mustParseConstraint(t, "^1.0")},
		{Pkg: "leaf2", Constraint: mustParseConstraint(t, "^1.0")},
	}
	for i := 1; i <= 10; i++ {
		name := fmt.Sprintf("noise%d", i)
		pkgs[name] = []mockPackage{
			{ver: mustParseVersion(t, "1.0.0"), deps: nil},
		}
		roots = append(roots, Dependency{
			Pkg: name, Constraint: mustParseConstraint(t, "^1.0"),
		})
	}

	provider := &mockProvider{packages: pkgs}
	decisions := assertSolved(t, provider, roots)

	heavyV := decisions["heavy"]
	if heavyV.String() != "1.15.0" {
		t.Errorf("heavy=%s; want 1.15.0 (only one compatible with leafs' t^15.0)", heavyV)
	}
	// t must be in [15.0, 16.0).
	tv := decisions["t"]
	lo := mustParseVersion(t, "15.0.0")
	hi := mustParseVersion(t, "15.2.0")
	if version.Compare(tv, lo) < 0 || version.Compare(tv, hi) > 0 {
		t.Errorf("t=%s; want in [15.0, 15.2]", tv)
	}
}

// --- Scale-up: dual heavy packages with overlapping windows ---

// Two heavy packages (each 30 versions) sharing a transitive dep `t`.
// Constraints structured so the overlap is narrow — only a specific
// heavy1 + heavy2 combination works. More demanding than the single-
// heavy test above because the solver must align two large version
// spaces.
func TestSolvability_DualHeavyOverlap(t *testing.T) {
	if testing.Short() {
		t.Skip("scale test; skip in -short mode")
	}

	pkgs := map[string][]mockPackage{}

	// heavy1: 30 versions; heavy1@i requires t in [i, i+5).
	heavy1 := make([]mockPackage, 0, 30)
	for i := 1; i <= 30; i++ {
		heavy1 = append(heavy1, mockPackage{
			ver: mustParseVersion(t, fmt.Sprintf("1.%d.0", i)),
			deps: []Dependency{
				{Pkg: "t", Constraint: mustParseConstraint(t,
					fmt.Sprintf(">=%d.0,<%d.0", i, i+5))},
			},
		})
	}
	pkgs["heavy1"] = heavy1

	// heavy2: 30 versions; heavy2@i requires t in [30-i, 32-i).
	// Reverse direction — so heavy1 and heavy2 only overlap in a
	// narrow t window.
	heavy2 := make([]mockPackage, 0, 30)
	for i := 1; i <= 30; i++ {
		lo := 30 - i
		if lo < 1 {
			lo = 1
		}
		hi := 32 - i
		if hi < lo+1 {
			hi = lo + 1
		}
		heavy2 = append(heavy2, mockPackage{
			ver: mustParseVersion(t, fmt.Sprintf("1.%d.0", i)),
			deps: []Dependency{
				{Pkg: "t", Constraint: mustParseConstraint(t,
					fmt.Sprintf(">=%d.0,<%d.0", lo, hi))},
			},
		})
	}
	pkgs["heavy2"] = heavy2

	// t: 50 versions.
	tv := make([]mockPackage, 0, 50)
	for i := 1; i <= 50; i++ {
		tv = append(tv, mockPackage{
			ver: mustParseVersion(t, fmt.Sprintf("%d.0.0", i)),
		})
	}
	pkgs["t"] = tv

	roots := []Dependency{
		{Pkg: "heavy1", Constraint: mustParseConstraint(t, "^1.0")},
		{Pkg: "heavy2", Constraint: mustParseConstraint(t, "^1.0")},
	}

	provider := &mockProvider{packages: pkgs}
	assertSolved(t, provider, roots)
	// No specific assertion on which version — many valid
	// configurations exist; we just require the solver finds one.
}
