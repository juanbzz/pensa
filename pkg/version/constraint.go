package version

import "fmt"

// Constraint represents a version constraint that the resolver operates on.
type Constraint interface {
	IsEmpty() bool
	IsAny() bool
	Allows(v Version) bool
	AllowsAll(other Constraint) bool
	AllowsAny(other Constraint) bool
	Intersect(other Constraint) Constraint
	Union(other Constraint) Constraint
	Difference(other Constraint) Constraint
	String() string
}

// Compile-time interface checks.
var (
	_ Constraint = (*exactConstraint)(nil)
	_ Constraint = (*Range)(nil)
	_ Constraint = (*Union)(nil)
	_ Constraint = (*emptyConstraint)(nil)
	_ Constraint = (*anyConstraint)(nil)
)

// --- anyConstraint ---

type anyConstraint struct{}

func AnyConstraint() Constraint         { return &anyConstraint{} }
func (a *anyConstraint) IsEmpty() bool   { return false }
func (a *anyConstraint) IsAny() bool     { return true }
func (a *anyConstraint) Allows(Version) bool { return true }
func (a *anyConstraint) AllowsAll(other Constraint) bool { return other.IsAny() || other.IsEmpty() }
func (a *anyConstraint) AllowsAny(other Constraint) bool { return !other.IsEmpty() }
func (a *anyConstraint) String() string  { return "*" }

func (a *anyConstraint) Intersect(other Constraint) Constraint { return other }
func (a *anyConstraint) Union(Constraint) Constraint            { return a }
func (a *anyConstraint) Difference(other Constraint) Constraint {
	if other.IsAny() {
		return empty
	}
	if other.IsEmpty() {
		return a
	}
	// any minus a range = union of the two complementary ranges.
	if r, ok := other.(*Range); ok {
		var parts []Constraint
		if r.min != nil {
			parts = append(parts, NewRange(nil, r.min, false, !r.includeMin))
		}
		if r.max != nil {
			parts = append(parts, NewRange(r.max, nil, !r.includeMax, false))
		}
		if len(parts) == 0 {
			return empty
		}
		return NewUnion(parts...)
	}
	// any minus a point = !=
	if ec, ok := other.(*exactConstraint); ok {
		v := ec.version
		return NewUnion(
			NewRange(nil, &v, false, false),
			NewRange(&v, nil, false, false),
		)
	}
	return a
}

// --- emptyConstraint ---

var empty = &emptyConstraint{}

type emptyConstraint struct{}

func EmptyConstraint() Constraint          { return empty }
func (e *emptyConstraint) IsEmpty() bool    { return true }
func (e *emptyConstraint) IsAny() bool      { return false }
func (e *emptyConstraint) Allows(Version) bool { return false }
func (e *emptyConstraint) AllowsAll(other Constraint) bool { return other.IsEmpty() }
func (e *emptyConstraint) AllowsAny(Constraint) bool       { return false }
func (e *emptyConstraint) Intersect(Constraint) Constraint  { return empty }
func (e *emptyConstraint) Union(other Constraint) Constraint { return other }
func (e *emptyConstraint) Difference(Constraint) Constraint  { return empty }
func (e *emptyConstraint) String() string                    { return "<empty>" }

// --- exactConstraint (a single version as a constraint) ---

type exactConstraint struct {
	version Version
}

func ExactVersion(v Version) Constraint {
	return &exactConstraint{version: v}
}

func (ec *exactConstraint) IsEmpty() bool { return false }
func (ec *exactConstraint) IsAny() bool   { return false }

func (ec *exactConstraint) Allows(v Version) bool {
	this, other := ec.version, v
	// If specifier has no local, ignore candidate's local.
	if !this.IsLocal() && other.IsLocal() {
		other = other.WithoutLocal()
	}
	return Compare(this, other) == 0
}

func (ec *exactConstraint) AllowsAll(other Constraint) bool {
	if other.IsEmpty() {
		return true
	}
	if o, ok := other.(*exactConstraint); ok {
		return ec.Allows(o.version)
	}
	return false
}

