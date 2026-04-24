package cli

import (
	"testing"

	"pensa.sh/pensa/pkg/pep508"
)

// extras filtering audit.
//
// indexProvider.Dependencies uses isExtrasOnly + isRequestedExtra to
// decide which transitive deps to include during resolution. These
// helpers walk the Marker AST (replacing an earlier fragile
// text-search implementation):
//
//   - isExtrasOnly: true iff the marker's AST contains any
//     CompareMarker referencing the `extra` variable.
//
//   - isRequestedExtra: true iff Marker.Evaluate returns true for at
//     least one requested extra (with env.Extra set to that value and
//     platform/Python fields stubbed permissively). Operators other
//     than literal `extra == 'X'` (e.g. `extra != 'X'`) are handled
//     correctly because we rely on the marker's own evaluation logic.
//
// Together they produce these observable behaviors at resolve time:
//
//   marker                                       include-when-requested
//   ---------------------------------------------- ---------------------
//   (nil)                                        always
//   extra == 'a'                                 only if a ∈ requested
//   extra != 'a'                                 only if some requested
//                                                extra != 'a'
//   extra == 'a' and python_version >= '3.8'     if a ∈ requested (python
//                                                gate uses permissive env)
//   extra == 'a' or extra == 'b'                 if a OR b ∈ requested
//   python_version >= '3.8'                      always (no extras var)

// helper: parse a marker, wrap it in a Dependency so we can test the
// helpers with the actual API surface.
func depWithMarker(t *testing.T, src string) pep508.Dependency {
	t.Helper()
	m, err := pep508.ParseMarker(src)
	if err != nil {
		t.Fatalf("ParseMarker(%q): %v", src, err)
	}
	return pep508.Dependency{Name: "x", Markers: m}
}

// --- isExtrasOnly ---

func TestExtrasProp_IsExtrasOnlyDetectsExtra(t *testing.T) {
	tests := []struct {
		marker string
		want   bool
	}{
		{`extra == 'security'`, true},
		{`extra == "security"`, true},
		{`python_version >= "3.8"`, false},
		{`sys_platform == "linux"`, false},

		// Gray area: mixed conditions. The current impl says TRUE
		// (any "extra ==" anywhere). Document the observable behavior.
		{`extra == 'security' and python_version >= "3.8"`, true},
		{`python_version >= "3.8" and extra == 'security'`, true},
		{`extra == 'a' or extra == 'b'`, true},
	}
	for _, tc := range tests {
		t.Run(tc.marker, func(t *testing.T) {
			d := depWithMarker(t, tc.marker)
			if got := isExtrasOnly(d); got != tc.want {
				t.Errorf("isExtrasOnly(%q) = %v; want %v", tc.marker, got, tc.want)
			}
		})
	}
}

// Nil marker is not extras-only.
func TestExtrasProp_NilMarkerIsNotExtrasOnly(t *testing.T) {
	d := pep508.Dependency{Name: "x", Markers: nil}
	if isExtrasOnly(d) {
		t.Error("nil marker should not be treated as extras-only")
	}
}

// --- isRequestedExtra ---

func TestExtrasProp_IsRequestedExtraMatchesLiteral(t *testing.T) {
	tests := []struct {
		marker    string
		requested []string
		want      bool
	}{
		{`extra == 'security'`, []string{"security"}, true},
		{`extra == "security"`, []string{"security"}, true},
		{`extra == 'security'`, []string{"docs"}, false},
		{`extra == 'security'`, []string{"docs", "security"}, true},
		{`extra == 'security' or extra == 'docs'`, []string{"docs"}, true},
		{`extra == 'security' or extra == 'docs'`, []string{"other"}, false},

		// Empty request → always false.
		{`extra == 'security'`, nil, false},
		{`extra == 'security'`, []string{}, false},
	}
	for _, tc := range tests {
		d := depWithMarker(t, tc.marker)
		if got := isRequestedExtra(d, tc.requested); got != tc.want {
			t.Errorf("isRequestedExtra(%q, %v) = %v; want %v",
				tc.marker, tc.requested, got, tc.want)
		}
	}
}

// --- AST-walker correctness ---

// `extra != 'a'` references the extra variable, so it's extras-gated.
// Inclusion depends on the evaluated extra: with requested=['a'],
// `extra != 'a'` evaluates FALSE → exclude. With requested=['b'],
// TRUE → include.
func TestExtrasProp_ExtraNotEqualsHandled(t *testing.T) {
	d := depWithMarker(t, `extra != 'a'`)

	if !isExtrasOnly(d) {
		t.Error("`extra != 'a'` references extra — should be classified as extras-only")
	}
	if isRequestedExtra(d, []string{"a"}) {
		t.Error("`extra != 'a'` with requested=['a'] should NOT include")
	}
	if !isRequestedExtra(d, []string{"b"}) {
		t.Error("`extra != 'a'` with requested=['b'] should include")
	}
	if !isRequestedExtra(d, []string{"a", "b"}) {
		t.Error("`extra != 'a'` with requested=['a','b'] should include (b satisfies)")
	}
}

// `extra == 'a' and python_version >= '3.8'`: extras-gated because of
// the extras subexpression. permissiveEnv has PythonVersion=3.99, so
// the python_version gate evaluates true; inclusion reduces to the
// extra gate.
func TestExtrasProp_ExtraWithPythonGate(t *testing.T) {
	d := depWithMarker(t, `extra == 'a' and python_version >= "3.8"`)

	if !isExtrasOnly(d) {
		t.Error("mixed extra+python marker should be extras-gated")
	}
	if !isRequestedExtra(d, []string{"a"}) {
		t.Error("`extra == 'a' and python_version >= '3.8'` should include when 'a' requested")
	}
	if isRequestedExtra(d, []string{"b"}) {
		t.Error("should exclude when only 'b' requested")
	}
}

// A marker gated on python_version BELOW the permissive env's Python
// (3.99) evaluates false in permissiveEnv, so even if the extra
// matches, the dep isn't included — the marker's python gate wins.
//
// This is a conservative choice: we err on exclude when install-time
// Python may differ. (Documented in permissiveEnv's comment.)
func TestExtrasProp_ExtraWithUpperPythonGate(t *testing.T) {
	d := depWithMarker(t, `extra == 'a' and python_version < "3.0"`)

	if !isExtrasOnly(d) {
		t.Error("mixed extra+python marker should be extras-gated")
	}
	// permissiveEnv has PythonVersion=3.99 → python_version < 3.0 is
	// false → full marker is false → exclude regardless of extra.
	if isRequestedExtra(d, []string{"a"}) {
		t.Error("marker with impossible python gate should not include dep")
	}
}

// String-literal collision — the AST walk no longer misclassifies a
// marker whose VALUE happens to contain "extra ==".
func TestExtrasProp_StringLiteralNoCollision(t *testing.T) {
	d := depWithMarker(t, `sys_platform == "this extra == thing"`)

	if isExtrasOnly(d) {
		t.Error("marker comparing sys_platform with an arbitrary string literal " +
			"should not be classified as extras-only")
	}
}
