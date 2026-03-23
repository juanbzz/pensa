//go:build integration

package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestGroupsIntegration_AddToDev(t *testing.T) {
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

	// Add pytest to dev group.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"add", "pytest", "-G", "dev"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa add -G dev failed: %v", err)
	}

	// Verify pyproject.toml has dev group.
	data, _ := os.ReadFile("pyproject.toml")
	content := string(data)
	if !strings.Contains(content, "pytest") {
		t.Error("pyproject.toml should contain pytest")
	}

	// Verify lock file has pytest with dev group.
	lockData, _ := os.ReadFile("poetry.lock")
	lockContent := string(lockData)
	if !strings.Contains(lockContent, "pytest") {
		t.Error("poetry.lock should contain pytest")
	}
}

func TestGroupsIntegration_InstallNoDev(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

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

	// Lock first.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa lock failed: %v", err)
	}

	// Verify lock has both packages.
	lockData, _ := os.ReadFile("poetry.lock")
	if !strings.Contains(string(lockData), "certifi") {
		t.Error("lock should contain certifi")
	}
	if !strings.Contains(string(lockData), "six") {
		t.Error("lock should contain six")
	}

	// Install with --no-dev — should only install certifi.
	cmd = newRootCmd()
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"install", "--no-dev", "--no-root"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa install --no-dev failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "certifi") {
		t.Error("should install certifi (main group)")
	}
	// six is in dev group and should not be installed with --no-dev.
	if strings.Contains(out, "six") {
		t.Error("should NOT install six with --no-dev")
	}
}

func TestGroupsIntegration_RemoveFromDev(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

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

	// Lock first.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa lock failed: %v", err)
	}

	// Remove six from dev group.
	cmd = newRootCmd()
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"remove", "six", "-G", "dev"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa remove -G dev failed: %v", err)
	}

	// Verify pyproject.toml no longer has six.
	data, _ := os.ReadFile("pyproject.toml")
	if strings.Contains(string(data), "six") {
		t.Error("pyproject.toml should not contain six after removal")
	}
}
