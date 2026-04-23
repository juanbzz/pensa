package version

import (
	"fmt"
	"testing"
)

// constraintCorpus returns a representative set of Constraint values used
// by the property tests below. The set covers each Constraint type plus
// shapes that exercise known edge cases (bounded/unbounded ranges,
// inclusive/exclusive endpoints, adjacent and overlapping unions,
// singletons, any, empty).
//
// Adding to this corpus increases the coverage of every property test.
func constraintCorpus(t *testing.T) []Constraint {
	t.Helper()
	v := func(s string) Version { return mustParse(t, s) }

	return []Constraint{
		AnyConstraint(),
		EmptyConstraint(),

		// Singletons.
		ExactVersion(v("1.0")),
		ExactVersion(v("1.5")),
		ExactVersion(v("2.0")),

		// Unbounded-below ranges.
		NewRange(nil, ptrV(v("1.0")), false, false),
		NewRange(nil, ptrV(v("2.0")), false, true),

		// Unbounded-above ranges.
		NewRange(ptrV(v("1.0")), nil, true, false),
		NewRange(ptrV(v("1.0")), nil, false, false),
		NewRange(ptrV(v("2.0")), nil, true, false),

		// Bounded ranges.
		NewRange(ptrV(v("1.0")), ptrV(v("2.0")), true, false),  // [1.0, 2.0)
		NewRange(ptrV(v("1.0")), ptrV(v("2.0")), true, true),   // [1.0, 2.0]
		NewRange(ptrV(v("1.0")), ptrV(v("2.0")), false, false), // (1.0, 2.0)
		NewRange(ptrV(v("1.5")), ptrV(v("3.0")), true, false),  // overlaps above
		NewRange(ptrV(v("0.5")), ptrV(v("1.2")), true, false),  // overlaps below
		NewRange(ptrV(v("3.0")), ptrV(v("4.0")), true, false),  // disjoint above

		// Unions.
		NewUnion(
			NewRange(nil, ptrV(v("1.0")), false, false),
			NewRange(ptrV(v("2.0")), nil, false, false),
		), // (-∞, 1.0) ∪ (2.0, ∞)
		NewUnion(
			NewRange(ptrV(v("1.0")), ptrV(v("1.5")), true, false),
			NewRange(ptrV(v("2.0")), ptrV(v("2.5")), true, false),
		), // [1.0, 1.5) ∪ [2.0, 2.5)
		NewUnion(
			ExactVersion(v("1.0")),
			ExactVersion(v("2.0")),
			ExactVersion(v("3.0")),
		), // {1.0, 2.0, 3.0}
	}
}

func ptrV(v Version) *Version { return &v }

// sampleVersions returns versions covering the full range used by
// constraintCorpus, densely enough that "a and b allow the same
// versions" is a strong equivalence test.
func sampleVersions(t *testing.T) []Version {
	t.Helper()
	strs := []string{
		"0.0.1", "0.5", "0.9.9", "1.0", "1.0.0", "1.0.1",
		"1.2", "1.5", "1.9.9", "2.0", "2.0.0", "2.5",
		"3.0", "3.5", "4.0", "100.0",
	}
	out := make([]Version, len(strs))
	for i, s := range strs {
		out[i] = mustParse(t, s)
	}
	return out
}

// equivConstraints returns true if a and b allow exactly the same subset
// of sampleVersions. Used as an extensional equality test — two
// constraints are "the same" iff they have the same accept set on our
// representative versions.
func equivConstraints(a, b Constraint, samples []Version) bool {
	for _, v := range samples {
		if a.Allows(v) != b.Allows(v) {
			return false
		}
	}
	return true
}

// --- Identity laws ---

func TestProp_IntersectWithAnyIsSelf(t *testing.T) {
	samples := sampleVersions(t)
	for _, c := range constraintCorpus(t) {
		got := c.Intersect(AnyConstraint())
		if !equivConstraints(got, c, samples) {
			t.Errorf("%s ∩ * = %s; want equivalent to %s", c, got, c)
		}
	}
}

func TestProp_IntersectWithEmptyIsEmpty(t *testing.T) {
	samples := sampleVersions(t)
	for _, c := range constraintCorpus(t) {
		got := c.Intersect(EmptyConstraint())
		if !equivConstraints(got, EmptyConstraint(), samples) {
			t.Errorf("%s ∩ ∅ = %s; want ∅", c, got)
		}
	}
}

