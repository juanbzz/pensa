package index

import (
	"fmt"
	"sync"

	"pensa.sh/pensa/pkg/pep508"
	"pensa.sh/pensa/pkg/version"
)

var _ Repository = (*CachedClient)(nil)

type CachedClient struct {
	client   *PyPIClient
	resCache *ResolutionCache
	packages sync.Map // string → *PackageInfo
	details  sync.Map // string → *VersionDetail
	// resMu serializes writes to the resolution cache. Writers use
	// copy-on-write: they clone the ResolutionPackage.Deps map before
	// mutating, then swap the pointer back via resCache.Put. Readers
	// access the old pointer (via resCache.Get's sync.Map), whose
	// Deps map is now immutable, without locking.
	resMu sync.Mutex
	wg    sync.WaitGroup // tracks background cache update goroutines
}

func NewCachedClient(client *PyPIClient, resCache *ResolutionCache) *CachedClient {
	return &CachedClient{client: client, resCache: resCache}
}

// Wait blocks until all background cache update goroutines complete.
// Call before Flush() to ensure all in-memory updates are visible.
func (c *CachedClient) Wait() {
	c.wg.Wait()
}

// FreshPackageInfo bypasses the resolution cache and fetches full PackageInfo
// from PyPI (disk cache or network). Use when RequiresPython data is needed.
func (c *CachedClient) FreshPackageInfo(name string) (*PackageInfo, error) {
	normalized := pep508.NormalizeName(name)

	info, err := c.client.GetPackageInfo(name)
	if err != nil {
		return nil, err
	}

	c.packages.Store(normalized, info)
	return info, nil
}

func (c *CachedClient) GetPackageInfo(name string) (*PackageInfo, error) {
	normalized := pep508.NormalizeName(name)
	if v, ok := c.packages.Load(normalized); ok {
		return v.(*PackageInfo), nil
	}

	// Check resolution cache for a fast reconstruct (avoids parsing large JSON).
	if c.resCache != nil {
		if pkg, err := c.resCache.Get(normalized); err == nil && len(pkg.Versions) > 0 {
			info := pkg.ToPackageInfo()
			c.packages.Store(normalized, info)
			return info, nil
		}
	}

	info, err := c.client.GetPackageInfo(name)
	if err != nil {
		return nil, err
	}

	// Update resolution cache with version list (in-memory only; flushed later).
	if c.resCache != nil {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			c.updateResolutionCache(info)
		}()
	}

	c.packages.Store(normalized, info)
	return info, nil
}

// VersionDetailIfCached returns cached detail for (name, ver) without
// triggering a fetch. Checks the in-memory sync.Map first, then the
// copy-on-write resolution cache (disk-backed but lock-free-for-read).
// Returns (nil, false) on miss. Used by the solver's range-batching
// widening to grow learned clauses across cached neighbors without
// paying network cost.
func (c *CachedClient) VersionDetailIfCached(name string, ver version.Version) (*VersionDetail, bool) {
	key := fmt.Sprintf("%s/%s", name, ver)
	if v, ok := c.details.Load(key); ok {
		return v.(*VersionDetail), true
	}
	if c.resCache != nil {
		if detail := c.getFromResolutionCache(name, ver); detail != nil {
			c.details.Store(key, detail)
			return detail, true
		}
	}
	return nil, false
}

func (c *CachedClient) GetVersionDetail(name string, ver version.Version) (*VersionDetail, error) {
	key := fmt.Sprintf("%s/%s", name, ver)
	if v, ok := c.details.Load(key); ok {
		return v.(*VersionDetail), nil
	}

	// Check resolution cache before hitting PyPI.
	if c.resCache != nil {
		if detail := c.getFromResolutionCache(name, ver); detail != nil {
			c.details.Store(key, detail)
			return detail, nil
		}
	}

	detail, err := c.fetchVersionDetail(name, ver)
	if err != nil {
		return nil, err
	}

	// Store in resolution cache for next run (in-memory only; flushed later).
	if c.resCache != nil {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			c.storeInResolutionCache(name, detail)
		}()
	}

	c.details.Store(key, detail)
	return detail, nil
}

// fetchVersionDetail routes to PEP 658 or JSON API based on cached PackageInfo.
func (c *CachedClient) fetchVersionDetail(name string, ver version.Version) (*VersionDetail, error) {
	normalized := pep508.NormalizeName(name)

	// If we have PackageInfo cached, use it to decide the fetch strategy.
	if v, ok := c.packages.Load(normalized); ok {
		info := v.(*PackageInfo)
		wheel := info.BestWheel(ver)
		if wheel != nil && wheel.CoreMetadata {
			return c.client.FetchPEP658Metadata(wheel.URL + ".metadata")
		}
		return c.client.FetchJSONAPI(normalized, ver)
	}

	// No PackageInfo cached — fall back to full path.
	return c.client.GetVersionDetail(name, ver)
}

func (c *CachedClient) getFromResolutionCache(name string, ver version.Version) *VersionDetail {
	normalized := pep508.NormalizeName(name)
	// Lock-free read: copy-on-write writers (store/updateResolutionCache)
	// publish a fresh *ResolutionPackage via resCache.Put, leaving the
	// old pointer's Deps map immutable. Any snapshot we read here is
	// internally consistent even if a concurrent writer is about to
	// publish a newer one.
	pkg, err := c.resCache.Get(normalized)
	if err != nil {
		return nil
	}
	entry, ok := pkg.Deps[ver.String()]
	if !ok {
		return nil
	}

	detail := entry.ToVersionDetail(normalized)
	v, err := version.Parse(entry.Version)
	if err == nil {
		detail.Version = v
	}
	return detail
}

func (c *CachedClient) updateResolutionCache(info *PackageInfo) {
	c.resMu.Lock()
	defer c.resMu.Unlock()

	normalized := pep508.NormalizeName(info.Name)
	existing, _ := c.resCache.Get(normalized)
	if existing == nil {
		c.resCache.Put(FromPackageInfo(info))
		return
	}
	// Copy-on-write: build a fresh package (re-using the old Deps map
	// verbatim is safe, since readers only access what was published
	// before their Get returned, and the old pointer's Deps isn't
	// mutated after publication). But we still need a NEW top-level
	// struct so concurrent readers with the old pointer aren't affected.
	fresh := FromPackageInfo(info)
	fresh.Deps = existing.Deps
	c.resCache.Put(fresh)
}

func (c *CachedClient) storeInResolutionCache(name string, detail *VersionDetail) {
	c.resMu.Lock()
	defer c.resMu.Unlock()

	normalized := pep508.NormalizeName(name)
	old, _ := c.resCache.Get(normalized)
	var fresh *ResolutionPackage
	if old == nil {
		fresh = &ResolutionPackage{
			Name: normalized,
			Deps: map[string]ResolutionEntry{
				detail.Version.String(): FromVersionDetail(detail),
			},
		}
	} else {
		// Copy-on-write: clone Deps before mutating so concurrent readers
		// holding the old pointer see an immutable snapshot. The top-level
		// struct is also new; the new pointer is published via
		// resCache.Put which uses sync.Map internally.
		fresh = &ResolutionPackage{
			Name:      old.Name,
			Versions:  old.Versions,
			PEP658:    old.PEP658,
			WheelURLs: old.WheelURLs,
			Deps:      make(map[string]ResolutionEntry, len(old.Deps)+1),
		}
		for k, v := range old.Deps {
			fresh.Deps[k] = v
		}
		fresh.Deps[detail.Version.String()] = FromVersionDetail(detail)
	}
	c.resCache.Put(fresh)
}
