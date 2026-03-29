package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"

	"github.com/juanbzz/pensa/internal/lockfile"
	"github.com/juanbzz/pensa/internal/pyproject"
)

// setupCheckTest creates a temp dir with matching pyproject.toml and poetry.lock.
func setupCheckTest(t *testing.T) string {
	t.Helper()
	lf := testLockFile()
	dir := setupTestDir(t, lf)

	// Write a pyproject.toml and compute its hash for the lock metadata.
	pyprojectContent := `[tool.poetry]
name = "test-project"
version = "0.1.0"
description = ""

[tool.poetry.dependencies]
python = "^3.8"
requests = "^2.31"
`
	pyprojectPath := filepath.Join(dir, "pyproject.toml")
	if err := os.WriteFile(pyprojectPath, []byte(pyprojectContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Update lock file with correct content hash (dep-only hash).
	proj, _ := pyproject.ParsePyProject([]byte(pyprojectContent))
	lf.Metadata.ContentHash = proj.DependencyHash()
	if err := lockfile.WriteLockFile(filepath.Join(dir, "poetry.lock"), lf); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestCheck_Passes(t *testing.T) {
	assert := is.New(t)
	setupCheckTest(t)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"check"})

	assert.NoErr(cmd.Execute())
	assert.True(strings.Contains(buf.String(), "All checks passed"))
}

func TestCheck_NoLockFile(t *testing.T) {
	assert := is.New(t)
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	// Write pyproject.toml but no lock file.
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`[tool.poetry]
name = "test"
version = "0.1.0"
`), 0644)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"check"})

	err := cmd.Execute()
	assert.True(err != nil) // expected error when no poetry.lock
}

func TestCheck_NoPyproject(t *testing.T) {
	assert := is.New(t)
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"check"})

	err := cmd.Execute()
	assert.True(err != nil) // expected error when no pyproject.toml
}

func TestCheck_HashMismatch(t *testing.T) {
	assert := is.New(t)
	dir := setupCheckTest(t)

	// Modify pyproject.toml to change a dependency (triggers hash mismatch).
	pyprojectPath := filepath.Join(dir, "pyproject.toml")
	os.WriteFile(pyprojectPath, []byte(`[tool.poetry]
name = "test-project"
version = "0.1.0"
description = ""

[tool.poetry.dependencies]
python = "^3.8"
requests = "^3.0"
`), 0644)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"check"})

	err := cmd.Execute()
	assert.True(err != nil) // expected check to fail on hash mismatch

	out := buf.String()
	assert.True(strings.Contains(out, "content hash mismatch"))
}

func TestCheck_MissingDep(t *testing.T) {
	assert := is.New(t)
	dir := setupCheckTest(t)

	// Add flask to pyproject.toml (not in lock file).
	pyprojectContent := `[tool.poetry]
name = "test-project"
version = "0.1.0"
description = ""

[tool.poetry.dependencies]
python = "^3.8"
requests = "^2.31"
flask = "^3.0"
`
	pyprojectPath := filepath.Join(dir, "pyproject.toml")
	os.WriteFile(pyprojectPath, []byte(pyprojectContent), 0644)

	// Update lock hash to match new pyproject (so only the missing dep is reported).
	proj, _ := pyproject.ParsePyProject([]byte(pyprojectContent))
	lf := testLockFile()
	lf.Metadata.ContentHash = proj.DependencyHash()
	lockfile.WriteLockFile(filepath.Join(dir, "poetry.lock"), lf)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"check"})

	err := cmd.Execute()
	assert.True(err != nil) // expected check to fail on missing dep

	out := buf.String()
	assert.True(strings.Contains(out, "flask"))
}

func TestCheck_MultipleIssues(t *testing.T) {
	assert := is.New(t)
	dir := setupCheckTest(t)

	// Change pyproject to have different hash AND a missing dep.
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`[tool.poetry]
name = "test-project"
version = "0.2.0"
description = "changed"

[tool.poetry.dependencies]
python = "^3.8"
requests = "^2.31"
flask = "^3.0"
`), 0644)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"check"})

	err := cmd.Execute()
	assert.True(err != nil) // expected check to fail

	out := buf.String()
	assert.True(strings.Contains(out, "content hash mismatch"))
	assert.True(strings.Contains(out, "flask"))
	assert.True(strings.Contains(err.Error(), "2 issues"))
}
