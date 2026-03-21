package version

import (
	"testing"
)

func TestParse_Basic(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1.0", "1.0"},
		{"1.2.3", "1.2.3"},
		{"0.0.0", "0.0.0"},
		{"10.20.30", "10.20.30"},
		{"1.2.3.4", "1.2.3.4"},
	}
	for _, tt := range tests {
		v, err := Parse(tt.input)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.input, err)
			continue
		}
		if got := v.String(); got != tt.want {
			t.Errorf("Parse(%q).String() = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParse_Epoch(t *testing.T) {
	v, err := Parse("2!1.0")
	if err != nil {
		t.Fatal(err)
	}
	if v.Epoch() != 2 {
		t.Errorf("epoch = %d, want 2", v.Epoch())
	}
	if v.String() != "2!1.0" {
		t.Errorf("String() = %q, want %q", v.String(), "2!1.0")
	}
}

func TestParse_PreRelease(t *testing.T) {
	tests := []struct {
		input string
		want  string
		kind  PreKind
		num   int
	}{
		{"1.0a1", "1.0a1", Alpha, 1},
		{"1.0b2", "1.0b2", Beta, 2},
		{"1.0rc3", "1.0rc3", RC, 3},
		{"1.0a0", "1.0a0", Alpha, 0},
	}
	for _, tt := range tests {
		v, err := Parse(tt.input)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.input, err)
			continue
		}
		if got := v.String(); got != tt.want {
			t.Errorf("Parse(%q).String() = %q, want %q", tt.input, got, tt.want)
		}
		if v.pre == nil {
			t.Errorf("Parse(%q).pre is nil", tt.input)
			continue
		}
		if v.pre.Kind != tt.kind {
			t.Errorf("Parse(%q).pre.Kind = %v, want %v", tt.input, v.pre.Kind, tt.kind)
		}
		if v.pre.Num != tt.num {
			t.Errorf("Parse(%q).pre.Num = %d, want %d", tt.input, v.pre.Num, tt.num)
		}
	}
}

func TestParse_PostRelease(t *testing.T) {
	tests := []struct {
		input string
		want  string
		post  int
	}{
		{"1.0.post1", "1.0.post1", 1},
		{"1.0.post0", "1.0.post0", 0},
	}
	for _, tt := range tests {
		v, err := Parse(tt.input)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.input, err)
			continue
		}
		if got := v.String(); got != tt.want {
			t.Errorf("Parse(%q).String() = %q, want %q", tt.input, got, tt.want)
		}
		if v.post == nil || *v.post != tt.post {
			t.Errorf("Parse(%q).post = %v, want %d", tt.input, v.post, tt.post)
		}
	}
}

func TestParse_DevRelease(t *testing.T) {
	v, err := Parse("1.0.dev4")
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "1.0.dev4" {
		t.Errorf("String() = %q, want %q", v.String(), "1.0.dev4")
	}
	if v.dev == nil || *v.dev != 4 {
		t.Errorf("dev = %v, want 4", v.dev)
	}
}

