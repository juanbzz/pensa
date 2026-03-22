//go:build integration

package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}

func TestLockIntegration_SingleDep(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	os.WriteFile("pyproject.toml", []byte(`
[project]
name = "test-project"
version = "0.1.0"
requires-python = ">=3.8"
dependencies = [
    "certifi>=2023.0.0",
]

[build-system]
requires = ["poetry-core"]
build-backend = "poetry.core.masonry.api"
`), 0644)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("goetry lock failed: %v", err)
	}

	// Verify poetry.lock was created.
	data, err := os.ReadFile("poetry.lock")
	if err != nil {
		t.Fatal("poetry.lock not created")
	}
	content := string(data)

	// Verify structure.
	if !strings.Contains(content, `name = "certifi"`) {
		t.Error("poetry.lock missing certifi")
	}
	if !strings.Contains(content, "[metadata]") {
		t.Error("poetry.lock missing metadata")
	}
	if !strings.Contains(content, `lock-version = "2.1"`) {
		t.Error("poetry.lock missing lock-version")
	}
	if !strings.Contains(content, `python-versions = ">=3.8"`) {
		t.Error("poetry.lock missing python-versions")
	}
	if !strings.Contains(content, "[[package]]") {
		t.Error("poetry.lock missing [[package]]")
	}

	// Verify output message.
	output := buf.String()
	if !strings.Contains(output, "Resolved 1 packages") {
		t.Errorf("unexpected output: %s", output)
	}
}

func TestLockIntegration_TransitiveDeps(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	os.WriteFile("pyproject.toml", []byte(`
[project]
name = "test-project"
version = "0.1.0"
requires-python = ">=3.8"
dependencies = [
    "requests>=2.28.0,<3.0.0",
]

[build-system]
requires = ["poetry-core"]
build-backend = "poetry.core.masonry.api"
`), 0644)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("goetry lock failed: %v", err)
	}

	data, err := os.ReadFile("poetry.lock")
	if err != nil {
		t.Fatal("poetry.lock not created")
	}
	content := string(data)

	// requests has 4 transitive deps: certifi, charset-normalizer, idna, urllib3
	expectedPkgs := []string{"certifi", "charset-normalizer", "idna", "requests", "urllib3"}
	for _, pkg := range expectedPkgs {
		if !strings.Contains(content, `name = "`+pkg+`"`) {
			t.Errorf("poetry.lock missing package %q", pkg)
		}
	}

	// Verify requests has dependencies listed.
	if !strings.Contains(content, "[package.dependencies]") {
		t.Error("poetry.lock missing [package.dependencies] section")
	}

	// Verify output message.
	output := buf.String()
	if !strings.Contains(output, "Resolved 5 packages") {
		t.Errorf("unexpected output: %s", output)
	}
}

func TestLockIntegration_PoetryFormat(t *testing.T) {
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
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("goetry lock with Poetry format failed: %v", err)
	}

	data, err := os.ReadFile("poetry.lock")
	if err != nil {
		t.Fatal("poetry.lock not created")
	}
	content := string(data)

	if !strings.Contains(content, `name = "certifi"`) {
		t.Error("poetry.lock missing certifi")
	}
}

func TestLockIntegration_NoDeps(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	os.WriteFile("pyproject.toml", []byte(`
[project]
name = "test-project"
version = "0.1.0"

[build-system]
requires = ["poetry-core"]
build-backend = "poetry.core.masonry.api"
`), 0644)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("goetry lock with no deps failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No dependencies to lock") {
		t.Errorf("expected 'No dependencies' message, got: %s", output)
	}

	// No poetry.lock should be created.
	if _, err := os.Stat("poetry.lock"); err == nil {
		t.Error("poetry.lock should not be created when there are no dependencies")
	}
}

func TestLockIntegration_NoPyproject(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"lock"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no pyproject.toml exists")
	}
}
