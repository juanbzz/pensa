package resolve

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"pensa.sh/pensa/pkg/version"
)

// propagateResult represents the outcome of propagating an incompatibility.
type propagateResult struct {
	conflict bool   // true if all terms are satisfied (conflict detected)
	pkg      string // non-empty when a new derivation was added for this package
}

// SolveError indicates the resolver could not find a valid solution.
type SolveError struct {
	Incompatibility *Incompatibility
	Root            string
}

func (e *SolveError) Error() string {
	conflicts := collectConflicts(e.Incompatibility, map[*Incompatibility]bool{})
	if len(conflicts) == 0 {
		return fmt.Sprintf("version solving failed: %s", e.Incompatibility)
	}
	projectName := e.Root
	if projectName == "" {
		projectName = "your project"
	}
	lines := make([]string, len(conflicts))
	for i, c := range conflicts {
		lines[i] = "  - " + formatConflict(c, projectName)
	}
	return "version solving failed:\n" + strings.Join(lines, "\n")
}

func collectConflicts(incompat *Incompatibility, seen map[*Incompatibility]bool) []*Incompatibility {
	if seen[incompat] {
		return nil
	}
	seen[incompat] = true

	if cause, ok := incompat.Cause.(ConflictCause); ok {
		var result []*Incompatibility
		result = append(result, collectConflicts(cause.Conflict, seen)...)
		result = append(result, collectConflicts(cause.Other, seen)...)
		return result
	}

	if _, ok := incompat.Cause.(RootCause); ok {
		return nil
	}

	return []*Incompatibility{incompat}
}

func formatConflict(incompat *Incompatibility, projectName string) string {
	pkg := func(name string) string {
		if name == rootPkg {
			return projectName
		}
		return name
	}

	switch incompat.Cause.(type) {
	case DependencyCause:
		if len(incompat.Terms) == 2 {
			depender := incompat.Terms[0]
			dep := incompat.Terms[1]
			return fmt.Sprintf("%s (%s) depends on %s (%s)",
				pkg(depender.Pkg), depender.Constraint,
				pkg(dep.Pkg), dep.Constraint)
		}
	case NoVersionsCause:
		if len(incompat.Terms) == 1 {
			return fmt.Sprintf("no versions of %s match %s",
				pkg(incompat.Terms[0].Pkg), incompat.Terms[0].Constraint)
		}
	}
	return incompat.String()
}

// Solver implements the PubGrub version solving algorithm.
type Solver struct {
	provider          Provider
	root              string
	rootDeps          []Dependency
	incompatibilities map[string][]*Incompatibility
	// contradicted tracks clauses that currently can't fire: value is
	// the decision level at which the contradiction was established.
	// Zero means "not contradicted". On backtrack to level L we drop
	// entries with value > L — those contradictions depended on
	// assignments that are now undone. Clauses contradicted at level
	// 0 (from root-level constraints) persist permanently.
	contradicted map[*Incompatibility]int
	solution     *PartialSolution
	priorities   map[string]int
	// alreadyListed memoizes emitted base dependency clauses keyed by
	// (pkg_range, dep_pkg, dep_constraint). Prevents re-emission when
	// the solver revisits (pkg, v) via propagation or backtrack —
	// without it, range-valued base clauses explode into overlapping
	// duplicates and propagation slows to a crawl.
	alreadyListed map[string]struct{}
}

// NewSolver creates a new PubGrub solver.
func NewSolver(provider Provider, root string, rootDeps []Dependency) *Solver {
	return &Solver{
		provider:          provider,
		root:              root,
		rootDeps:          rootDeps,
		incompatibilities: make(map[string][]*Incompatibility),
		contradicted:      make(map[*Incompatibility]int),
		solution:          NewPartialSolution(),
		priorities:        make(map[string]int),
		alreadyListed:     make(map[string]struct{}),
	}
}

