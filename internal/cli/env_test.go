package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juanbzz/pensa/internal/python"
)

func TestEnv_PrintsPath(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	// Create a fake venv.
	venvPath := filepath.Join(dir, ".venv")
	os.MkdirAll(venvPath, 0755)
	os.WriteFile(filepath.Join(venvPath, "pyvenv.cfg"), []byte("home = /usr/bin\n"), 0644)

	if !python.VenvExists(venvPath) {
		t.Fatal("venv should exist after creating pyvenv.cfg")
	}

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"env"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	out := strings.TrimSpace(buf.String())
	// On macOS, /var is a symlink to /private/var, so compare with Contains.
	if !strings.HasSuffix(out, ".venv") {
		t.Errorf("expected path ending in .venv, got %q", out)
	}
}

func TestEnv_Verbose(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	venvPath := filepath.Join(dir, ".venv")
	os.MkdirAll(venvPath, 0755)
	os.WriteFile(filepath.Join(venvPath, "pyvenv.cfg"), []byte("home = /usr/bin\n"), 0644)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"env", "-v"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "Path:") {
		t.Error("verbose output should contain 'Path:'")
	}
	if !strings.Contains(out, "Python:") {
		t.Error("verbose output should contain 'Python:'")
	}
	if !strings.Contains(out, "Executable:") {
		t.Error("verbose output should contain 'Executable:'")
	}
}

func TestEnv_NoVenv(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"env"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no venv exists")
	}
	if !strings.Contains(err.Error(), "no virtualenv found") {
		t.Errorf("error should mention no virtualenv, got: %s", err.Error())
	}
}