func (ec *exactConstraint) AllowsAny(other Constraint) bool {
	return !ec.Intersect(other).IsEmpty()
}

func (ec *exactConstraint) Intersect(other Constraint) Constraint {
	switch o := other.(type) {
	case *exactConstraint:
		if ec.Allows(o.version) {
			return o
		}
		return empty
	case *anyConstraint:
		return ec
	case *emptyConstraint:
		return empty
	default:
		if other.Allows(ec.version) {
			return ec
		}
		return empty
	}
}

func (ec *exactConstraint) Union(other Constraint) Constraint {
	if other.Allows(ec.version) {
		return other
	}
	return NewUnion(ec, other)
}

func (ec *exactConstraint) Difference(other Constraint) Constraint {
	if other.Allows(ec.version) {
		return empty
	}
	return ec
}

func (ec *exactConstraint) String() string {
	return ec.version.String()
}

func (ec *exactConstraint) Version() Version {
	return ec.version
}

// --- Range ---

// Range represents a version range with optional lower and upper bounds.
type Range struct {
	min, max               *Version
	includeMin, includeMax bool
}

func NewRange(min, max *Version, includeMin, includeMax bool) Constraint {
	if min == nil && max == nil {
		return AnyConstraint()
	}
	return &Range{min: min, max: max, includeMin: includeMin, includeMax: includeMax}
}

func (r *Range) IsEmpty() bool      { return false }
func (r *Range) IsAny() bool        { return r.min == nil && r.max == nil }
func (r *Range) Min() *Version      { return r.min }
func (r *Range) Max() *Version      { return r.max }
func (r *Range) IncludeMin() bool   { return r.includeMin }
func (r *Range) IncludeMax() bool   { return r.includeMax }

func (r *Range) Allows(v Version) bool {
	if r.min != nil {
		this, other := *r.min, v

		// >V must not allow post-releases of V unless V itself is a post-release.
		if !r.includeMin && !this.IsPostRelease() && other.IsPostRelease() {
			other = other.WithoutPostRelease()
		}
		// >V must not allow local versions of V.
		if !this.IsLocal() && other.IsLocal() {
			other = other.WithoutLocal()
		}

		c := Compare(other, this)
		if c < 0 {
			return false
		}
		if !r.includeMin && c == 0 {
			return false
		}
	}

	if r.max != nil {
		this, other := *r.max, v

		// <V must not allow pre-releases of V unless V itself is a pre-release.
		if !r.includeMax && !this.IsPreRelease() && other.IsPreRelease() {
			// Check if same release segment.
			if cmpRelease(this.release, other.release) == 0 {
				return false
			}
		}

		// Allow weak equality for local versions with <=.
		if !this.IsLocal() && other.IsLocal() {
			other = other.WithoutLocal()
		}

		c := Compare(other, this)
		if c > 0 {
			return false
		}
		if !r.includeMax && c == 0 {
			return false
		}
	}

	return true
}

func (r *Range) AllowsAll(other Constraint) bool {
	switch o := other.(type) {
	case *emptyConstraint:
		return true
	case *exactConstraint:
		return r.Allows(o.version)
	case *Range:
		return !o.allowsLower(r) && !o.allowsHigher(r)
	case *Union:
		for _, c := range o.constraints {
			if !r.AllowsAll(c) {
				return false
			}
		}
		return true
	case *anyConstraint:
		return r.IsAny()
	}
	return false
}

func (r *Range) AllowsAny(other Constraint) bool {
	switch o := other.(type) {
	case *emptyConstraint:
		return false
	case *exactConstraint:
		return r.Allows(o.version)
	case *Range:
		return !o.isStrictlyLower(r) && !o.isStrictlyHigher(r)
	case *Union:
		for _, c := range o.constraints {
			if r.AllowsAny(c) {
				return true
			}
		}
		return false
	case *anyConstraint:
		return true
	}
	return false
}

func (r *Range) Intersect(other Constraint) Constraint {
	switch o := other.(type) {
	case *emptyConstraint:
		return empty
	case *anyConstraint:
		return r
	case *exactConstraint:
		if r.Allows(o.version) {
			return o
		}
		return empty
	case *Union:
		return o.Intersect(r)
	case *Range:
		return r.intersectRange(o)
	}
	return empty
}

