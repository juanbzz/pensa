package cli

import (
	"context"

	"pensa.sh/pensa/internal/lockfile"
	"pensa.sh/pensa/internal/resolve"
	"pensa.sh/pensa/pkg/version"
)

// lockOptions controls how resolveAndLock handles existing locked versions.
type lockOptions struct {
	upgrade         bool     // ignore all pinned versions (re-resolve fresh)
	upgradePackages []string // ignore pinned versions for these packages only
	onlyGroups      []string // resolve only these groups (main is always included)
	withoutGroups   []string // skip these groups during resolution
}

var _ resolve.Provider = (*lockedProvider)(nil)

// lockedProvider wraps a resolve.Provider to prefer already-locked
// versions. Preferred(pkg) returns the pinned version; the solver
// checks it before iterating the (sorted newest-first) version slice.
// When the pinned version no longer satisfies current constraints, the
// solver falls through to the normal sort order naturally.
type lockedProvider struct {
	underlying      resolve.Provider
	pinned          map[string]version.Version
	upgradePackages map[string]bool
}

// newLockedProvider creates a provider that prefers versions from an existing lock file.
// Packages in upgradePackages are excluded from pinning.
func newLockedProvider(underlying resolve.Provider, lf *lockfile.LockFile, upgradePackages []string) *lockedProvider {
	pinned := make(map[string]version.Version, len(lf.Packages))
	for _, pkg := range lf.Packages {
		v, err := version.Parse(pkg.Version)
		if err != nil {
			continue
		}
		pinned[normalizeName(pkg.Name)] = v
	}

	upgrades := make(map[string]bool, len(upgradePackages))
	for _, name := range upgradePackages {
		upgrades[normalizeName(name)] = true
	}

	return &lockedProvider{
		underlying:      underlying,
		pinned:          pinned,
		upgradePackages: upgrades,
	}
}

func (p *lockedProvider) Versions(ctx context.Context, pkg string) ([]version.Version, error) {
	// Pass through — the solver uses Preferred() to bias picks
	// toward locked versions, rather than relying on slice order
	// (which the solver re-sorts newest-first anyway).
	return p.underlying.Versions(ctx, pkg)
}

// Preferred returns the locked version for pkg, if any.
func (p *lockedProvider) Preferred(ctx context.Context, pkg string) (version.Version, bool) {
	normalized := normalizeName(pkg)
	if p.upgradePackages[normalized] {
		return version.Version{}, false
	}
	pin, ok := p.pinned[normalized]
	if !ok {
		// Fall through to any upstream preference.
		return p.underlying.Preferred(ctx, pkg)
	}
	return pin, true
}

func (p *lockedProvider) Dependencies(ctx context.Context, pkg string, ver version.Version) ([]resolve.Dependency, error) {
	return p.underlying.Dependencies(ctx, pkg, ver)
}

func (p *lockedProvider) DependenciesIfCached(ctx context.Context, pkg string, ver version.Version) ([]resolve.Dependency, bool) {
	return p.underlying.DependenciesIfCached(ctx, pkg, ver)
}
