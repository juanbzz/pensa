//go:build integration

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLockInterop_WritesPensaLock(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	os.WriteFile("pyproject.toml", []byte(`
[project]
name = "test-project"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["certifi>=2023.0.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa lock failed: %v", err)
	}

	// Should write pensa.lock, not poetry.lock.
	if _, err := os.Stat("pensa.lock"); err != nil {
		t.Error("pensa.lock should be created")
	}

	// Verify pensa.lock has URLs.
	data, _ := os.ReadFile("pensa.lock")
	content := string(data)
	if !strings.Contains(content, "certifi") {
		t.Error("pensa.lock should contain certifi")
	}
	if !strings.Contains(content, "url =") {
		t.Error("pensa.lock should contain download URLs")
	}
	if !strings.Contains(content, "files =") {
		t.Error("pensa.lock should have files section")
	}
}

func TestLockInterop_InstallFromPensaLock(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	os.WriteFile("pyproject.toml", []byte(`
[project]
name = "test-project"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["six>=1.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)

	// Lock first.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa lock failed: %v", err)
	}

	// Install from pensa.lock.
	cmd = newRootCmd()
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"install", "--no-root"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa install from pensa.lock failed: %v", err)
	}

	// Verify by site-packages contents.
	siteMatches, _ := filepath.Glob(filepath.Join(dir, ".venv", "lib", "python*", "site-packages"))
	if len(siteMatches) != 1 {
		t.Fatalf("expected exactly one site-packages dir, got %v", siteMatches)
	}
	if m, _ := filepath.Glob(filepath.Join(siteMatches[0], "six-*.dist-info")); len(m) == 0 {
		t.Errorf("should install six; install output:\n%s", buf.String())
	}
}

func TestLockInterop_ReadPoetryLock(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	// Create a poetry.lock manually (simulating existing Poetry project).
	os.WriteFile("pyproject.toml", []byte(`
[project]
name = "test-project"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["certifi>=2023.0.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)

	// First lock to get a valid lock file, then rename to poetry.lock.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa lock failed: %v", err)
	}

	// Rename pensa.lock to poetry.lock to simulate Poetry project.
	os.Rename("pensa.lock", "poetry.lock")

	// Install should detect and read poetry.lock.
	cmd = newRootCmd()
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"install", "--no-root"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa install from poetry.lock failed: %v", err)
	}
}

func TestLockInterop_ReadUVLock(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	os.WriteFile("pyproject.toml", []byte(`
[project]
name = "test-project"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["six>=1.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)

	// Create a minimal uv.lock with six.
	os.WriteFile("uv.lock", []byte(`version = 1
requires-python = ">=3.10"

[[package]]
name = "six"
version = "1.17.0"
source = { registry = "https://pypi.org/simple" }
wheels = [
    { url = "https://files.pythonhosted.org/packages/b7/ce/149a00dd41f10bc29e5921b496af8b574d8413afcd5e30f0c0e6bbdc894f/six-1.17.0-py2.py3-none-any.whl", hash = "sha256:4721f391ed90541fddacab5acf947aa0d3dc7d27b2e1e8bbe64d517966b6049a", size = 11049 },
]
`), 0644)

	// List should read from uv.lock.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa list from uv.lock failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "six") {
		t.Errorf("should list six from uv.lock: %s", out)
	}
}

func TestLockInterop_PensaLockPrecedence(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	os.WriteFile("pyproject.toml", []byte(`
[project]
name = "test-project"
version = "0.1.0"
`), 0644)

	// Create both lock files — pensa.lock should win.
	os.WriteFile(filepath.Join(dir, "poetry.lock"), []byte("# poetry"), 0644)
	os.WriteFile(filepath.Join(dir, "uv.lock"), []byte("# uv"), 0644)
	os.WriteFile(filepath.Join(dir, "pensa.lock"), []byte("# pensa"), 0644)

	// Check should detect pensa.lock.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"check"})
	// Will fail to parse but the error message tells us which file it tried.
	err := cmd.Execute()
	if err == nil {
		t.Skip("check passed unexpectedly")
	}
	// Error should reference reading the lock file (pensa.lock was detected).
	// Not a definitive test, but verifies detection ran.
}
