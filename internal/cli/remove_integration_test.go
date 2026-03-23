//go:build integration

package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRemoveIntegration(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	// Start with requests + certifi as direct deps.
	os.WriteFile("pyproject.toml", []byte(`
[tool.poetry]
name = "test-project"
version = "0.1.0"

[tool.poetry.dependencies]
python = "^3.8"
requests = "^2.28"
certifi = ">=2023.0.0"

[build-system]
requires = ["poetry-core"]
build-backend = "poetry.core.masonry.api"
`), 0644)

	// Lock first.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa lock failed: %v", err)
	}

	// Remove requests.
	cmd = newRootCmd()
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"remove", "requests"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa remove failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Removing requests") {
		t.Errorf("expected 'Removing requests' in output, got: %s", out)
	}

	// Verify pyproject.toml no longer has requests.
	data, err := os.ReadFile("pyproject.toml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "requests") {
		t.Error("pyproject.toml should not contain requests after removal")
	}
	// certifi should still be there.
	if !strings.Contains(string(data), "certifi") {
		t.Error("pyproject.toml should still contain certifi")
	}

	// poetry.lock should still exist (certifi remains).
	if _, err := os.Stat("poetry.lock"); err != nil {
		t.Error("poetry.lock should still exist when deps remain")
	}
}

func TestRemoveIntegration_LastDep(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	os.WriteFile("pyproject.toml", []byte(`
[tool.poetry]
name = "test-project"
version = "0.1.0"

[tool.poetry.dependencies]
python = "^3.8"
certifi = ">=2023.0.0"

[build-system]
requires = ["poetry-core"]
build-backend = "poetry.core.masonry.api"
`), 0644)

	// Lock first.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa lock failed: %v", err)
	}

	// Remove the last dep.
	cmd = newRootCmd()
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"remove", "certifi"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa remove failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "No dependencies remaining") {
		t.Errorf("expected 'No dependencies remaining', got: %s", out)
	}

	// poetry.lock should be removed.
	if _, err := os.Stat("poetry.lock"); err == nil {
		t.Error("poetry.lock should be removed when no deps remain")
	}
}

func TestRemoveIntegration_NotFound(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	os.WriteFile("pyproject.toml", []byte(`
[tool.poetry]
name = "test-project"
version = "0.1.0"

[tool.poetry.dependencies]
python = "^3.8"
certifi = ">=2023.0.0"

[build-system]
requires = ["poetry-core"]
build-backend = "poetry.core.masonry.api"
`), 0644)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"remove", "nonexistent"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent package")
	}
	if !strings.Contains(err.Error(), "is not a dependency") {
		t.Errorf("error should mention 'is not a dependency', got: %s", err.Error())
	}
}
