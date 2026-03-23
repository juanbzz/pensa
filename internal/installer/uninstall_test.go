package installer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUninstallPackage(t *testing.T) {
	dir := t.TempDir()

	// Create a fake installed package with RECORD.
	distInfo := filepath.Join(dir, "mypkg-1.0.0.dist-info")
	pkgDir := filepath.Join(dir, "mypkg")
	os.MkdirAll(distInfo, 0755)
	os.MkdirAll(pkgDir, 0755)

	os.WriteFile(filepath.Join(pkgDir, "__init__.py"), []byte("# mypkg"), 0644)
	os.WriteFile(filepath.Join(pkgDir, "core.py"), []byte("# core"), 0644)
	os.WriteFile(filepath.Join(distInfo, "METADATA"), []byte("Name: mypkg\nVersion: 1.0.0\n"), 0644)
	os.WriteFile(filepath.Join(distInfo, "RECORD"), []byte(
		"mypkg/__init__.py,sha256=abc,10\n"+
			"mypkg/core.py,sha256=def,20\n"+
			"mypkg-1.0.0.dist-info/METADATA,,\n"+
			"mypkg-1.0.0.dist-info/RECORD,,\n",
	), 0644)

	err := UninstallPackage(dir, "mypkg", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}

	// Package files should be gone.
	if _, err := os.Stat(filepath.Join(pkgDir, "__init__.py")); !os.IsNotExist(err) {
		t.Error("__init__.py should be deleted")
	}
	if _, err := os.Stat(filepath.Join(pkgDir, "core.py")); !os.IsNotExist(err) {
		t.Error("core.py should be deleted")
	}

	// dist-info should be gone.
	if _, err := os.Stat(distInfo); !os.IsNotExist(err) {
		t.Error("dist-info directory should be deleted")
	}

	// Empty package directory should be cleaned up.
	if _, err := os.Stat(pkgDir); !os.IsNotExist(err) {
		t.Error("empty package directory should be cleaned up")
	}
}

func TestUninstallPackage_NoRecord(t *testing.T) {
	dir := t.TempDir()

	// Create dist-info without RECORD.
	distInfo := filepath.Join(dir, "mypkg-1.0.0.dist-info")
	pkgDir := filepath.Join(dir, "mypkg")
	os.MkdirAll(distInfo, 0755)
	os.MkdirAll(pkgDir, 0755)
	os.WriteFile(filepath.Join(distInfo, "METADATA"), []byte("Name: mypkg\n"), 0644)
	os.WriteFile(filepath.Join(pkgDir, "__init__.py"), []byte("# mypkg"), 0644)

	err := UninstallPackage(dir, "mypkg", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}

	// dist-info and package dir should be removed (fallback behavior).
	if _, err := os.Stat(distInfo); !os.IsNotExist(err) {
		t.Error("dist-info should be deleted even without RECORD")
	}
	if _, err := os.Stat(pkgDir); !os.IsNotExist(err) {
		t.Error("package dir should be deleted as fallback")
	}
}

func TestReadRecord(t *testing.T) {
	dir := t.TempDir()
	recordPath := filepath.Join(dir, "RECORD")

	content := "pkg/__init__.py,sha256=abc,100\npkg/mod.py,sha256=def,200\npkg-1.0.dist-info/RECORD,,\n"
	os.WriteFile(recordPath, []byte(content), 0644)

	files, err := readRecord(recordPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}
	if files[0] != "pkg/__init__.py" {
		t.Errorf("files[0] = %q", files[0])
	}
}
