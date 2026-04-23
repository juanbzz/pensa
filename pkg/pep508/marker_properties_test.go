package pep508

import (
	"testing"
)

// markerEnvCorpus returns several realistic environments used by the
// property tests below. Coverage includes mainstream platforms, fringe
// Python versions that exercise PEP 440 vs string-comparison semantics
// (e.g. 3.10 vs 3.9), and the optional "extra" marker.
func markerEnvCorpus() []Environment {
	return []Environment{
		// Linux CPython 3.11 (the baseline in TestMarker_Evaluate).
		{
			PythonVersion:                "3.11", PythonFullVersion: "3.11.4",
			OSName: "posix", SysPlatform: "linux",
			PlatformSystem: "Linux", PlatformMachine: "x86_64",
			PlatformPythonImplementation: "CPython", ImplementationName: "cpython",
			ImplementationVersion: "3.11.4",
		},
		// macOS, arm64, Python 3.12 (covers darwin + arm).
		{
			PythonVersion:                "3.12", PythonFullVersion: "3.12.1",
			OSName: "posix", SysPlatform: "darwin",
			PlatformSystem: "Darwin", PlatformMachine: "arm64",
			PlatformPythonImplementation: "CPython", ImplementationName: "cpython",
			ImplementationVersion: "3.12.1",
		},
		// Windows, Python 3.9.
		{
			PythonVersion:                "3.9", PythonFullVersion: "3.9.18",
			OSName: "nt", SysPlatform: "win32",
			PlatformSystem: "Windows", PlatformMachine: "AMD64",
			PlatformPythonImplementation: "CPython", ImplementationName: "cpython",
			ImplementationVersion: "3.9.18",
		},
		// Python 3.10 — specifically trips PEP 440 vs string compare
		// because "3.10" < "3.9" lexically but 3.10 > 3.9 as versions.
		{
			PythonVersion:                "3.10", PythonFullVersion: "3.10.12",
			OSName: "posix", SysPlatform: "linux",
			PlatformSystem: "Linux", PlatformMachine: "x86_64",
			PlatformPythonImplementation: "CPython", ImplementationName: "cpython",
			ImplementationVersion: "3.10.12",
		},
		// extra="docs" — exercises the `extra == 'docs'` style markers.
		{
			PythonVersion: "3.11", PythonFullVersion: "3.11.4",
			OSName: "posix", SysPlatform: "linux",
			PlatformPythonImplementation: "CPython",
			Extra:                        "docs",
		},
	}
}

// markerExprs is a corpus of marker-expression strings covering each
// operator, both variable positions (left and right), all legacy aliases,
// and nested AND/OR/parens.
func markerExprs() []string {
	return []string{
		// Basic comparisons (each op).
		`python_version == "3.11"`,
		`python_version != "3.11"`,
		`python_version < "3.11"`,
		`python_version <= "3.11"`,
		`python_version > "3.11"`,
		`python_version >= "3.11"`,
		`python_version ~= "3.11"`,
		`python_full_version === "3.11.4"`,

		// PEP 440 gotcha versions.
		`python_version >= "3.10"`,
		`python_version > "3.9"`,

		// String-valued markers.
		`sys_platform == "linux"`,
		`platform_machine == "arm64"`,
		`implementation_name != "pypy"`,

		// in / not in.
		`"linux" in sys_platform`,
		`"darwin" not in sys_platform`,

		// Variable-on-right placements.
		`"3.11" == python_version`,
		`"cpython" == implementation_name`,

		// Logical operators + nesting.
		`python_version >= "3.8" and sys_platform == "linux"`,
		`python_version < "3.0" or sys_platform == "linux"`,
		`os_name == "posix" and (python_version >= "3.10" or sys_platform == "darwin")`,

		// Legacy aliases.
		`os.name == "posix"`,
		`sys.platform == "linux"`,

		// extra-gated clauses.
		`extra == "docs"`,
		`python_version >= "3.8" and extra == "docs"`,

		// AnyMarker (empty string parses to AnyMarker).
		``,
	}
}

