package pyproject

import (
	"testing"
)

func TestParsePoetryDependency_String(t *testing.T) {
	dep, err := ParsePoetryDependency("requests", "^2.13.0")
	if err != nil {
		t.Fatal(err)
	}
	if dep.Name != "requests" {
		t.Errorf("Name = %q", dep.Name)
	}
	if dep.Constraint == nil {
		t.Error("expected constraint")
	}
}

func TestParsePoetryDependency_TableWithVersion(t *testing.T) {
	dep, err := ParsePoetryDependency("requests", map[string]interface{}{
		"version": "^2.13.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dep.Constraint == nil {
		t.Error("expected constraint")
	}
}

func TestParsePoetryDependency_WithExtras(t *testing.T) {
	dep, err := ParsePoetryDependency("requests", map[string]interface{}{
		"version": "^2.13.0",
		"extras":  []interface{}{"security"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(dep.Extras) != 1 || dep.Extras[0] != "security" {
		t.Errorf("Extras = %v", dep.Extras)
	}
}

func TestParsePoetryDependency_WithPythonConstraint(t *testing.T) {
	dep, err := ParsePoetryDependency("flask", map[string]interface{}{
		"version": "^2.0",
		"python":  "^3.8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dep.Markers == nil {
		t.Fatal("expected markers from python constraint")
	}
}

func TestParsePoetryDependency_WithMarkers(t *testing.T) {
	dep, err := ParsePoetryDependency("pathlib2", map[string]interface{}{
		"version": "^2.3",
		"markers": `python_version < "3.0"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if dep.Markers == nil {
		t.Fatal("expected markers")
	}
}

func TestParsePoetryDependency_PathDep(t *testing.T) {
	dep, err := ParsePoetryDependency("my-pkg", map[string]interface{}{
		"path": "../my-pkg",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dep.URL != "file://../my-pkg" {
		t.Errorf("URL = %q", dep.URL)
	}
}

func TestParsePoetryDependency_GitDep(t *testing.T) {
	dep, err := ParsePoetryDependency("my-pkg", map[string]interface{}{
		"git":    "https://github.com/user/repo.git",
		"branch": "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dep.URL != "git+https://github.com/user/repo.git@main" {
		t.Errorf("URL = %q", dep.URL)
	}
}

func TestParsePoetryDependency_NameNormalization(t *testing.T) {
	dep, err := ParsePoetryDependency("My_Package", "^1.0")
	if err != nil {
		t.Fatal(err)
	}
	if dep.Name != "my-package" {
		t.Errorf("Name = %q, want %q", dep.Name, "my-package")
	}
}

func TestParsePoetryDependency_InvalidVersion(t *testing.T) {
	_, err := ParsePoetryDependency("bad", "not-a-version!!!")
	if err == nil {
		t.Error("expected error for invalid version")
	}
}
