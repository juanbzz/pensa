//go:build integration

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/matryer/is"
)

// End-to-end: pin a famously-old dep, lock it, then run `pensa outdated`.
// Asserts the table flags the dep, exit is non-zero (errOutdatedFound),
// and --json emits parseable [OutdatedEntry].
func TestOutdated_FlagsOldDepAndExitsNonZero(t *testing.T) {
	assert := is.New(t)
	useTempCache(t)
	dir := t.TempDir()
	chdir(t, dir)

	// idna 2.0 (2016) is many versions behind anything current — a stable
	// pick for "must show up as outdated" without coupling to today's pin.
	assert.NoErr(os.WriteFile("pyproject.toml", []byte(`
[project]
name = "demo"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["idna==2.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644))

	// Lock so outdated has something to read.
	lockCmd := newRootCmd()
	lockCmd.SetOut(io.Discard)
	lockCmd.SetErr(io.Discard)
	lockCmd.SetArgs([]string{"lock"})
	if err := lockCmd.Execute(); err != nil {
		t.Fatalf("pensa lock failed: %v", err)
	}

	// Table form.
	tblCmd := newRootCmd()
	tblBuf := new(bytes.Buffer)
	tblCmd.SetOut(tblBuf)
	tblCmd.SetErr(io.Discard)
	tblCmd.SetArgs([]string{"outdated"})
	if err := tblCmd.Execute(); err == nil {
		t.Fatal("expected non-zero exit (errOutdatedFound), got nil")
	}
	tbl := tblBuf.String()
	if !strings.Contains(tbl, "idna") {
		t.Errorf("table output missing idna:\n%s", tbl)
	}
	if !strings.Contains(tbl, "2.0") {
		t.Errorf("table output missing current version 2.0:\n%s", tbl)
	}

	// JSON form.
	jsonCmd := newRootCmd()
	jsonBuf := new(bytes.Buffer)
	jsonCmd.SetOut(jsonBuf)
	jsonCmd.SetErr(io.Discard)
	jsonCmd.SetArgs([]string{"outdated", "--json"})
	_ = jsonCmd.Execute() // non-nil error expected; we only care about the body
	var entries []OutdatedEntry
	if err := json.Unmarshal(jsonBuf.Bytes(), &entries); err != nil {
		t.Fatalf("--json output isn't parseable JSON: %v\nbody:\n%s", err, jsonBuf.String())
	}
	found := false
	for _, e := range entries {
		if e.Name == "idna" {
			found = true
			assert.Equal(e.Current, "2.0")
			assert.True(e.Latest != "")
			assert.True(e.Level == "major" || e.Level == "minor" || e.Level == "patch")
		}
	}
	if !found {
		t.Errorf("JSON output missing idna entry:\n%s", jsonBuf.String())
	}
}

