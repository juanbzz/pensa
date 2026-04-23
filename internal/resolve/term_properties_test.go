package resolve

import (
	"testing"

	"pensa.sh/pensa/pkg/version"
)

// termCorpus builds Term values covering both polarities over a
// representative set of constraints. All terms share the same pkg name
// (Term.Relation/Intersect panic on cross-package calls, which is by
// design — tested separately).
func termCorpus(t *testing.T) []Term {
	t.Helper()
	v := func(s string) version.Version {
		parsed, err := version.Parse(s)
		if err != nil {
			t.Fatalf("version.Parse(%q): %v", s, err)
		}
		return parsed
	}

	constraints := []version.Constraint{
		version.AnyConstraint(),
		version.EmptyConstraint(),
		version.ExactVersion(v("1.0")),
		version.ExactVersion(v("2.0")),
		version.NewRange(ptrVer(v("1.0")), ptrVer(v("2.0")), true, false),  // [1.0, 2.0)
		version.NewRange(ptrVer(v("1.0")), ptrVer(v("2.0")), false, false), // (1.0, 2.0)
		version.NewRange(ptrVer(v("1.5")), nil, true, false),               // [1.5, ∞)
		version.NewRange(nil, ptrVer(v("1.5")), false, false),              // (-∞, 1.5)
		version.NewUnion(
			version.NewRange(nil, ptrVer(v("1.0")), false, false),
			version.NewRange(ptrVer(v("2.0")), nil, false, false),
		), // (-∞, 1.0) ∪ (2.0, ∞)
	}

	var terms []Term
	for _, c := range constraints {
		terms = append(terms, Term{Pkg: "a", Constraint: c, Positive: true})
		terms = append(terms, Term{Pkg: "a", Constraint: c, Positive: false})
	}
	return terms
}

func ptrVer(v version.Version) *version.Version { return &v }

// sampleVersions for Term tests — covers the endpoints of the corpus
// ranges densely so that "t1.allowedSet matches t2.allowedSet" is a
// faithful equivalence on our terms.
func termSampleVersions(t *testing.T) []version.Version {
	t.Helper()
	strs := []string{
		"0.5", "0.9", "1.0", "1.0.1", "1.2", "1.5", "1.7", "1.9", "2.0", "2.0.1", "2.5", "3.0",
	}
	out := make([]version.Version, len(strs))
	for i, s := range strs {
		var err error
		out[i], err = version.Parse(s)
		if err != nil {
			t.Fatalf("version.Parse(%q): %v", s, err)
		}
	}
	return out
}

// allowedBy reports whether a term allows the version. For a positive
// term this is just constraint.Allows; for a negative term it's the
// complement.
func allowedBy(tm Term, v version.Version) bool {
	if tm.Positive {
		return tm.Constraint.Allows(v)
	}
	return !tm.Constraint.Allows(v)
}

// --- Inverse laws ---

// Inverse flips polarity; applying it twice gives back the original.
func TestTermProp_InverseInvolutive(t *testing.T) {
	samples := termSampleVersions(t)
	for _, tm := range termCorpus(t) {
		twice := tm.Inverse().Inverse()
		if twice.Pkg != tm.Pkg || twice.Positive != tm.Positive {
			t.Errorf("%s inverse-twice = %s; pkg/polarity differs", tm, twice)
		}
		for _, v := range samples {
			if allowedBy(twice, v) != allowedBy(tm, v) {
				t.Errorf("%s inverse-twice mismatch at %s", tm, v)
			}
		}
	}
}

// A term and its inverse have complementary allowed sets on every version.
func TestTermProp_InverseComplementary(t *testing.T) {
	samples := termSampleVersions(t)
	for _, tm := range termCorpus(t) {
		inv := tm.Inverse()
		for _, v := range samples {
			if allowedBy(tm, v) == allowedBy(inv, v) {
				t.Errorf("%s and its inverse both %v at %s", tm, allowedBy(tm, v), v)
			}
		}
	}
}

// --- Self-relation ---

// Every term's allowed set is a subset of itself.
func TestTermProp_RelationSelfIsSubset(t *testing.T) {
	for _, tm := range termCorpus(t) {
		if r := tm.Relation(tm); r != Subset {
			t.Errorf("%s.Relation(self) = %v; want Subset", tm, r)
		}
	}
}

func TestTermProp_SatisfiesSelf(t *testing.T) {
	for _, tm := range termCorpus(t) {
		if !tm.Satisfies(tm) {
			t.Errorf("%s should Satisfies itself", tm)
		}
	}
}

// --- Relation semantics ---

