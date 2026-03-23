package resolve

import (
	"github.com/juanbzz/pensa/pkg/version"
)

// Assignment is a Term in the partial solution with additional metadata.
type Assignment struct {
	Term
	DecisionLevel int
	Index         int
	Cause         *Incompatibility // nil for decisions
}

// IsDecision returns true if this assignment is a decision (not derived).
func (a Assignment) IsDecision() bool {
	return a.Cause == nil
}

// NewDecision creates a decision assignment for a specific package version.
func NewDecision(pkg string, ver version.Version, level, index int) Assignment {
	return Assignment{
		Term: Term{
			Pkg:        pkg,
			Constraint: version.ExactVersion(ver),
			Positive:   true,
		},
		DecisionLevel: level,
		Index:         index,
	}
}

// NewDerivation creates a derived assignment from an incompatibility.
func NewDerivation(t Term, cause *Incompatibility, level, index int) Assignment {
	return Assignment{
		Term:          t,
		DecisionLevel: level,
		Index:         index,
		Cause:         cause,
	}
}
