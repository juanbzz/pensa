package index

import (
	"testing"
)

func TestVersionFromFilename_Wheel(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"requests-2.31.0-py3-none-any.whl", "2.31.0"},
		{"numpy-1.24.0-cp311-cp311-manylinux_2_17_x86_64.whl", "1.24.0"},
		{"Flask-2.3.2-py3-none-any.whl", "2.3.2"},
	}
	for _, tt := range tests {
		v, err := VersionFromFilename(tt.filename)
		if err != nil {
			t.Errorf("VersionFromFilename(%q) error: %v", tt.filename, err)
			continue
		}
		if got := v.String(); got != tt.want {
			t.Errorf("VersionFromFilename(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

func TestVersionFromFilename_Sdist(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"requests-2.31.0.tar.gz", "2.31.0"},
		{"Flask-2.3.2.tar.gz", "2.3.2"},
		{"my-package-1.0.0.zip", "1.0.0"},
	}
	for _, tt := range tests {
		v, err := VersionFromFilename(tt.filename)
		if err != nil {
			t.Errorf("VersionFromFilename(%q) error: %v", tt.filename, err)
			continue
		}
		if got := v.String(); got != tt.want {
			t.Errorf("VersionFromFilename(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

func TestVersionFromFilename_Invalid(t *testing.T) {
	invalids := []string{
		"not-a-package",
		"",
		"readme.md",
	}
	for _, s := range invalids {
		_, err := VersionFromFilename(s)
		if err == nil {
			t.Errorf("VersionFromFilename(%q) expected error", s)
		}
	}
}

func TestParseMetadata(t *testing.T) {
	metadata := []byte(`Metadata-Version: 2.1
Name: requests
Version: 2.31.0
Requires-Python: >=3.7
Requires-Dist: charset-normalizer (<4,>=2)
Requires-Dist: idna (<4,>=2.5)
Requires-Dist: urllib3 (<3,>=1.21.1)
Requires-Dist: certifi (>=2017.4.17)
Requires-Dist: PySocks (!=1.5.7,>=1.5.6) ; extra == 'socks'

This is the body and should be ignored.
`)

	detail, err := ParseMetadata(metadata)
	if err != nil {
		t.Fatal(err)
	}

	if detail.Name != "requests" {
		t.Errorf("Name = %q", detail.Name)
	}
	if detail.Version.String() != "2.31.0" {
		t.Errorf("Version = %q", detail.Version.String())
	}
	if detail.RequiresPython != ">=3.7" {
		t.Errorf("RequiresPython = %q", detail.RequiresPython)
	}
	if len(detail.Dependencies) != 5 {
		t.Fatalf("Dependencies count = %d, want 5", len(detail.Dependencies))
	}
	if detail.Dependencies[0].Name != "charset-normalizer" {
		t.Errorf("first dep = %q", detail.Dependencies[0].Name)
	}
}

func TestPackageInfo_Versions(t *testing.T) {
	info := &PackageInfo{
		Name: "requests",
		Files: []FileInfo{
			{Filename: "requests-2.31.0-py3-none-any.whl"},
			{Filename: "requests-2.31.0.tar.gz"},
			{Filename: "requests-2.30.0-py3-none-any.whl"},
		},
	}

	versions := info.Versions()
	if len(versions) != 2 {
		t.Fatalf("Versions count = %d, want 2", len(versions))
	}
}

func TestPackageInfo_BestWheel(t *testing.T) {
	info := &PackageInfo{
		Name: "requests",
		Files: []FileInfo{
			{Filename: "requests-2.31.0.tar.gz"},
			{Filename: "requests-2.31.0-cp311-cp311-manylinux.whl", CoreMetadata: true},
			{Filename: "requests-2.31.0-py3-none-any.whl", CoreMetadata: true},
		},
	}

	v, _ := VersionFromFilename("requests-2.31.0-py3-none-any.whl")
	wheel := info.BestWheel(v)
	if wheel == nil {
		t.Fatal("expected a wheel")
	}
	if wheel.Filename != "requests-2.31.0-py3-none-any.whl" {
		t.Errorf("BestWheel = %q, want py3-none-any", wheel.Filename)
	}
}
