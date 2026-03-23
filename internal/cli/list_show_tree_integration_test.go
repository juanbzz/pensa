//go:build integration

package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// setupIntegrationProject creates a pyproject.toml with requests, runs lock,
// and returns the output buffer. The test is left in the temp dir.
func setupIntegrationProject(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	chdir(t, dir)

	os.WriteFile("pyproject.toml", []byte(`
[tool.poetry]
name = "test-project"
version = "0.1.0"

[tool.poetry.dependencies]
python = "^3.8"
requests = "^2.28"

[build-system]
requires = ["poetry-core"]
build-backend = "poetry.core.masonry.api"
`), 0644)

	// Lock to generate poetry.lock with real PyPI data.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa lock failed: %v", err)
	}
}

func TestListIntegration(t *testing.T) {
	setupIntegrationProject(t)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa list failed: %v", err)
	}

	out := buf.String()
	for _, pkg := range []string{"requests", "certifi", "urllib3", "idna", "charset-normalizer"} {
		if !strings.Contains(out, pkg) {
			t.Errorf("list output missing %q", pkg)
		}
	}
}

func TestListIntegration_TopLevel(t *testing.T) {
	setupIntegrationProject(t)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"list", "--top-level"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa list --top-level failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "requests") {
		t.Error("top-level should include requests")
	}
	// certifi is a transitive dep, should not appear.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "certifi") {
			t.Error("top-level should not include transitive dep certifi")
		}
	}
}

func TestShowIntegration(t *testing.T) {
	setupIntegrationProject(t)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"show", "requests"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa show requests failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "requests") {
		t.Error("missing package name")
	}
	if !strings.Contains(out, "version") {
		t.Error("missing version label")
	}
	// requests depends on certifi.
	if !strings.Contains(out, "certifi") {
		t.Error("missing dependency certifi")
	}
}

func TestShowIntegration_NotFound(t *testing.T) {
	setupIntegrationProject(t)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"show", "nonexistent-pkg-xyz"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent package")
	}
}

func TestTreeIntegration(t *testing.T) {
	setupIntegrationProject(t)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"tree"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa tree failed: %v", err)
	}

	out := buf.String()
	// Should have tree characters.
	if !strings.Contains(out, "├──") && !strings.Contains(out, "└──") {
		t.Error("tree output missing box-drawing characters")
	}
	if !strings.Contains(out, "requests") {
		t.Error("tree missing requests")
	}
}

func TestTreeIntegration_SinglePackage(t *testing.T) {
	setupIntegrationProject(t)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"tree", "requests"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa tree requests failed: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if !strings.HasPrefix(lines[0], "requests") {
		t.Errorf("first line should be requests, got: %s", lines[0])
	}
	if !strings.Contains(out, "certifi") {
		t.Error("tree missing dep certifi")
	}
}

func TestTreeIntegration_TopLevel(t *testing.T) {
	setupIntegrationProject(t)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"tree", "--top-level"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa tree --top-level failed: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	// Only requests should be a root line (no indent).
	for _, line := range lines {
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "├") && !strings.HasPrefix(line, "└") && !strings.HasPrefix(line, "│") && !strings.HasPrefix(line, "requests") {
			t.Errorf("unexpected root in top-level tree: %s", line)
		}
	}
}
