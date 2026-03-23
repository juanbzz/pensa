package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/juanbzz/pensa/internal/lockfile"
)

func TestList_AllPackages(t *testing.T) {
	setupTestDir(t, testLockFile())

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	for _, pkg := range []string{"certifi", "charset-normalizer", "idna", "requests", "urllib3"} {
		if !strings.Contains(out, pkg) {
			t.Errorf("output missing package %q", pkg)
		}
	}

	// Should be alphabetically sorted.
	certIdx := strings.Index(out, "certifi")
	reqIdx := strings.Index(out, "requests")
	if certIdx > reqIdx {
		t.Error("packages not sorted alphabetically")
	}
}

func TestList_TopLevel(t *testing.T) {
	dir := setupTestDir(t, testLockFile())
	writePyprojectWithDeps(t, dir, `requests = "^2.31"`)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"list", "--top-level"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	if !strings.Contains(out, "requests") {
		t.Error("top-level should include requests")
	}
	if strings.Contains(out, "certifi") {
		t.Error("top-level should not include transitive dep certifi")
	}
}

func TestList_Empty(t *testing.T) {
	lf := &lockfile.LockFile{
		Metadata: lockfile.LockMetadata{
			LockVersion:    "2.1",
			PythonVersions: ">=3.8",
			ContentHash:    "empty",
		},
	}
	setupTestDir(t, lf)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "No packages found") {
		t.Errorf("expected 'No packages found', got: %s", out)
	}
}

func TestList_NoLockFile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"list"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no poetry.lock")
	}
}
