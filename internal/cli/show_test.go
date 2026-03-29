package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestShow_PackageDetail(t *testing.T) {
	assert := is.New(t)
	setupTestDir(t, testLockFile())

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"show", "requests"})

	assert.NoErr(cmd.Execute())

	out := buf.String()

	assert.True(strings.Contains(out, "requests"))   // package name
	assert.True(strings.Contains(out, "2.31.0"))      // version
	assert.True(strings.Contains(out, "Requires:"))   // Requires line
	assert.True(strings.Contains(out, "certifi"))      // dependency certifi
	assert.True(strings.Contains(out, "urllib3"))       // dependency urllib3
	assert.True(strings.Contains(out, "Required-by:")) // Required-by line
}

func TestShow_PackageNotFound(t *testing.T) {
	assert := is.New(t)
	setupTestDir(t, testLockFile())

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"show", "nonexistent"})

	err := cmd.Execute()
	assert.True(err != nil) // expected error for nonexistent package
	assert.True(strings.Contains(err.Error(), "not found"))
}

func TestShow_NoArgs(t *testing.T) {
	assert := is.New(t)
	setupTestDir(t, testLockFile())

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"show"})

	err := cmd.Execute()
	assert.True(err != nil) // expected error when no package arg given
}

func TestShow_NoLockFile(t *testing.T) {
	assert := is.New(t)
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"show", "requests"})

	err := cmd.Execute()
	assert.True(err != nil) // expected error when no poetry.lock
}
