package cli

import (
	"runtime"
	"testing"

	"github.com/matryer/is"

	"pensa.sh/pensa/pkg/pep508"
	"pensa.sh/pensa/pkg/version"
)

// Plain-dep marker filtering audit.
//
// Two-part contract on markerKeepsDep:
//
// 1. Drop deps whose python_version marker is unsatisfiable across
//    the project's requires-python range. Surfaced by stripe 11.0.0:
//    typing-extensions<=4.2.0 ; python_version < "3.7" — irrelevant
//    on Python 3.11+ projects but slipped through unfiltered, then
//    produced a self-contradictory constraint pair.
//
// 2. Drop deps whose platform marker (sys_platform, platform_system,
//    implementation_name, etc.) doesn't match the host. Surfaced by
//    win-precise-time on macOS — declared as
//    "win-precise-time ; sys_platform == \"win32\"" by a transitive,
//    leaked into the lock, then failed at install with a clang
//    error trying to build a Windows-only sdist.
//
// Both classes must drop. Mixed markers evaluate the conjunction /
// disjunction as a whole, sweeping python_version while holding the
// platform fixed to the host.

// hostSysPlatform returns the sys_platform value for the test host.
// PEP 508 spec: sys_platform = sys.platform, which is "darwin",
// "linux", or "win32" (Go's runtime.GOOS uses "windows" — needs
// translation).
func hostSysPlatform() string {
	if runtime.GOOS == "windows" {
		return "win32"
	}
	return runtime.GOOS
}

// foreignSysPlatform returns a sys_platform value that's NEVER the
// host, so tests for platform-mismatched markers work on any CI.
func foreignSysPlatform() string {
	if runtime.GOOS == "windows" {
		return "darwin"
	}
	return "win32"
}

func mustParseMarker(t *testing.T, s string) pep508.Marker {
	t.Helper()
	m, err := pep508.ParseMarker(s)
	if err != nil {
		t.Fatalf("ParseMarker(%q): %v", s, err)
	}
	return m
}

func mustParseRequiresPython(t *testing.T, s string) version.Constraint {
	t.Helper()
	c, err := version.ParseConstraint(s)
	if err != nil {
		t.Fatalf("ParseConstraint(%q): %v", s, err)
	}
	return c
}

// Nil / empty marker means the dep is always kept.
func TestMarkerKeepsDep_NilAndAny(t *testing.T) {
	assert := is.New(t)
	pyReq := mustParseRequiresPython(t, ">=3.11,<4")

	assert.True(markerKeepsDep(nil, pyReq))
	assert.True(markerKeepsDep(pep508.AnyMarker{}, pyReq))
}

// python_version markers outside the project range are dropped.
func TestMarkerKeepsDep_PythonOutOfRange(t *testing.T) {
	assert := is.New(t)
	pyReq := mustParseRequiresPython(t, ">=3.11,<4")

	// The stripe 11.0.0 case.
	m := mustParseMarker(t, `python_version < "3.7"`)
	assert.True(!markerKeepsDep(m, pyReq))

	// Exact older version.
	m = mustParseMarker(t, `python_version == "3.9"`)
	assert.True(!markerKeepsDep(m, pyReq))
}

// python_version markers inside or overlapping the project range are kept.
func TestMarkerKeepsDep_PythonInRange(t *testing.T) {
	assert := is.New(t)
	pyReq := mustParseRequiresPython(t, ">=3.11,<4")

	m := mustParseMarker(t, `python_version >= "3.7"`)
	assert.True(markerKeepsDep(m, pyReq))

	m = mustParseMarker(t, `python_version >= "3.11"`)
	assert.True(markerKeepsDep(m, pyReq))

	// Partial overlap: some of the project range satisfies.
	m = mustParseMarker(t, `python_version < "3.12"`)
	assert.True(markerKeepsDep(m, pyReq))
}

// Platform markers that match the host are kept.
func TestMarkerKeepsDep_PlatformMatchKept(t *testing.T) {
	assert := is.New(t)
	pyReq := mustParseRequiresPython(t, ">=3.11,<4")

	m := mustParseMarker(t, `sys_platform == "`+hostSysPlatform()+`"`)
	assert.True(markerKeepsDep(m, pyReq))

	m = mustParseMarker(t, `implementation_name == "cpython"`)
	assert.True(markerKeepsDep(m, pyReq))
}

