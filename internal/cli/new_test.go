package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestNew_CreatesProject(t *testing.T) {
	assert := is.New(t)
	dir := t.TempDir()

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"new", filepath.Join(dir, "myproject")})

	assert.NoErr(cmd.Execute())

	projectDir := filepath.Join(dir, "myproject")

	// Check all files exist.
	for _, name := range []string{"pyproject.toml", "README.md", "myproject/__init__.py", "myproject/__main__.py", ".gitignore"} {
		_, err := os.Stat(filepath.Join(projectDir, name))
		assert.NoErr(err) // file should exist
	}

	// Check pyproject.toml content.
	data, _ := os.ReadFile(filepath.Join(projectDir, "pyproject.toml"))
	content := string(data)
	assert.True(strings.Contains(content, `name = "myproject"`))
	assert.True(strings.Contains(content, `version = "0.1.0"`))
	assert.True(strings.Contains(content, "requires-python"))

	// Check output.
	assert.True(strings.Contains(buf.String(), "Created project myproject"))
}

func TestNew_CurrentDir(t *testing.T) {
	assert := is.New(t)
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"new"})

	assert.NoErr(cmd.Execute())

	// pyproject.toml should exist in current dir.
	_, err := os.Stat(filepath.Join(dir, "pyproject.toml"))
	assert.NoErr(err)
}

func TestNew_ExistingPyproject(t *testing.T) {
	assert := is.New(t)
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	// Create existing pyproject.toml.
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"existing\"\n"), 0644)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"new"})

	err := cmd.Execute()
	assert.True(err != nil) // expected error when pyproject.toml exists
	assert.True(strings.Contains(err.Error(), "already exists"))
}

func TestNew_NonEmptyDir(t *testing.T) {
	assert := is.New(t)
	dir := t.TempDir()
	targetDir := filepath.Join(dir, "myproject")
	os.MkdirAll(targetDir, 0755)
	os.WriteFile(filepath.Join(targetDir, "existing.txt"), []byte("hello"), 0644)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"new", targetDir})

	err := cmd.Execute()
	assert.True(err != nil) // expected error for non-empty dir
	assert.True(strings.Contains(err.Error(), "not empty"))
}

func TestNew_EmptyDirSucceeds(t *testing.T) {
	assert := is.New(t)
	dir := t.TempDir()
	targetDir := filepath.Join(dir, "myproject")
	os.MkdirAll(targetDir, 0755)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"new", targetDir})

	assert.NoErr(cmd.Execute())

	_, err := os.Stat(filepath.Join(targetDir, "pyproject.toml"))
	assert.NoErr(err)
}

func TestNew_CustomName(t *testing.T) {
	assert := is.New(t)
	dir := t.TempDir()

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"new", filepath.Join(dir, "mydir"), "--name", "custom-project"})

	assert.NoErr(cmd.Execute())

	data, _ := os.ReadFile(filepath.Join(dir, "mydir", "pyproject.toml"))
	assert.True(strings.Contains(string(data), `name = "custom-project"`))
	assert.True(strings.Contains(buf.String(), "custom-project"))
}

func TestNew_PackageContent(t *testing.T) {
	assert := is.New(t)
	dir := t.TempDir()

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"new", filepath.Join(dir, "myproject")})

	assert.NoErr(cmd.Execute())

	// Check __main__.py has entry point.
	data, _ := os.ReadFile(filepath.Join(dir, "myproject", "myproject", "__main__.py"))
	content := string(data)
	assert.True(strings.Contains(content, "Hello from myproject"))
	assert.True(strings.Contains(content, `if __name__ == "__main__"`))

	// Check __init__.py exists.
	initData, _ := os.ReadFile(filepath.Join(dir, "myproject", "myproject", "__init__.py"))
	assert.True(strings.Contains(string(initData), "myproject"))

	// Check pyproject.toml has scripts entry.
	pyData, _ := os.ReadFile(filepath.Join(dir, "myproject", "pyproject.toml"))
	assert.True(strings.Contains(string(pyData), "[project.scripts]"))
}
