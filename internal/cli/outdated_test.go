package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/matryer/is"

	"pensa.sh/pensa/internal/lockfile"
	"pensa.sh/pensa/pkg/version"
)

// findOutdated is the pure logic: given a lock-derived list and a
// latest-version lookup, decide which packages are behind and classify
// the gap as patch/minor/major. Skips silently when the lookup fails or
// either version is unparseable — a partial PyPI outage shouldn't kill
// the whole report.
func TestFindOutdated(t *testing.T) {
	assert := is.New(t)

	pkgs := []lockfile.LockedPackage{
		{Name: "uptodate", Version: "1.2.3"},
		{Name: "patchbehind", Version: "1.2.3"},
		{Name: "minorbehind", Version: "1.2.0"},
		{Name: "majorbehind", Version: "1.0.0"},
		{Name: "lookupfails", Version: "1.0.0"},
		{Name: "aheadoflatest", Version: "5.0.0"}, // prerelease/pin — skip
		{Name: "unparseable", Version: "not.a.version"},
	}

	fake := map[string]string{
		"uptodate":      "1.2.3",
		"patchbehind":   "1.2.5",
		"minorbehind":   "1.5.0",
		"majorbehind":   "2.0.0",
		"aheadoflatest": "3.0.0",
	}
	lookup := func(name string) (version.Version, error) {
		v, ok := fake[name]
		if !ok {
			return version.Version{}, fmt.Errorf("no version found for %s", name)
		}
		return version.Parse(v)
	}

	out := findOutdated(pkgs, lookup)

	byName := make(map[string]OutdatedEntry, len(out))
	for _, e := range out {
		byName[e.Name] = e
	}

	assert.Equal(len(out), 3) // patchbehind, minorbehind, majorbehind

	assert.Equal(byName["patchbehind"].Current, "1.2.3")
	assert.Equal(byName["patchbehind"].Latest, "1.2.5")
	assert.Equal(byName["patchbehind"].Level, "patch")

	assert.Equal(byName["minorbehind"].Latest, "1.5.0")
	assert.Equal(byName["minorbehind"].Level, "minor")

	assert.Equal(byName["majorbehind"].Latest, "2.0.0")
	assert.Equal(byName["majorbehind"].Level, "major")

	_, hasUptodate := byName["uptodate"]
	assert.True(!hasUptodate)
	_, hasFails := byName["lookupfails"]
	assert.True(!hasFails)
	_, hasAhead := byName["aheadoflatest"]
	assert.True(!hasAhead)
}

// Empty-input cases — both writers must produce something sensible.
// Table prints a friendly "up to date" line so callers can tell the
// command actually ran. JSON emits [] (not null) so jq pipelines and
// CI scripts don't choke.
func TestOutdatedWriters_EmptyInputs(t *testing.T) {
	assert := is.New(t)

	var tbl bytes.Buffer
	writeOutdatedTable(&tbl, nil)
	assert.True(strings.Contains(tbl.String(), "up to date"))

	var js bytes.Buffer
	writeOutdatedJSON(&js, nil)
	out := strings.TrimSpace(js.String())
	assert.Equal(out, "[]")

	// And the round-trip works: encoded output parses back to an empty slice.
	var got []OutdatedEntry
	assert.NoErr(json.Unmarshal(js.Bytes(), &got))
	assert.Equal(len(got), 0)
}
