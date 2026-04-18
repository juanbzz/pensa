package cli

import (
	"pensa.sh/pensa/internal/index"
	"pensa.sh/pensa/internal/resolve"
	"pensa.sh/pensa/pkg/version"
)

var _ resolve.Provider = (*prefetchProvider)(nil)

// prefetchProvider wraps a resolve.Provider and speculatively prefetches
// GetVersionDetail for upcoming candidate versions when the solver commits
// to a version. This hides network latency when the solver backtracks.
type prefetchProvider struct {
	inner    resolve.Provider
	client   *index.CachedClient
	versions map[string][]version.Version // pkg → sorted versions (cached from Versions() calls)
	sem      chan struct{}                 // bounds prefetch concurrency
}

const maxPrefetch = 10

func newPrefetchProvider(inner resolve.Provider, client *index.CachedClient, concurrency int) *prefetchProvider {
	return &prefetchProvider{
		inner:    inner,
		client:   client,
		versions: make(map[string][]version.Version),
		sem:      make(chan struct{}, concurrency),
	}
}

func (p *prefetchProvider) Versions(pkg string) ([]version.Version, error) {
	vs, err := p.inner.Versions(pkg)
	if err != nil {
		return nil, err
	}
	p.versions[pkg] = vs
	return vs, nil
}

func (p *prefetchProvider) Dependencies(pkg string, ver version.Version) ([]resolve.Dependency, error) {
	deps, err := p.inner.Dependencies(pkg, ver)
	if err != nil {
		return nil, err
	}

	p.prefetchNextVersions(pkg, ver)

	return deps, nil
}

// prefetchNextVersions fires background GetVersionDetail calls for the next
// N versions below the current one. When the solver backtracks, the next
// candidate is already in the in-memory cache.
func (p *prefetchProvider) prefetchNextVersions(pkg string, current version.Version) {
	vs, ok := p.versions[pkg]
	if !ok {
		return
	}

	found := false
	count := 0
	for _, v := range vs {
		if !found {
			if version.Compare(v, current) == 0 {
				found = true
			}
			continue
		}
		if count >= maxPrefetch {
			break
		}
		count++
		go func(name string, ver version.Version) {
			p.sem <- struct{}{}
			defer func() { <-p.sem }()
			p.client.GetVersionDetail(name, ver)
		}(pkg, v)
	}
}
