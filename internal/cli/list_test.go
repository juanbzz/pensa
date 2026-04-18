package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/matryer/is"

	"pensa.sh/pensa/internal/lockfile"
)

func TestList_AllPackages(t *testing.T) {
	assert := is.New(t)
	setupTestDir(t, testLockFile())

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"list"})

	assert.NoErr(cmd.Execute())

	out := buf.String()

	for _, pkg := range []string{"certifi", "charset-normalizer", "idna", "requests", "urllib3"} {
		assert.True(strings.Contains(out, pkg)) // output should contain package
	}

	// Should be alphabetically sorted.
	certIdx := strings.Index(out, "certifi")
	reqIdx := strings.Index(out, "requests")
	assert.True(certIdx < reqIdx) // packages should be sorted alphabetically
}

func TestList_TopLevel(t *testing.T) {
	assert := is.New(t)
	dir := setupTestDir(t, testLockFile())
	writePyprojectWithDeps(t, dir, `requests = "^2.31"`)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"list", "--top-level"})

	assert.NoErr(cmd.Execute())

	out := buf.String()

	assert.True(strings.Contains(out, "requests"))    // top-level should include requests
	assert.True(!strings.Contains(out, "certifi"))     // top-level should not include transitive dep certifi
}

func TestList_Empty(t *testing.T) {
	assert := is.New(t)
	lf := &lockfile.LockFile{
		Metadata: lockfile.LockMetadata{
			LockVersion:    "2.1",
			PythonVersions: ">=3.8",
			ContentHash:    "empty",
		},
	}
	setupTestDir(t, lf)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"list"})

	assert.NoErr(cmd.Execute())

	out := buf.String()
	assert.True(strings.Contains(out, "No packages found"))
}

func TestList_NoLockFile(t *testing.T) {
	assert := is.New(t)
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"list"})

	err := cmd.Execute()
	assert.True(err != nil) // expected error when no poetry.lock
}