func (r *Range) intersectRange(other *Range) Constraint {
	var intersectMin *Version
	var intersectIncludeMin bool

	if r.allowsLower(other) {
		if r.isStrictlyLower(other) {
			return empty
		}
		intersectMin = other.min
		intersectIncludeMin = other.includeMin
	} else {
		if other.isStrictlyLower(r) {
			return empty
		}
		intersectMin = r.min
		intersectIncludeMin = r.includeMin
	}

	var intersectMax *Version
	var intersectIncludeMax bool

	if r.allowsHigher(other) {
		intersectMax = other.max
		intersectIncludeMax = other.includeMax
	} else {
		intersectMax = r.max
		intersectIncludeMax = r.includeMax
	}

	if intersectMin != nil && intersectMax != nil {
		c := Compare(*intersectMin, *intersectMax)
		if c > 0 {
			return empty
		}
		if c == 0 && intersectIncludeMin && intersectIncludeMax {
			return ExactVersion(*intersectMin)
		}
		if c == 0 {
			return empty
		}
	}

	return NewRange(intersectMin, intersectMax, intersectIncludeMin, intersectIncludeMax)
}

func (r *Range) Union(other Constraint) Constraint {
	switch o := other.(type) {
	case *emptyConstraint:
		return r
	case *anyConstraint:
		return o
	case *exactConstraint:
		if r.Allows(o.version) {
			return r
		}
		return NewUnion(r, o)
	case *Range:
		edgesTouch := r.edgesTouch(o)
		if !edgesTouch && !r.AllowsAny(o) {
			return NewUnion(r, o)
		}
		return r.unionRange(o)
	case *Union:
		return NewUnion(append([]Constraint{r}, o.constraints...)...)
	}
	return NewUnion(r, other)
}

func (r *Range) edgesTouch(other *Range) bool {
	if r.max != nil && other.min != nil && Compare(*r.max, *other.min) == 0 {
		return r.includeMax || other.includeMin
	}
	if r.min != nil && other.max != nil && Compare(*r.min, *other.max) == 0 {
		return r.includeMin || other.includeMax
	}
	return false
}

func (r *Range) unionRange(other *Range) Constraint {
	var unionMin *Version
	var unionIncludeMin bool

	if r.allowsLower(other) {
		unionMin = r.min
		unionIncludeMin = r.includeMin
	} else {
		unionMin = other.min
		unionIncludeMin = other.includeMin
	}

	var unionMax *Version
	var unionIncludeMax bool

	if r.allowsHigher(other) {
		unionMax = r.max
		unionIncludeMax = r.includeMax
	} else {
		unionMax = other.max
		unionIncludeMax = other.includeMax
	}

	return NewRange(unionMin, unionMax, unionIncludeMin, unionIncludeMax)
}

func (r *Range) Difference(other Constraint) Constraint {
	switch o := other.(type) {
	case *emptyConstraint:
		return r
	case *anyConstraint:
		return empty
	case *exactConstraint:
		if !r.Allows(o.version) {
			return r
		}
		v := o.version
		before := NewRange(r.min, &v, r.includeMin, false)
		after := NewRange(&v, r.max, false, r.includeMax)
		return NewUnion(before, after)
	case *Range:
		if !r.AllowsAny(o) {
			return r
		}
		return r.differenceRange(o)
	case *Union:
		result := Constraint(r)
		for _, c := range o.constraints {
			result = result.Difference(c)
			if result.IsEmpty() {
				return empty
			}
		}
		return result
	}
	return r
}

func (r *Range) differenceRange(other *Range) Constraint {
	var before, after Constraint

	if r.allowsLower(other) {
		before = NewRange(r.min, other.min, r.includeMin, !other.includeMin)
	}
	if r.allowsHigher(other) {
		after = NewRange(other.max, r.max, !other.includeMax, r.includeMax)
	}

	if before == nil && after == nil {
		return empty
	}
	if before == nil {
		return after
	}
	if after == nil {
		return before
	}
	return NewUnion(before, after)
}

