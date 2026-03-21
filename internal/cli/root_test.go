package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmd_ExecutesWithoutError(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRootCmd_HelpContainsDescription(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "fast Python package manager") {
		t.Errorf("expected help to contain description, got:\n%s", output)
	}
}

func TestVersionCmd_PrintsVersion(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "goetry dev") {
		t.Errorf("expected version output, got: %s", output)
	}
}

func TestVersionCmd_PrintsCustomVersion(t *testing.T) {
	original := Version
	Version = "1.2.3"
	defer func() { Version = original }()

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "goetry 1.2.3") {
		t.Errorf("expected 'goetry 1.2.3', got: %s", output)
	}
}