// Solve finds a set of package versions satisfying all constraints.
func (s *Solver) Solve(ctx context.Context) (*SolverResult, error) {
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
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if iterations > 10000 {
			return nil, fmt.Errorf("solver: exceeded 10000 iterations")
		}
		pkg, err := s.choosePackageVersion(ctx)
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
			if _, done := s.contradicted[incompats[i]]; done {
				continue
			}
			result := s.propagateIncompatibility(incompats[i])

			if result.conflict {
				rootCause, err := s.resolveConflict(incompats[i])
				if err != nil {
					return err
				}
				result2 := s.propagateIncompatibility(rootCause)
				if result2.conflict {
					return fmt.Errorf("BUG: propagation after conflict resolution yielded another conflict")
				}
				if result2.pkg != "" {
					changed = map[string]bool{result2.pkg: true}
				}
				break
			}

			if result.pkg != "" {
				changed[result.pkg] = true
			}
		}
	}

	return nil
}

func (s *Solver) propagateIncompatibility(incompat *Incompatibility) propagateResult {
	var unsatisfied *Term

	for i := range incompat.Terms {
		t := &incompat.Terms[i]
		rel := s.solution.Relation(*t)

		if rel == Disjoint {
			s.markContradicted(incompat)
			return propagateResult{}
		}

		if rel == Overlapping {
			if unsatisfied != nil {
				return propagateResult{}
			}
			unsatisfied = t
		}
	}

	if unsatisfied == nil {
		return propagateResult{conflict: true}
	}

	s.markContradicted(incompat)

	inv := unsatisfied.Inverse()
	s.solution.Derive(inv, incompat)
	return propagateResult{pkg: unsatisfied.Pkg}
}

// markContradicted records that `incompat` has been fully processed
// at the current decision level — future propagation passes should
// skip it until we backtrack past that level. Called from two
// branches of propagateIncompatibility, both safely skippable:
//
//  1. Disjoint branch: a term is disjoint with the solution, so
//     the clause cannot fire here.
//  2. Unit-propagation branch: all-but-one term is satisfied and
//     the remaining term has been derived. The clause has fired
//     and can't yield new information at this level.
//
// On backtrack to level L, clearContradictedAbove(L) drops entries
// recorded above L since those assumptions have been undone.
// Stored level is clamped to a minimum of 1 so the zero value
// remains the "not stored" sentinel.
func (s *Solver) markContradicted(incompat *Incompatibility) {
	level := s.solution.DecisionLevel()
	if level < 1 {
		level = 1
	}
	s.contradicted[incompat] = level
}

// clearContradictedAbove drops contradicted entries recorded at
// decision levels above `level`. Called by resolveConflict after
// backtracking — assignments at those levels are undone, so any
// contradictions that depended on them may no longer hold.
func (s *Solver) clearContradictedAbove(level int) {
	for ic, lvl := range s.contradicted {
		if lvl > level {
			delete(s.contradicted, ic)
		}
	}
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
			return nil, &SolveError{Incompatibility: incompat, Root: s.root}
		}

		if previousSatisfierLevel < mostRecentSatisfier.DecisionLevel || mostRecentSatisfier.IsDecision() {
			s.solution.Backtrack(previousSatisfierLevel)
			// Selective clear: drop contradictions from levels above
			// the backtrack target. Contradictions established at or
			// below the target are still valid — no need to
			// re-propagate them.
			s.clearContradictedAbove(previousSatisfierLevel)
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

	return nil, &SolveError{Incompatibility: incompat, Root: s.root}
}

