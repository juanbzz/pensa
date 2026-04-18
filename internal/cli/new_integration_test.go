//go:build integration

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewIntegration_ProjectIsBuildable(t *testing.T) {
	dir := t.TempDir()

	// Create a new project.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"new", filepath.Join(dir, "testpkg")})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa new failed: %v", err)
	}

	projectDir := filepath.Join(dir, "testpkg")
	chdir(t, projectDir)

	// Build should succeed — proper package structure exists.
	cmd = newRootCmd()
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"build"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa build failed on new project: %v", err)
	}

	// Verify dist/ has artifacts.
	distDir := filepath.Join(projectDir, "dist")
	entries, err := os.ReadDir(distDir)
	if err != nil {
		t.Fatalf("dist/ not created: %v", err)
	}

	hasWheel := false
	hasSdist := false
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".whl") {
			hasWheel = true
		}
		if strings.HasSuffix(e.Name(), ".tar.gz") {
			hasSdist = true
		}
	}

	if !hasWheel {
		t.Error("build should produce a wheel")
	}
	if !hasSdist {
		t.Error("build should produce an sdist")
	}
}

func TestNewIntegration_ProjectIsInstallable(t *testing.T) {
	dir := t.TempDir()

	// Create a new project and add a dep so lock file is created.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"new", filepath.Join(dir, "mypkg")})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa new failed: %v", err)
	}

	projectDir := filepath.Join(dir, "mypkg")
	chdir(t, projectDir)

	// Add a dep to create lock file.
	cmd = newRootCmd()
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"add", "six"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa add failed: %v", err)
	}

	// Install again — should succeed with editable install.
	cmd = newRootCmd()
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"install"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pensa install failed on new project: %v", err)
	}

	out := buf.String()
	// Should not warn on a properly structured project.
	if strings.Contains(out, "Warning") {
		t.Errorf("install should not warn on a properly structured project: %s", out)
	}

	// Effect: the new project should be installed editable — look for the
	// editable-impl .pth or project dist-info in site-packages.
	siteMatches, _ := filepath.Glob(filepath.Join(projectDir, ".venv", "lib", "python*", "site-packages"))
	if len(siteMatches) != 1 {
		t.Fatalf("expected exactly one site-packages dir, got %v", siteMatches)
	}
	if m, _ := filepath.Glob(filepath.Join(siteMatches[0], "mypkg-*.dist-info")); len(m) == 0 {
		t.Errorf("expected mypkg to be editable-installed; install output:\n%s", out)
	}
}
