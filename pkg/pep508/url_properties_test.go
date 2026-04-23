package pep508

import "testing"

// URL-based dependency parsing (PEP 508 `pkg @ url` form).
// Existing coverage: one basic URL test, one URL+marker test. These
// property tests exercise the forms pensa is most likely to encounter
// in real pyproject.toml files and transitive metadata.

// Git URLs include an `@ref` suffix after the URL — our parser must
// keep the full string, treating only the FIRST unescaped '@' (the
// one separating name from URL) as a delimiter.
func TestURLProp_GitURLWithRef(t *testing.T) {
	cases := []struct {
		input   string
		wantURL string
	}{
		{
			`pkg @ git+https://github.com/user/repo.git`,
			"git+https://github.com/user/repo.git",
		},
		{
			`pkg @ git+https://github.com/user/repo.git@main`,
			"git+https://github.com/user/repo.git@main",
		},
		{
			`pkg @ git+https://github.com/user/repo.git@v1.0.0`,
			"git+https://github.com/user/repo.git@v1.0.0",
		},
		{
			`pkg @ git+ssh://user@host.example.com/repo.git@abc123`,
			"git+ssh://user@host.example.com/repo.git@abc123",
		},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			dep, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.input, err)
			}
			if dep.URL != tc.wantURL {
				t.Errorf("URL = %q; want %q", dep.URL, tc.wantURL)
			}
			if dep.Name != "pkg" {
				t.Errorf("Name = %q; want pkg", dep.Name)
			}
		})
	}
}

// File URLs for local installations.
func TestURLProp_FileURL(t *testing.T) {
	input := `mypkg @ file:///path/to/mypkg-1.0.tar.gz`
	dep, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if dep.URL != "file:///path/to/mypkg-1.0.tar.gz" {
		t.Errorf("URL = %q", dep.URL)
	}
}

// URL with extras.
func TestURLProp_URLWithExtras(t *testing.T) {
	input := `requests[security,socks] @ https://github.com/psf/requests/archive/v2.0.tar.gz`
	dep, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if dep.Name != "requests" {
		t.Errorf("Name = %q", dep.Name)
	}
	if len(dep.Extras) != 2 || dep.Extras[0] != "security" || dep.Extras[1] != "socks" {
		t.Errorf("Extras = %v", dep.Extras)
	}
	if dep.URL != "https://github.com/psf/requests/archive/v2.0.tar.gz" {
		t.Errorf("URL = %q", dep.URL)
	}
}

// URL with trailing whitespace is trimmed.
func TestURLProp_URLWhitespaceTrimmed(t *testing.T) {
	input := `pkg @ https://example.com/archive.zip   `
	dep, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if dep.URL != "https://example.com/archive.zip" {
		t.Errorf("URL = %q (trailing whitespace not trimmed?)", dep.URL)
	}
}

// Name normalization applies to URL deps too.
func TestURLProp_NameNormalizationAppliesToURL(t *testing.T) {
	input := `My.Package @ https://example.com/mypackage.zip`
	dep, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if dep.Name != "my-package" {
		t.Errorf("Name = %q; want normalized my-package", dep.Name)
	}
}

// URL with a marker clause at the end.
func TestURLProp_URLWithMarker(t *testing.T) {
	input := `pkg @ https://example.com/pkg.zip ; python_version >= "3.8"`
	dep, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if dep.URL != "https://example.com/pkg.zip" {
		t.Errorf("URL = %q (expected marker to be stripped from URL)", dep.URL)
	}
	if dep.Markers == nil {
		t.Error("expected markers to be parsed")
	}
}

// When a URL dep has no version specifier (correctly, since URLs
// point to a specific artifact), Constraint is nil. This is how the
// install path tells URL deps apart from versioned deps.
func TestURLProp_URLDepHasNoConstraint(t *testing.T) {
	dep, err := Parse(`pkg @ https://example.com/pkg.zip`)
	if err != nil {
		t.Fatal(err)
	}
	if dep.Constraint != nil {
		t.Errorf("URL dep should have no Constraint; got %s", dep.Constraint)
	}
	if dep.URL == "" {
		t.Error("URL should be set")
	}
}

// Versioned dep (no @) has nil URL and non-nil Constraint.
func TestURLProp_VersionedDepHasNoURL(t *testing.T) {
	dep, err := Parse(`pkg >= 1.0`)
	if err != nil {
		t.Fatal(err)
	}
	if dep.URL != "" {
		t.Errorf("versioned dep should have no URL; got %q", dep.URL)
	}
	if dep.Constraint == nil {
		t.Error("versioned dep should have a Constraint")
	}
}
