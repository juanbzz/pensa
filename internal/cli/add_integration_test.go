//go:build integration

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/adrg/xdg"
	"github.com/matryer/is"
	"github.com/vmihailenco/msgpack/v5"

	"pensa.sh/pensa/internal/index"
	"pensa.sh/pensa/internal/lockfile"
)

// goetry-176: 'pensa add' uses raw PyPIClient and refreshes the on-disk
// Simple-API cache for the added package, but pensa's resolution cache
// (resCache/<pkg>.msgpack) may still hold the pre-refresh view. The
// subsequent lock step loads that stale view and fails with "no versions
// of X match >=<latest>,<4.0".
//
// Pre-write a stale resolution-cache entry for idna with hilariously old
// versions. Without the fix the lock step inside `pensa add idna` will
// reconstruct PackageInfo from these versions, find no candidate >=
// pyproject's just-written constraint, and fail. With the fix add
// invalidates the resCache entry for any added package, so lock falls
// through to the fresh Simple-API cache and resolves the real version.
func TestAdd_InvalidatesStaleResolutionCacheEntry(t *testing.T) {
	assert := is.New(t)
	useTempCache(t)

	// Stale resCache entry for idna.
	resDir := filepath.Join(xdg.CacheHome, "pensa", "resolution")
	assert.NoErr(os.MkdirAll(resDir, 0755))
	stale := &index.ResolutionPackage{
		Name:     "idna",
		Versions: []string{"0.1", "0.2"},
		Deps:     map[string]index.ResolutionEntry{},
	}
	data, err := msgpack.Marshal(stale)
	assert.NoErr(err)
	assert.NoErr(os.WriteFile(filepath.Join(resDir, "idna.msgpack"), data, 0644))

	// Minimal project.
	dir := t.TempDir()
	chdir(t, dir)
	assert.NoErr(os.WriteFile("pyproject.toml", []byte(`
[project]
name = "demo"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = []

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644))

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"add", "idna"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa add idna failed (lock saw stale resCache?): %v\noutput:\n%s",
			err, buf.String())
	}

	// The lock must have a real idna — not the stale fakes.
	lockPath, _ := lockfile.DetectLockFile(dir)
	assert.True(lockPath != "")
	lf, err := lockfile.ReadLockFile(lockPath)
	assert.NoErr(err)
	found := false
	for _, p := range lf.Packages {
		if p.Name == "idna" {
			found = true
			if p.Version == "0.1" || p.Version == "0.2" {
				t.Errorf("lock contains a stale fake idna version %s — resCache wasn't invalidated", p.Version)
			}
		}
	}
	if !found {
		t.Error("lock missing idna entirely")
	}
}
