package cli

import (
	"testing"

	"github.com/matryer/is"

	"pensa.sh/pensa/internal/lockfile"
	"pensa.sh/pensa/internal/python"
)

// canInstallOnPlatform is the pre-filter sync.go uses to skip packages
// at install time. Returning false silently drops the package; returning
// true hands it off to ResolvePackage (which may build from sdist).
// Getting this wrong either silently omits installable packages or
// attempts hopeless downloads.

func macOSArm64Py311() *python.PythonInfo {
	return &python.PythonInfo{Major: 3, Minor: 11, Patch: 7, GOOS: "darwin"}
}

func linuxPy311() *python.PythonInfo {
	return &python.PythonInfo{Major: 3, Minor: 11, Patch: 7, GOOS: "linux"}
}

func filesFromNames(names ...string) []lockfile.PackageFile {
	out := make([]lockfile.PackageFile, 0, len(names))
	for _, n := range names {
		out = append(out, lockfile.PackageFile{File: n})
	}
	return out
}

// psycopg2-style: Windows wheels plus a buildable sdist. Must be
// accepted on macOS so buildFromSdist gets a chance to run.
func TestCanInstallOnPlatform_WindowsWheelsPlusSdist(t *testing.T) {
	assert := is.New(t)
	files := filesFromNames(
		"psycopg2-2.9.10-cp311-cp311-win_amd64.whl",
		"psycopg2-2.9.10-cp311-cp311-win32.whl",
		"psycopg2-2.9.10.tar.gz",
	)
	assert.True(canInstallOnPlatform(files, macOSArm64Py311()))
}

// pywin32-style: Windows wheels, no sdist. Correctly skipped on macOS —
// there's no way to install and attempting a download would fail.
func TestCanInstallOnPlatform_WindowsWheelsNoSdist(t *testing.T) {
	assert := is.New(t)
	files := filesFromNames(
		"pywin32-306-cp311-cp311-win_amd64.whl",
		"pywin32-306-cp311-cp311-win32.whl",
	)
	assert.True(!canInstallOnPlatform(files, macOSArm64Py311()))
}

// Sdist-only package (common for smaller pure-Python libs). Accepted.
func TestCanInstallOnPlatform_SdistOnly(t *testing.T) {
	assert := is.New(t)
	files := filesFromNames("somepkg-1.0.0.tar.gz")
	assert.True(canInstallOnPlatform(files, macOSArm64Py311()))
}

// Universal wheel: accepted everywhere.
func TestCanInstallOnPlatform_UniversalWheel(t *testing.T) {
	assert := is.New(t)
	files := filesFromNames("django-5.2.1-py3-none-any.whl")
	assert.True(canInstallOnPlatform(files, macOSArm64Py311()))
}

// CPython wheel matching current interpreter + platform: accepted.
func TestCanInstallOnPlatform_MatchingCPythonWheel(t *testing.T) {
	assert := is.New(t)
	files := filesFromNames(
		"cryptography-45.0.5-cp311-abi3-macosx_10_9_universal2.whl",
	)
	assert.True(canInstallOnPlatform(files, macOSArm64Py311()))
}

// Empty file list: downstream will emit a clear error. We allow it
// through so the failure is explicit rather than silently skipping.
func TestCanInstallOnPlatform_EmptyFiles(t *testing.T) {
	assert := is.New(t)
	assert.True(canInstallOnPlatform(nil, macOSArm64Py311()))
}

// goetry-c7c: abi3 wheels are forward-compatible with any CPython
// >= the tagged minor. psutil-7.2.2-cp36-abi3-... must be accepted on
// a cp310 venv even when no sdist is present — the pre-filter previously
// only matched the exact cpython tag or "-py3-", silently skipping the
// abi3 wheel and (if no sdist) the whole package.
func TestCanInstallOnPlatform_Abi3WheelOnly(t *testing.T) {
	assert := is.New(t)
	files := filesFromNames("psutil-7.2.2-cp36-abi3-macosx_11_0_arm64.whl")
	assert.True(canInstallOnPlatform(files, macOSArm64Py311()))
}

// Companion: abi3 wheel from an OLDER cpython minor must still be
// accepted on a NEWER venv (cp36-abi3 on cp311 is the whole point of
// abi3). Reverse direction (cp312-abi3 on cp310) must NOT be accepted —
// the venv's interpreter doesn't have the abi3 surface the wheel
// requires.
func TestCanInstallOnPlatform_Abi3MinorBound(t *testing.T) {
	assert := is.New(t)
	older := filesFromNames("pkg-1.0-cp36-abi3-macosx_11_0_arm64.whl")
	assert.True(canInstallOnPlatform(older, macOSArm64Py311()))

	newer := filesFromNames("pkg-1.0-cp312-abi3-macosx_11_0_arm64.whl")
	py310 := &python.PythonInfo{Major: 3, Minor: 10, Patch: 0, GOOS: "darwin"}
	assert.True(!canInstallOnPlatform(newer, py310))
}

// Regression for goetry-hhx: platform decisions must follow the
// PythonInfo's target OS, not the host's runtime.GOOS. A macOS-only
// wheel with no sdist is installable on darwin but not on linux,
// regardless of where the test runs. This is the assertion that broke
// CI when wheelMatchesPlatform read runtime.GOOS directly.
func TestCanInstallOnPlatform_RespectsTargetOS(t *testing.T) {
	assert := is.New(t)
	macOnly := filesFromNames("cryptography-45.0.5-cp311-cp311-macosx_11_0_arm64.whl")

	assert.True(canInstallOnPlatform(macOnly, macOSArm64Py311())) // darwin: usable
	assert.True(!canInstallOnPlatform(macOnly, linuxPy311()))     // linux: not usable

	linuxOnly := filesFromNames("cryptography-45.0.5-cp311-cp311-manylinux_2_17_x86_64.whl")
	assert.True(!canInstallOnPlatform(linuxOnly, macOSArm64Py311())) // darwin: not usable
	assert.True(canInstallOnPlatform(linuxOnly, linuxPy311()))       // linux: usable
}

// wheelMatchesPlatform must answer about the OS it is handed, not the
// host. Direct unit coverage of the seam goetry-hhx restored.
func TestWheelMatchesPlatform_UsesGivenOS(t *testing.T) {
	assert := is.New(t)
	mac := "cryptography-45.0.5-cp311-abi3-macosx_10_9_universal2.whl"
	lin := "cryptography-45.0.5-cp311-cp311-manylinux_2_17_x86_64.whl"
	win := "cryptography-45.0.5-cp311-cp311-win_amd64.whl"

	assert.True(wheelMatchesPlatform(mac, "darwin"))
	assert.True(!wheelMatchesPlatform(mac, "linux"))
	assert.True(wheelMatchesPlatform(lin, "linux"))
	assert.True(!wheelMatchesPlatform(lin, "windows"))
	assert.True(wheelMatchesPlatform(win, "windows"))
	assert.True(!wheelMatchesPlatform(win, "darwin"))
}