// Platform markers that don't match the host are dropped — the
// win-precise-time case.
func TestMarkerKeepsDep_PlatformMismatchDropped(t *testing.T) {
	assert := is.New(t)
	pyReq := mustParseRequiresPython(t, ">=3.11,<4")

	m := mustParseMarker(t, `sys_platform == "`+foreignSysPlatform()+`"`)
	assert.True(!markerKeepsDep(m, pyReq))

	m = mustParseMarker(t, `implementation_name == "pypy"`)
	assert.True(!markerKeepsDep(m, pyReq))
}

// AND-mixed markers: dropped when EITHER clause is unsatisfiable.
// Conjunction short-circuits: a foreign-platform marker AND'd with
// anything is false everywhere.
func TestMarkerKeepsDep_MixedAndDroppedWhenEitherFails(t *testing.T) {
	assert := is.New(t)
	pyReq := mustParseRequiresPython(t, ">=3.11,<4")

	// Both clauses unsatisfiable.
	m := mustParseMarker(t, `python_version < "3.7" and sys_platform == "`+foreignSysPlatform()+`"`)
	assert.True(!markerKeepsDep(m, pyReq))

	// Python in range but platform foreign — still drop (AND short-circuit).
	m = mustParseMarker(t, `python_version >= "3.11" and sys_platform == "`+foreignSysPlatform()+`"`)
	assert.True(!markerKeepsDep(m, pyReq))
}

// OR-mixed markers: kept when EITHER clause is satisfiable.
func TestMarkerKeepsDep_MixedOrKeptWhenEitherSucceeds(t *testing.T) {
	assert := is.New(t)
	pyReq := mustParseRequiresPython(t, ">=3.11,<4")

	// Foreign platform OR'd with in-range python — keep.
	m := mustParseMarker(t, `python_version >= "3.8" or sys_platform == "`+foreignSysPlatform()+`"`)
	assert.True(markerKeepsDep(m, pyReq))

	// Out-of-range python OR'd with host platform — keep.
	m = mustParseMarker(t, `python_version < "3.7" or sys_platform == "`+hostSysPlatform()+`"`)
	assert.True(markerKeepsDep(m, pyReq))
}

// Nil requires-python falls back to a wide python sweep (3.0–3.99).
// python_version markers are kept when at least one sample matches;
// platform markers still get filtered against the host.
func TestMarkerKeepsDep_NilRequiresPython(t *testing.T) {
	assert := is.New(t)

	// 3.0 sample matches, so the dep applies on some Python — keep.
	m := mustParseMarker(t, `python_version < "3.7"`)
	assert.True(markerKeepsDep(m, nil))

	// Platform mismatch still drops even with no pyReq.
	m = mustParseMarker(t, `sys_platform == "`+foreignSysPlatform()+`"`)
	assert.True(!markerKeepsDep(m, nil))
}

// Projects with no upper bound (`>=3.11`) are the most common
// requires-python shape in the wild; must still filter out-of-range
// markers.
func TestMarkerKeepsDep_UnboundedUpper(t *testing.T) {
	assert := is.New(t)
	pyReq := mustParseRequiresPython(t, ">=3.11")

	// Outside range — must drop.
	m := mustParseMarker(t, `python_version < "3.7"`)
	assert.True(!markerKeepsDep(m, pyReq))

	// In range — must keep.
	m = mustParseMarker(t, `python_version >= "3.11"`)
	assert.True(markerKeepsDep(m, pyReq))
}

// Caret and tilde ranges (`^3.10`, `~=3.11`) are common in Poetry-
// style pyprojects. Same filtering expected as explicit ranges.
func TestMarkerKeepsDep_CaretAndTildeRanges(t *testing.T) {
	assert := is.New(t)

	m := mustParseMarker(t, `python_version < "3.7"`)

	assert.True(!markerKeepsDep(m, mustParseRequiresPython(t, "^3.10")))
	assert.True(!markerKeepsDep(m, mustParseRequiresPython(t, "~=3.11")))
}
