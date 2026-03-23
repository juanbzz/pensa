//go:build integration

package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuild_Integration(t *testing.T) {
	dir := t.TempDir()

	// Create a minimal Python project with hatchling backend.
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "testpkg"
version = "0.1.0"

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)

	// Create the package.
	pkgDir := filepath.Join(dir, "testpkg")
	os.MkdirAll(pkgDir, 0755)
	os.WriteFile(filepath.Join(pkgDir, "__init__.py"), []byte("# testpkg\n"), 0644)

	outputDir := filepath.Join(dir, "dist")

	result, err := Build(Options{
		ProjectDir: dir,
		OutputDir:  outputDir,
		Wheel:      true,
		Sdist:      true,
	})
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	if len(result.Files) != 2 {
		t.Fatalf("expected 2 artifacts, got %d: %v", len(result.Files), result.Files)
	}

	hasWheel := false
	hasSdist := false
	for _, f := range result.Files {
		base := filepath.Base(f)
		if strings.HasSuffix(base, ".whl") {
			hasWheel = true
		}
		if strings.HasSuffix(base, ".tar.gz") {
			hasSdist = true
		}
		// Verify file actually exists.
		if _, err := os.Stat(f); err != nil {
			t.Errorf("artifact %s doesn't exist: %v", base, err)
		}
	}

	if !hasWheel {
		t.Error("missing wheel artifact")
	}
	if !hasSdist {
		t.Error("missing sdist artifact")
	}
}

func TestBuild_WheelOnly(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "testpkg"
version = "0.1.0"

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)

	pkgDir := filepath.Join(dir, "testpkg")
	os.MkdirAll(pkgDir, 0755)
	os.WriteFile(filepath.Join(pkgDir, "__init__.py"), []byte(""), 0644)

	result, err := Build(Options{
		ProjectDir: dir,
		OutputDir:  filepath.Join(dir, "dist"),
		Wheel:      true,
		Sdist:      false,
	})
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	if len(result.Files) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(result.Files))
	}
	if !strings.HasSuffix(filepath.Base(result.Files[0]), ".whl") {
		t.Errorf("expected wheel, got %s", result.Files[0])
	}
}

func TestBuild_Editable(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "testpkg"
version = "0.1.0"

[project.scripts]
testpkg = "testpkg.__main__:main"

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)

	pkgDir := filepath.Join(dir, "testpkg")
	os.MkdirAll(pkgDir, 0755)
	os.WriteFile(filepath.Join(pkgDir, "__init__.py"), []byte(""), 0644)
	os.WriteFile(filepath.Join(pkgDir, "__main__.py"), []byte("def main(): print('hello')\n"), 0644)

	result, err := Build(Options{
		ProjectDir: dir,
		OutputDir:  filepath.Join(dir, "dist"),
		Editable:   true,
	})
	if err != nil {
		t.Fatalf("editable build failed: %v", err)
	}

	if len(result.Files) != 1 {
		t.Fatalf("expected 1 editable wheel, got %d", len(result.Files))
	}
	if !strings.HasSuffix(filepath.Base(result.Files[0]), ".whl") {
		t.Errorf("expected .whl, got %s", result.Files[0])
	}

	// Verify file exists.
	if _, err := os.Stat(result.Files[0]); err != nil {
		t.Errorf("editable wheel doesn't exist: %v", err)
	}
}

func TestBuild_EditableNoPackage(t *testing.T) {
	dir := t.TempDir()

	// Project with build-system but no actual package directory.
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "testpkg"
version = "0.1.0"

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)

	// No testpkg/ directory — editable build should fail.
	_, err := Build(Options{
		ProjectDir: dir,
		OutputDir:  filepath.Join(dir, "dist"),
		Editable:   true,
	})
	if err == nil {
		t.Fatal("expected error when no package directory exists for editable build")
	}
}

func TestBuild_NoBuildSystem(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "testpkg"
version = "0.1.0"
`), 0644)

	_, err := Build(Options{
		ProjectDir: dir,
		OutputDir:  filepath.Join(dir, "dist"),
		Wheel:      true,
		Sdist:      true,
	})
	if err == nil {
		t.Fatal("expected error when no build-system")
	}
}
