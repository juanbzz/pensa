package resolve

import (
	"fmt"
	"sort"

	"github.com/juanbzz/goetry/pkg/version"
)

// sentinel value for conflict detection.
var conflict = &struct{}{}

// SolveError indicates the resolver could not find a valid solution.
type SolveError struct {
	Incompatibility *Incompatibility
}

func (e *SolveError) Error() string {
	return fmt.Sprintf("version solving failed: %s", e.Incompatibility)
}

// Solver implements the PubGrub version solving algorithm.
type Solver struct {
	provider          Provider
	root              string
	rootDeps          []Dependency
	incompatibilities map[string][]*Incompatibility
	contradicted      map[*Incompatibility]bool
	solution          *PartialSolution
}

// NewSolver creates a new PubGrub solver.
func NewSolver(provider Provider, root string, rootDeps []Dependency) *Solver {
	return &Solver{
		provider:          provider,
		root:              root,
		rootDeps:          rootDeps,
		incompatibilities: make(map[string][]*Incompatibility),
		contradicted:      make(map[*Incompatibility]bool),
		solution:          NewPartialSolution(),
	}
}

// Solve finds a set of package versions satisfying all constraints.
func (s *Solver) Solve() (*SolverResult, error) {
	s.addIncompatibility(&Incompatibility{
		Terms: []Term{{Pkg: rootPkg, Constraint: version.AnyConstraint(), Positive: false}},
		Cause: RootCause{},
	})

	for _, dep := range s.rootDeps {
		s.addIncompatibility(&Incompatibility{
			Terms: []Term{
				{Pkg: rootPkg, Constraint: version.AnyConstraint(), Positive: true},
				{Pkg: dep.Pkg, Constraint: dep.Constraint, Positive: false},
			},
			Cause: DependencyCause{},
		})
	}

	s.solution.Decide(rootPkg, version.Version{})

	if err := s.propagate(rootPkg); err != nil {
		return nil, err
	}

	for iterations := 0; ; iterations++ {
		if iterations > 1000 {
			return nil, fmt.Errorf("solver: exceeded 1000 iterations")
		}
		pkg, err := s.choosePackageVersion()
		if err != nil {
			return nil, err
		}
		if pkg == "" {
			break
		}
		if err := s.propagate(pkg); err != nil {
			return nil, err
		}
	}

	decisions := s.solution.Decisions()
	delete(decisions, rootPkg)

	return &SolverResult{
		Decisions: decisions,
		Attempts:  s.solution.Attempts(),
	}, nil
}

func (s *Solver) propagate(pkg string) error {
	changed := map[string]bool{pkg: true}

	for len(changed) > 0 {
		var current string
		for p := range changed {
			current = p
			break
		}
		delete(changed, current)

		incompats := s.incompatibilities[current]
		for i := len(incompats) - 1; i >= 0; i-- {
			if s.contradicted[incompats[i]] {
				continue
			}
			result := s.propagateIncompatibility(incompats[i])

			if result == conflict {
				rootCause, err := s.resolveConflict(incompats[i])
				if err != nil {
					return err
				}
				result2 := s.propagateIncompatibility(rootCause)
				if result2 == conflict {
					return fmt.Errorf("BUG: propagation after conflict resolution yielded another conflict")
				}
				if pkgName, ok := result2.(string); ok {
					changed = map[string]bool{pkgName: true}
				}
				break
			}

			if pkgName, ok := result.(string); ok {
				changed[pkgName] = true
			}
		}
	}

	return nil
}

func (s *Solver) propagateIncompatibility(incompat *Incompatibility) interface{} {
	var unsatisfied *Term

	for i := range incompat.Terms {
		t := &incompat.Terms[i]
		rel := s.solution.Relation(*t)

		if rel == Disjoint {
			s.contradicted[incompat] = true
			return nil
		}

		if rel == Overlapping {
			if unsatisfied != nil {
				return nil
			}
			unsatisfied = t
		}
	}

	if unsatisfied == nil {
		return conflict
	}

	s.contradicted[incompat] = true

	inv := unsatisfied.Inverse()
	s.solution.Derive(inv, incompat)
	return unsatisfied.Pkg
}

