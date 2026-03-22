package resolve

import (
	"fmt"
	"strings"
)

// IncompatibilityCause describes why an incompatibility exists.
type IncompatibilityCause interface {
	causeMarker()
}

// RootCause means the root package must be selected.
type RootCause struct{}

func (RootCause) causeMarker() {}

// DependencyCause means package A depends on package B.
type DependencyCause struct{}

func (DependencyCause) causeMarker() {}

// NoVersionsCause means no versions match the constraint.
type NoVersionsCause struct{}

func (NoVersionsCause) causeMarker() {}

// ConflictCause means this was derived during conflict resolution.
type ConflictCause struct {
	Conflict *Incompatibility
	Other    *Incompatibility
}

func (ConflictCause) causeMarker() {}

// Incompatibility is a set of terms that cannot all be true simultaneously.
type Incompatibility struct {
	Terms []Term
	Cause IncompatibilityCause
}

// IsFailure returns true if this incompatibility means the solve has failed
// (the root package cannot be selected).
func (i *Incompatibility) IsFailure() bool {
	if len(i.Terms) == 0 {
		return true
	}
	if len(i.Terms) == 1 && i.Terms[0].Pkg == rootPkg {
		return true
	}
	return false
}

// rootPkg is a sentinel name for the root project.
const rootPkg = "$root"

func (i *Incompatibility) String() string {
	switch i.Cause.(type) {
	case DependencyCause:
		if len(i.Terms) == 2 {
			return fmt.Sprintf("%s depends on %s", i.Terms[0], termToDepString(i.Terms[1]))
		}
	case NoVersionsCause:
		if len(i.Terms) == 1 {
			return fmt.Sprintf("no versions of %s match %s", i.Terms[0].Pkg, i.Terms[0].Constraint)
		}
	case RootCause:
		if len(i.Terms) == 1 {
			return fmt.Sprintf("%s is %s", i.Terms[0].Pkg, i.Terms[0].Constraint)
		}
	}

	parts := make([]string, len(i.Terms))
	for j, t := range i.Terms {
		parts[j] = t.String()
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func termToDepString(t Term) string {
	if !t.Positive {
		return fmt.Sprintf("%s %s", t.Pkg, t.Constraint)
	}
	return fmt.Sprintf("not %s %s", t.Pkg, t.Constraint)
}
