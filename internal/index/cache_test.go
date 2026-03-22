package index

import (
	"testing"
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