// Relation's three cases match the extensional definition on samples.
// (We treat vacuously-empty terms — positive empty, negative any — as
// "Subset of everything" which is the mathematical convention and
// matches pensa's Relation implementation.)
func TestTermProp_RelationMatchesExtensional(t *testing.T) {
	samples := termSampleVersions(t)
	corpus := termCorpus(t)

	for _, a := range corpus {
		for _, b := range corpus {
			got := a.Relation(b)

			// Build allowed-sample sets for each term.
			aAllowed := map[string]bool{}
			bAllowed := map[string]bool{}
			for _, v := range samples {
				if allowedBy(a, v) {
					aAllowed[v.String()] = true
				}
				if allowedBy(b, v) {
					bAllowed[v.String()] = true
				}
			}

			// Subset: every v in aAllowed is in bAllowed.
			want := Overlapping
			subset := true
			for k := range aAllowed {
				if !bAllowed[k] {
					subset = false
					break
				}
			}
			if subset {
				want = Subset
			} else {
				// Disjoint: no v in aAllowed is in bAllowed.
				disjoint := true
				for k := range aAllowed {
					if bAllowed[k] {
						disjoint = false
						break
					}
				}
				if disjoint {
					want = Disjoint
				}
			}

			// When a's allowed set is empty (vacuously-empty term — e.g.
			// positive-empty-constraint or negative-any-constraint), any
			// Relation answer is mathematically consistent: empty is both
			// a subset of and disjoint from every set. pensa's Relation
			// returns Disjoint in this corner; Dart's returns Subset.
			// Both are valid; the property test accepts either provided
			// it isn't Overlapping (which would be wrong either way).
			if len(aAllowed) == 0 {
				if got == Overlapping {
					t.Errorf("(%s).Relation(%s) = Overlapping; vacuously-empty term should be Subset or Disjoint",
						a, b)
				}
				continue
			}
			if got != want {
				t.Errorf("(%s).Relation(%s) = %v; extensional want %v (aAllowed=%v bAllowed=%v)",
					a, b, got, want, aAllowed, bAllowed)
			}
		}
	}
}

// --- Intersect semantics ---

// (a ∩ b) is non-nil iff a and b have at least one version in common.
// When non-nil, its allowed set equals the intersection of a and b.
func TestTermProp_IntersectSemantics(t *testing.T) {
	samples := termSampleVersions(t)
	corpus := termCorpus(t)

	for _, a := range corpus {
		for _, b := range corpus {
			got := a.Intersect(b)

			// Build expected intersection set.
			want := map[string]bool{}
			for _, v := range samples {
				if allowedBy(a, v) && allowedBy(b, v) {
					want[v.String()] = true
				}
			}

			if got == nil {
				// Nil means "empty"; acceptable if expected set is empty.
				if len(want) > 0 {
					t.Errorf("(%s).Intersect(%s) = nil; expected non-empty (%v)",
						a, b, want)
				}
				continue
			}

			// Got term — its allowed set should equal want.
			for _, v := range samples {
				gotAllow := allowedBy(*got, v)
				wantAllow := want[v.String()]
				if gotAllow != wantAllow {
					t.Errorf("(%s ∩ %s = %s).allows(%s) = %v; want %v",
						a, b, got, v, gotAllow, wantAllow)
					break
				}
			}
		}
	}
}

// --- Idempotence ---

// Intersecting with self is a no-op (allowed set unchanged).
func TestTermProp_IntersectSelfIsSelf(t *testing.T) {
	samples := termSampleVersions(t)
	for _, tm := range termCorpus(t) {
		got := tm.Intersect(tm)
		if got == nil {
			// Degenerate self-empty is fine (e.g. positive empty).
			// Check that the term is actually vacuously-empty.
			for _, v := range samples {
				if allowedBy(tm, v) {
					t.Errorf("%s.Intersect(self) = nil but %s allows %s",
						tm, tm, v)
					break
				}
			}
			continue
		}
		for _, v := range samples {
			if allowedBy(*got, v) != allowedBy(tm, v) {
				t.Errorf("%s.Intersect(self).allows(%s) != %s.allows(%s)",
					tm, v, tm, v)
				break
			}
		}
	}
}

// --- Difference ---

// (a \ b) == a ∩ b.Inverse() — Difference's actual definition.
func TestTermProp_DifferenceIsIntersectInverse(t *testing.T) {
	samples := termSampleVersions(t)
	corpus := termCorpus(t)
	for _, a := range corpus {
		for _, b := range corpus {
			diff := a.Difference(b)
			viaInv := a.Intersect(b.Inverse())

			if (diff == nil) != (viaInv == nil) {
				t.Errorf("(%s \\ %s) nil-ness disagrees with intersect-inverse: diff=%v inv=%v",
					a, b, diff, viaInv)
				continue
			}
			if diff == nil {
				continue
			}
			for _, v := range samples {
				if allowedBy(*diff, v) != allowedBy(*viaInv, v) {
					t.Errorf("(%s \\ %s) and (%s ∩ %s.Inverse) differ at %s",
						a, b, a, b, v)
					break
				}
			}
		}
	}
}

// --- Cross-package safety ---

// Relation panics for different-pkg terms. This is intentional — callers
// are responsible for filtering by pkg.
func TestTermProp_RelationDifferentPkgPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic from Relation with mismatched Pkg")
		}
	}()
	t1 := Term{Pkg: "a", Constraint: version.AnyConstraint(), Positive: true}
	t2 := Term{Pkg: "b", Constraint: version.AnyConstraint(), Positive: true}
	_ = t1.Relation(t2)
}
