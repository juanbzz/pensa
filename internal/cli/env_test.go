package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"

	"github.com/juanbzz/pensa/internal/python"
)

func TestEnv_PrintsPath(t *testing.T) {
	assert := is.New(t)
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	// Create a fake venv.
	venvPath := filepath.Join(dir, ".venv")
	os.MkdirAll(venvPath, 0755)
	os.WriteFile(filepath.Join(venvPath, "pyvenv.cfg"), []byte("home = /usr/bin\n"), 0644)

	assert.True(python.VenvExists(venvPath)) // venv should exist after creating pyvenv.cfg

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"env"})

	assert.NoErr(cmd.Execute())

	out := strings.TrimSpace(buf.String())
	// On macOS, /var is a symlink to /private/var, so compare with Contains.
	assert.True(strings.HasSuffix(out, ".venv"))
}

func TestEnv_Verbose(t *testing.T) {
	assert := is.New(t)
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

	assert.NoErr(cmd.Execute())

	out := buf.String()
	assert.True(strings.Contains(out, "Path:"))
	assert.True(strings.Contains(out, "Python:"))
	assert.True(strings.Contains(out, "Executable:"))
}

func TestEnv_NoVenv(t *testing.T) {
	assert := is.New(t)
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"env"})

	err := cmd.Execute()
	assert.True(err != nil) // expected error when no venv exists
	assert.True(strings.Contains(err.Error(), "no virtualenv found"))
}
