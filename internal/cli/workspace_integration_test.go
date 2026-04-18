//go:build integration

package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writeFile := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	writeFile(filepath.Join(dir, "pyproject.toml"), `
[project]
name = "test-workspace"
version = "0.1.0"
requires-python = ">=3.10"

[tool.pensa.workspace]
members = ["apps/api", "apps/worker"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`)

	apiDir := filepath.Join(dir, "apps", "api")
	writeFile(filepath.Join(apiDir, "pyproject.toml"), `
[project]
name = "test-api"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["six>=1.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`)
	writeFile(filepath.Join(apiDir, "test_api", "__init__.py"), "")

	workerDir := filepath.Join(dir, "apps", "worker")
	writeFile(filepath.Join(workerDir, "pyproject.toml"), `
[project]
name = "test-worker"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["certifi>=2023.0.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`)
	writeFile(filepath.Join(workerDir, "test_worker", "__init__.py"), "")

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

// TestWorkspaceIntegration_InstallHonorsVenvPythonVersion is a regression test
// for a real-world bug discovered in a production uv-built repo.
//
// THE BUG
//   `pensa install` called python.Discover() to pick a Python interpreter off
//   PATH, then reused that PythonInfo's Major/Minor for every path calculation
//   — including SitePackagesDir(venv). When the venv had been created by a
//   DIFFERENT Python (e.g. uv pinned 3.12 while the host's python3 on PATH was
//   3.10), pensa computed site-packages as .venv/lib/python3.10/site-packages
//   while the venv itself ran from .venv/lib/python3.12/site-packages.
//
//   Editables got written to the phantom 3.10 directory. No error, no warning.
//   `pensa run python -c "import <member>"` then failed with ModuleNotFoundError
//   because .venv/bin/python (=3.12) only loads from python3.12/site-packages.
//   In a workspace this silently broke every `--package <name>` invocation.
//
// WHY THIS TEST EXISTS
//   The happy-path install test (TestWorkspaceIntegration_Install) didn't catch
//   this because pensa CREATES the venv from the same host Python it discovers,
//   so host==venv and the paths agree. The bug only surfaces when the venv
//   pre-exists AND was built with a different Python than host — the exact
//   shape of any project migrating off uv or pyenv.
//
//   We reproduce the mismatch deterministically: let pensa create the venv
//   normally, then rewrite pyvenv.cfg to claim a fake Python minor (+5 so it
//   can't collide with the host). A correctly-fixed pensa reads pyvenv.cfg and
//   writes editables to .venv/lib/pythonX.{Y+5}/site-packages. A buggy pensa
//   still trusts python.Discover() and writes to the host's dir.
//
//   The fix belongs in internal/python (a FromVenv helper) and in install.go
//   (use it instead of Discover when a venv already exists). Do NOT "fix" this
//   test by aligning the dirs — the whole point is that pyvenv.cfg is the
//   source of truth, not host PATH.
func TestWorkspaceIntegration_InstallHonorsVenvPythonVersion(t *testing.T) {
	dir := setupWorkspace(t)
	chdir(t, dir)

	runPensa := func(args ...string) string {
		cmd := newRootCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("pensa %s failed: %v\n%s", strings.Join(args, " "), err, buf.String())
		}
		return buf.String()
	}

	runPensa("lock")
	runPensa("install")

	// Find the real site-packages pensa created (matches host Python).
	hostSiteMatches, _ := filepath.Glob(filepath.Join(dir, ".venv", "lib", "python*", "site-packages"))
	if len(hostSiteMatches) != 1 {
		t.Fatalf("expected exactly one site-packages dir after first install, got %v", hostSiteMatches)
	}
	hostSite := hostSiteMatches[0]
	hostLibDir := filepath.Dir(hostSite) // .venv/lib/pythonX.Y

	// Pick a Python minor version that deliberately doesn't match the host's, so
	// pyvenv.cfg and host-Discover disagree. Hopping +5 minors is safe: no real
	// host has the result on PATH.
	base := filepath.Base(hostLibDir) // "pythonX.Y"
	var hostMajor, hostMinor int
	if _, err := fmt.Sscanf(base, "python%d.%d", &hostMajor, &hostMinor); err != nil {
		t.Fatalf("parse %s: %v", base, err)
	}
	fakeMinor := hostMinor + 5
	fakeDir := filepath.Join(dir, ".venv", "lib", fmt.Sprintf("python%d.%d", hostMajor, fakeMinor))
	fakeSite := filepath.Join(fakeDir, "site-packages")
	if err := os.MkdirAll(fakeSite, 0755); err != nil {
		t.Fatal(err)
	}

	// Rewrite pyvenv.cfg so the venv "claims" the fake version.
	cfg := fmt.Sprintf("home = %s\nversion_info = %d.%d.0\n",
		filepath.Dir(filepath.Join(dir, ".venv", "bin")), hostMajor, fakeMinor)
	if err := os.WriteFile(filepath.Join(dir, ".venv", "pyvenv.cfg"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	// Wipe editables from the host-version site-packages so we can tell where the
	// next install lands.
	for _, name := range []string{"test_api", "test_worker"} {
		for _, g := range []string{name + "-*.dist-info", "_" + name + "*.pth", "_editable_impl_" + name + "*.pth"} {
			matches, _ := filepath.Glob(filepath.Join(hostSite, g))
			for _, m := range matches {
				os.RemoveAll(m)
			}
		}
	}

	runPensa("install")

	for _, name := range []string{"test_api", "test_worker"} {
		fakeDI, _ := filepath.Glob(filepath.Join(fakeSite, name+"-*.dist-info"))
		hostDI, _ := filepath.Glob(filepath.Join(hostSite, name+"-*.dist-info"))
		if len(fakeDI) == 0 {
			t.Errorf("%s: editable dist-info missing from site-packages dictated by pyvenv.cfg (%s); host-dir holds: %v",
				name, fakeSite, hostDI)
		}
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
