package index

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
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

// CacheMeta stores HTTP cache policy for a cached response.
type CacheMeta struct {
	ETag   string `json:"etag,omitempty"`
	Date   int64  `json:"date"`    // unix timestamp when cached
	MaxAge int    `json:"max_age"` // seconds, from Cache-Control: max-age
}

// Fresh reports whether the cached entry is still within its max-age window.
func (m *CacheMeta) Fresh() bool {
	if m.MaxAge <= 0 {
		return false
	}
	return time.Now().Unix()-m.Date < int64(m.MaxAge)
}

// GetMeta returns the cache metadata sidecar for the given key, or nil.
func (c *Cache) GetMeta(key string) *CacheMeta {
	path := c.path(key) + ".meta"
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var meta CacheMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil
	}
	return &meta
}

// PutWithMeta stores data and its HTTP cache metadata atomically.
func (c *Cache) PutWithMeta(key string, data []byte, meta CacheMeta) error {
	if err := c.Put(key, data); err != nil {
		return err
	}
	metaData, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path(key)+".meta", metaData, 0644)
}

// UpdateMeta updates only the metadata sidecar without touching the data.
func (c *Cache) UpdateMeta(key string, meta CacheMeta) error {
	metaData, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path(key)+".meta", metaData, 0644)
}

func (c *Cache) path(key string) string {
	return filepath.Join(c.dir, key)
}
