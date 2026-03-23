package cli

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juanbzz/goetry/internal/lockfile"
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

	// Update lock file with correct content hash.
	h := sha256.Sum256([]byte(pyprojectContent))
	lf.Metadata.ContentHash = fmt.Sprintf("%x", h)
	if err := lockfile.WriteLockFile(filepath.Join(dir, "poetry.lock"), lf); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestCheck_Passes(t *testing.T) {
	setupCheckTest(t)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"check"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected check to pass, got: %v", err)
	}

	if !strings.Contains(buf.String(), "All checks passed") {
		t.Errorf("expected 'All checks passed', got: %s", buf.String())
	}
}

func TestCheck_NoLockFile(t *testing.T) {
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
	if err == nil {
		t.Fatal("expected error when no poetry.lock")
	}
}

func TestCheck_NoPyproject(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"check"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no pyproject.toml")
	}
}

func TestCheck_HashMismatch(t *testing.T) {
	dir := setupCheckTest(t)

	// Modify pyproject.toml to change the hash.
	pyprojectPath := filepath.Join(dir, "pyproject.toml")
	os.WriteFile(pyprojectPath, []byte(`[tool.poetry]
name = "test-project"
version = "0.2.0"
description = "modified"

[tool.poetry.dependencies]
python = "^3.8"
requests = "^2.31"
`), 0644)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"check"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected check to fail on hash mismatch")
	}

	out := buf.String()
	if !strings.Contains(out, "content hash mismatch") {
		t.Errorf("expected hash mismatch message, got: %s", out)
	}
}

func TestCheck_MissingDep(t *testing.T) {
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
	h := sha256.Sum256([]byte(pyprojectContent))
	lf := testLockFile()
	lf.Metadata.ContentHash = fmt.Sprintf("%x", h)
	lockfile.WriteLockFile(filepath.Join(dir, "poetry.lock"), lf)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"check"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected check to fail on missing dep")
	}

	out := buf.String()
	if !strings.Contains(out, "flask") {
		t.Errorf("expected missing flask message, got: %s", out)
	}
}

func TestCheck_MultipleIssues(t *testing.T) {
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
	if err == nil {
		t.Fatal("expected check to fail")
	}

	out := buf.String()
	if !strings.Contains(out, "content hash mismatch") {
		t.Error("should report hash mismatch")
	}
	if !strings.Contains(out, "flask") {
		t.Error("should report missing flask")
	}
	if !strings.Contains(err.Error(), "2 issues") {
		t.Errorf("should report 2 issues, got: %s", err.Error())
	}
}