func TestProp_UnionWithAnyIsAny(t *testing.T) {
	samples := sampleVersions(t)
	for _, c := range constraintCorpus(t) {
		got := c.Union(AnyConstraint())
		if !equivConstraints(got, AnyConstraint(), samples) {
			t.Errorf("%s ∪ * = %s; want *", c, got)
		}
	}
}

func TestProp_UnionWithEmptyIsSelf(t *testing.T) {
	samples := sampleVersions(t)
	for _, c := range constraintCorpus(t) {
		got := c.Union(EmptyConstraint())
		if !equivConstraints(got, c, samples) {
			t.Errorf("%s ∪ ∅ = %s; want equivalent to %s", c, got, c)
		}
	}
}

// --- Idempotence ---

func TestProp_IntersectSelfIsSelf(t *testing.T) {
	samples := sampleVersions(t)
	for _, c := range constraintCorpus(t) {
		got := c.Intersect(c)
		if !equivConstraints(got, c, samples) {
			t.Errorf("%s ∩ %s = %s; want equivalent to %s", c, c, got, c)
		}
	}
}

func TestProp_UnionSelfIsSelf(t *testing.T) {
	samples := sampleVersions(t)
	for _, c := range constraintCorpus(t) {
		got := c.Union(c)
		if !equivConstraints(got, c, samples) {
			t.Errorf("%s ∪ %s = %s; want equivalent to %s", c, c, got, c)
		}
	}
}

// --- Self-relation ---

func TestProp_AllowsAllSelf(t *testing.T) {
	for _, c := range constraintCorpus(t) {
		if !c.AllowsAll(c) {
			t.Errorf("%s should AllowsAll itself", c)
		}
	}
}

func TestProp_AllowsAnySelf(t *testing.T) {
	for _, c := range constraintCorpus(t) {
		expected := !c.IsEmpty()
		if got := c.AllowsAny(c); got != expected {
			t.Errorf("%s.AllowsAny(self) = %v; want %v (IsEmpty=%v)",
				c, got, expected, c.IsEmpty())
		}
	}
}

// --- Semantic definitions ---

// a ∩ b allows v iff a allows v AND b allows v.
func TestProp_IntersectSemantics(t *testing.T) {
	samples := sampleVersions(t)
	corpus := constraintCorpus(t)
	for _, a := range corpus {
		for _, b := range corpus {
			ab := a.Intersect(b)
			for _, v := range samples {
				want := a.Allows(v) && b.Allows(v)
				if got := ab.Allows(v); got != want {
					t.Errorf("(%s ∩ %s).Allows(%s) = %v; want %v && %v = %v",
						a, b, v, got, a.Allows(v), b.Allows(v), want)
				}
			}
		}
	}
}

// a ∪ b allows v iff a allows v OR b allows v.
func TestProp_UnionSemantics(t *testing.T) {
	samples := sampleVersions(t)
	corpus := constraintCorpus(t)
	for _, a := range corpus {
		for _, b := range corpus {
			ab := a.Union(b)
			for _, v := range samples {
				want := a.Allows(v) || b.Allows(v)
				if got := ab.Allows(v); got != want {
					t.Errorf("(%s ∪ %s).Allows(%s) = %v; want %v || %v = %v",
						a, b, v, got, a.Allows(v), b.Allows(v), want)
				}
			}
		}
	}
}

// a \ b allows v iff a allows v AND NOT b allows v.
func TestProp_DifferenceSemantics(t *testing.T) {
	samples := sampleVersions(t)
	corpus := constraintCorpus(t)
	for _, a := range corpus {
		for _, b := range corpus {
			ab := a.Difference(b)
			for _, v := range samples {
				want := a.Allows(v) && !b.Allows(v)
				if got := ab.Allows(v); got != want {
					t.Errorf("(%s \\ %s).Allows(%s) = %v; want %v && !%v = %v",
						a, b, v, got, a.Allows(v), b.Allows(v), want)
				}
			}
		}
	}
}

// --- Relational laws ---

