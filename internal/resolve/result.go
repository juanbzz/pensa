package resolve

import "github.com/juanbzz/pensa/pkg/version"

// SolverResult contains the resolved package versions.
type SolverResult struct {
	Decisions map[string]version.Version
	Attempts  int
}
