package resolve

import (
	"testing"

	"pensa.sh/pensa/pkg/version"
)

// propagateIncompatibility is the decision table that drives the
// solver: given the current solution state, each clause (incompat)
// either
//   - does nothing (not-yet-applicable: two or more terms Overlapping),
//   - marks itself contradicted (any term Disjoint with the state),
//   - derives the inverse of its one Overlapping term, or
//   - signals conflict (all terms Subset of state — clause violated).
//
// These tests construct a Solver in a controlled state and verify
// the decision-table output for each case. The point is to catch
// regressions in the logic that the rest of the solver builds on.

// newTestSolver creates a Solver with an empty state; tests use
// helpers below to seed it with positive/negative constraints per
// package.
func newTestSolver(t *testing.T) *Solver {
	t.Helper()
	return NewSolver(&mockProvider{packages: map[string][]mockPackage{}}, "test", nil)
}

// seedPositive adds a positive derivation for pkg with the given
// constraint. Calling this is equivalent to the solver having derived
// "pkg must be in constraint" from some earlier clause.
func seedPositive(s *Solver, pkg string, c version.Constraint) {
	s.solution.Derive(Term{Pkg: pkg, Constraint: c, Positive: true}, rootIncompat())
}

func seedNegative(s *Solver, pkg string, c version.Constraint) {
	s.solution.Derive(Term{Pkg: pkg, Constraint: c, Positive: false}, rootIncompat())
}

// rootIncompat is used as a dummy cause when seeding derivations.
// PartialSolution doesn't inspect the cause during Derive; it's only
// used later by conflict resolution which these tests don't exercise.
func rootIncompat() *Incompatibility {
	return &Incompatibility{
		Terms: []Term{{Pkg: rootPkg, Constraint: version.AnyConstraint(), Positive: false}},
		Cause: RootCause{},
	}
}

func constV(t *testing.T, s string) version.Version {
	t.Helper()
	v, err := version.Parse(s)
	if err != nil {
		t.Fatalf("version.Parse(%q): %v", s, err)
	}
	return v
}

// --- Scenario: clause with all terms Subset → conflict ---

func TestPropagateProp_AllSubsetIsConflict(t *testing.T) {
	s := newTestSolver(t)
	v1 := constV(t, "1.5")
	v2 := constV(t, "2.5")

	// Seed: pkg a at exactly 1.5, pkg b at exactly 2.5.
	seedPositive(s, "a", version.ExactVersion(v1))
	seedPositive(s, "b", version.ExactVersion(v2))

	// Clause: {a == 1.5, b == 2.5}. Both terms are Subset of state
	// (state's a-term equals the clause's a-term). The clause is
	// violated — both terms hold simultaneously.
	incompat := &Incompatibility{
		Terms: []Term{
			{Pkg: "a", Constraint: version.ExactVersion(v1), Positive: true},
			{Pkg: "b", Constraint: version.ExactVersion(v2), Positive: true},
		},
		Cause: DependencyCause{},
	}

	result := s.propagateIncompatibility(incompat)
	if !result.conflict {
		t.Errorf("expected conflict, got %+v", result)
	}
}

// --- Scenario: any term Disjoint → contradicted, return empty ---

func TestPropagateProp_AnyDisjointIsContradicted(t *testing.T) {
	s := newTestSolver(t)

	// Seed: pkg a is in [1.0, 2.0).
	seedPositive(s, "a",
		version.NewRange(ptrVer(constV(t, "1.0")), ptrVer(constV(t, "2.0")), true, false))

	// Clause term: a == 3.0 (Disjoint with state's [1.0, 2.0)).
	incompat := &Incompatibility{
		Terms: []Term{
			{Pkg: "a", Constraint: version.ExactVersion(constV(t, "3.0")), Positive: true},
			{Pkg: "b", Constraint: version.AnyConstraint(), Positive: true},
		},
		Cause: DependencyCause{},
	}

	result := s.propagateIncompatibility(incompat)
	if result.conflict {
		t.Error("expected no conflict (clause contradicted), got conflict")
	}
	if result.pkg != "" {
		t.Errorf("expected no derivation, got pkg=%q", result.pkg)
	}
	if !s.contradicted[incompat] {
		t.Error("expected clause to be marked contradicted")
	}
}

// --- Scenario: exactly one Overlapping term → derive inverse ---

func TestPropagateProp_OneOverlappingDerives(t *testing.T) {
	s := newTestSolver(t)

	v15 := constV(t, "1.5")
	// Seed: pkg a at exactly 1.5.
	seedPositive(s, "a", version.ExactVersion(v15))

	// Clause: {a == 1.5, b == 2.5}. term 1 is Subset. term 2 is
	// Overlapping (no state about b → Relation returns Overlapping).
	// Expected: derive !b==2.5 → Overlapping term's pkg returned.
	incompat := &Incompatibility{
		Terms: []Term{
			{Pkg: "a", Constraint: version.ExactVersion(v15), Positive: true},
			{Pkg: "b", Constraint: version.ExactVersion(constV(t, "2.5")), Positive: true},
		},
		Cause: DependencyCause{},
	}

	result := s.propagateIncompatibility(incompat)
	if result.conflict {
		t.Error("expected derivation, got conflict")
	}
	if result.pkg != "b" {
		t.Errorf("expected derivation for pkg=b, got pkg=%q", result.pkg)
	}
	// After derivation, solution should have a negative term for b.
	rel := s.solution.Relation(Term{
		Pkg: "b", Constraint: version.ExactVersion(constV(t, "2.5")), Positive: true,
	})
	if rel != Disjoint {
		t.Errorf("after deriving !b==2.5, Relation(b==2.5) = %v; want Disjoint", rel)
	}
}

