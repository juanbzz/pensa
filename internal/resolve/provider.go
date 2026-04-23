package resolve

import (
	"pensa.sh/pensa/pkg/version"
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
	// DependenciesIfCached returns deps only if they're already in the
	// provider's cache; never triggers a fresh fetch. Returns
	// (nil, false) on miss. Used by range-batching (R1) in
	// choosePackageVersion to widen base clauses over already-known
	// neighbor versions without paying synchronous fetch cost.
	DependenciesIfCached(pkg string, ver version.Version) ([]Dependency, bool)
}
