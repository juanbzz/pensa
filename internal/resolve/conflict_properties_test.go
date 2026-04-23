package resolve

import (
	"fmt"
	"testing"

	"pensa.sh/pensa/pkg/version"
)

// resolveConflict is the CDCL-style loop that combines the current
// conflicting clause with the causes of the most-recent-satisfier
// assignment, iterating until it can either backtrack or reach a
// root-level failure.
//
// It's hard to unit-test in isolation (needs a constructed assignment
// history). Instead we drive it via Solve over crafted graphs and
// verify two properties on the result:
//
//   1. End-to-end scenarios: every graph whose solution we know by
//      hand produces that solution.
//
//   2. Consistency invariant: for any successful Solve, every
//      learned-plus-initial incompat is satisfied by the final
//      decisions (no clause fires as conflict). Equivalently, the
//      decisions are a model of every clause.

// --- Scenario tests: graphs with hand-computed solutions ---

// Triangle conflict: a v2 depends on b ^2, b v2 depends on c ^2,
// c's only version is 1.0. Must backtrack a to v1 which accepts c 1.x.
func TestResolveConflict_TriangleBacktrack(t *testing.T) {
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
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
			},
		},
	}
	solver := NewSolver(provider, "proj", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, "^1.0")},
	})
	result, err := solver.Solve()
	if err != nil {
		t.Fatalf("expected solution, got error: %v", err)
	}
	if result.Decisions["a"].String() != "1.0.0" {
		t.Errorf("a=%s; want 1.0.0", result.Decisions["a"])
	}
	if result.Decisions["c"].String() != "1.0.0" {
		t.Errorf("c=%s; want 1.0.0", result.Decisions["c"])
	}
}

// Diamond: a and b both depend on c. a v1 → c ^1, b v1 → c ^2.
// c has versions 1 and 2 only. Only c==2 works if b is picked;
// only c==1 works if a is picked. Can't have both a and b.
// With root requiring both, unsolvable.
func TestResolveConflict_DiamondUnsolvable(t *testing.T) {
	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "c", Constraint: mustParseConstraint(t, "^1.0")},
				}},
			},
			"b": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "c", Constraint: mustParseConstraint(t, "^2.0")},
				}},
			},
			"c": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
				{ver: mustParseVersion(t, "2.0.0"), deps: nil},
			},
		},
	}
	solver := NewSolver(provider, "proj", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, "^1.0")},
		{Pkg: "b", Constraint: mustParseConstraint(t, "^1.0")},
	})
	_, err := solver.Solve()
	if err == nil {
		t.Error("expected conflict error on diamond with incompatible c constraints")
	}
}

// Deep backtrack: a has 5 versions; each a@N requires b@N. b's only
// valid version is 2. Solver must backtrack through a@5, a@4, a@3
// before finding a@2.
func TestResolveConflict_DeepBacktrack(t *testing.T) {
	aVersions := []mockPackage{}
	for i := 1; i <= 5; i++ {
		aVersions = append(aVersions, mockPackage{
			ver: mustParseVersion(t, fmt.Sprintf("%d.0.0", i)),
			deps: []Dependency{
				{Pkg: "b", Constraint: mustParseConstraint(t, fmt.Sprintf("==%d.0.0", i))},
			},
		})
	}
	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": aVersions,
			"b": {
				{ver: mustParseVersion(t, "2.0.0"), deps: nil},
			},
		},
	}
	solver := NewSolver(provider, "proj", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, ">=1.0,<=5.0")},
	})
	result, err := solver.Solve()
	if err != nil {
		t.Fatalf("expected solution, got error: %v", err)
	}
	if result.Decisions["a"].String() != "2.0.0" {
		t.Errorf("a=%s; want 2.0.0 (only version compatible with b==2.0.0)", result.Decisions["a"])
	}
}

