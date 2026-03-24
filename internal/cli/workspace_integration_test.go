//go:build integration

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Root pyproject.toml with workspace config.
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "test-workspace"
version = "0.1.0"
requires-python = ">=3.10"

[tool.pensa.workspace]
members = ["apps/api", "apps/worker"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)

	// API member.
	apiDir := filepath.Join(dir, "apps", "api")
	os.MkdirAll(apiDir, 0755)
	os.WriteFile(filepath.Join(apiDir, "pyproject.toml"), []byte(`
[project]
name = "test-api"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["six>=1.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)
	// Create package dir so editable install works.
	os.MkdirAll(filepath.Join(apiDir, "test_api"), 0755)
	os.WriteFile(filepath.Join(apiDir, "test_api", "__init__.py"), []byte(""), 0644)

	// Worker member.
	workerDir := filepath.Join(dir, "apps", "worker")
	os.MkdirAll(workerDir, 0755)
	os.WriteFile(filepath.Join(workerDir, "pyproject.toml"), []byte(`
[project]
name = "test-worker"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["certifi>=2023.0.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)
	os.MkdirAll(filepath.Join(workerDir, "test_worker"), 0755)
	os.WriteFile(filepath.Join(workerDir, "test_worker", "__init__.py"), []byte(""), 0644)

	return dir
}

func TestWorkspaceIntegration_Lock(t *testing.T) {
	dir := setupWorkspace(t)
	chdir(t, dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa lock in workspace failed: %v", err)
	}

	out := buf.String()

	// Should mention workspace.
	if !strings.Contains(out, "workspace") || !strings.Contains(out, "2 members") {
		t.Errorf("should mention workspace with 2 members: %s", out)
	}

	// Should create pensa.lock at workspace root.
	if _, err := os.Stat(filepath.Join(dir, "pensa.lock")); err != nil {
		t.Error("pensa.lock should be created at workspace root")
	}

	// Lock file should contain both members' deps.
	lockData, _ := os.ReadFile(filepath.Join(dir, "pensa.lock"))
	content := string(lockData)
	if !strings.Contains(content, "six") {
		t.Error("pensa.lock should contain six (from api member)")
	}
	if !strings.Contains(content, "certifi") {
		t.Error("pensa.lock should contain certifi (from worker member)")
	}
}

func TestWorkspaceIntegration_LockFromMemberDir(t *testing.T) {
	dir := setupWorkspace(t)
	// cd into a member directory — should still lock from workspace root.
	chdir(t, filepath.Join(dir, "apps", "api"))

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa lock from member dir failed: %v", err)
	}

	// Lock file should be at workspace root, not in member dir.
	if _, err := os.Stat(filepath.Join(dir, "pensa.lock")); err != nil {
		t.Error("pensa.lock should be at workspace root")
	}
	if _, err := os.Stat(filepath.Join(dir, "apps", "api", "pensa.lock")); err == nil {
		t.Error("pensa.lock should NOT be in member dir")
	}
}

func TestWorkspaceIntegration_Install(t *testing.T) {
	dir := setupWorkspace(t)
	chdir(t, dir)

	// Lock first.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa lock failed: %v", err)
	}

	// Install.
	cmd = newRootCmd()
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"install", "--no-root"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa install in workspace failed: %v", err)
	}

	out := buf.String()

	// Should install deps from both members.
	if !strings.Contains(out, "six") && !strings.Contains(out, "certifi") && !strings.Contains(out, "up to date") {
		t.Errorf("should install deps from both members: %s", out)
	}

	// Venv should be at workspace root.
	if _, err := os.Stat(filepath.Join(dir, ".venv")); err != nil {
		t.Error(".venv should be at workspace root")
	}
}

func TestWorkspaceIntegration_UVFormat(t *testing.T) {
	dir := t.TempDir()

	// Root with uv workspace format.
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "uv-workspace"
version = "0.1.0"
requires-python = ">=3.10"

[tool.uv.workspace]
members = ["lib"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)

	os.MkdirAll(filepath.Join(dir, "lib"), 0755)
	os.WriteFile(filepath.Join(dir, "lib", "pyproject.toml"), []byte(`
[project]
name = "mylib"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["six>=1.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)

	chdir(t, dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa lock with uv workspace failed: %v", err)
	}

	// Should detect uv workspace and create pensa.lock.
	if _, err := os.Stat(filepath.Join(dir, "pensa.lock")); err != nil {
		t.Error("pensa.lock should be created for uv workspace")
	}
}
