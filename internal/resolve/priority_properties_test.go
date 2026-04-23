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

// KNOWN behavior, not asserted as correct: on a tie, the FIRST
// candidate in the input slice wins. Since the input comes from
// PartialSolution.Unsatisfied() which walks a Go map, the first
// candidate is effectively random. This makes chooseBest behavior
// NON-DETERMINISTIC across runs for graphs with priority ties — a
// known source of flakiness that motivates the Layer 3 fix.
//
// A naive lexical tiebreak was tried and reverted (2026-04-23) —
// it made lifeandhomes pathological (14+ minute runs), because the
// alphabetically-first pkg happens to be a bad starting decision
// on that graph. The proper fix needs a state-aware tiebreak (MCV,
// VSIDS, or dep-fanout), not just determinism.
func TestPriorityProp_TieFirstWins(t *testing.T) {
	s := solverWithPriorities(map[string]int{
		"a": 50,
		"b": 50,
		"c": 50,
	})
	if got := s.chooseBest([]string{"a", "b", "c"}); got != "a" {
		t.Errorf("got %q; want 'a' (first on tie)", got)
	}
	if got := s.chooseBest([]string{"b", "a", "c"}); got != "b" {
		t.Errorf("got %q; want 'b' (first on tie)", got)
	}
	if got := s.chooseBest([]string{"c", "b", "a"}); got != "c" {
		t.Errorf("got %q; want 'c' (first on tie)", got)
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

// --- Determinism failure demonstration ---

// This test illustrates the non-determinism: given the same priorities
// but different input orderings of equally-prioritized pkgs, chooseBest
// produces different results. This is a real flakiness source that
// Layer 3 must address — BUT only together with a smarter tiebreak,
// because naive lexical ordering picked a pathological first pkg on
// lifeandhomes-class graphs (14+ minute hang).
func TestPriorityProp_NonDeterministicOnTies(t *testing.T) {
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
	if len(seen) != 4 {
		t.Errorf("expected 4 distinct picks across shuffled inputs; got %d: %v",
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
