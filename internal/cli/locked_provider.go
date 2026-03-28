package cli

import (
	"github.com/juanbzz/pensa/internal/lockfile"
	"github.com/juanbzz/pensa/internal/resolve"
	"github.com/juanbzz/pensa/pkg/version"
)

// lockOptions controls how resolveAndLock handles existing locked versions.
type lockOptions struct {
	upgrade         bool     // ignore all pinned versions (re-resolve fresh)
	upgradePackages []string // ignore pinned versions for these packages only
}

var _ resolve.Provider = (*lockedProvider)(nil)

// lockedProvider wraps a resolve.Provider to prefer already-locked versions.
// When a package has a pinned version, it's moved to the front of the version
// list so the solver picks it first. If the pinned version no longer satisfies
// constraints, the solver falls through to the next version naturally.
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

func (p *lockedProvider) Versions(pkg string) ([]version.Version, error) {
	versions, err := p.underlying.Versions(pkg)
	if err != nil {
		return nil, err
	}

	normalized := normalizeName(pkg)

	// Skip pinning for packages being upgraded.
	if p.upgradePackages[normalized] {
		return versions, nil
	}

	pin, ok := p.pinned[normalized]
	if !ok {
		return versions, nil
	}

	// Move pinned version to front so solver picks it first.
	for i, v := range versions {
		if version.Compare(v, pin) == 0 {
			versions[0], versions[i] = versions[i], versions[0]
			break
		}
	}

	return versions, nil
}

func (p *lockedProvider) Dependencies(pkg string, ver version.Version) ([]resolve.Dependency, error) {
	return p.underlying.Dependencies(pkg, ver)
}
