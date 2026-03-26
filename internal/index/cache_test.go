package index

import (
	"testing"
	"time"

	"github.com/matryer/is"
)

func TestCache_PutGet(t *testing.T) {
	cache := NewCache(t.TempDir())

	data := []byte(`{"test": true}`)
	if err := cache.Put("simple/requests.json", data); err != nil {
		t.Fatal(err)
	}

	got, err := cache.Get("simple/requests.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Errorf("Get = %q, want %q", got, data)
	}
}

func TestCache_Miss(t *testing.T) {
	cache := NewCache(t.TempDir())

	got, err := cache.Get("nonexistent/key")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil for cache miss, got %q", got)
	}
}

func TestCache_NestedKeys(t *testing.T) {
	cache := NewCache(t.TempDir())

	data := []byte(`metadata`)
	if err := cache.Put("metadata/requests/2.31.0.json", data); err != nil {
		t.Fatal(err)
	}

	got, err := cache.Get("metadata/requests/2.31.0.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "metadata" {
		t.Errorf("Get = %q", got)
	}
}

func TestCacheMeta_Fresh(t *testing.T) {
	assert := is.New(t)

	meta := &CacheMeta{
		ETag:   `"abc"`,
		Date:   time.Now().Unix(),
		MaxAge: 600,
	}
	assert.True(meta.Fresh())
}

func TestCacheMeta_Stale(t *testing.T) {
	assert := is.New(t)

	meta := &CacheMeta{
		ETag:   `"abc"`,
		Date:   time.Now().Unix() - 700, // 700s ago, max-age=600
		MaxAge: 600,
	}
	assert.True(!meta.Fresh())
}

func TestCacheMeta_NoMaxAge(t *testing.T) {
	assert := is.New(t)

	meta := &CacheMeta{
		ETag: `"abc"`,
		Date: time.Now().Unix(),
	}
	assert.True(!meta.Fresh()) // no max-age means not fresh
}

func TestCache_PutWithMeta_GetMeta(t *testing.T) {
	assert := is.New(t)

	cache := NewCache(t.TempDir())
	meta := CacheMeta{
		ETag:   `"abc123"`,
		Date:   time.Now().Unix(),
		MaxAge: 600,
	}

	err := cache.PutWithMeta("simple/requests.json", []byte(`{}`), meta)
	assert.NoErr(err)

	got := cache.GetMeta("simple/requests.json")
	assert.True(got != nil)
	assert.Equal(got.ETag, `"abc123"`)
	assert.Equal(got.MaxAge, 600)
}

func TestCache_GetMeta_Missing(t *testing.T) {
	assert := is.New(t)

	cache := NewCache(t.TempDir())
	got := cache.GetMeta("nonexistent")
	assert.True(got == nil)
}
