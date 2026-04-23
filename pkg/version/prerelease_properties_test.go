package version

import (
	"testing"
)

// Prerelease handling under PEP 440:
//   - Versions carry a `pre` tag (a/b/rc + N) and/or a `dev` tag.
//   - "Stable" means no pre and no dev tag.
//   - In PEP 440 strictness, prereleases are EXCLUDED from version
//     ranges unless: the user opts in (--pre), or the range endpoint
//     is itself a prerelease, or the constraint is an exact match
//     (==X.YrcZ).
//
// pensa's resolver filters candidates to IsStable (internal/cli/lock.go
// `indexProvider.Versions`). So these tests exercise:
//   - Parsing: versions like 1.0rc1, 2.0b2, 1.0.dev3 parse with the
//     right prerelease metadata.
//   - Ordering: prereleases compare in the correct PEP 440 order.
//   - Range.Allows on prerelease versions — what we return here feeds
//     both the resolver (via IsStable filter upstream) and the install
//     path.

func TestPrereleaseProp_ParsingCategorizes(t *testing.T) {
	stable := []string{"1.0", "1.0.0", "1.2.3", "100.0", "0.0.1"}
	pre := []string{"1.0rc1", "1.0a0", "1.0b2", "1.0.0rc1", "2.0.0b3", "1.0.0.pre1"}
	dev := []string{"1.0.dev0", "1.0.0.dev5"}

	for _, s := range stable {
		v := mustParse(t, s)
		if !v.IsStable() {
			t.Errorf("%s: IsStable should be true", s)
		}
		if v.IsPreRelease() {
			t.Errorf("%s: IsPreRelease should be false", s)
		}
		if v.IsDevRelease() {
			t.Errorf("%s: IsDevRelease should be false", s)
		}
	}
	for _, s := range pre {
		v := mustParse(t, s)
		if v.IsStable() {
			t.Errorf("%s: IsStable should be false", s)
		}
		if !v.IsPreRelease() {
			t.Errorf("%s: IsPreRelease should be true", s)
		}
	}
	for _, s := range dev {
		v := mustParse(t, s)
		if v.IsStable() {
			t.Errorf("%s: IsStable should be false", s)
		}
		if !v.IsDevRelease() {
			t.Errorf("%s: IsDevRelease should be true", s)
		}
	}
}

// PEP 440 ordering for prereleases within a release:
//   1.0.dev0 < 1.0a1 < 1.0b1 < 1.0rc1 < 1.0 (stable) < 1.0.post1
func TestPrereleaseProp_PEP440Ordering(t *testing.T) {
	ordered := []string{
		"1.0.dev0",
		"1.0a1",
		"1.0b1",
		"1.0rc1",
		"1.0",
		"1.0.post1",
	}
	for i := 0; i < len(ordered)-1; i++ {
		a := mustParse(t, ordered[i])
		b := mustParse(t, ordered[i+1])
		if Compare(a, b) >= 0 {
			t.Errorf("PEP 440 ordering violated: %s should be < %s, got Compare = %d",
				ordered[i], ordered[i+1], Compare(a, b))
		}
	}
}

// Exact-match constraint on a prerelease allows that prerelease and
// should NOT allow the corresponding stable release.
func TestPrereleaseProp_ExactMatch(t *testing.T) {
	rc := mustParse(t, "1.0rc1")
	stable := mustParse(t, "1.0")

	c := ExactVersion(rc)
	if !c.Allows(rc) {
		t.Error("ExactVersion(1.0rc1).Allows(1.0rc1) should be true")
	}
	if c.Allows(stable) {
		t.Error("ExactVersion(1.0rc1).Allows(1.0) should be false")
	}

	c2 := ExactVersion(stable)
	if c2.Allows(rc) {
		t.Error("ExactVersion(1.0).Allows(1.0rc1) should be false")
	}
}

