package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew_CreatesProject(t *testing.T) {
	dir := t.TempDir()

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"new", filepath.Join(dir, "myproject")})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	projectDir := filepath.Join(dir, "myproject")

	// Check all files exist.
	for _, name := range []string{"pyproject.toml", "README.md", "main.py", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(projectDir, name)); err != nil {
			t.Errorf("missing file: %s", name)
		}
	}

	// Check pyproject.toml content.
	data, _ := os.ReadFile(filepath.Join(projectDir, "pyproject.toml"))
	content := string(data)
	if !strings.Contains(content, `name = "myproject"`) {
		t.Errorf("pyproject.toml missing project name, got:\n%s", content)
	}
	if !strings.Contains(content, `version = "0.1.0"`) {
		t.Error("pyproject.toml missing version")
	}
	if !strings.Contains(content, "requires-python") {
		t.Error("pyproject.toml missing requires-python")
	}

	// Check output.
	if !strings.Contains(buf.String(), "Created project myproject") {
		t.Errorf("unexpected output: %s", buf.String())
	}
}

func TestNew_CurrentDir(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"new"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// pyproject.toml should exist in current dir.
	if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err != nil {
		t.Error("pyproject.toml not created in current dir")
	}
}

func TestNew_ExistingPyproject(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	// Create existing pyproject.toml.
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"existing\"\n"), 0644)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"new"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when pyproject.toml exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists', got: %s", err.Error())
	}
}

func TestNew_NonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	targetDir := filepath.Join(dir, "myproject")
	os.MkdirAll(targetDir, 0755)
	os.WriteFile(filepath.Join(targetDir, "existing.txt"), []byte("hello"), 0644)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"new", targetDir})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-empty dir")
	}
	if !strings.Contains(err.Error(), "not empty") {
		t.Errorf("error should mention 'not empty', got: %s", err.Error())
	}
}

func TestNew_EmptyDirSucceeds(t *testing.T) {
	dir := t.TempDir()
	targetDir := filepath.Join(dir, "myproject")
	os.MkdirAll(targetDir, 0755)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"new", targetDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("should succeed in empty dir: %v", err)
	}

	if _, err := os.Stat(filepath.Join(targetDir, "pyproject.toml")); err != nil {
		t.Error("pyproject.toml not created in empty dir")
	}
}

func TestNew_CustomName(t *testing.T) {
	dir := t.TempDir()

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"new", filepath.Join(dir, "mydir"), "--name", "custom-project"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "mydir", "pyproject.toml"))
	if !strings.Contains(string(data), `name = "custom-project"`) {
		t.Errorf("pyproject.toml should use custom name, got:\n%s", string(data))
	}

	if !strings.Contains(buf.String(), "custom-project") {
		t.Errorf("output should mention custom name: %s", buf.String())
	}
}

func TestNew_MainPyContent(t *testing.T) {
	dir := t.TempDir()

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"new", filepath.Join(dir, "myproject")})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "myproject", "main.py"))
	content := string(data)
	if !strings.Contains(content, "Hello from myproject") {
		t.Errorf("main.py should greet with project name, got:\n%s", content)
	}
	if !strings.Contains(content, `if __name__ == "__main__"`) {
		t.Error("main.py missing __main__ guard")
	}
}