func TestParse_Local(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1.0+local.1", "1.0+local.1"},
		{"1.0+ubuntu-1", "1.0+ubuntu.1"},
		{"1.0+Ubuntu_1", "1.0+ubuntu.1"},
	}
	for _, tt := range tests {
		v, err := Parse(tt.input)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.input, err)
			continue
		}
		if got := v.String(); got != tt.want {
			t.Errorf("Parse(%q).String() = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParse_Combined(t *testing.T) {
	v, err := Parse("1!2.3a1.post2.dev3+local")
	if err != nil {
		t.Fatal(err)
	}
	if v.epoch != 1 {
		t.Errorf("epoch = %d, want 1", v.epoch)
	}
	if v.String() != "1!2.3a1.post2.dev3+local" {
		t.Errorf("String() = %q", v.String())
	}
}

func TestParse_Normalization(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// v-prefix
		{"v1.0", "1.0"},
		{"V1.0", "1.0"},
		// Leading zeros
		{"01.02.03", "1.2.3"},
		// Pre-release spellings
		{"1.0alpha1", "1.0a1"},
		{"1.0ALPHA1", "1.0a1"},
		{"1.0beta2", "1.0b2"},
		{"1.0c3", "1.0rc3"},
		{"1.0pre1", "1.0rc1"},
		{"1.0preview1", "1.0rc1"},
		// Pre-release separators
		{"1.0.a1", "1.0a1"},
		{"1.0-a1", "1.0a1"},
		{"1.0_a1", "1.0a1"},
		// Implicit pre-release number
		{"1.0a", "1.0a0"},
		// Post-release spellings
		{"1.0.rev1", "1.0.post1"},
		{"1.0.r1", "1.0.post1"},
		// Implicit post from dash-number
		{"1.0-1", "1.0.post1"},
		// Implicit post number
		{"1.0.post", "1.0.post0"},
		// Dev normalization
		{"1.0.dev", "1.0.dev0"},
		{"1.0dev3", "1.0.dev3"},
		// Whitespace
		{"  1.0  ", "1.0"},
	}
	for _, tt := range tests {
		v, err := Parse(tt.input)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.input, err)
			continue
		}
		if got := v.String(); got != tt.want {
			t.Errorf("Parse(%q).String() = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParse_Invalid(t *testing.T) {
	invalids := []string{
		"",
		"not.a.version",
		"abc",
		"1.0.0.0.0.0.0.0.0foobar",
	}
	for _, s := range invalids {
		_, err := Parse(s)
		if err == nil {
			t.Errorf("Parse(%q) expected error, got nil", s)
		}
	}
}

func TestCompare_Ordering(t *testing.T) {
	// PEP 440 canonical ordering.
	ordered := []string{
		"1.0.dev0",
		"1.0.dev1",
		"1.0a1",
		"1.0a2",
		"1.0b1",
		"1.0rc1",
		"1.0",
		"1.0.post1",
		"1.0.post2",
		"1.1",
		"2.0",
	}
	for i := 0; i < len(ordered)-1; i++ {
		a, _ := Parse(ordered[i])
		b, _ := Parse(ordered[i+1])
		if c := Compare(a, b); c >= 0 {
			t.Errorf("Compare(%q, %q) = %d, want < 0", ordered[i], ordered[i+1], c)
		}
		if c := Compare(b, a); c <= 0 {
			t.Errorf("Compare(%q, %q) = %d, want > 0", ordered[i+1], ordered[i], c)
		}
	}
}

func TestCompare_Equal(t *testing.T) {
	tests := []struct {
		a, b string
	}{
		{"1.0", "1.0"},
		{"1.0", "1.0.0"},
		{"1.0.0", "1.0.0.0"},
	}
	for _, tt := range tests {
		a, _ := Parse(tt.a)
		b, _ := Parse(tt.b)
		if c := Compare(a, b); c != 0 {
			t.Errorf("Compare(%q, %q) = %d, want 0", tt.a, tt.b, c)
		}
	}
}

func TestCompare_Epoch(t *testing.T) {
	a, _ := Parse("1!0.1")
	b, _ := Parse("2.0")
	if c := Compare(a, b); c <= 0 {
		t.Errorf("Compare(%q, %q) = %d, want > 0", "1!0.1", "2.0", c)
	}
}

func TestCompare_Local(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		// No local < has local.
		{"1.0", "1.0+local", -1},
		// Numeric > alpha.
		{"1.0+1", "1.0+abc", 1},
		// Numeric comparison.
		{"1.0+1", "1.0+2", -1},
		// Alpha comparison.
		{"1.0+abc", "1.0+def", -1},
		// More segments > fewer.
		{"1.0+a.b", "1.0+a", 1},
	}
	for _, tt := range tests {
		a, _ := Parse(tt.a)
		b, _ := Parse(tt.b)
		got := Compare(a, b)
		if got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCompare_DevOnlyPrePreRelease(t *testing.T) {
	// 1.0.dev0 should sort before 1.0a0
	a, _ := Parse("1.0.dev0")
	b, _ := Parse("1.0a0")
	if c := Compare(a, b); c >= 0 {
		t.Errorf("Compare(1.0.dev0, 1.0a0) = %d, want < 0", c)
	}
}

func TestVersion_Predicates(t *testing.T) {
	tests := []struct {
		input string
		pre   bool
		post  bool
		dev   bool
		local bool
		stable bool
	}{
		{"1.0", false, false, false, false, true},
		{"1.0a1", true, false, false, false, false},
		{"1.0.post1", false, true, false, false, true},
		{"1.0.dev0", false, false, true, false, false},
		{"1.0+local", false, false, false, true, true},
		{"1.0a1.dev0", true, false, true, false, false},
	}
	for _, tt := range tests {
		v, err := Parse(tt.input)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.input, err)
			continue
		}
		if v.IsPreRelease() != tt.pre {
			t.Errorf("%q.IsPreRelease() = %v, want %v", tt.input, v.IsPreRelease(), tt.pre)
		}
		if v.IsPostRelease() != tt.post {
			t.Errorf("%q.IsPostRelease() = %v, want %v", tt.input, v.IsPostRelease(), tt.post)
		}
		if v.IsDevRelease() != tt.dev {
			t.Errorf("%q.IsDevRelease() = %v, want %v", tt.input, v.IsDevRelease(), tt.dev)
		}
		if v.IsLocal() != tt.local {
			t.Errorf("%q.IsLocal() = %v, want %v", tt.input, v.IsLocal(), tt.local)
		}
		if v.IsStable() != tt.stable {
			t.Errorf("%q.IsStable() = %v, want %v", tt.input, v.IsStable(), tt.stable)
		}
	}
}