func (s *Solver) resolveConflict(incompat *Incompatibility) (*Incompatibility, error) {
	newIncompat := false

	for !incompat.IsFailure() {
		var mostRecentTerm *Term
		var mostRecentSatisfier *Assignment
		var difference *Term
		previousSatisfierLevel := 1

		for i := range incompat.Terms {
			t := &incompat.Terms[i]
			satisfier := s.solution.Satisfier(*t)

			if mostRecentSatisfier == nil || mostRecentSatisfier.Index < satisfier.Index {
				if mostRecentSatisfier != nil {
					if mostRecentSatisfier.DecisionLevel > previousSatisfierLevel {
						previousSatisfierLevel = mostRecentSatisfier.DecisionLevel
					}
				}
				mostRecentTerm = t
				mostRecentSatisfier = &satisfier
				difference = nil
			} else {
				if satisfier.DecisionLevel > previousSatisfierLevel {
					previousSatisfierLevel = satisfier.DecisionLevel
				}
			}

			if mostRecentTerm == t {
				difference = mostRecentSatisfier.Term.Difference(*mostRecentTerm)
				if difference != nil {
					diffInv := difference.Inverse()
					diffSatisfier := s.solution.Satisfier(diffInv)
					if diffSatisfier.DecisionLevel > previousSatisfierLevel {
						previousSatisfierLevel = diffSatisfier.DecisionLevel
					}
				}
			}
		}

		if mostRecentSatisfier == nil {
			return nil, &SolveError{Incompatibility: incompat}
		}

		if previousSatisfierLevel < mostRecentSatisfier.DecisionLevel || mostRecentSatisfier.IsDecision() {
			s.solution.Backtrack(previousSatisfierLevel)
			s.contradicted = make(map[*Incompatibility]bool)
			if newIncompat {
				s.addIncompatibility(incompat)
			}
			return incompat, nil
		}

		// Combine with the cause of the most recent satisfier.
		var newTerms []Term
		for i := range incompat.Terms {
			if &incompat.Terms[i] != mostRecentTerm {
				newTerms = append(newTerms, incompat.Terms[i])
			}
		}
		for _, t := range mostRecentSatisfier.Cause.Terms {
			if t.Pkg != mostRecentSatisfier.Pkg {
				newTerms = append(newTerms, t)
			}
		}
		if difference != nil {
			inv := difference.Inverse()
			if inv.Pkg != mostRecentSatisfier.Pkg {
				newTerms = append(newTerms, inv)
			}
		}

		incompat = &Incompatibility{
			Terms: coalesceTerms(newTerms),
			Cause: ConflictCause{Conflict: incompat, Other: mostRecentSatisfier.Cause},
		}
		newIncompat = true
	}

	return nil, &SolveError{Incompatibility: incompat}
}

func (s *Solver) choosePackageVersion() (string, error) {
	unsatisfied := s.solution.Unsatisfied()
	if len(unsatisfied) == 0 {
		return "", nil
	}

	pkg := s.chooseBest(unsatisfied)

	versions, err := s.provider.Versions(pkg)
	if err != nil {
		return "", fmt.Errorf("fetch versions for %s: %w", pkg, err)
	}

	sort.Slice(versions, func(i, j int) bool {
		return version.Compare(versions[i], versions[j]) > 0
	})

	var chosen *version.Version
	for i := range versions {
		v := versions[i]
		if s.solution.Relation(Term{Pkg: pkg, Constraint: version.ExactVersion(v), Positive: true}) != Disjoint {
			chosen = &v
			break
		}
	}

	if chosen == nil {
		positiveTerm := s.positiveTermFor(pkg)
		s.addIncompatibility(&Incompatibility{
			Terms: []Term{positiveTerm},
			Cause: NoVersionsCause{},
		})
		return pkg, nil
	}

	deps, err := s.provider.Dependencies(pkg, *chosen)
	if err != nil {
		return "", fmt.Errorf("fetch dependencies for %s %s: %w", pkg, chosen, err)
	}

	chosenConstraint := version.ExactVersion(*chosen)
	for _, dep := range deps {
		s.addIncompatibility(&Incompatibility{
			Terms: []Term{
				{Pkg: pkg, Constraint: chosenConstraint, Positive: true},
				{Pkg: dep.Pkg, Constraint: dep.Constraint, Positive: false},
			},
			Cause: DependencyCause{},
		})
	}

	s.solution.Decide(pkg, *chosen)

	return pkg, nil
}

func (s *Solver) positiveTermFor(pkg string) Term {
	if pos, ok := s.solution.positive[pkg]; ok {
		return *pos
	}
	return Term{Pkg: pkg, Constraint: version.AnyConstraint(), Positive: true}
}

func (s *Solver) chooseBest(pkgs []string) string {
	if len(pkgs) == 1 {
		return pkgs[0]
	}
	sort.Strings(pkgs)
	return pkgs[0]
}

func (s *Solver) addIncompatibility(incompat *Incompatibility) {
	for _, t := range incompat.Terms {
		s.incompatibilities[t.Pkg] = append(s.incompatibilities[t.Pkg], incompat)
	}
}

// coalesceTerms merges terms about the same package.
func coalesceTerms(terms []Term) []Term {
	byPkg := make(map[string]*Term)
	var order []string

	for _, t := range terms {
		if existing, ok := byPkg[t.Pkg]; ok {
			merged := existing.Intersect(t)
			if merged != nil {
				byPkg[t.Pkg] = merged
			} else if t.Positive {
				byPkg[t.Pkg] = &t
			}
		} else {
			tc := t
			byPkg[t.Pkg] = &tc
			order = append(order, t.Pkg)
		}
	}

	result := make([]Term, 0, len(byPkg))
	for _, pkg := range order {
		if t, ok := byPkg[pkg]; ok {
			result = append(result, *t)
		}
	}
	return result
}
