package version

import "testing"

func TestParseConstraint_Allows(t *testing.T) {
	tests := []struct {
		constraint string
		allows     []string
		rejects    []string
	}{
		{
			">=1.0",
			[]string{"1.0", "1.1", "2.0"},
			[]string{"0.9", "0.1"},
		},
		{
			">1.0",
			[]string{"1.0.1", "1.1", "2.0"},
			[]string{"1.0", "0.9"},
		},
		{
			"<2.0",
			[]string{"1.9", "1.0", "0.1"},
			[]string{"2.0", "2.0a1", "3.0"},
		},
		{
			"<=2.0",
			[]string{"2.0", "1.9", "1.0"},
			[]string{"2.0.1", "3.0"},
		},
		{
			"==1.0",
			[]string{"1.0", "1.0.0"},
			[]string{"1.0.1", "0.9"},
		},
		{
			"!=1.5",
			[]string{"1.4", "1.6", "2.0"},
			[]string{"1.5"},
		},
		{
			">=1.0,<2.0",
			[]string{"1.0", "1.5", "1.9.9"},
			[]string{"0.9", "2.0"},
		},
		{
			"*",
			[]string{"1.0", "999.0", "0.0.1"},
			nil,
		},
		{
			"1.0",
			[]string{"1.0"},
			[]string{"1.1", "0.9"},
		},
		// PEP 440 exclusive edge cases.
		{
			">1.7",
			[]string{"1.7.1", "1.8", "2.0"},
			[]string{"1.7", "1.7.0.post1"},
		},
		// Wildcard.
		{
			"==1.0.*",
			[]string{"1.0", "1.0.1", "1.0.99"},
			[]string{"1.1", "0.9"},
		},
		{
			"!=1.0.*",
			[]string{"1.1", "0.9", "2.0"},
			[]string{"1.0", "1.0.1"},
		},
		// PEP 440 compatible release.
		{
			"~=1.4.2",
			[]string{"1.4.2", "1.4.9"},
			[]string{"1.5.0", "1.4.1", "1.3.0"},
		},
		{
			"~=1.4",
			[]string{"1.4", "1.9.9"},
			[]string{"2.0", "1.3"},
		},
		// Poetry caret.
		{
			"^1.2.3",
			[]string{"1.2.3", "1.9.0", "1.99.99"},
			[]string{"2.0.0", "1.2.2"},
		},
		{
			"^0.2.3",
			[]string{"0.2.3", "0.2.9"},
			[]string{"0.3.0", "0.2.2"},
		},
		{
			"^0.0.3",
			[]string{"0.0.3"},
			[]string{"0.0.4", "0.0.2"},
		},
		// Poetry tilde.
		{
			"~1.2",
			[]string{"1.2", "1.2.9"},
			[]string{"1.3.0", "1.1"},
		},
		{
			"~1.2.3",
			[]string{"1.2.3", "1.2.99"},
			[]string{"1.3.0", "1.2.2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.constraint, func(t *testing.T) {
			c, err := ParseConstraint(tt.constraint)
			if err != nil {
				t.Fatalf("ParseConstraint(%q) error: %v", tt.constraint, err)
			}
			for _, v := range tt.allows {
				ver := mustParse(t, v)
				if !c.Allows(ver) {
					t.Errorf("%q should allow %q", tt.constraint, v)
				}
			}
			for _, v := range tt.rejects {
				ver := mustParse(t, v)
				if c.Allows(ver) {
					t.Errorf("%q should reject %q", tt.constraint, v)
				}
			}
		})
	}
}

func TestParseConstraint_Or(t *testing.T) {
	c, err := ParseConstraint(">=1.0 || <0.5")
	if err != nil {
		t.Fatal(err)
	}
	if !c.Allows(mustParse(t, "1.5")) {
		t.Error("should allow 1.5")
	}
	if !c.Allows(mustParse(t, "0.3")) {
		t.Error("should allow 0.3")
	}
	if c.Allows(mustParse(t, "0.7")) {
		t.Error("should reject 0.7")
	}
}

func TestParseConstraint_ArbitraryEquality(t *testing.T) {
	c, err := ParseConstraint("===foobar")
	if err != nil {
		t.Fatal(err)
	}
	// Won't match any parsed version since "foobar" isn't a valid PEP 440 version.
	v, _ := Parse("1.0")
	if c.Allows(v) {
		t.Error("===foobar should not match 1.0")
	}
}

func TestParseConstraint_Invalid(t *testing.T) {
	invalids := []string{
		">>1.0",
		"<<1.0",
		">=",
	}
	for _, s := range invalids {
		_, err := ParseConstraint(s)
		if err == nil {
			t.Errorf("ParseConstraint(%q) expected error", s)
		}
	}
}

func TestParseConstraint_String(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{">=1.0", ">=1.0"},
		{"<2.0", "<2.0"},
		{"*", "*"},
	}
	for _, tt := range tests {
		c, err := ParseConstraint(tt.input)
		if err != nil {
			t.Fatalf("ParseConstraint(%q) error: %v", tt.input, err)
		}
		if got := c.String(); got != tt.want {
			t.Errorf("ParseConstraint(%q).String() = %q, want %q", tt.input, got, tt.want)
		}
	}
}