// Transitive conflict through multiple levels: a → b → c → d, where
// d has narrow constraint that only certain a versions allow.
func TestResolveConflict_TransitiveChain(t *testing.T) {
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
					{Pkg: "d", Constraint: mustParseConstraint(t, "^3.0")}, // d 3.x doesn't exist
				}},
			},
			"d": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
			},
		},
	}
	solver := NewSolver(provider, "proj", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, ">=1.0")},
	})
	result, err := solver.Solve()
	if err != nil {
		t.Fatalf("expected solution, got error: %v", err)
	}
	// Must pick a=1 to reach d compatible with c's constraint.
	if result.Decisions["a"].String() != "1.0.0" {
		t.Errorf("a=%s; want 1.0.0", result.Decisions["a"])
	}
	if result.Decisions["d"].String() != "1.0.0" {
		t.Errorf("d=%s; want 1.0.0", result.Decisions["d"])
	}
}

// Shared dependency, multiple valid solutions — verify solver picks
// a valid one (not which one; that's priority/heuristic work).
func TestResolveConflict_MultipleValid(t *testing.T) {
	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "c", Constraint: mustParseConstraint(t, ">=1.0,<=2.0")},
				}},
			},
			"b": {
				{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{
					{Pkg: "c", Constraint: mustParseConstraint(t, ">=1.5,<=3.0")},
				}},
			},
			"c": {
				{ver: mustParseVersion(t, "1.0.0"), deps: nil},
				{ver: mustParseVersion(t, "1.5.0"), deps: nil},
				{ver: mustParseVersion(t, "2.0.0"), deps: nil},
				{ver: mustParseVersion(t, "2.5.0"), deps: nil},
				{ver: mustParseVersion(t, "3.0.0"), deps: nil},
			},
		},
	}
	solver := NewSolver(provider, "proj", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, "^1.0")},
		{Pkg: "b", Constraint: mustParseConstraint(t, "^1.0")},
	})
	result, err := solver.Solve()
	if err != nil {
		t.Fatalf("expected solution, got error: %v", err)
	}
	c := result.Decisions["c"]
	lo := mustParseVersion(t, "1.5.0")
	hi := mustParseVersion(t, "2.0.0")
	if version.Compare(c, lo) < 0 || version.Compare(c, hi) > 0 {
		t.Errorf("c=%s; want in [1.5, 2.0]", c)
	}
}

// --- Consistency invariant ---

// verifyDecisionsConsistent checks that the final decisions satisfy
// every initial (non-learned) clause in the solver. Learned clauses
// are logical consequences of initial ones, so we only need to check
// the initial set. But we verify all for safety.
func verifyDecisionsConsistent(t *testing.T, s *Solver, result *SolverResult) {
	t.Helper()

	// Promote decisions to a map of Term-style positive assertions.
	// For each pkg with a decision, the solution's positive term is
	// ExactVersion(decision). Pkgs without decisions were never
	// constrained enough to need one.
	decided := map[string]version.Version{}
	for p, v := range result.Decisions {
		decided[p] = v
	}

	// Walk every clause stored in the solver. A clause is "satisfied"
	// (as an incompat) iff NOT every term holds simultaneously.
	// Equivalently: at least one term is NOT satisfied by decisions.
	//
	// A term `pos(pkg, C)` is "satisfied by decisions" iff we decided
	// pkg in C. `neg(pkg, C)` iff we decided pkg not-in C.
	seen := map[*Incompatibility]bool{}
	for _, incompats := range s.incompatibilities {
		for _, ic := range incompats {
			if seen[ic] {
				continue
			}
			seen[ic] = true

			allSatisfied := true
			for _, tm := range ic.Terms {
				if tm.Pkg == rootPkg {
					// Root is always "decided" but has no concrete version.
					// Positive root term always holds; negative is vacuous.
					if !tm.Positive {
						allSatisfied = false
					}
					continue
				}
				v, ok := decided[tm.Pkg]
				if !ok {
					// Undecided pkg — the clause can't fire against it.
					// Treat as "not satisfied" so the clause doesn't
					// register as violated.
					allSatisfied = false
					continue
				}
				termHolds := tm.Constraint.Allows(v)
				if !tm.Positive {
					termHolds = !termHolds
				}
				if !termHolds {
					allSatisfied = false
					break
				}
			}
			if allSatisfied {
				t.Errorf("clause %s is fully satisfied by final decisions %v — solver produced an inconsistent solution",
					ic, decided)
			}
		}
	}
}

