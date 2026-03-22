package index

import (
	"os"
	"path/filepath"
)

// Cache provides on-disk caching for repository data.
type Cache struct {
	dir string
}

// NewCache creates a cache rooted at the given directory.
func NewCache(dir string) *Cache {
	return &Cache{dir: dir}
}

// Get returns cached data for the key, or nil if not cached.
func (c *Cache) Get(key string) ([]byte, error) {
	path := c.path(key)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Put stores data in the cache under the given key.
func (c *Cache) Put(key string, data []byte) error {
	path := c.path(key)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (c *Cache) path(key string) string {
	return filepath.Join(c.dir, key)
}