// allowsLower returns true if r allows versions lower than other.
func (r *Range) allowsLower(other *Range) bool {
	if r.min == nil {
		return other.min != nil
	}
	if other.min == nil {
		return false
	}
	c := Compare(*r.min, *other.min)
	if c < 0 {
		return true
	}
	if c > 0 {
		return false
	}
	return r.includeMin && !other.includeMin
}

// allowsHigher returns true if r allows versions higher than other.
func (r *Range) allowsHigher(other *Range) bool {
	if r.max == nil {
		return other.max != nil
	}
	if other.max == nil {
		return false
	}
	c := Compare(*r.max, *other.max)
	if c > 0 {
		return true
	}
	if c < 0 {
		return false
	}
	return r.includeMax && !other.includeMax
}

// isStrictlyLower returns true if r is entirely below other with no overlap.
func (r *Range) isStrictlyLower(other *Range) bool {
	if r.max == nil || other.min == nil {
		return false
	}
	c := Compare(*r.max, *other.min)
	if c < 0 {
		return true
	}
	if c > 0 {
		return false
	}
	return !r.includeMax || !other.includeMin
}

// isStrictlyHigher returns true if r is entirely above other with no overlap.
func (r *Range) isStrictlyHigher(other *Range) bool {
	return other.isStrictlyLower(r)
}

func (r *Range) String() string {
	var s string
	if r.min != nil {
		if r.includeMin {
			s += ">="
		} else {
			s += ">"
		}
		s += r.min.String()
	}
	if r.max != nil {
		if r.min != nil {
			s += ","
		}
		if r.includeMax {
			s += "<="
		} else {
			s += "<"
		}
		s += r.max.String()
	}
	if r.min == nil && r.max == nil {
		return "*"
	}
	return s
}

// --- Union ---

// Union represents a disjunction (OR) of constraints.
type Union struct {
	constraints []Constraint
}

// NewUnion creates a union, flattening nested unions and removing empties.
func NewUnion(constraints ...Constraint) Constraint {
	var flat []Constraint
	for _, c := range constraints {
		switch ct := c.(type) {
		case *emptyConstraint:
			continue
		case *anyConstraint:
			return c
		case *Union:
			flat = append(flat, ct.constraints...)
		default:
			flat = append(flat, c)
		}
	}
	switch len(flat) {
	case 0:
		return empty
	case 1:
		return flat[0]
	}
	return &Union{constraints: flat}
}

func (u *Union) IsEmpty() bool { return false }
func (u *Union) IsAny() bool   { return false }

func (u *Union) Allows(v Version) bool {
	for _, c := range u.constraints {
		if c.Allows(v) {
			return true
		}
	}
	return false
}

func (u *Union) AllowsAll(other Constraint) bool {
	// Conservative: true only if one of our constraints allows all.
	for _, c := range u.constraints {
		if c.AllowsAll(other) {
			return true
		}
	}
	return false
}

func (u *Union) AllowsAny(other Constraint) bool {
	for _, c := range u.constraints {
		if c.AllowsAny(other) {
			return true
		}
	}
	return false
}

func (u *Union) Intersect(other Constraint) Constraint {
	var results []Constraint
	for _, c := range u.constraints {
		result := c.Intersect(other)
		if !result.IsEmpty() {
			results = append(results, result)
		}
	}
	if len(results) == 0 {
		return empty
	}
	return NewUnion(results...)
}

func (u *Union) Union(other Constraint) Constraint {
	return NewUnion(append(u.constraints, other)...)
}

func (u *Union) Difference(other Constraint) Constraint {
	var results []Constraint
	for _, c := range u.constraints {
		result := c.Difference(other)
		if !result.IsEmpty() {
			results = append(results, result)
		}
	}
	if len(results) == 0 {
		return empty
	}
	return NewUnion(results...)
}

func (u *Union) String() string {
	parts := make([]string, len(u.constraints))
	for i, c := range u.constraints {
		parts[i] = c.String()
	}
	return fmt.Sprintf("%s", joinOr(parts))
}

func joinOr(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += " || " + p
	}
	return result
}
