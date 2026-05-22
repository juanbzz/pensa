//go:build integration

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Real-world shape borrowed from pgmarketing / startapp: PEP 621
// project with [project.optional-dependencies]. Before the fix in
// internal/pyproject/poetry.go, ResolveAllDependencies dropped these
// groups silently — the lock came out missing every dev/infra package.

func TestOptionalDepsIntegration_LockIncludesOptionalGroups(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	os.WriteFile("pyproject.toml", []byte(`
[project]
name = "test-project"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["certifi>=2023.0.0"]

[project.optional-dependencies]
dev = ["six>=1.0"]
infra = ["wheel>=0.40"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa lock failed: %v", err)
	}

	lockData, _ := os.ReadFile("pensa.lock")
	content := string(lockData)

	if !strings.Contains(content, "certifi") {
		t.Error("lock should contain certifi (main)")
	}
	if !strings.Contains(content, "six") {
		t.Error("lock should contain six ([project.optional-dependencies].dev)")
	}
	if !strings.Contains(content, "wheel") {
		t.Error("lock should contain wheel ([project.optional-dependencies].infra)")
	}
}

func TestOptionalDepsIntegration_PEP735BeatsOptionalDeps(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	// Same group name 'dev' defined in both PEP 735 and PEP 621.
	// PEP 735 must win: six locked, wheel not.
	os.WriteFile("pyproject.toml", []byte(`
[project]
name = "test-project"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["certifi>=2023.0.0"]

[project.optional-dependencies]
dev = ["wheel>=0.40"]

[dependency-groups]
dev = ["six>=1.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa lock failed: %v", err)
	}

	lockData, _ := os.ReadFile("pensa.lock")
	content := string(lockData)

	if !strings.Contains(content, "six") {
		t.Error("lock should contain six (PEP 735 dev wins)")
	}
	if strings.Contains(content, "wheel") {
		t.Error("lock should NOT contain wheel (PEP 621 dev loses to PEP 735)")
	}
}

func TestOptionalDepsIntegration_InstallNoDevExcludesOptional(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	os.WriteFile("pyproject.toml", []byte(`
[project]
name = "test-project"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["certifi>=2023.0.0"]

[project.optional-dependencies]
dev = ["six>=1.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa lock failed: %v", err)
	}

	cmd = newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"install", "--no-dev", "--no-root"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa install --no-dev failed: %v", err)
	}

	siteMatches, _ := filepath.Glob(filepath.Join(dir, ".venv", "lib", "python*", "site-packages"))
	if len(siteMatches) != 1 {
		t.Fatalf("expected exactly one site-packages dir, got %v", siteMatches)
	}
	if m, _ := filepath.Glob(filepath.Join(siteMatches[0], "certifi-*.dist-info")); len(m) == 0 {
		t.Error("should install certifi (main group)")
	}
	if m, _ := filepath.Glob(filepath.Join(siteMatches[0], "six-*.dist-info")); len(m) > 0 {
		t.Error("should NOT install six with --no-dev (it's in [project.optional-dependencies].dev)")
	}
}
