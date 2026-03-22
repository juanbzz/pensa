package python

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateVenv(t *testing.T) {
	py, err := Discover()
	if err != nil {
		t.Skip("no Python found, skipping venv test")
	}

	venvPath := filepath.Join(t.TempDir(), ".venv")
	if err := CreateVenv(venvPath, py); err != nil {
		t.Fatalf("CreateVenv error: %v", err)
	}

	// Verify pyvenv.cfg exists.
	cfgData, err := os.ReadFile(filepath.Join(venvPath, "pyvenv.cfg"))
	if err != nil {
		t.Fatal("pyvenv.cfg not created")
	}
	cfg := string(cfgData)
	if !strings.Contains(cfg, "home = ") {
		t.Error("pyvenv.cfg missing home")
	}
	if !strings.Contains(cfg, "include-system-site-packages = false") {
		t.Error("pyvenv.cfg missing include-system-site-packages")
	}
	if !strings.Contains(cfg, "version = ") {
		t.Error("pyvenv.cfg missing version")
	}

	// Verify python symlink exists and resolves.
	pythonLink := filepath.Join(venvPath, "bin", "python3")
	target, err := os.Readlink(pythonLink)
	if err != nil {
		t.Fatalf("python3 symlink not created: %v", err)
	}
	if target != py.Path {
		t.Errorf("python3 symlink points to %q, want %q", target, py.Path)
	}

	// Verify python → python3 symlink.
	shortLink := filepath.Join(venvPath, "bin", "python")
	target, err = os.Readlink(shortLink)
	if err != nil {
		t.Fatal("python symlink not created")
	}
	if target != "python3" {
		t.Errorf("python symlink points to %q, want %q", target, "python3")
	}

	// Verify site-packages directory exists.
	spDir := py.SitePackagesDir(venvPath)
	if _, err := os.Stat(spDir); err != nil {
		t.Errorf("site-packages not created: %v", err)
	}

	// Verify VenvExists.
	if !VenvExists(venvPath) {
		t.Error("VenvExists returned false for created venv")
	}
}

func TestVenvExists_False(t *testing.T) {
	if VenvExists(filepath.Join(t.TempDir(), "nonexistent")) {
		t.Error("VenvExists returned true for nonexistent path")
	}
}