func TestVersion_NextMajor(t *testing.T) {
	v, _ := Parse("1.2.3")
	got := v.NextMajor().String()
	if got != "2.0.0" {
		t.Errorf("NextMajor() = %q, want %q", got, "2.0.0")
	}
}

func TestVersion_NextMinor(t *testing.T) {
	v, _ := Parse("1.2.3")
	got := v.NextMinor().String()
	if got != "1.3.0" {
		t.Errorf("NextMinor() = %q, want %q", got, "1.3.0")
	}
}

func TestVersion_NextPatch(t *testing.T) {
	v, _ := Parse("1.2.3")
	got := v.NextPatch().String()
	if got != "1.2.4" {
		t.Errorf("NextPatch() = %q, want %q", got, "1.2.4")
	}
}

func TestVersion_NextStable(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1.0", "1.1"},
		{"1.2.3", "1.2.4"},
		{"1.0a1", "1.0"},
		{"1.0.dev0", "1.0"},
	}
	for _, tt := range tests {
		v, _ := Parse(tt.input)
		got := v.NextStable().String()
		if got != tt.want {
			t.Errorf("%q.NextStable() = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestVersion_FirstDevRelease(t *testing.T) {
	v, _ := Parse("1.0")
	got := v.FirstDevRelease().String()
	if got != "1.0.dev0" {
		t.Errorf("FirstDevRelease() = %q, want %q", got, "1.0.dev0")
	}
}

func TestVersion_WithoutLocal(t *testing.T) {
	v, _ := Parse("1.0+local")
	got := v.WithoutLocal()
	if got.IsLocal() {
		t.Error("WithoutLocal() still has local")
	}
	if got.String() != "1.0" {
		t.Errorf("WithoutLocal() = %q, want %q", got.String(), "1.0")
	}
}

func TestVersion_WithoutPostRelease(t *testing.T) {
	v, _ := Parse("1.0.post1")
	got := v.WithoutPostRelease()
	if got.IsPostRelease() {
		t.Error("WithoutPostRelease() still has post")
	}
	if got.String() != "1.0" {
		t.Errorf("WithoutPostRelease() = %q, want %q", got.String(), "1.0")
	}
}

func TestVersion_Accessors(t *testing.T) {
	v, _ := Parse("1.2.3")
	if v.Major() != 1 {
		t.Errorf("Major() = %d, want 1", v.Major())
	}
	if v.Minor() != 2 {
		t.Errorf("Minor() = %d, want 2", v.Minor())
	}
	if v.Patch() != 3 {
		t.Errorf("Patch() = %d, want 3", v.Patch())
	}
}
