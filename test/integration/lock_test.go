//go:build integration

package integration

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/testcontainers/testcontainers-go"
)

func buildGoetry(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "goetry")

	// Cross-compile for linux. Detect host arch for matching container arch.
	goarch := "amd64"
	if runtime.GOARCH == "arm64" {
		goarch = "arm64"
	}

	// Find the module root (where go.mod is).
	modRoot := findModuleRoot(t)
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/goetry")
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+goarch, "CGO_ENABLED=0")
	cmd.Dir = modRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build goetry: %s\n%s", err, out)
	}
	return bin
}

func setupContainer(t *testing.T) testcontainers.Container {
	t.Helper()
	ctx := context.Background()

	goetryBin := buildGoetry(t)

	container, err := testcontainers.Run(ctx, "python:3.11-slim",
		testcontainers.WithCmd("sleep", "infinity"),
	)
	if err != nil {
		t.Fatalf("start container: %v", err)
	}
	t.Cleanup(func() { testcontainers.CleanupContainer(t, container) })

	// Install Poetry inside the container.
	execInContainer(t, container, "pip", "install", "poetry")

	// Create work directory.
	execInContainer(t, container, "mkdir", "-p", "/work")

	// Copy goetry binary into container.
	if err := container.CopyFileToContainer(ctx, goetryBin, "/usr/local/bin/goetry", 0755); err != nil {
		t.Fatalf("copy goetry to container: %v", err)
	}

	// Verify both tools work.
	execInContainer(t, container, "poetry", "--version")
	execInContainer(t, container, "goetry", "version")

	return container
}

func execInContainer(t *testing.T, container testcontainers.Container, cmd ...string) string {
	t.Helper()
	ctx := context.Background()
	exitCode, reader, err := container.Exec(ctx, cmd)
	if err != nil {
		t.Fatalf("exec %v: %v", cmd, err)
	}
	output, _ := io.ReadAll(reader)
	if exitCode != 0 {
		t.Fatalf("exec %v exited %d:\n%s", cmd, exitCode, output)
	}
	return string(output)
}

func execInDir(t *testing.T, container testcontainers.Container, dir string, cmdStr string) string {
	t.Helper()
	return execInContainer(t, container, "sh", "-c", "cd "+dir+" && "+cmdStr)
}

func extractPackageNames(lockContent string) []string {
	re := regexp.MustCompile(`(?m)^name = "([^"]+)"`)
	matches := re.FindAllStringSubmatch(lockContent, -1)
	var names []string
	for _, m := range matches {
		names = append(names, m[1])
	}
	return names
}

func sliceContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod")
		}
		dir = parent
	}
}

// --- Tests ---

func TestDropIn_PoetryInitGoetryLock(t *testing.T) {
	container := setupContainer(t)

	// Poetry creates a project with requests.
	execInDir(t, container, "/work",
		"poetry init -n --name test-project --dependency requests")
	execInDir(t, container, "/work", "poetry lock")

	// Verify Poetry created a lock file.
	poetryLock := execInDir(t, container, "/work", "cat poetry.lock")
	if !strings.Contains(poetryLock, "requests") {
		t.Fatal("Poetry lock file missing requests")
	}
	poetryPackages := extractPackageNames(poetryLock)
	t.Logf("Poetry resolved: %v", poetryPackages)

	// Remove Poetry's lock file, keep pyproject.toml.
	execInDir(t, container, "/work", "rm poetry.lock")

	// Goetry locks the same project.
	goetryOutput := execInDir(t, container, "/work", "goetry lock")
	t.Logf("Goetry output: %s", goetryOutput)

	// Verify goetry created a lock file.
	goetryLock := execInDir(t, container, "/work", "cat poetry.lock")
	if !strings.Contains(goetryLock, "requests") {
		t.Fatal("Goetry lock file missing requests")
	}
	goetryPackages := extractPackageNames(goetryLock)
	t.Logf("Goetry resolved: %v", goetryPackages)

	// Compare: goetry should resolve the same packages as Poetry.
	for _, pkg := range poetryPackages {
		if !sliceContains(goetryPackages, pkg) {
			t.Errorf("goetry missing package %q that Poetry resolved", pkg)
		}
	}

	// Verify goetry printed a resolution summary.
	if !strings.Contains(goetryOutput, "Resolved") {
		t.Error("goetry lock didn't print resolution summary")
	}
}

func TestDropIn_PoetryFormatLock(t *testing.T) {
	container := setupContainer(t)

	// Write a Poetry-format pyproject.toml directly.
	execInContainer(t, container, "sh", "-c", `cat > /work/pyproject.toml << 'PYEOF'
[tool.poetry]
name = "test-project"
version = "0.1.0"

[tool.poetry.dependencies]
python = "^3.8"
certifi = ">=2023.0.0"

[build-system]
requires = ["poetry-core"]
build-backend = "poetry.core.masonry.api"
PYEOF`)

	// Goetry locks it.
	goetryOutput := execInDir(t, container, "/work", "goetry lock")
	t.Logf("Goetry output: %s", goetryOutput)

	// Verify lock file contains certifi.
	lockContent := execInDir(t, container, "/work", "cat poetry.lock")
	if !strings.Contains(lockContent, `name = "certifi"`) {
		t.Error("lock file missing certifi")
	}
	if !strings.Contains(lockContent, "[metadata]") {
		t.Error("lock file missing metadata section")
	}
}

func TestDropIn_PoetryAddThenGoetryAdd(t *testing.T) {
	container := setupContainer(t)

	// Poetry creates a project and adds requests.
	execInDir(t, container, "/work",
		"poetry init -n --name test-project --python '>=3.9'")
	execInDir(t, container, "/work", "poetry add requests")

	// Verify Poetry state.
	poetryPyproject := execInDir(t, container, "/work", "cat pyproject.toml")
	t.Logf("Poetry pyproject.toml:\n%s", poetryPyproject)
	if !strings.Contains(poetryPyproject, "requests") {
		t.Fatal("Poetry pyproject.toml missing requests")
	}
	poetryLock := execInDir(t, container, "/work", "cat poetry.lock")
	poetryPackagesBefore := extractPackageNames(poetryLock)
	t.Logf("Poetry packages before goetry add: %v", poetryPackagesBefore)

	// Goetry adds httpx to the same project.
	goetryOutput := execInDir(t, container, "/work", "goetry add httpx")
	t.Logf("Goetry add output: %s", goetryOutput)

	// Verify pyproject.toml was updated with httpx.
	updatedPyproject := execInDir(t, container, "/work", "cat pyproject.toml")
	if !strings.Contains(updatedPyproject, "httpx") {
		t.Error("pyproject.toml missing httpx after goetry add")
	}
	// requests should still be there.
	if !strings.Contains(updatedPyproject, "requests") {
		t.Error("pyproject.toml lost requests after goetry add")
	}

	// Verify poetry.lock was updated.
	goetryLock := execInDir(t, container, "/work", "cat poetry.lock")
	goetryPackages := extractPackageNames(goetryLock)
	t.Logf("Packages after goetry add httpx: %v", goetryPackages)

	// Should have both requests and httpx (plus their transitive deps).
	if !sliceContains(goetryPackages, "requests") {
		t.Error("lock file missing requests")
	}
	if !sliceContains(goetryPackages, "httpx") {
		t.Error("lock file missing httpx")
	}

	// Verify goetry output mentions resolution.
	if !strings.Contains(goetryOutput, "Resolved") {
		t.Error("goetry add didn't print resolution summary")
	}
}