// Consistency holds after solving any graph we've tested.
func TestResolveConflict_ConsistencyInvariant(t *testing.T) {
	// Reuse the same graph shapes as above.
	cases := []struct {
		name    string
		pkgs    map[string][]mockPackage
		roots   []Dependency
		wantErr bool
	}{
		{
			name: "triangle",
			pkgs: map[string][]mockPackage{
				"a": {
					{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{{Pkg: "b", Constraint: mustParseConstraint(t, "^1.0")}}},
					{ver: mustParseVersion(t, "2.0.0"), deps: []Dependency{{Pkg: "b", Constraint: mustParseConstraint(t, "^2.0")}}},
				},
				"b": {
					{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{{Pkg: "c", Constraint: mustParseConstraint(t, "^1.0")}}},
					{ver: mustParseVersion(t, "2.0.0"), deps: []Dependency{{Pkg: "c", Constraint: mustParseConstraint(t, "^2.0")}}},
				},
				"c": {{ver: mustParseVersion(t, "1.0.0"), deps: nil}},
			},
			roots: []Dependency{{Pkg: "a", Constraint: mustParseConstraint(t, "^1.0")}},
		},
		{
			name: "multiple valid",
			pkgs: map[string][]mockPackage{
				"a": {{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{{Pkg: "c", Constraint: mustParseConstraint(t, ">=1.0,<=2.0")}}}},
				"b": {{ver: mustParseVersion(t, "1.0.0"), deps: []Dependency{{Pkg: "c", Constraint: mustParseConstraint(t, ">=1.5,<=3.0")}}}},
				"c": {
					{ver: mustParseVersion(t, "1.0.0"), deps: nil},
					{ver: mustParseVersion(t, "1.5.0"), deps: nil},
					{ver: mustParseVersion(t, "2.0.0"), deps: nil},
					{ver: mustParseVersion(t, "3.0.0"), deps: nil},
				},
			},
			roots: []Dependency{
				{Pkg: "a", Constraint: mustParseConstraint(t, "^1.0")},
				{Pkg: "b", Constraint: mustParseConstraint(t, "^1.0")},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := &mockProvider{packages: tc.pkgs}
			solver := NewSolver(provider, "proj", tc.roots)
			result, err := solver.Solve()
			if err != nil {
				if !tc.wantErr {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			verifyDecisionsConsistent(t, solver, result)
		})
	}
}

// --- Backtrack stats sanity ---

// Attempts count reflects actual backtracking.
func TestResolveConflict_AttemptsReflectBacktracks(t *testing.T) {
	// No-conflict graph — one attempt expected.
	provider := &mockProvider{
		packages: map[string][]mockPackage{
			"a": {{ver: mustParseVersion(t, "1.0.0"), deps: nil}},
		},
	}
	solver := NewSolver(provider, "proj", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, "^1.0")},
	})
	result, err := solver.Solve()
	if err != nil {
		t.Fatal(err)
	}
	if result.Attempts != 1 {
		t.Errorf("no-conflict graph: Attempts=%d, want 1", result.Attempts)
	}

	// Deep backtrack — expect Attempts > 1.
	aVersions := []mockPackage{}
	for i := 1; i <= 5; i++ {
		aVersions = append(aVersions, mockPackage{
			ver: mustParseVersion(t, fmt.Sprintf("%d.0.0", i)),
			deps: []Dependency{
				{Pkg: "b", Constraint: mustParseConstraint(t, fmt.Sprintf("==%d.0.0", i))},
			},
		})
	}
	provider2 := &mockProvider{
		packages: map[string][]mockPackage{
			"a": aVersions,
			"b": {{ver: mustParseVersion(t, "2.0.0"), deps: nil}},
		},
	}
	solver2 := NewSolver(provider2, "proj", []Dependency{
		{Pkg: "a", Constraint: mustParseConstraint(t, ">=1.0,<=5.0")},
	})
	result2, err := solver2.Solve()
	if err != nil {
		t.Fatal(err)
	}
	if result2.Attempts < 2 {
		t.Errorf("deep-backtrack graph: Attempts=%d, want >= 2", result2.Attempts)
	}
}
