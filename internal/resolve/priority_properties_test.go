package resolve

import (
	"sort"
	"testing"

	"pensa.sh/pensa/pkg/version"
)

// Priority heuristic audit (goetry-eos Layer 3)
//
// chooseBest picks the next package to decide from a slice of
// unsatisfied package names. The policy:
//
//   - Iterate pkgs[0:] and take the strict-highest priority.
//   - priorities map is populated by addIncompatibility, whose
//     constraintPriority returns: singleton=100, bounded=50, any=10.
//   - On ties, the FIRST candidate in the input slice wins (strict
//     `>` in chooseBest — the old item retains its spot).
//
// These tests pin the behavior so any heuristic change can be
// evaluated against a concrete reference. Failures surface bugs.

// Helper: make a Solver with a seeded priorities map. Bypasses Solve's
// normal flow — we're testing chooseBest in isolation.
func solverWithPriorities(pris map[string]int) *Solver {
	return &Solver{
		priorities: pris,
	}
}

// chooseBest with a single candidate returns it (trivially).
func TestPriorityProp_SingleCandidate(t *testing.T) {
	s := solverWithPriorities(map[string]int{})
	got := s.chooseBest([]string{"only"})
	if got != "only" {
		t.Errorf("got %q; want only", got)
	}
}

// Strict highest priority wins.
func TestPriorityProp_HighestPriorityWins(t *testing.T) {
	s := solverWithPriorities(map[string]int{
		"low":  10,
		"mid":  50,
		"high": 100,
	})
	// Shuffle input orderings — result should be "high" regardless.
	orderings := [][]string{
		{"low", "mid", "high"},
		{"high", "mid", "low"},
		{"mid", "high", "low"},
		{"low", "high", "mid"},
	}
	for _, order := range orderings {
		if got := s.chooseBest(order); got != "high" {
			t.Errorf("order %v: got %q; want high", order, got)
		}
	}
}

// chooseBest tiebreaks ties (equal priority) by fanout — the number
// of clauses referencing the pkg — with name as the final
// deterministic fallback. When fanout is also equal (e.g. in this
// test where s.incompatibilities is empty, so all pkgs have 0
// fanout), name-ascending wins, which is deterministic.
func TestPriorityProp_TieTiebreaksByFanoutThenName(t *testing.T) {
	s := solverWithPriorities(map[string]int{
		"a": 50,
		"b": 50,
		"c": 50,
	})
	// Empty incompatibilities → fanout 0 for all → lexical wins.
	for _, order := range [][]string{
		{"a", "b", "c"},
		{"b", "a", "c"},
		{"c", "b", "a"},
	} {
		if got := s.chooseBest(order); got != "a" {
			t.Errorf("order %v: got %q; want 'a' (all fanout 0 → lexical)", order, got)
		}
	}
}

// Higher fanout wins over lexical.
func TestPriorityProp_FanoutBeatsLexical(t *testing.T) {
	ic := &Incompatibility{}
	s := &Solver{
		priorities: map[string]int{"a": 50, "b": 50, "c": 50},
		incompatibilities: map[string][]*Incompatibility{
			// 'c' has the most clauses referencing it; should win the
			// tiebreak despite losing the lexical subtest.
			"a": {ic},
			"b": {ic, ic},
			"c": {ic, ic, ic},
		},
	}
	for _, order := range [][]string{
		{"a", "b", "c"},
		{"b", "a", "c"},
		{"c", "b", "a"},
	} {
		if got := s.chooseBest(order); got != "c" {
			t.Errorf("order %v: got %q; want 'c' (fanout 3 > 2 > 1)", order, got)
		}
	}
}

// Missing entries in the priorities map are treated as priority 0.
// The map lookup returns the zero int value for absent keys.
func TestPriorityProp_MissingPkgIsLowestPriority(t *testing.T) {
	s := solverWithPriorities(map[string]int{
		"known": 50,
	})
	got := s.chooseBest([]string{"unknown", "known"})
	if got != "known" {
		t.Errorf("got %q; want known (50 > 0 for unknown)", got)
	}
}

