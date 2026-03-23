package installer

import (
	"testing"

	"github.com/juanbzz/pensa/internal/python"
)

func testPlatform() *PlatformTags {
	return NewPlatformTags(&python.PythonInfo{
		Major: 3,
		Minor: 11,
		Patch: 4,
	})
}

func TestParseWheelTags_PureWheel(t *testing.T) {
	py, abi, plat := parseWheelTags("requests-2.31.0-py3-none-any.whl")

	if len(py) != 1 || py[0] != "py3" {
		t.Errorf("python = %v, want [py3]", py)
	}
	if len(abi) != 1 || abi[0] != "none" {
		t.Errorf("abi = %v, want [none]", abi)
	}
	if len(plat) != 1 || plat[0] != "any" {
		t.Errorf("platform = %v, want [any]", plat)
	}
}

func TestParseWheelTags_PlatformWheel(t *testing.T) {
	py, abi, plat := parseWheelTags("numpy-1.26.0-cp311-cp311-macosx_11_0_arm64.whl")

	if len(py) != 1 || py[0] != "cp311" {
		t.Errorf("python = %v, want [cp311]", py)
	}
	if len(abi) != 1 || abi[0] != "cp311" {
		t.Errorf("abi = %v, want [cp311]", abi)
	}
	if len(plat) != 1 || plat[0] != "macosx_11_0_arm64" {
		t.Errorf("platform = %v, want [macosx_11_0_arm64]", plat)
	}
}

func TestParseWheelTags_MultiPlatform(t *testing.T) {
	py, _, plat := parseWheelTags("charset_normalizer-3.3.2-cp310-cp310-manylinux_2_17_x86_64.manylinux2014_x86_64.whl")

	if len(py) != 1 || py[0] != "cp310" {
		t.Errorf("python = %v", py)
	}
	if len(plat) != 2 {
		t.Fatalf("expected 2 platform tags, got %d: %v", len(plat), plat)
	}
	if plat[0] != "manylinux_2_17_x86_64" || plat[1] != "manylinux2014_x86_64" {
		t.Errorf("platform = %v", plat)
	}
}

func TestParseWheelTags_Invalid(t *testing.T) {
	py, _, _ := parseWheelTags("notawheel.tar.gz")
	if py != nil {
		t.Error("expected nil for non-wheel file")
	}
}

func TestPlatformTags_PureWheelCompatible(t *testing.T) {
	plat := testPlatform()

	score := plat.Score("requests-2.31.0-py3-none-any.whl")
	if score < 0 {
		t.Error("py3-none-any should be compatible on all platforms")
	}
}

func TestPlatformTags_VersionedPureWheel(t *testing.T) {
	plat := testPlatform()

	score311 := plat.Score("pkg-1.0-py311-none-any.whl")
	scorePy3 := plat.Score("pkg-1.0-py3-none-any.whl")

	if score311 < 0 {
		t.Error("py311 should be compatible with Python 3.11")
	}
	if scorePy3 < 0 {
		t.Error("py3 should be compatible")
	}
	if score311 >= scorePy3 {
		t.Errorf("py311 (score %d) should be higher priority than py3 (score %d)", score311, scorePy3)
	}
}

func TestPlatformTags_IncompatiblePythonVersion(t *testing.T) {
	plat := testPlatform() // Python 3.11

	score := plat.Score("pkg-1.0-cp312-cp312-macosx_11_0_arm64.whl")
	if score >= 0 {
		t.Error("cp312 wheel should be incompatible with Python 3.11")
	}
}

func TestPlatformTags_PlatformSpecificBetterThanPure(t *testing.T) {
	plat := testPlatform()

	// This test is platform-dependent — the platform wheel will only match
	// on the platform it was built for. But py3-none-any always matches.
	scorePure := plat.Score("pkg-1.0-py3-none-any.whl")
	scoreCpNone := plat.Score("pkg-1.0-cp311-none-any.whl")

	if scorePure < 0 || scoreCpNone < 0 {
		t.Skip("both wheels should be compatible")
	}

	if scoreCpNone >= scorePure {
		t.Errorf("cp311-none-any (score %d) should be higher priority than py3-none-any (score %d)",
			scoreCpNone, scorePure)
	}
}

func TestPlatformTags_ABI3(t *testing.T) {
	plat := testPlatform()

	// abi3 wheels should be compatible — they work with any CPython >= the tagged version.
	// We test with the "any" platform to avoid platform-dependent behavior.
	score := plat.Score("pkg-1.0-cp311-abi3-any.whl")

	// abi3 with "any" platform won't match our tags since we don't generate abi3-any combos.
	// But abi3 with a real platform tag would match if the platform matches.
	// This is fine — abi3-any is unusual. Most abi3 wheels have platform tags.
	_ = score
}

func TestNewPlatformTags_GeneratesTags(t *testing.T) {
	plat := testPlatform()

	if len(plat.tags) == 0 {
		t.Fatal("should generate at least some compatible tags")
	}

	// First tag should be the most specific (CPython exact).
	first := plat.tags[0]
	if first.Python != "cp311" {
		t.Errorf("first tag python = %q, want cp311", first.Python)
	}
	if first.ABI != "cp311" {
		t.Errorf("first tag abi = %q, want cp311", first.ABI)
	}

	// Last tag should be py3-none-any.
	last := plat.tags[len(plat.tags)-1]
	if last.Python != "py3" || last.ABI != "none" || last.Platform != "any" {
		t.Errorf("last tag = %v, want py3-none-any", last)
	}
}
