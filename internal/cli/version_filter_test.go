package cli

import (
	"testing"

	"pensa.sh/pensa/internal/index"
	"pensa.sh/pensa/pkg/version"
)

// filterCandidateVersions is the resolver-level version filter.
// It's called for every package every time the solver asks for
// candidate versions. Two concerns:
//
//   1. Drop non-stable versions (prereleases, dev releases) — the
//      resolver defaults to stable only, matching pip/Poetry/uv.
//   2. Drop versions whose requires-python rules out the project.
//
// Plus a fallback: if EVERY version is filtered out, return all so
// pure-prerelease packages don't break resolution.
//
// These tests cover the expected behaviors plus one known gap
// documented in the filter's comment.

// packageInfoFrom builds a *PackageInfo with synthetic FileInfo
// entries for the given versions. Each version gets one wheel file
// with an optional requires-python string.
func packageInfoFrom(t *testing.T, versions []string, requiresPython map[string]string) *index.PackageInfo {
	t.Helper()
	info := &index.PackageInfo{Name: "pkg"}
	for _, vstr := range versions {
		info.Files = append(info.Files, index.FileInfo{
			Filename:       "pkg-" + vstr + "-py3-none-any.whl",
			RequiresPython: requiresPython[vstr],
		})
	}
	return info
}

// --- Stable-only filtering ---

// Mix of stable + prereleases → only stable returned.
func TestVersionFilterProp_StableOnly(t *testing.T) {
	info := packageInfoFrom(t, []string{"1.0", "1.0rc1", "1.5", "2.0a1", "2.0"}, nil)
	got := filterCandidateVersions(info, nil, nil)

	wantSet := map[string]bool{"1.0": true, "1.5": true, "2.0": true}
	for _, v := range got {
		if !wantSet[v.String()] {
			t.Errorf("unexpected version in filtered output: %s", v)
		}
	}
	if len(got) != len(wantSet) {
		t.Errorf("got %d versions, want %d", len(got), len(wantSet))
	}
}

// Only prereleases → fallback returns all.
func TestVersionFilterProp_OnlyPrereleasesFallback(t *testing.T) {
	info := packageInfoFrom(t, []string{"0.5a1", "0.5b2", "0.5rc1"}, nil)
	got := filterCandidateVersions(info, nil, nil)

	if len(got) != 3 {
		t.Errorf("fallback should return all 3 prereleases, got %d", len(got))
	}
}

// Dev releases are also filtered as non-stable.
func TestVersionFilterProp_DevReleaseFiltered(t *testing.T) {
	info := packageInfoFrom(t, []string{"1.0", "1.0.dev0", "1.0.dev5", "2.0"}, nil)
	got := filterCandidateVersions(info, nil, nil)

	gotSet := map[string]bool{}
	for _, v := range got {
		gotSet[v.String()] = true
	}
	if !gotSet["1.0"] || !gotSet["2.0"] {
		t.Errorf("stable versions missing from output: %v", gotSet)
	}
	if gotSet["1.0.dev0"] || gotSet["1.0.dev5"] {
		t.Errorf("dev releases should be filtered out: %v", gotSet)
	}
}

// --- requires-python filtering ---

// requires-python incompatibility excludes the version.
func TestVersionFilterProp_RequiresPythonExcludes(t *testing.T) {
	info := packageInfoFrom(t,
		[]string{"1.0", "2.0", "3.0"},
		map[string]string{
			"1.0": ">=3.7",
			"2.0": ">=3.12", // requires Python >= 3.12
			"3.0": ">=3.8",
		})
	projectPython, err := version.ParseConstraint(">=3.8,<3.11")
	if err != nil {
		t.Fatal(err)
	}
	// Caller passes a function that answers "does package's
	// requires-python allow project's target?". For this test we
	// implement it inline using the real helper logic.
	allows := func(pkgRp string) bool {
		return pythonRangesOverlap(projectPython, pkgRp)
	}
	got := filterCandidateVersions(info, projectPython, allows)

	// 2.0 requires >=3.12 which doesn't intersect with project's 3.8-3.10.
	// 1.0 and 3.0 fine (>=3.7, >=3.8 both accommodate 3.8-3.10).
	gotSet := map[string]bool{}
	for _, v := range got {
		gotSet[v.String()] = true
	}
	if !gotSet["1.0"] {
		t.Error("1.0 should be included (>=3.7)")
	}
	if gotSet["2.0"] {
		t.Error("2.0 should be excluded (requires Python >=3.12)")
	}
	if !gotSet["3.0"] {
		t.Error("3.0 should be included (>=3.8)")
	}
}

// Missing requires-python → version is kept (fallback: trust the version).
func TestVersionFilterProp_MissingRequiresPythonKept(t *testing.T) {
	info := packageInfoFrom(t,
		[]string{"1.0"},
		map[string]string{}) // no requires-python declared
	projectPython, err := version.ParseConstraint(">=3.8")
	if err != nil {
		t.Fatal(err)
	}
	allows := func(pkgRp string) bool {
		return pythonRangesOverlap(projectPython, pkgRp)
	}
	got := filterCandidateVersions(info, projectPython, allows)
	if len(got) != 1 || got[0].String() != "1.0" {
		t.Errorf("version without requires-python should be kept; got %v", got)
	}
}

// --- Known gap: exact prerelease pin ---

// DOCUMENTED BUG: if the user pins `pkg==1.0rc1` and the package has
// stable 1.0 released, filterCandidateVersions drops 1.0rc1 because
// it's not stable. The fallback only fires when ALL versions are
// prereleases, which isn't the case here. The solver then can't
// satisfy the exact-pin constraint and reports NoVersions.
//
// This test documents the current behavior. A proper fix requires
// inspecting user constraints (or an `--pre` opt-in) to selectively
// allow prereleases that are explicitly requested.
func TestVersionFilterProp_GapExactPrereleasePin(t *testing.T) {
	info := packageInfoFrom(t,
		[]string{"0.9", "1.0rc1", "1.0"},
		nil)
	got := filterCandidateVersions(info, nil, nil)

	gotSet := map[string]bool{}
	for _, v := range got {
		gotSet[v.String()] = true
	}
	// Current (buggy) behavior: rc1 is missing.
	if gotSet["1.0rc1"] {
		t.Error("1.0rc1 is currently filtered out; this test flips when we add " +
			"opt-in-prerelease support. If you see this failure, update the " +
			"fallback logic docs.")
	}
	// Stable versions are there.
	if !gotSet["0.9"] || !gotSet["1.0"] {
		t.Errorf("stable versions missing: %v", gotSet)
	}
}