// --- constraintPriority levels ---

func TestPriorityProp_ConstraintPriorityLevels(t *testing.T) {
	v1 := mustParseVersion(t, "1.0.0")
	cases := []struct {
		name string
		c    version.Constraint
		want int
	}{
		{"singleton", version.ExactVersion(v1), 100},
		{"bounded", mustParseConstraint(t, ">=1.0,<2.0"), 50},
		{"unbounded-above", mustParseConstraint(t, ">=1.0"), 50},
		{"any", version.AnyConstraint(), 10},
	}
	for _, tc := range cases {
		if got := constraintPriority(tc.c); got != tc.want {
			t.Errorf("constraintPriority(%s) = %d; want %d", tc.name, got, tc.want)
		}
	}
}

// --- Accumulation via addIncompatibility ---

// The priorities map is MONOTONICALLY INCREASING — once a pkg's
// priority goes up (e.g. because a singleton clause was added for it),
// it never comes back down, even if that singleton clause gets
// contradicted or backtracked past.
//
// This is load-bearing for eos: it means priorities don't reflect the
// CURRENT solver state, they reflect the MAX priority ever seen. A
// package that temporarily had a tight constraint stays "high
// priority" forever. When the solver is choosing between two
// packages, it picks based on stale history, not current constraints.
func TestPriorityProp_MonotonicallyIncreasing(t *testing.T) {
	s := &Solver{
		incompatibilities: map[string][]*Incompatibility{},
		priorities:        map[string]int{},
	}
	v := mustParseVersion(t, "1.0.0")

	// Start with a bounded term for `a` — priority becomes 50.
	s.addIncompatibility(&Incompatibility{
		Terms: []Term{
			{Pkg: "a", Constraint: mustParseConstraint(t, ">=1.0,<2.0"), Positive: true},
		},
		Cause: DependencyCause{},
	})
	if s.priorities["a"] != 50 {
		t.Errorf("after bounded term: priorities[a] = %d; want 50", s.priorities["a"])
	}

	// Add a singleton term for `a` — priority bumps to 100.
	s.addIncompatibility(&Incompatibility{
		Terms: []Term{
			{Pkg: "a", Constraint: version.ExactVersion(v), Positive: true},
		},
		Cause: DependencyCause{},
	})
	if s.priorities["a"] != 100 {
		t.Errorf("after singleton term: priorities[a] = %d; want 100", s.priorities["a"])
	}

	// Add ANOTHER bounded term for `a` — priority stays 100 (never
	// decreases). In reality the current state of `a` may no longer
	// involve a singleton constraint, but chooseBest doesn't know.
	s.addIncompatibility(&Incompatibility{
		Terms: []Term{
			{Pkg: "a", Constraint: mustParseConstraint(t, ">=3.0"), Positive: true},
		},
		Cause: DependencyCause{},
	})
	if s.priorities["a"] != 100 {
		t.Errorf("after adding bounded term on top: priorities[a] = %d; want 100 (monotonic)",
			s.priorities["a"])
	}
}

// --- Determinism invariant ---

// chooseBest is now deterministic: same priorities + same fanout +
// same pkg set → same pick regardless of input slice order. (Go's
// randomized map iteration in PartialSolution.Unsatisfied can no
// longer leak into solver behavior.)
func TestPriorityProp_DeterministicAcrossOrderings(t *testing.T) {
	s := solverWithPriorities(map[string]int{
		"a": 50, "b": 50, "c": 50, "d": 50,
	})
	seen := map[string]bool{}
	orderings := [][]string{
		{"a", "b", "c", "d"},
		{"b", "c", "d", "a"},
		{"c", "d", "a", "b"},
		{"d", "a", "b", "c"},
	}
	for _, order := range orderings {
		seen[s.chooseBest(order)] = true
	}
	if len(seen) != 1 {
		t.Errorf("expected 1 deterministic pick across orderings; got %d: %v",
			len(seen), sortedKeys(seen))
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