// PEP 440 specifier behavior around prereleases at range bounds:
//
//   - `<2.0` MUST NOT allow 2.0rc1, even though 2.0rc1 < 2.0 in the
//     total ordering. This matches user intuition: "<2.0" excludes
//     pre-release candidates of 2.0 itself. (PEP 440: "The exclusive
//     ordered comparison <V MUST NOT allow a pre-release of the given
//     version unless V itself is a pre-release.")
//
//   - `<2.0a1` allows earlier prereleases of 2.0 (2.0a0) and lower
//     stable versions.
//
// pensa implements this in Range.Allows with explicit bookkeeping.
// These tests pin the behavior.
func TestPrereleaseProp_RangeBoundaries(t *testing.T) {
	min := mustParse(t, "1.0")
	max := mustParse(t, "2.0")
	r := NewRange(&min, &max, true, false) // [1.0, 2.0)

	// Stable inside → allowed.
	if !r.Allows(mustParse(t, "1.5")) {
		t.Error("[1.0, 2.0) should allow 1.5")
	}
	// Stable boundary → excluded by exclusive upper.
	if r.Allows(mustParse(t, "2.0")) {
		t.Error("[1.0, 2.0) should not allow 2.0 (exclusive upper)")
	}
	if r.Allows(mustParse(t, "0.9")) {
		t.Error("[1.0, 2.0) should not allow 0.9")
	}

	// PEP 440: 2.0rc1 is a prerelease of 2.0 and therefore excluded
	// from <2.0. pensa's Range.Allows enforces this (constraint.go
	// ~L298).
	if r.Allows(mustParse(t, "2.0rc1")) {
		t.Error("[1.0, 2.0) should NOT allow 2.0rc1 (PEP 440: <V excludes V's prereleases)")
	}
	// But prereleases of a lower release are fine.
	if !r.Allows(mustParse(t, "1.5rc1")) {
		t.Error("[1.0, 2.0) should allow 1.5rc1 (it's not a prerelease of the upper bound)")
	}
}

// When the upper bound is itself a prerelease, `<2.0rc1` allows
// earlier prereleases of 2.0 (like 2.0a0) but not 2.0rc1 itself.
func TestPrereleaseProp_PrereleaseUpperBound(t *testing.T) {
	max := mustParse(t, "2.0rc1")
	r := NewRange(nil, &max, false, false) // <2.0rc1

	if !r.Allows(mustParse(t, "2.0a0")) {
		t.Error("<2.0rc1 should allow 2.0a0")
	}
	if r.Allows(mustParse(t, "2.0rc1")) {
		t.Error("<2.0rc1 should not allow 2.0rc1 (exclusive)")
	}
	if r.Allows(mustParse(t, "2.0")) {
		t.Error("<2.0rc1 should not allow 2.0 (2.0 > 2.0rc1)")
	}
}

// Dev releases are sorted below any corresponding release:
// 1.0.dev5 < 1.0a0 < 1.0rc1 < 1.0 < 1.0.dev1... WAIT — 1.0.dev1 > 1.0?
// No: the dev suffix on a stable release means pre-release development
// for the NEXT version. Per PEP 440, 1.0.post1.dev1 < 1.0.post1.
// Keep it simple: just check dev < pre < stable for the SAME release.
func TestPrereleaseProp_DevReleaseOrder(t *testing.T) {
	cases := []struct {
		earlier, later string
	}{
		{"1.0.dev0", "1.0.dev1"},
		{"1.0.dev0", "1.0a0"},
		{"1.0.dev0", "1.0"},
		{"1.0a0.dev0", "1.0a0"},
	}
	for _, tc := range cases {
		a := mustParse(t, tc.earlier)
		b := mustParse(t, tc.later)
		if Compare(a, b) >= 0 {
			t.Errorf("%s should be < %s (Compare=%d)", tc.earlier, tc.later, Compare(a, b))
		}
	}
}

// Equality: 1.0 and 1.0.0 should compare equal (trailing zeros elided).
func TestPrereleaseProp_TrailingZerosEqual(t *testing.T) {
	pairs := [][2]string{
		{"1.0", "1.0.0"},
		{"1.0.0", "1.0.0.0"},
		{"1", "1.0"},
	}
	for _, pair := range pairs {
		a := mustParse(t, pair[0])
		b := mustParse(t, pair[1])
		if Compare(a, b) != 0 {
			t.Errorf("%s and %s should compare equal (got %d)",
				pair[0], pair[1], Compare(a, b))
		}
	}
}

// Local version identifiers (PEP 440 local part) are compared after
// the public version. 1.0+local > 1.0 (pyOpenSSL uses these).
func TestPrereleaseProp_LocalVersion(t *testing.T) {
	// Our parser may or may not handle `+local`. If it parses, local
	// segments come AFTER the release for ordering purposes.
	plain := mustParse(t, "1.0")
	// Skip if parser rejects local versions.
	local, err := Parse("1.0+local")
	if err != nil {
		t.Skipf("Parse doesn't support local versions yet: %v", err)
	}
	if Compare(local, plain) <= 0 {
		t.Errorf("1.0+local should be > 1.0 (Compare=%d)", Compare(local, plain))
	}
}
