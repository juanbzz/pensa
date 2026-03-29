package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestTree_AllPackages(t *testing.T) {
	assert := is.New(t)
	setupTestDir(t, testLockFile())

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"tree"})

	assert.NoErr(cmd.Execute())

	out := buf.String()

	assert.True(strings.Contains(out, "├──") || strings.Contains(out, "└──")) // tree output should have box-drawing characters
	assert.True(strings.Contains(out, "requests 2.31.0"))
}

func TestTree_SinglePackage(t *testing.T) {
	assert := is.New(t)
	setupTestDir(t, testLockFile())

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"tree", "requests"})

	assert.NoErr(cmd.Execute())

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	// First line should be the root package.
	assert.True(strings.HasPrefix(lines[0], "requests 2.31.0"))

	// Should show deps as children.
	assert.True(strings.Contains(out, "certifi"))
	assert.True(strings.Contains(out, "urllib3"))
}

func TestTree_PackageNotFound(t *testing.T) {
	assert := is.New(t)
	setupTestDir(t, testLockFile())

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"tree", "nonexistent"})

	err := cmd.Execute()
	assert.True(err != nil) // expected error for nonexistent package
	assert.True(strings.Contains(err.Error(), "not found"))
}

func TestTree_TopLevel(t *testing.T) {
	assert := is.New(t)
	dir := setupTestDir(t, testLockFile())
	writePyprojectWithDeps(t, dir, `requests = "^2.31"`)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"tree", "--top-level"})

	assert.NoErr(cmd.Execute())

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	// Only requests should be a root (first line without prefix).
	assert.True(strings.HasPrefix(lines[0], "requests"))

	// certifi should appear as a child (indented), not as a root.
	for _, line := range lines {
		assert.True(!strings.HasPrefix(line, "certifi")) // certifi should not be a root in top-level tree
	}
}

func TestTree_NoLockFile(t *testing.T) {
	assert := is.New(t)
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"tree"})

	err := cmd.Execute()
	assert.True(err != nil) // expected error when no poetry.lock
}
