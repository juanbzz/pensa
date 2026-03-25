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

	"github.com/matryer/is"
	"github.com/testcontainers/testcontainers-go"
)

func buildPensa(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "pensa")

	// Cross-compile for linux. Detect host arch for matching container arch.
	goarch := "amd64"
	if runtime.GOARCH == "arm64" {
		goarch = "arm64"
	}

	// Find the module root (where go.mod is).
	modRoot := findModuleRoot(t)
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/pensa")
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+goarch, "CGO_ENABLED=0")
	cmd.Dir = modRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build pensa: %s\n%s", err, out)
	}
	return bin
}

func setupPensaContainer(t *testing.T) testcontainers.Container {
	t.Helper()
	ctx := context.Background()

	pensaBin := buildPensa(t)

	container, err := testcontainers.Run(ctx, "python:3.11-slim",
		testcontainers.WithCmd("sleep", "infinity"),
	)
	if err != nil {
		t.Fatalf("start container: %v", err)
	}
	t.Cleanup(func() { testcontainers.CleanupContainer(t, container) })

	execInContainer(t, container, "mkdir", "-p", "/work")

	if err := container.CopyFileToContainer(ctx, pensaBin, "/usr/local/bin/pensa", 0755); err != nil {
		t.Fatalf("copy pensa to container: %v", err)
	}

	execInContainer(t, container, "pensa", "version")

	return container
}

func setupContainer(t *testing.T) testcontainers.Container {
	t.Helper()
	ctx := context.Background()

	pensaBin := buildPensa(t)

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

	// Copy pensa binary into container.
	if err := container.CopyFileToContainer(ctx, pensaBin, "/usr/local/bin/pensa", 0755); err != nil {
		t.Fatalf("copy pensa to container: %v", err)
	}

	// Verify both tools work.
	execInContainer(t, container, "poetry", "--version")
	execInContainer(t, container, "pensa", "version")

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

func TestDropIn_PoetryInitPensaLock(t *testing.T) {
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

	// Pensa locks the same project.
	pensaOutput := execInDir(t, container, "/work", "pensa lock")
	t.Logf("Pensa output: %s", pensaOutput)

	// Verify pensa created a lock file.
	pensaLock := execInDir(t, container, "/work", "cat pensa.lock")
	if !strings.Contains(pensaLock, "requests") {
		t.Fatal("Pensa lock file missing requests")
	}
	pensaPackages := extractPackageNames(pensaLock)
	t.Logf("Pensa resolved: %v", pensaPackages)

	// Compare: pensa should resolve the same packages as Poetry.
	for _, pkg := range poetryPackages {
		if !sliceContains(pensaPackages, pkg) {
			t.Errorf("pensa missing package %q that Poetry resolved", pkg)
		}
	}

	// Verify pensa printed a resolution summary.
	if !strings.Contains(pensaOutput, "Resolved") {
		t.Error("pensa lock didn't print resolution summary")
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

	// Pensa locks it.
	pensaOutput := execInDir(t, container, "/work", "pensa lock")
	t.Logf("Pensa output: %s", pensaOutput)

	// Verify lock file contains certifi.
	lockContent := execInDir(t, container, "/work", "cat pensa.lock")
	if !strings.Contains(lockContent, `name = "certifi"`) {
		t.Error("lock file missing certifi")
	}
	if !strings.Contains(lockContent, "[metadata]") {
		t.Error("lock file missing metadata section")
	}
}

func TestDropIn_PoetryAddThenPensaAdd(t *testing.T) {
	container := setupContainer(t)

	// Poetry creates a project and adds requests.
	execInDir(t, container, "/work",
		"poetry init -n --name test-project --python '>=3.10'")
	execInDir(t, container, "/work", "poetry add requests")

	// Verify Poetry state.
	poetryPyproject := execInDir(t, container, "/work", "cat pyproject.toml")
	t.Logf("Poetry pyproject.toml:\n%s", poetryPyproject)
	if !strings.Contains(poetryPyproject, "requests") {
		t.Fatal("Poetry pyproject.toml missing requests")
	}
	poetryLock := execInDir(t, container, "/work", "cat poetry.lock")
	poetryPackagesBefore := extractPackageNames(poetryLock)
	t.Logf("Poetry packages before pensa add: %v", poetryPackagesBefore)

	// Pensa adds httpx to the same project.
	pensaOutput := execInDir(t, container, "/work", "pensa add httpx")
	t.Logf("Pensa add output: %s", pensaOutput)

	// Verify pyproject.toml was updated with httpx.
	updatedPyproject := execInDir(t, container, "/work", "cat pyproject.toml")
	if !strings.Contains(updatedPyproject, "httpx") {
		t.Error("pyproject.toml missing httpx after pensa add")
	}
	// requests should still be there.
	if !strings.Contains(updatedPyproject, "requests") {
		t.Error("pyproject.toml lost requests after pensa add")
	}

	// Verify poetry.lock was updated.
	pensaLock := execInDir(t, container, "/work", "cat pensa.lock")
	pensaPackages := extractPackageNames(pensaLock)
	t.Logf("Packages after pensa add httpx: %v", pensaPackages)

	// Should have both requests and httpx (plus their transitive deps).
	if !sliceContains(pensaPackages, "requests") {
		t.Error("lock file missing requests")
	}
	if !sliceContains(pensaPackages, "httpx") {
		t.Error("lock file missing httpx")
	}

	// Verify pensa output mentions resolution.
	if !strings.Contains(pensaOutput, "Resolved") {
		t.Error("pensa add didn't print resolution summary")
	}
}

func TestRun_AutoSync(t *testing.T) {
	assert := is.New(t)
	container := setupPensaContainer(t)

	// Create a minimal project with a dependency.
	execInContainer(t, container, "sh", "-c", `cat > /work/pyproject.toml << 'EOF'
[project]
name = "testproject"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["six"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
EOF`)

	// Lock the project.
	execInDir(t, container, "/work", "pensa lock")

	// Run without a venv — auto-sync should create it and install.
	output := execInDir(t, container, "/work", "pensa run python -c 'import six; print(six.__version__)' 2>&1")
	t.Logf("First run output:\n%s", output)

	assert.True(strings.Contains(output, "syncing venv"))
	assert.True(strings.Contains(output, "1."))

	// Run again — should skip sync.
	output2 := execInDir(t, container, "/work", "pensa run python -c 'import six; print(six.__version__)' 2>&1")
	t.Logf("Second run output:\n%s", output2)

	assert.True(!strings.Contains(output2, "syncing venv"))

	// Run with --no-sync.
	output3 := execInDir(t, container, "/work", "pensa run --no-sync python -c 'print(\"skipped\")' 2>&1")
	t.Logf("--no-sync output:\n%s", output3)

	assert.True(!strings.Contains(output3, "syncing venv"))
	assert.True(strings.Contains(output3, "skipped"))
}