// evalAllEnvs runs marker against every env in the corpus, returning a
// bool vector. Two markers are "environmentally equivalent" iff their
// bool vectors match — used as extensional equality in the tests below.
func evalAllEnvs(m Marker) []bool {
	envs := markerEnvCorpus()
	out := make([]bool, len(envs))
	for i, env := range envs {
		out[i] = m.Evaluate(env)
	}
	return out
}

func boolVecEqual(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- Round-trip ---

// ParseMarker(m.String()) evaluates identically to m across all envs.
func TestMarkerProp_StringRoundTrip(t *testing.T) {
	for _, src := range markerExprs() {
		t.Run(src, func(t *testing.T) {
			m, err := ParseMarker(src)
			if err != nil {
				t.Fatalf("ParseMarker(%q): %v", src, err)
			}
			repr := m.String()
			reparsed, err := ParseMarker(repr)
			if err != nil {
				t.Fatalf("ParseMarker(%q) after stringifying %q: %v", repr, src, err)
			}
			before := evalAllEnvs(m)
			after := evalAllEnvs(reparsed)
			if !boolVecEqual(before, after) {
				t.Errorf("round-trip changed evaluation:\n  src=%q\n  repr=%q\n  before=%v\n  after=%v",
					src, repr, before, after)
			}
		})
	}
}

// --- Operator complementarity ---

// For comparison operators that partition the line (==/!=, </>=, <=/>),
// exactly one holds per env. Tested on markers we can pair up by
// flipping the operator.
func TestMarkerProp_OperatorComplements(t *testing.T) {
	pairs := [][2]string{
		{`python_version == "3.11"`, `python_version != "3.11"`},
		{`python_version < "3.11"`, `python_version >= "3.11"`},
		{`python_version <= "3.11"`, `python_version > "3.11"`},
		{`sys_platform == "linux"`, `sys_platform != "linux"`},
	}
	for _, pair := range pairs {
		t.Run(pair[0]+" / "+pair[1], func(t *testing.T) {
			a, err := ParseMarker(pair[0])
			if err != nil {
				t.Fatal(err)
			}
			b, err := ParseMarker(pair[1])
			if err != nil {
				t.Fatal(err)
			}
			for _, env := range markerEnvCorpus() {
				if a.Evaluate(env) == b.Evaluate(env) {
					t.Errorf("%q and %q both %v in env %+v",
						pair[0], pair[1], a.Evaluate(env), env)
				}
			}
		})
	}
}

// `in` and `not in` should always produce complementary results.
func TestMarkerProp_InNotInComplements(t *testing.T) {
	pairs := [][2]string{
		{`"linux" in sys_platform`, `"linux" not in sys_platform`},
		{`"win" in sys_platform`, `"win" not in sys_platform`},
		{`"3.1" in python_version`, `"3.1" not in python_version`},
	}
	for _, pair := range pairs {
		a, err := ParseMarker(pair[0])
		if err != nil {
			t.Fatal(err)
		}
		b, err := ParseMarker(pair[1])
		if err != nil {
			t.Fatal(err)
		}
		for _, env := range markerEnvCorpus() {
			if a.Evaluate(env) == b.Evaluate(env) {
				t.Errorf("`%s` and `%s` both %v in env %+v",
					pair[0], pair[1], a.Evaluate(env), env)
			}
		}
	}
}

// --- AND/OR semantics ---

// `(a and b).Evaluate == a.Evaluate && b.Evaluate` for every env.
func TestMarkerProp_AndIsPointwiseAnd(t *testing.T) {
	combos := [][2]string{
		{`python_version >= "3.8"`, `sys_platform == "linux"`},
		{`python_version < "3.0"`, `os_name == "nt"`},
		{`platform_machine == "arm64"`, `sys_platform == "darwin"`},
	}
	for _, pair := range combos {
		src := pair[0] + " and " + pair[1]
		whole, err := ParseMarker(src)
		if err != nil {
			t.Fatal(err)
		}
		left, _ := ParseMarker(pair[0])
		right, _ := ParseMarker(pair[1])
		for _, env := range markerEnvCorpus() {
			want := left.Evaluate(env) && right.Evaluate(env)
			if got := whole.Evaluate(env); got != want {
				t.Errorf("(%s).Evaluate = %v; want %v in env %+v",
					src, got, want, env)
			}
		}
	}
}

// `(a or b).Evaluate == a.Evaluate || b.Evaluate` for every env.
func TestMarkerProp_OrIsPointwiseOr(t *testing.T) {
	combos := [][2]string{
		{`python_version < "3.0"`, `sys_platform == "linux"`},
		{`os_name == "nt"`, `platform_machine == "arm64"`},
	}
	for _, pair := range combos {
		src := pair[0] + " or " + pair[1]
		whole, err := ParseMarker(src)
		if err != nil {
			t.Fatal(err)
		}
		left, _ := ParseMarker(pair[0])
		right, _ := ParseMarker(pair[1])
		for _, env := range markerEnvCorpus() {
			want := left.Evaluate(env) || right.Evaluate(env)
			if got := whole.Evaluate(env); got != want {
				t.Errorf("(%s).Evaluate = %v; want %v in env %+v",
					src, got, want, env)
			}
		}
	}
}

// --- PEP 440 semantics for version markers ---

// python_version uses PEP 440 comparison, not lexical: "3.10" > "3.9"
// as versions even though "3.10" < "3.9" as strings.
func TestMarkerProp_PythonVersionPEP440(t *testing.T) {
	env := Environment{PythonVersion: "3.10"}

	cases := []struct {
		marker string
		want   bool
		reason string
	}{
		{`python_version > "3.9"`, true, "3.10 > 3.9 by PEP 440"},
		{`python_version > "3.09"`, true, "3.10 > 3.09 by PEP 440"},
		{`python_version >= "3.10"`, true, "3.10 >= 3.10"},
		{`python_version < "3.11"`, true, "3.10 < 3.11"},
		{`python_version == "3.10"`, true, "3.10 == 3.10"},
		{`python_version < "3.9"`, false, "3.10 not < 3.9 (would be true under lexical)"},
	}
	for _, tc := range cases {
		t.Run(tc.marker, func(t *testing.T) {
			m, err := ParseMarker(tc.marker)
			if err != nil {
				t.Fatal(err)
			}
			if got := m.Evaluate(env); got != tc.want {
				t.Errorf("%q with PythonVersion=3.10: got %v, want %v (%s)",
					tc.marker, got, tc.want, tc.reason)
			}
		})
	}
}

// --- Legacy aliases ---

// os.name/sys.platform/etc should resolve identically to their
// underscored forms.
func TestMarkerProp_LegacyAliases(t *testing.T) {
	pairs := [][2]string{
		{`os.name == "posix"`, `os_name == "posix"`},
		{`sys.platform == "linux"`, `sys_platform == "linux"`},
		{`platform.version == "test"`, `platform_version == "test"`},
		{`platform.machine == "arm64"`, `platform_machine == "arm64"`},
		{`platform.python_implementation == "CPython"`, `platform_python_implementation == "CPython"`},
		{`python_implementation == "CPython"`, `platform_python_implementation == "CPython"`},
	}
	for _, pair := range pairs {
		t.Run(pair[0], func(t *testing.T) {
			a, err := ParseMarker(pair[0])
			if err != nil {
				t.Fatal(err)
			}
			b, err := ParseMarker(pair[1])
			if err != nil {
				t.Fatal(err)
			}
			if !boolVecEqual(evalAllEnvs(a), evalAllEnvs(b)) {
				t.Errorf("legacy alias %q and canonical %q disagree across envs",
					pair[0], pair[1])
			}
		})
	}
}
