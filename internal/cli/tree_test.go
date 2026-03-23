package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestTree_AllPackages(t *testing.T) {
	setupTestDir(t, testLockFile())

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"tree"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	if !strings.Contains(out, "├──") && !strings.Contains(out, "└──") {
		t.Error("tree output missing box-drawing characters")
	}
	if !strings.Contains(out, "requests 2.31.0") {
		t.Error("missing requests in tree")
	}
}

func TestTree_SinglePackage(t *testing.T) {
	setupTestDir(t, testLockFile())

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"tree", "requests"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	// First line should be the root package.
	if !strings.HasPrefix(lines[0], "requests 2.31.0") {
		t.Errorf("first line should be requests root, got: %s", lines[0])
	}

	// Should show deps as children.
	if !strings.Contains(out, "certifi") {
		t.Error("missing dep certifi in tree")
	}
	if !strings.Contains(out, "urllib3") {
		t.Error("missing dep urllib3 in tree")
	}
}

func TestTree_PackageNotFound(t *testing.T) {
	setupTestDir(t, testLockFile())

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"tree", "nonexistent"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent package")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %s", err.Error())
	}
}

func TestTree_TopLevel(t *testing.T) {
	dir := setupTestDir(t, testLockFile())
	writePyprojectWithDeps(t, dir, `requests = "^2.31"`)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"tree", "--top-level"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	// Only requests should be a root (first line without prefix).
	if !strings.HasPrefix(lines[0], "requests") {
		t.Errorf("first root should be requests, got: %s", lines[0])
	}

	// certifi should appear as a child (indented), not as a root.
	for _, line := range lines {
		if strings.HasPrefix(line, "certifi") {
			t.Error("certifi should not be a root in top-level tree")
		}
	}
}

func TestTree_NoLockFile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"tree"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no poetry.lock")
	}
}
