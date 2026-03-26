//go:build integration

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func setupRunProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "run-test"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["six>=1.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)

	os.MkdirAll(filepath.Join(dir, "run_test"), 0755)
	os.WriteFile(filepath.Join(dir, "run_test", "__init__.py"), []byte(""), 0644)

	return dir
}

func TestRun_AutoSync(t *testing.T) {
	assert := is.New(t)
	dir := setupRunProject(t)
	chdir(t, dir)

	// Lock first (run needs a lock file).
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	err := cmd.Execute()
	assert.NoErr(err)

	// No venv yet — run should auto-create + install + execute.
	cmd = newRootCmd()
	cmd.SetArgs([]string{"run", "python", "-c", "import six; print(six.__version__)"})
	err = cmd.Execute()
	assert.NoErr(err) // command succeeded = sync worked + six was importable

	// Venv should exist now.
	_, err = os.Stat(filepath.Join(dir, ".venv"))
	assert.NoErr(err)
}

func TestRun_AlreadySynced(t *testing.T) {
	assert := is.New(t)
	dir := setupRunProject(t)
	chdir(t, dir)

	// Lock + install first.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	err := cmd.Execute()
	assert.NoErr(err)

	cmd = newRootCmd()
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"install"})
	err = cmd.Execute()
	assert.NoErr(err)

	// Run should be a fast no-op sync + execute successfully.
	cmd = newRootCmd()
	cmd.SetArgs([]string{"run", "python", "-c", "print('fast')"})
	err = cmd.Execute()
	assert.NoErr(err)
}

func TestRun_NoSync_WithoutVenv_Errors(t *testing.T) {
	assert := is.New(t)
	dir := setupRunProject(t)
	chdir(t, dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"run", "--no-sync", "python", "-c", "print('hi')"})

	err := cmd.Execute()
	assert.True(err != nil)
	assert.True(strings.Contains(err.Error(), "no virtualenv found"))
}

func TestExtractFlag(t *testing.T) {
	assert := is.New(t)

	found, rest := extractFlag([]string{"--no-sync", "python", "-c", "hi"}, "--no-sync")
	assert.True(found)
	assert.Equal(len(rest), 3)
	assert.Equal(rest[0], "python")

	found, rest = extractFlag([]string{"python", "-c", "hi"}, "--no-sync")
	assert.True(!found)
	assert.Equal(len(rest), 3)

	// Stop at -- separator.
	found, rest = extractFlag([]string{"--", "--no-sync", "cmd"}, "--no-sync")
	assert.True(!found)
	assert.Equal(len(rest), 3)
}
