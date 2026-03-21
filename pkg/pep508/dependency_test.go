package pep508

import (
	"testing"
)

func TestParse_BasicName(t *testing.T) {
	dep, err := Parse("requests")
	if err != nil {
		t.Fatal(err)
	}
	if dep.Name != "requests" {
		t.Errorf("Name = %q, want %q", dep.Name, "requests")
	}
	if dep.Constraint != nil {
		t.Error("expected nil constraint")
	}
	if dep.Markers != nil {
		t.Error("expected nil markers")
	}
}

func TestParse_WithVersion(t *testing.T) {
	dep, err := Parse("requests>=2.0")
	if err != nil {
		t.Fatal(err)
	}
	if dep.Name != "requests" {
		t.Errorf("Name = %q", dep.Name)
	}
	if dep.Constraint == nil {
		t.Fatal("expected constraint")
	}
}

func TestParse_WithExtras(t *testing.T) {
	dep, err := Parse("requests[security]>=2.8.1")
	if err != nil {
		t.Fatal(err)
	}
	if dep.Name != "requests" {
		t.Errorf("Name = %q", dep.Name)
	}
	if len(dep.Extras) != 1 || dep.Extras[0] != "security" {
		t.Errorf("Extras = %v, want [security]", dep.Extras)
	}
	if dep.Constraint == nil {
		t.Fatal("expected constraint")
	}
}

func TestParse_MultipleExtras(t *testing.T) {
	dep, err := Parse("requests[security, tests]>=2.8.1,==2.8.*")
	if err != nil {
		t.Fatal(err)
	}
	if len(dep.Extras) != 2 {
		t.Fatalf("Extras = %v, want 2 extras", dep.Extras)
	}
	if dep.Extras[0] != "security" || dep.Extras[1] != "tests" {
		t.Errorf("Extras = %v", dep.Extras)
	}
}

func TestParse_URLDependency(t *testing.T) {
	dep, err := Parse("pip @ https://github.com/pypa/pip/archive/1.3.1.zip")
	if err != nil {
		t.Fatal(err)
	}
	if dep.Name != "pip" {
		t.Errorf("Name = %q", dep.Name)
	}
	if dep.URL != "https://github.com/pypa/pip/archive/1.3.1.zip" {
		t.Errorf("URL = %q", dep.URL)
	}
}

func TestParse_WithMarkers(t *testing.T) {
	dep, err := Parse(`argparse; python_version < "2.7"`)
	if err != nil {
		t.Fatal(err)
	}
	if dep.Name != "argparse" {
		t.Errorf("Name = %q", dep.Name)
	}
	if dep.Markers == nil {
		t.Fatal("expected markers")
	}
}

func TestParse_ComplexMarkers(t *testing.T) {
	dep, err := Parse(`name; os_name == 'a' and (os_name == 'b' or os_name == 'c')`)
	if err != nil {
		t.Fatal(err)
	}
	if dep.Name != "name" {
		t.Errorf("Name = %q", dep.Name)
	}
	if dep.Markers == nil {
		t.Fatal("expected markers")
	}
}

func TestParse_NameNormalization(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Name_With.Mixed-case", "name-with-mixed-case"},
		{"Django", "django"},
		{"my__package", "my-package"},
		{"some.pkg", "some-pkg"},
	}
	for _, tt := range tests {
		dep, err := Parse(tt.input)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.input, err)
			continue
		}
		if dep.Name != tt.want {
			t.Errorf("Parse(%q).Name = %q, want %q", tt.input, dep.Name, tt.want)
		}
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"requests", "requests"},
		{"Requests", "requests"},
		{"my_package", "my-package"},
		{"my.package", "my-package"},
		{"my-package", "my-package"},
		{"My__Complex...Name", "my-complex-name"},
	}
	for _, tt := range tests {
		got := NormalizeName(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParse_ParenthesizedVersion(t *testing.T) {
	dep, err := Parse("requests (>=2.0,<3.0)")
	if err != nil {
		t.Fatal(err)
	}
	if dep.Constraint == nil {
		t.Fatal("expected constraint")
	}
}

func TestParse_URLWithMarkers(t *testing.T) {
	dep, err := Parse(`name[fred] @ http://foo.com ; python_version == '2.7'`)
	if err != nil {
		t.Fatal(err)
	}
	if dep.Name != "name" {
		t.Errorf("Name = %q", dep.Name)
	}
	if len(dep.Extras) != 1 || dep.Extras[0] != "fred" {
		t.Errorf("Extras = %v", dep.Extras)
	}
	if dep.URL != "http://foo.com" {
		t.Errorf("URL = %q", dep.URL)
	}
	if dep.Markers == nil {
		t.Fatal("expected markers")
	}
}

func TestParse_Invalid(t *testing.T) {
	invalids := []string{
		"",
		"[extras]",
	}
	for _, s := range invalids {
		_, err := Parse(s)
		if err == nil {
			t.Errorf("Parse(%q) expected error", s)
		}
	}
}
