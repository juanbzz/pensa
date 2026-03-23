package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestShow_PackageDetail(t *testing.T) {
	setupTestDir(t, testLockFile())

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"show", "requests"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	if !strings.Contains(out, "requests") {
		t.Error("missing package name")
	}
	if !strings.Contains(out, "2.31.0") {
		t.Error("missing version")
	}
	if !strings.Contains(out, "Python HTTP for Humans.") {
		t.Error("missing description")
	}
	if !strings.Contains(out, "certifi") {
		t.Error("missing dependency certifi")
	}
	if !strings.Contains(out, "urllib3") {
		t.Error("missing dependency urllib3")
	}
}

func TestShow_PackageNotFound(t *testing.T) {
	setupTestDir(t, testLockFile())

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"show", "nonexistent"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent package")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %s", err.Error())
	}
}

func TestShow_NoArgs(t *testing.T) {
	setupTestDir(t, testLockFile())

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"show"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no package arg given")
	}
}

func TestShow_NoLockFile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"show", "requests"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no poetry.lock")
	}
}
