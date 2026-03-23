package resolve

import (
	"github.com/juanbzz/pensa/pkg/version"
)

// Dependency represents a package dependency for the resolver.
type Dependency struct {
	Pkg        string
	Constraint version.Constraint
}

// Provider bridges the resolver to the package index.
type Provider interface {
	// Versions returns available versions for a package, sorted newest-first.
	Versions(pkg string) ([]version.Version, error)
	// Dependencies returns the dependencies for a specific package version.
	Dependencies(pkg string, ver version.Version) ([]Dependency, error)
}