// --- Scenario: two Overlapping terms → no-op ---

func TestPropagateProp_TwoOverlappingIsNoop(t *testing.T) {
	s := newTestSolver(t)

	// State is empty (no terms about a or b); both terms Overlapping.
	incompat := &Incompatibility{
		Terms: []Term{
			{Pkg: "a", Constraint: version.ExactVersion(constV(t, "1.5")), Positive: true},
			{Pkg: "b", Constraint: version.ExactVersion(constV(t, "2.5")), Positive: true},
		},
		Cause: DependencyCause{},
	}

	result := s.propagateIncompatibility(incompat)
	if result.conflict {
		t.Error("expected no-op, got conflict")
	}
	if result.pkg != "" {
		t.Errorf("expected no-op, got derivation for pkg=%q", result.pkg)
	}
	if s.contradicted[incompat] {
		t.Error("expected clause to not be marked contradicted on no-op")
	}
}

// --- Scenario: single-term clause with Subset state → conflict ---

// A single positive term in an incompat means "this term cannot hold";
// if the state satisfies it, that's a conflict.
func TestPropagateProp_SingleTermSubsetIsConflict(t *testing.T) {
	s := newTestSolver(t)

	v := constV(t, "1.0")
	seedPositive(s, "a", version.ExactVersion(v))

	incompat := &Incompatibility{
		Terms: []Term{{Pkg: "a", Constraint: version.ExactVersion(v), Positive: true}},
		Cause: NoVersionsCause{},
	}

	result := s.propagateIncompatibility(incompat)
	if !result.conflict {
		t.Errorf("expected conflict on single-term Subset clause, got %+v", result)
	}
}

// --- Scenario: negative term handling ---

// Clause `{NOT a==2.0, b==1.0}`:
//   - state: a in [1.0, 3.0) → NOT a==2.0 has Relation Overlapping (state's a
//     includes 2.0, which the negative excludes — partial overlap).
//   - state: no b → b==1.0 is Overlapping.
// Two Overlapping terms → no-op (not enough narrowed yet).
func TestPropagateProp_NegativeTermOverlapping(t *testing.T) {
	s := newTestSolver(t)

	seedPositive(s, "a",
		version.NewRange(ptrVer(constV(t, "1.0")), ptrVer(constV(t, "3.0")), true, false))

	incompat := &Incompatibility{
		Terms: []Term{
			{Pkg: "a", Constraint: version.ExactVersion(constV(t, "2.0")), Positive: false},
			{Pkg: "b", Constraint: version.ExactVersion(constV(t, "1.0")), Positive: true},
		},
		Cause: DependencyCause{},
	}

	result := s.propagateIncompatibility(incompat)
	if result.conflict {
		t.Error("expected no-op, got conflict")
	}
	if result.pkg != "" {
		t.Errorf("expected no-op (two overlapping), got derivation for pkg=%q", result.pkg)
	}
}

// --- Scenario: derivation updates state correctly ---

// After deriving inv(b==1.0), a subsequent propagate on a clause that
// requires b==1.0 should see it as Disjoint (b cannot be 1.0).
func TestPropagateProp_DerivationPropagates(t *testing.T) {
	s := newTestSolver(t)

	v15 := constV(t, "1.5")
	v10 := constV(t, "1.0")

	// Seed: a == 1.5.
	seedPositive(s, "a", version.ExactVersion(v15))

	// First clause: {a==1.5, b==1.0}. Derive !b==1.0.
	first := &Incompatibility{
		Terms: []Term{
			{Pkg: "a", Constraint: version.ExactVersion(v15), Positive: true},
			{Pkg: "b", Constraint: version.ExactVersion(v10), Positive: true},
		},
		Cause: DependencyCause{},
	}
	r1 := s.propagateIncompatibility(first)
	if r1.pkg != "b" {
		t.Fatalf("setup: expected derivation for b, got %+v", r1)
	}

	// Second clause: {b==1.0}. Single positive term. State now has
	// !b==1.0, so b==1.0 is Disjoint with state → clause contradicted.
	second := &Incompatibility{
		Terms: []Term{{Pkg: "b", Constraint: version.ExactVersion(v10), Positive: true}},
		Cause: NoVersionsCause{},
	}
	r2 := s.propagateIncompatibility(second)
	if r2.conflict {
		t.Error("expected contradicted, got conflict")
	}
	if !s.contradicted[second] {
		t.Error("expected second clause to be contradicted after prior derivation")
	}
}
