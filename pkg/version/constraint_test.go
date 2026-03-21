package version

import "testing"

func mustParse(t *testing.T, s string) Version {
	t.Helper()
	v, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", s, err)
	}
	return v
}

func TestRange_Allows_Basic(t *testing.T) {
	tests := []struct {
		name    string
		min     *Version
		max     *Version
		inclMin bool
		inclMax bool
		version string
		want    bool
	}{
		{">=1.0 allows 1.0", ptr(mustParse(t, "1.0")), nil, true, false, "1.0", true},
		{">=1.0 allows 1.1", ptr(mustParse(t, "1.0")), nil, true, false, "1.1", true},
		{">=1.0 rejects 0.9", ptr(mustParse(t, "1.0")), nil, true, false, "0.9", false},
		{">1.0 rejects 1.0", ptr(mustParse(t, "1.0")), nil, false, false, "1.0", false},
		{">1.0 allows 1.0.1", ptr(mustParse(t, "1.0")), nil, false, false, "1.0.1", true},
		{"<2.0 allows 1.9", nil, ptr(mustParse(t, "2.0")), false, false, "1.9", true},
		{"<2.0 rejects 2.0", nil, ptr(mustParse(t, "2.0")), false, false, "2.0", false},
		{"<=2.0 allows 2.0", nil, ptr(mustParse(t, "2.0")), false, true, "2.0", true},
		{"<=2.0 rejects 2.0.1", nil, ptr(mustParse(t, "2.0")), false, true, "2.0.1", false},
		{"bounded >=1.0,<2.0 allows 1.5", ptr(mustParse(t, "1.0")), ptr(mustParse(t, "2.0")), true, false, "1.5", true},
		{"bounded >=1.0,<2.0 rejects 2.0", ptr(mustParse(t, "1.0")), ptr(mustParse(t, "2.0")), true, false, "2.0", false},
		{"bounded >=1.0,<2.0 rejects 0.9", ptr(mustParse(t, "1.0")), ptr(mustParse(t, "2.0")), true, false, "0.9", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Range{min: tt.min, max: tt.max, includeMin: tt.inclMin, includeMax: tt.inclMax}
			v := mustParse(t, tt.version)
			if got := r.Allows(v); got != tt.want {
				t.Errorf("Range.Allows(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestRange_Allows_ExclusiveEdgeCases(t *testing.T) {
	// >1.7 must not allow 1.7.0.post1 (PEP 440).
	t.Run(">1.7 rejects 1.7.0.post1", func(t *testing.T) {
		min := mustParse(t, "1.7")
		r := &Range{min: &min, includeMin: false}
		v := mustParse(t, "1.7.0.post1")
		if r.Allows(v) {
			t.Error("expected >1.7 to reject 1.7.0.post1")
		}
	})

	// >1.7 allows 1.7.1.
	t.Run(">1.7 allows 1.7.1", func(t *testing.T) {
		min := mustParse(t, "1.7")
		r := &Range{min: &min, includeMin: false}
		v := mustParse(t, "1.7.1")
		if !r.Allows(v) {
			t.Error("expected >1.7 to allow 1.7.1")
		}
	})

	// >1.7.post2 allows 1.7.0.post3.
	t.Run(">1.7.post2 allows 1.7.post3", func(t *testing.T) {
		min := mustParse(t, "1.7.post2")
		r := &Range{min: &min, includeMin: false}
		v := mustParse(t, "1.7.post3")
		if !r.Allows(v) {
			t.Error("expected >1.7.post2 to allow 1.7.post3")
		}
	})

	// <2.0 must not allow 2.0a1.
	t.Run("<2.0 rejects 2.0a1", func(t *testing.T) {
		max := mustParse(t, "2.0")
		r := &Range{max: &max, includeMax: false}
		v := mustParse(t, "2.0a1")
		if r.Allows(v) {
			t.Error("expected <2.0 to reject 2.0a1")
		}
	})

	// <2.0 allows 1.9.9.
	t.Run("<2.0 allows 1.9.9", func(t *testing.T) {
		max := mustParse(t, "2.0")
		r := &Range{max: &max, includeMax: false}
		v := mustParse(t, "1.9.9")
		if !r.Allows(v) {
			t.Error("expected <2.0 to allow 1.9.9")
		}
	})
}

func TestExactConstraint_Allows(t *testing.T) {
	tests := []struct {
		constraint string
		version    string
		want       bool
	}{
		{"1.0", "1.0", true},
		{"1.0", "1.0.0", true},
		{"1.0", "1.0.1", false},
		{"1.0", "1.0+local", true}, // local ignored when constraint has none
		{"1.0+local", "1.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.constraint+"_allows_"+tt.version, func(t *testing.T) {
			cv := mustParse(t, tt.constraint)
			c := ExactVersion(cv)
			v := mustParse(t, tt.version)
			if got := c.Allows(v); got != tt.want {
				t.Errorf("ExactVersion(%q).Allows(%q) = %v, want %v", tt.constraint, tt.version, got, tt.want)
			}
		})
	}
}

func TestRange_Intersect(t *testing.T) {
	// >=1.0,<2.0 intersect >=1.5,<3.0 → >=1.5,<2.0
	r1 := &Range{
		min: ptr(mustParse(t, "1.0")), max: ptr(mustParse(t, "2.0")),
		includeMin: true, includeMax: false,
	}
	r2 := &Range{
		min: ptr(mustParse(t, "1.5")), max: ptr(mustParse(t, "3.0")),
		includeMin: true, includeMax: false,
	}
	result := r1.Intersect(r2)

	if result.IsEmpty() {
		t.Fatal("expected non-empty intersection")
	}
	if !result.Allows(mustParse(t, "1.5")) {
		t.Error("intersection should allow 1.5")
	}
	if !result.Allows(mustParse(t, "1.9")) {
		t.Error("intersection should allow 1.9")
	}
	if result.Allows(mustParse(t, "1.0")) {
		t.Error("intersection should not allow 1.0")
	}
	if result.Allows(mustParse(t, "2.0")) {
		t.Error("intersection should not allow 2.0")
	}
}

func TestRange_Union(t *testing.T) {
	// >=1.0,<1.5 union >=1.5,<2.0 → >=1.0,<2.0
	r1 := &Range{
		min: ptr(mustParse(t, "1.0")), max: ptr(mustParse(t, "1.5")),
		includeMin: true, includeMax: false,
	}
	r2 := &Range{
		min: ptr(mustParse(t, "1.5")), max: ptr(mustParse(t, "2.0")),
		includeMin: true, includeMax: false,
	}
	result := r1.Union(r2)

	if !result.Allows(mustParse(t, "1.0")) {
		t.Error("union should allow 1.0")
	}
	if !result.Allows(mustParse(t, "1.5")) {
		t.Error("union should allow 1.5")
	}
	if !result.Allows(mustParse(t, "1.9")) {
		t.Error("union should allow 1.9")
	}
	if result.Allows(mustParse(t, "2.0")) {
		t.Error("union should not allow 2.0")
	}
}

func TestRange_Difference(t *testing.T) {
	// >=1.0,<2.0 minus ==1.5 → <1.5 || >1.5 within range
	r := &Range{
		min: ptr(mustParse(t, "1.0")), max: ptr(mustParse(t, "2.0")),
		includeMin: true, includeMax: false,
	}
	v := mustParse(t, "1.5")
	result := r.Difference(ExactVersion(v))

	if !result.Allows(mustParse(t, "1.0")) {
		t.Error("difference should allow 1.0")
	}
	if !result.Allows(mustParse(t, "1.4")) {
		t.Error("difference should allow 1.4")
	}
	if result.Allows(mustParse(t, "1.5")) {
		t.Error("difference should not allow 1.5")
	}
	if !result.Allows(mustParse(t, "1.6")) {
		t.Error("difference should allow 1.6")
	}
}

func TestUnion_Allows(t *testing.T) {
	// !=1.5 is <1.5 || >1.5.
	v := mustParse(t, "1.5")
	c := NewUnion(
		NewRange(nil, &v, false, false),
		NewRange(&v, nil, false, false),
	)

	if !c.Allows(mustParse(t, "1.4")) {
		t.Error("!=1.5 should allow 1.4")
	}
	if !c.Allows(mustParse(t, "1.6")) {
		t.Error("!=1.5 should allow 1.6")
	}
	if c.Allows(mustParse(t, "1.5")) {
		t.Error("!=1.5 should not allow 1.5")
	}
}

func TestEmptyConstraint(t *testing.T) {
	e := EmptyConstraint()
	if !e.IsEmpty() {
		t.Error("expected IsEmpty")
	}
	if e.Allows(mustParse(t, "1.0")) {
		t.Error("empty should not allow anything")
	}
}

func TestAnyConstraint(t *testing.T) {
	a := AnyConstraint()
	if !a.IsAny() {
		t.Error("expected IsAny")
	}
	if !a.Allows(mustParse(t, "1.0")) {
		t.Error("any should allow everything")
	}
	if !a.Allows(mustParse(t, "999.0")) {
		t.Error("any should allow everything")
	}
}

func ptr(v Version) *Version {
	return &v
}