func (s *Solver) choosePackageVersion(ctx context.Context) (string, error) {
	unsatisfied := s.solution.Unsatisfied()
	if len(unsatisfied) == 0 {
		return "", nil
	}

	pkg := s.chooseBest(unsatisfied)

	versions, err := s.provider.Versions(ctx, pkg)
	if err != nil {
		return "", fmt.Errorf("fetch versions for %s: %w", pkg, err)
	}

	sort.Slice(versions, func(i, j int) bool {
		return version.Compare(versions[i], versions[j]) > 0
	})

	var chosen *version.Version
	// Lockfile preference: when the provider advertises a preferred
	// version (typically the prior-run lock pin), try it before the
	// newest-first scan. Keeps warm re-locks stable — reuse the pin
	// when still valid instead of drifting to a newer release. Falls
	// through when the preferred version is absent, yanked, or
	// contradicted by current state.
	if pref, ok := s.provider.Preferred(ctx, pkg); ok {
		for i := range versions {
			if version.Compare(versions[i], pref) != 0 {
				continue
			}
			if s.solution.Relation(Term{Pkg: pkg, Constraint: version.ExactVersion(pref), Positive: true}) != Disjoint {
				v := versions[i]
				chosen = &v
			}
			break
		}
	}
	if chosen == nil {
		for i := range versions {
			v := versions[i]
			if s.solution.Relation(Term{Pkg: pkg, Constraint: version.ExactVersion(v), Positive: true}) != Disjoint {
				chosen = &v
				break
			}
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

	deps, err := s.provider.Dependencies(ctx, pkg, *chosen)
	if err != nil {
		return "", fmt.Errorf("fetch dependencies for %s %s: %w", pkg, chosen, err)
	}

	// Widen chosenConstraint to the contiguous same-deps range around
	// `chosen` via dependencyBounds. Without this the solver treats
	// every picked version as an isolated singleton; widening means one
	// learned clause via resolution covers every version in the range.
	chosenConstraint := version.ExactVersion(*chosen)
	if lo, hi, ok := s.dependencyBounds(ctx, pkg, *chosen, deps, versions); ok {
		chosenConstraint = version.NewRange(&lo, &hi, true, true)
	}

	for _, dep := range deps {
		// Skip emission when an identical (pkg_range, dep_pkg,
		// dep_constraint) clause has already been added. Without this,
		// every revisit of (pkg, v) emits duplicate clauses, tanking
		// propagation.
		depConstraintStr := "*"
		if dep.Constraint != nil {
			depConstraintStr = dep.Constraint.String()
		}
		key := pkg + "|" + chosenConstraint.String() + "|" + dep.Pkg + "|" + depConstraintStr
		if _, seen := s.alreadyListed[key]; seen {
			continue
		}
		s.alreadyListed[key] = struct{}{}
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

// dependencyBounds walks outward from chosen to find the widest contiguous
// range of versions whose dependency list matches chosen's. Versions outside
// the currently-allowed positive term are skipped as range boundaries.
//
// `versions` must be the list returned by provider.Versions(pkg), sorted
// newest-first (which is what choosePackageVersion sorts it into).
//
// Returns (lo, hi, true) when a range is found; returns (_, _, false) when
// the range degenerates to the singleton `chosen`, in which case the caller
// should fall back to ExactVersion.
//
// The caller wraps (lo, hi) as a closed interval: unenumerated versions in
// (lo, hi) are also excluded. Safe for PyPI (Versions() enumerates fully);
// custom indexes with version gaps are a known limitation.
func (s *Solver) dependencyBounds(ctx context.Context, pkg string, chosen version.Version, deps []Dependency, versions []version.Version) (version.Version, version.Version, bool) {
	idx := -1
	for i := range versions {
		if version.Compare(versions[i], chosen) == 0 {
			idx = i
			break
		}
	}
	if idx < 0 {
		return chosen, chosen, false
	}

	lo, hi := chosen, chosen

	// versions is sorted newest-first, so lower indices are higher versions.
	// Walk up (toward higher versions).
	for i := idx - 1; i >= 0; i-- {
		if !s.canExtendBound(ctx, pkg, versions[i], deps) {
			break
		}
		hi = versions[i]
	}
	// Walk down (toward lower versions).
	for i := idx + 1; i < len(versions); i++ {
		if !s.canExtendBound(ctx, pkg, versions[i], deps) {
			break
		}
		lo = versions[i]
	}

	if version.Compare(lo, hi) == 0 {
		return lo, hi, false
	}
	return lo, hi, true
}

// canExtendBound checks whether `neighbor` can be merged into the
// range around a chosen version. Returns false when:
//   - neighbor is already disallowed by the current positive term, OR
//   - neighbor's deps aren't in the provider cache (cache-only probe —
//     never triggers a fetch), OR
//   - neighbor's deps differ from `deps`.
//
// Cache-only semantics keep the hot path from waiting on network
// fetches while still widening as the cache warms. Versions the
// solver fetches on its own earlier picks accumulate in the cache,
// so successive widening walks reach further.
func (s *Solver) canExtendBound(ctx context.Context, pkg string, neighbor version.Version, deps []Dependency) bool {
	rel := s.solution.Relation(Term{Pkg: pkg, Constraint: version.ExactVersion(neighbor), Positive: true})
	if rel == Disjoint {
		return false
	}
	nDeps, cached := s.provider.DependenciesIfCached(ctx, pkg, neighbor)
	if !cached {
		return false
	}
	return depsEqual(deps, nDeps)
}

// depsEqual compares two dependency lists by (pkg name, rendered
// constraint string). String comparison is exact for Pensa's constraints:
// both sides come from the same provider parse path, so equivalent
// constraints round-trip to the same string. Structural equality would
// require a deeper Constraint API; the string check is sufficient here.
func depsEqual(a, b []Dependency) bool {
	if len(a) != len(b) {
		return false
	}
	byName := make(map[string]string, len(a))
	for _, d := range a {
		c := ""
		if d.Constraint != nil {
			c = d.Constraint.String()
		}
		byName[d.Pkg] = c
	}
	for _, d := range b {
		c := ""
		if d.Constraint != nil {
			c = d.Constraint.String()
		}
		if byName[d.Pkg] != c {
			return false
		}
	}
	return true
}

func (s *Solver) positiveTermFor(pkg string) Term {
	if pos, ok := s.solution.positive[pkg]; ok {
		return *pos
	}
	return Term{Pkg: pkg, Constraint: version.AnyConstraint(), Positive: true}
}

// chooseBest key: (priority desc, fanout desc, name asc).
//
// Priority comes from constraintPriority (singleton > bounded > any)
// seeded by addIncompatibility.
//
// Fanout = len(s.incompatibilities[pkg]) — the number of clauses
// referencing the pkg. More clauses = more constraints already
// narrowing the pkg = better candidate to decide next. Cheap O(1)
// proxy for "how constrained is this package"; a true
// most-constrained-variable count (valid versions remaining) costs
// O(versions) per call, which regressed larger corpora in
// benchmarking.
//
// Name tiebreak is the final deterministic fallback — without it,
// two pkgs with the same priority AND same fanout would pick based
// on input-slice order, which comes from Go's randomized map
// iteration in PartialSolution.Unsatisfied. With the fanout term
// discriminating most cases, the name tiebreak rarely fires but
// eliminates the non-determinism flake that the old code had.
func (s *Solver) chooseBest(pkgs []string) string {
	if len(pkgs) == 1 {
		return pkgs[0]
	}

	best := pkgs[0]
	bestPri := s.priorities[best]
	bestFan := len(s.incompatibilities[best])

	for _, pkg := range pkgs[1:] {
		pri := s.priorities[pkg]
		fan := len(s.incompatibilities[pkg])
		better := false
		if pri > bestPri {
			better = true
		} else if pri == bestPri {
			if fan > bestFan {
				better = true
			} else if fan == bestFan && pkg < best {
				better = true
			}
		}
		if better {
			best = pkg
			bestPri = pri
			bestFan = fan
		}
	}
	return best
}

func constraintPriority(c version.Constraint) int {
	if version.IsSingleton(c) {
		return 100
	}
	if !c.IsAny() {
		return 50
	}
	return 10
}

func (s *Solver) addIncompatibility(incompat *Incompatibility) {
	for _, t := range incompat.Terms {
		s.incompatibilities[t.Pkg] = append(s.incompatibilities[t.Pkg], incompat)
		// Update priority based on constraint shape — tighter constraints get higher priority.
		if pri := constraintPriority(t.Constraint); pri > s.priorities[t.Pkg] {
			s.priorities[t.Pkg] = pri
		}
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
