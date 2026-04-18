//go:build integration

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPEP735Integration_AddWritesPEP735(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	os.WriteFile("pyproject.toml", []byte(`
[project]
name = "test-project"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = []

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)

	// Add pytest to dev group.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"add", "six", "-G", "dev"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa add -G dev failed: %v", err)
	}

	// Verify pyproject.toml uses [dependency-groups], not [tool.poetry.group].
	data, _ := os.ReadFile("pyproject.toml")
	content := string(data)

	if !strings.Contains(content, "dependency-groups") {
		t.Error("should write to [dependency-groups] (PEP 735)")
	}
	if strings.Contains(content, "tool.poetry.group") {
		t.Error("should NOT write to [tool.poetry.group]")
	}
	if !strings.Contains(content, "six") {
		t.Error("pyproject.toml should contain six")
	}
}

func TestPEP735Integration_InstallNoDev(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	// Use PEP 735 format directly.
	os.WriteFile("pyproject.toml", []byte(`
[project]
name = "test-project"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["certifi>=2023.0.0"]

[dependency-groups]
dev = ["six>=1.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)

	// Lock.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa lock failed: %v", err)
	}

	// Install with --no-dev.
	cmd = newRootCmd()
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"install", "--no-dev", "--no-root"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa install --no-dev failed: %v", err)
	}

	// Verify by site-packages contents: certifi in, six out.
	siteMatches, _ := filepath.Glob(filepath.Join(dir, ".venv", "lib", "python*", "site-packages"))
	if len(siteMatches) != 1 {
		t.Fatalf("expected exactly one site-packages dir, got %v", siteMatches)
	}
	if m, _ := filepath.Glob(filepath.Join(siteMatches[0], "certifi-*.dist-info")); len(m) == 0 {
		t.Error("should install certifi (main group)")
	}
	if m, _ := filepath.Glob(filepath.Join(siteMatches[0], "six-*.dist-info")); len(m) > 0 {
		t.Error("should NOT install six with --no-dev")
	}
}

func TestPEP735Integration_RemoveFromGroup(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	os.WriteFile("pyproject.toml", []byte(`
[project]
name = "test-project"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["certifi>=2023.0.0"]

[dependency-groups]
dev = ["six>=1.0"]

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

	// Remove six from dev.
	cmd = newRootCmd()
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"remove", "six", "-G", "dev"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa remove -G dev failed: %v", err)
	}

	data, _ := os.ReadFile("pyproject.toml")
	if strings.Contains(string(data), "six") {
		t.Error("pyproject.toml should not contain six after removal")
	}
}

func TestPEP735Integration_IncludeGroup(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	// Test include-group: dev includes test group.
	os.WriteFile("pyproject.toml", []byte(`
[project]
name = "test-project"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = []

[dependency-groups]
test = ["six>=1.0"]
dev = [{include-group = "test"}, "certifi>=2023.0.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)

	// Lock — should resolve both six and certifi under dev.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa lock failed: %v", err)
	}

	lockData, _ := os.ReadFile("pensa.lock")
	content := string(lockData)

	if !strings.Contains(content, "six") {
		t.Error("lock should contain six (from test group via include-group)")
	}
	if !strings.Contains(content, "certifi") {
		t.Error("lock should contain certifi (from dev group)")
	}
}

func TestPEP735Integration_PoetryGroupsFallback(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	// Project using Poetry groups (no PEP 735). Should still work.
	os.WriteFile("pyproject.toml", []byte(`
[tool.poetry]
name = "test-project"
version = "0.1.0"

[tool.poetry.dependencies]
python = "^3.8"
certifi = ">=2023.0.0"

[tool.poetry.group.dev.dependencies]
six = ">=1.0"

[build-system]
requires = ["poetry-core"]
build-backend = "poetry.core.masonry.api"
`), 0644)

	// Lock should still work with Poetry groups.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa lock with Poetry groups failed: %v", err)
	}

	lockData, _ := os.ReadFile("pensa.lock")
	content := string(lockData)

	if !strings.Contains(content, "certifi") {
		t.Error("lock should contain certifi")
	}
	if !strings.Contains(content, "six") {
		t.Error("lock should contain six from Poetry dev group")
	}
}