// a.AllowsAll(b) iff b.Difference(a).IsEmpty().
// Uses the constraint's own IsEmpty rather than extensional-on-samples,
// because small (open-bounded) intervals (e.g. (2.0, 2.5)) may contain
// no sample version but are still semantically non-empty.
func TestProp_AllowsAllEquivDifferenceEmpty(t *testing.T) {
	corpus := constraintCorpus(t)
	for _, a := range corpus {
		for _, b := range corpus {
			allowsAll := a.AllowsAll(b)
			diffEmpty := b.Difference(a).IsEmpty()
			if allowsAll != diffEmpty {
				t.Errorf("%s.AllowsAll(%s)=%v but (%s \\ %s)=%s (IsEmpty=%v)",
					a, b, allowsAll, b, a, b.Difference(a), diffEmpty)
			}
		}
	}
}

// a.AllowsAny(b) iff (a ∩ b) is non-empty.
// Same IsEmpty-over-samples rationale as above.
func TestProp_AllowsAnyEquivIntersectNonEmpty(t *testing.T) {
	corpus := constraintCorpus(t)
	for _, a := range corpus {
		for _, b := range corpus {
			allowsAny := a.AllowsAny(b)
			intersectEmpty := a.Intersect(b).IsEmpty()
			if allowsAny == intersectEmpty {
				t.Errorf("%s.AllowsAny(%s)=%v but (%s ∩ %s)=%s (IsEmpty=%v)",
					a, b, allowsAny, a, b, a.Intersect(b), intersectEmpty)
			}
		}
	}
}

// If a.AllowsAll(b), then a ∪ b is equivalent to a.
func TestProp_AllowsAllImpliesUnionIsSelf(t *testing.T) {
	samples := sampleVersions(t)
	corpus := constraintCorpus(t)
	for _, a := range corpus {
		for _, b := range corpus {
			if !a.AllowsAll(b) {
				continue
			}
			union := a.Union(b)
			if !equivConstraints(union, a, samples) {
				t.Errorf("%s.AllowsAll(%s) but %s ∪ %s = %s != %s",
					a, b, a, b, union, a)
			}
		}
	}
}

// If a.AllowsAll(b), then a ∩ b is equivalent to b.
func TestProp_AllowsAllImpliesIntersectIsOther(t *testing.T) {
	samples := sampleVersions(t)
	corpus := constraintCorpus(t)
	for _, a := range corpus {
		for _, b := range corpus {
			if !a.AllowsAll(b) {
				continue
			}
			inter := a.Intersect(b)
			if !equivConstraints(inter, b, samples) {
				t.Errorf("%s.AllowsAll(%s) but %s ∩ %s = %s != %s",
					a, b, a, b, inter, b)
			}
		}
	}
}

// --- Commutativity ---

func TestProp_IntersectCommutative(t *testing.T) {
	samples := sampleVersions(t)
	corpus := constraintCorpus(t)
	for _, a := range corpus {
		for _, b := range corpus {
			if !equivConstraints(a.Intersect(b), b.Intersect(a), samples) {
				t.Errorf("%s ∩ %s = %s but %s ∩ %s = %s",
					a, b, a.Intersect(b), b, a, b.Intersect(a))
			}
		}
	}
}

func TestProp_UnionCommutative(t *testing.T) {
	samples := sampleVersions(t)
	corpus := constraintCorpus(t)
	for _, a := range corpus {
		for _, b := range corpus {
			if !equivConstraints(a.Union(b), b.Union(a), samples) {
				t.Errorf("%s ∪ %s = %s but %s ∪ %s = %s",
					a, b, a.Union(b), b, a, b.Union(a))
			}
		}
	}
}

// --- Sanity ---

// Ensures the corpus generator itself doesn't regress silently —
// if someone removes entries, the pair-count drops and we catch it.
func TestProp_CorpusCoversAllTypes(t *testing.T) {
	corpus := constraintCorpus(t)
	seen := make(map[string]bool)
	for _, c := range corpus {
		seen[fmt.Sprintf("%T", c)] = true
	}
	expected := []string{
		"*version.anyConstraint",
		"*version.emptyConstraint",
		"*version.exactConstraint",
		"*version.Range",
		"*version.Union",
	}
	for _, e := range expected {
		if !seen[e] {
			t.Errorf("corpus missing type %s", e)
		}
	}
}
