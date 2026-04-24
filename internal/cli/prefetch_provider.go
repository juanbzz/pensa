package cli

import (
	"context"
	"sync"

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
	sem      chan struct{}                // bounds prefetch concurrency
	wg       sync.WaitGroup               // tracks in-flight speculative fetches so callers can drain before shutdown
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

func (p *prefetchProvider) Versions(ctx context.Context, pkg string) ([]version.Version, error) {
	vs, err := p.inner.Versions(ctx, pkg)
	if err != nil {
		return nil, err
	}
	p.versions[pkg] = vs
	return vs, nil
}

func (p *prefetchProvider) Dependencies(ctx context.Context, pkg string, ver version.Version) ([]resolve.Dependency, error) {
	deps, err := p.inner.Dependencies(ctx, pkg, ver)
	if err != nil {
		return nil, err
	}

	p.prefetchNextVersions(pkg, ver)

	return deps, nil
}

func (p *prefetchProvider) DependenciesIfCached(ctx context.Context, pkg string, ver version.Version) ([]resolve.Dependency, bool) {
	return p.inner.DependenciesIfCached(ctx, pkg, ver)
}

func (p *prefetchProvider) Preferred(ctx context.Context, pkg string) (version.Version, bool) {
	return p.inner.Preferred(ctx, pkg)
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
		p.wg.Add(1)
		go func(name string, ver version.Version) {
			defer p.wg.Done()
			p.sem <- struct{}{}
			defer func() { <-p.sem }()
			p.client.GetVersionDetail(name, ver)
		}(pkg, v)
	}
}

// WaitPrefetches blocks until every speculative prefetch goroutine
// has returned. Callers invoke this before flushing the resolution
// cache so no in-flight fetches race the writer.
func (p *prefetchProvider) WaitPrefetches() {
	p.wg.Wait()
}
