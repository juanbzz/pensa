package index

import (
	"fmt"
	"sync"

	"github.com/juanbzz/pensa/pkg/version"
)

var _ Repository = (*CachedClient)(nil)

type CachedClient struct {
	client   *PyPIClient
	packages sync.Map // string → *PackageInfo
	details  sync.Map // string → *VersionDetail
}

func NewCachedClient(client *PyPIClient) *CachedClient {
	return &CachedClient{client: client}
}

func (c *CachedClient) GetPackageInfo(name string) (*PackageInfo, error) {
	if v, ok := c.packages.Load(name); ok {
		return v.(*PackageInfo), nil
	}

	info, err := c.client.GetPackageInfo(name)
	if err != nil {
		return nil, err
	}

	c.packages.Store(name, info)
	return info, nil
}

func (c *CachedClient) GetVersionDetail(name string, ver version.Version) (*VersionDetail, error) {
	key := fmt.Sprintf("%s/%s", name, ver)
	if v, ok := c.details.Load(key); ok {
		return v.(*VersionDetail), nil
	}

	detail, err := c.client.GetVersionDetail(name, ver)
	if err != nil {
		return nil, err
	}

	c.details.Store(key, detail)
	return detail, nil
}
