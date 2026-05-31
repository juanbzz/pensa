//go:build integration

package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"

	"pensa.sh/pensa/internal/python"
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

// goetry-wzt: setuptools.build_meta's get_requires_for_build_editable
// runs egg_info as a side effect and writes "running egg_info\nwriting
// .../PKG-INFO\n..." to stdout BEFORE the script's print(json.dumps(...))
// of the actual dep list. The captured stdout must not leak that chatter
// back to the caller as a "dep" — the symptom was `pip install` choking
// with `ERROR: Invalid requirement: "running egg_info\n..."`.
func TestGetEditableBuildDeps_SetuptoolsNoEggInfoLeak(t *testing.T) {
	assert := is.New(t)
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "egg_info_test"
version = "0.1.0"

[build-system]
requires = ["setuptools>=64", "wheel"]
build-backend = "setuptools.build_meta"

[tool.setuptools.packages.find]
where = ["."]
`), 0644)
	pkgDir := filepath.Join(dir, "egg_info_test")
	assert.NoErr(os.MkdirAll(pkgDir, 0755))
	assert.NoErr(os.WriteFile(filepath.Join(pkgDir, "__init__.py"), []byte(""), 0644))

	py, err := python.Discover()
	assert.NoErr(err)

	buildVenv := filepath.Join(dir, "buildenv")
	venvPython, err := createVenv(py.Path, buildVenv)
	assert.NoErr(err)
	assert.NoErr(installDeps(venvPython, []string{"setuptools>=64", "wheel"}))

	deps, err := getEditableBuildDeps(venvPython, dir, "setuptools.build_meta", "")
	assert.NoErr(err)

	// A PEP 508 dep is a single token (name + optional extras + version
	// spec). Whitespace inside a returned entry means setuptools'
	// egg_info chatter — written to stdout during the hook call —
	// leaked through the parser. Likewise any chatter-prefix.
	for _, d := range deps {
		if strings.ContainsAny(d, "\n\t ") {
			t.Fatalf("dep contains whitespace (egg_info stdout leaked): %q", d)
		}
		for _, prefix := range []string{"running ", "writing ", "reading "} {
			if strings.HasPrefix(d, prefix) {
				t.Fatalf("got egg_info chatter as a dep: %q", d)
			}
		}
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

func TestBuildFromSdist_Integration(t *testing.T) {
	assert := is.New(t)
	dir := t.TempDir()

	// Create a minimal Python project.
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

	// Build an sdist from the project.
	sdistDir := filepath.Join(dir, "sdist-out")
	sdistResult, err := Build(Options{
		ProjectDir: dir,
		OutputDir:  sdistDir,
		Sdist:      true,
	})
	assert.NoErr(err)
	assert.Equal(len(sdistResult.Files), 1)

	// Build a wheel from that sdist.
	py, err := python.Discover()
	assert.NoErr(err)

	wheelDir := filepath.Join(dir, "wheel-out")
	wheelPath, err := BuildFromSdist(SdistBuildOptions{
		Name:      "testpkg",
		Version:   "0.1.0",
		SdistPath: sdistResult.Files[0],
		OutputDir: wheelDir,
		Python:    py,
	})
	assert.NoErr(err)
	assert.True(strings.HasSuffix(filepath.Base(wheelPath), ".whl"))

	_, err = os.Stat(wheelPath)
	assert.NoErr(err)
}

// goetry-c7c bug 2: when an sdist's pyproject declares only
// "setuptools" in build-system.requires (no "wheel"), the build env
// must still get the wheel package — setuptools < 70.1 needs it for
// bdist_wheel. Pin <70 to remove the modern-setuptools confound and
// fail deterministically without the fix.
func TestBuildFromSdist_SetuptoolsWithoutWheelInRequires(t *testing.T) {
	assert := is.New(t)
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "testsupkg"
version = "0.1.0"

[build-system]
requires = ["setuptools>=40.8.0,<70"]
build-backend = "setuptools.build_meta"

[tool.setuptools.packages.find]
where = ["."]
`), 0644)
	pkgDir := filepath.Join(dir, "testsupkg")
	assert.NoErr(os.MkdirAll(pkgDir, 0755))
	assert.NoErr(os.WriteFile(filepath.Join(pkgDir, "__init__.py"), []byte(""), 0644))

	sdistDir := filepath.Join(dir, "sdist-out")
	sdistResult, err := Build(Options{
		ProjectDir: dir,
		OutputDir:  sdistDir,
		Sdist:      true,
	})
	assert.NoErr(err)
	assert.Equal(len(sdistResult.Files), 1)

	py, err := python.Discover()
	assert.NoErr(err)

	wheelDir := filepath.Join(dir, "wheel-out")
	wheelPath, err := BuildFromSdist(SdistBuildOptions{
		Name:      "testsupkg",
		Version:   "0.1.0",
		SdistPath: sdistResult.Files[0],
		OutputDir: wheelDir,
		Python:    py,
	})
	assert.NoErr(err) // without the fix: "invalid command 'bdist_wheel'"
	assert.True(strings.HasSuffix(filepath.Base(wheelPath), ".whl"))
	_, err = os.Stat(wheelPath)
	assert.NoErr(err)
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
