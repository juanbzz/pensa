package cli

import (
	"testing"

	"github.com/matryer/is"

	"pensa.sh/pensa/pkg/pep508"
	"pensa.sh/pensa/pkg/version"
)

// Plain-dep marker filtering audit.
//
// Regression: indexProvider.Dependencies only evaluated markers for
// extras-gated deps. Plain deps carrying python_version markers (e.g.
// `stripe 11.0.0` with `typing-extensions<=4.2.0; python_version < "3.7"`)
// leaked through unfiltered, producing self-contradictory per-package
// constraints that broke the resolver even when a valid solution
// existed (stripe 11.6.0 + typing-extensions 4.14.1).
//
// markerKeepsDep is the post-fix gate: returns true iff the dep's
// marker could evaluate true for at least one Python version inside the
// project's requires-python range.

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

// Non-python markers (platform, implementation) are always kept:
// at resolve time we don't know the install-time environment, so we
// can't safely filter these. Install-time marker evaluation handles
// exclusion.
func TestMarkerKeepsDep_NonPythonAlwaysKept(t *testing.T) {
	assert := is.New(t)
	pyReq := mustParseRequiresPython(t, ">=3.11,<4")

	m := mustParseMarker(t, `platform_system == "Windows"`)
	assert.True(markerKeepsDep(m, pyReq))

	m = mustParseMarker(t, `implementation_name == "pypy"`)
	assert.True(markerKeepsDep(m, pyReq))

	m = mustParseMarker(t, `sys_platform == "darwin"`)
	assert.True(markerKeepsDep(m, pyReq))
}

// Mixed markers (python + platform) are kept: we don't have a sound
// way to partially evaluate them at resolve time. False-positives
// (keeping a dep that wouldn't apply at install) are harmless; the
// resolver treats the transitive constraint as one of many possible
// deps and can route around it.
func TestMarkerKeepsDep_MixedAlwaysKept(t *testing.T) {
	assert := is.New(t)
	pyReq := mustParseRequiresPython(t, ">=3.11,<4")

	m := mustParseMarker(t, `python_version < "3.7" and platform_system == "Windows"`)
	assert.True(markerKeepsDep(m, pyReq))

	m = mustParseMarker(t, `python_version >= "3.8" or sys_platform == "linux"`)
	assert.True(markerKeepsDep(m, pyReq))
}

// Nil requires-python means keep everything — no project-level Python
// constraint to evaluate against.
func TestMarkerKeepsDep_NilRequiresPython(t *testing.T) {
	assert := is.New(t)

	m := mustParseMarker(t, `python_version < "3.7"`)
	assert.True(markerKeepsDep(m, nil))
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
