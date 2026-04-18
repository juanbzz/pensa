package resolve

import (
	"fmt"

	"pensa.sh/pensa/pkg/version"
)

// SetRelation describes the relationship between two sets of versions.
type SetRelation int

const (
	Subset     SetRelation = iota // this is entirely within other
	Disjoint                      // no overlap
	Overlapping                   // partial overlap
)

// Term is a statement about a package: either "must use versions in constraint"
// (positive) or "must not use versions in constraint" (negative).
type Term struct {
	Pkg        string
	Constraint version.Constraint
	Positive   bool
}

// Inverse returns a term with the opposite polarity.
func (t Term) Inverse() Term {
	return Term{Pkg: t.Pkg, Constraint: t.Constraint, Positive: !t.Positive}
}

// Satisfies returns true if this term is a subset of other (implies other).
func (t Term) Satisfies(other Term) bool {
	return t.Pkg == other.Pkg && t.Relation(other) == Subset
}

// Relation returns how this term relates to other.
func (t Term) Relation(other Term) SetRelation {
	if t.Pkg != other.Pkg {
		panic(fmt.Sprintf("BUG: term.Relation called with different packages: %q vs %q", t.Pkg, other.Pkg))
	}

	if other.Positive {
		if t.Positive {
			// Both positive: is t ⊆ other?
			if other.Constraint.AllowsAll(t.Constraint) {
				return Subset
			}
			if !t.Constraint.AllowsAny(other.Constraint) {
				return Disjoint
			}
			return Overlapping
		}
		// t is negative, other is positive.
		if t.Constraint.AllowsAll(other.Constraint) {
			return Disjoint
		}
		return Overlapping
	}

	// other is negative.
	if t.Positive {
		// t is positive, other is negative.
		if !other.Constraint.AllowsAny(t.Constraint) {
			return Subset
		}
		if other.Constraint.AllowsAll(t.Constraint) {
			return Disjoint
		}
		return Overlapping
	}
	// Both negative.
	if t.Constraint.AllowsAll(other.Constraint) {
		return Subset
	}
	return Overlapping
}

// Intersect returns the intersection of this term with other, or nil if empty.
// Both terms must refer to the same package.
func (t Term) Intersect(other Term) *Term {
	if t.Pkg != other.Pkg {
		return nil
	}

	if t.Positive != other.Positive {
		// One positive, one negative: positive \ negative.
		pos, neg := t, other
		if !t.Positive {
			pos, neg = other, t
		}
		c := pos.Constraint.Difference(neg.Constraint)
		if c.IsEmpty() {
			return nil
		}
		return &Term{Pkg: t.Pkg, Constraint: c, Positive: true}
	}

	if t.Positive {
		// Both positive: intersect constraints.
		c := t.Constraint.Intersect(other.Constraint)
		if c.IsEmpty() {
			return nil
		}
		return &Term{Pkg: t.Pkg, Constraint: c, Positive: true}
	}

	// Both negative: union constraints.
	c := t.Constraint.Union(other.Constraint)
	return &Term{Pkg: t.Pkg, Constraint: c, Positive: false}
}

// Difference returns a term representing what this allows that other doesn't.
func (t Term) Difference(other Term) *Term {
	return t.Intersect(other.Inverse())
}

func (t Term) String() string {
	if !t.Positive {
		return fmt.Sprintf("not %s %s", t.Pkg, t.Constraint)
	}
	return fmt.Sprintf("%s %s", t.Pkg, t.Constraint)
}
