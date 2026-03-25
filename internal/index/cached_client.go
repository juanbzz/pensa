package index

import (
	"fmt"
	"sync"

	"github.com/juanbzz/pensa/pkg/pep508"
	"github.com/juanbzz/pensa/pkg/version"
)

var _ Repository = (*CachedClient)(nil)

type CachedClient struct {
	client    *PyPIClient
	resCache  *ResolutionCache
	packages  sync.Map // string → *PackageInfo
	details   sync.Map // string → *VersionDetail
}

func NewCachedClient(client *PyPIClient, resCache *ResolutionCache) *CachedClient {
	return &CachedClient{client: client, resCache: resCache}
}

func (c *CachedClient) GetPackageInfo(name string) (*PackageInfo, error) {
	if v, ok := c.packages.Load(name); ok {
		return v.(*PackageInfo), nil
	}

	info, err := c.client.GetPackageInfo(name)
	if err != nil {
		return nil, err
	}

	// Update resolution cache with version list.
	if c.resCache != nil {
		go c.updateResolutionCache(info)
	}

	c.packages.Store(name, info)
	return info, nil
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

	// Store in resolution cache for next run.
	if c.resCache != nil {
		go c.storeInResolutionCache(name, detail)
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
	normalized := pep508.NormalizeName(info.Name)
	existing, _ := c.resCache.Get(normalized)
	if existing == nil {
		existing = FromPackageInfo(info)
	} else {
		// Update version list but keep existing deps.
		fresh := FromPackageInfo(info)
		fresh.Deps = existing.Deps
		existing = fresh
	}
	c.resCache.Put(existing)
}

func (c *CachedClient) storeInResolutionCache(name string, detail *VersionDetail) {
	normalized := pep508.NormalizeName(name)
	pkg, _ := c.resCache.Get(normalized)
	if pkg == nil {
		pkg = &ResolutionPackage{
			Name: normalized,
			Deps: make(map[string]ResolutionEntry),
		}
	}
	pkg.Deps[detail.Version.String()] = FromVersionDetail(detail)
	c.resCache.Put(pkg)
}
