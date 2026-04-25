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
	return &python.PythonInfo{Major: 3, Minor: 11, Patch: 7}
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
