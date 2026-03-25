package resolve

import (
	"fmt"

	"github.com/juanbzz/pensa/pkg/version"
)

// PartialSolution tracks the solver's current best guess about package versions.
type PartialSolution struct {
	assignments []Assignment
	decisions   map[string]version.Version
	positive    map[string]*Term
	negative    map[string]*Term
	attempts    int
	backtracking bool
}

// NewPartialSolution creates an empty partial solution.
func NewPartialSolution() *PartialSolution {
	return &PartialSolution{
		decisions: make(map[string]version.Version),
		positive:  make(map[string]*Term),
		negative:  make(map[string]*Term),
		attempts:  1,
	}
}

// DecisionLevel returns the current decision level (number of decisions made).
func (ps *PartialSolution) DecisionLevel() int {
	return len(ps.decisions)
}

// Attempts returns the number of solution attempts.
func (ps *PartialSolution) Attempts() int {
	return ps.attempts
}

// Decide adds a decision for a package version.
func (ps *PartialSolution) Decide(pkg string, ver version.Version) {
	if ps.backtracking {
		ps.attempts++
	}
	ps.backtracking = false
	ps.decisions[pkg] = ver

	a := NewDecision(pkg, ver, ps.DecisionLevel(), len(ps.assignments))
	ps.assignments = append(ps.assignments, a)
	ps.register(a)
}

// Derive adds a derived assignment.
func (ps *PartialSolution) Derive(t Term, cause *Incompatibility) {
	a := NewDerivation(t, cause, ps.DecisionLevel(), len(ps.assignments))
	ps.assignments = append(ps.assignments, a)
	ps.register(a)
}

// Backtrack removes all assignments after the given decision level.
func (ps *PartialSolution) Backtrack(level int) {
	ps.backtracking = true

	packages := make(map[string]bool)
	for len(ps.assignments) > 0 && ps.assignments[len(ps.assignments)-1].DecisionLevel > level {
		removed := ps.assignments[len(ps.assignments)-1]
		ps.assignments = ps.assignments[:len(ps.assignments)-1]
		packages[removed.Pkg] = true
		if removed.IsDecision() {
			delete(ps.decisions, removed.Pkg)
		}
	}

	// Recompute positive/negative for affected packages.
	for pkg := range packages {
		delete(ps.positive, pkg)
		delete(ps.negative, pkg)
	}
	for _, a := range ps.assignments {
		if packages[a.Pkg] {
			ps.register(a)
		}
	}
}

// Relation returns how the current solution relates to a term.
func (ps *PartialSolution) Relation(t Term) SetRelation {
	if pos, ok := ps.positive[t.Pkg]; ok {
		return pos.Relation(t)
	}
	if neg, ok := ps.negative[t.Pkg]; ok {
		return neg.Relation(t)
	}
	return Overlapping
}

// Satisfies returns true if the solution satisfies the term.
func (ps *PartialSolution) Satisfies(t Term) bool {
	return ps.Relation(t) == Subset
}

// Satisfier returns the first assignment that, combined with all prior
// assignments for the same package, satisfies the term.
func (ps *PartialSolution) Satisfier(t Term) Assignment {
	var assigned *Term

	for _, a := range ps.assignments {
		if a.Pkg != t.Pkg {
			continue
		}

		if assigned == nil {
			at := a.Term
			assigned = &at
		} else {
			if merged := assigned.Intersect(a.Term); merged != nil {
				assigned = merged
			}
		}

		if assigned != nil && assigned.Satisfies(t) {
			return a
		}
	}

	panic(fmt.Sprintf("BUG: %s is not satisfied", t))
}

// Unsatisfied returns package names that have positive terms but no decision.
func (ps *PartialSolution) Unsatisfied() []string {
	var result []string
	for pkg := range ps.positive {
		if _, decided := ps.decisions[pkg]; !decided {
			result = append(result, pkg)
		}
	}
	return result
}

// Decisions returns a copy of the decisions map.
func (ps *PartialSolution) Decisions() map[string]version.Version {
	result := make(map[string]version.Version, len(ps.decisions))
	for k, v := range ps.decisions {
		result[k] = v
	}
	return result
}

func (ps *PartialSolution) register(a Assignment) {
	name := a.Pkg

	if old, ok := ps.positive[name]; ok {
		intersected := old.Intersect(a.Term)
		if intersected != nil {
			ps.positive[name] = intersected
		}
		return
	}

	old := ps.negative[name]
	var t *Term
	if old == nil {
		at := a.Term
		t = &at
	} else {
		t = a.Term.Intersect(*old)
	}

	if t == nil {
		return
	}

	if t.Positive {
		delete(ps.negative, name)
		ps.positive[name] = t
	} else {
		ps.negative[name] = t
	}
}
