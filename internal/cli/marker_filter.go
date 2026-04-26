package cli

import (
	"fmt"
	"runtime"

	"pensa.sh/pensa/pkg/pep508"
	"pensa.sh/pensa/pkg/version"
)

// markerKeepsDep reports whether a dep with marker m should be kept
// in the transitive graph given the project's requires-python range.
//
// Returns false (drop) when the marker evaluates false at every
// sampled Python version in the project's range against the current
// host platform. Returns true otherwise.
//
// Pensa locks for the current platform (no cross-platform locks
// today). Markers gated on a different OS (sys_platform == "win32"
// on macOS) or implementation (implementation_name == "pypy" on
// CPython) evaluate false at every sample point and are dropped at
// lock time. Without this filter, Windows-only packages like
// win-precise-time end up locked on macOS and fail at install time
// when their sdist can't build outside Windows.
//
// When pyReq is nil, the Python sweep defaults to [3.0, 3.99] —
// platform markers are still evaluated against the host.
func markerKeepsDep(m pep508.Marker, pyReq version.Constraint) bool {
	if m == nil {
		return true
	}
	if _, ok := m.(pep508.AnyMarker); ok {
		return true
	}
	env := currentPlatformEnv()
	for _, py := range pythonSamplePoints(pyReq) {
		env.PythonVersion = py
		env.PythonFullVersion = py + ".0"
		if m.Evaluate(env) {
			return true
		}
	}
	return false
}

// currentPlatformEnv returns a PEP 508 environment populated for the
// host pensa is running on. Used at lock time to filter deps whose
// marker can't apply on this platform.
//
// Cross-platform locking (where the lockfile records every
// platform-conditional dep regardless of host) would need a
// different model — recording per-package markers on the lockfile
// and re-evaluating at install time. uv supports this via [tool.uv]
// platforms config; pensa does not yet.
//
// Implementation is hard-coded to CPython. PyPy / GraalPy users
// get the wrong answer for `implementation_name`-gated markers
// (PyPy-only deps would be dropped, CPython-only deps would be
// kept incorrectly). Sniffing the venv's interpreter would fix it
// but adds a dependency on venv discovery before resolve; deferred
// until a user reports it.
func currentPlatformEnv() pep508.Environment {
	osName := "posix"
	sysPlat := runtime.GOOS
	plSys := "Linux"
	switch runtime.GOOS {
	case "darwin":
		plSys = "Darwin"
	case "windows":
		osName = "nt"
		sysPlat = "win32"
		plSys = "Windows"
	}
	machine := runtime.GOARCH
	switch machine {
	case "amd64":
		machine = "x86_64"
	case "arm64":
		// macOS/Windows arm64 report "arm64" via platform.machine();
		// Linux reports "aarch64". A Linux/arm64 host running pensa
		// must produce "aarch64" or it'll silently drop deps gated
		// on `platform_machine == "aarch64"`.
		if runtime.GOOS == "linux" {
			machine = "aarch64"
		}
	}
	return pep508.Environment{
		PythonVersion:                "3.99", // overridden by sampler
		PythonFullVersion:            "3.99.0",
		OSName:                       osName,
		SysPlatform:                  sysPlat,
		PlatformSystem:               plSys,
		PlatformMachine:              machine,
		PlatformPythonImplementation: "CPython",
		ImplementationName:           "cpython",
		ImplementationVersion:        "3.99.0",
	}
}

// pythonSamplePoints returns Python version strings spanning the
// project's requires-python range. Enumerates integer minor
// versions (3.11, 3.12, …) within the range up to a safety cap, so
// narrow markers like `python_version == "3.12"` aren't missed by a
// two-point lo/hi sweep.
func pythonSamplePoints(pyReq version.Constraint) []string {
	lo, hi, ok := pythonIntegerBounds(pyReq)
	if !ok {
		return []string{"3.0", "3.99"}
	}
	const maxSamples = 16
	var samples []string
	for minor := lo; minor <= hi && len(samples) < maxSamples; minor++ {
		samples = append(samples, fmt.Sprintf("3.%d", minor))
	}
	if len(samples) == 0 {
		return []string{"3.0", "3.99"}
	}
	return samples
}

// pythonIntegerBounds extracts the integer minor-version bounds of a
// Python 3.x requires-python range. Returns (lo, hi, true) for the
// simple Range cases real pyproject.toml files produce (`>=3.11`,
// `>=3.11,<4`, `^3.10`, `~=3.11`, etc.); returns (_, _, false) for
// anything else (Union, exact pin, Any, Empty) and the caller falls
// back to a default sweep across 3.0–3.99.
//
// The type assertion against *version.Range is deliberate: the
// Constraint interface has no bounds accessor, and the requires-python
// values in practice are always Ranges. Future Constraint
// implementations would fall through to the safe default — keeping
// more deps than strictly necessary, never dropping valid ones.
func pythonIntegerBounds(c version.Constraint) (int, int, bool) {
	r, ok := c.(*version.Range)
	if !ok {
		return 0, 0, false
	}
	lo := r.Min()
	if lo == nil {
		return 0, 0, false
	}
	loRel := lo.Release()
	if len(loRel) < 2 || loRel[0] != 3 {
		return 0, 0, false
	}
	loMinor := loRel[1]
	if !r.IncludeMin() {
		loMinor++
	}

	hiMinor := 99
	if hi := r.Max(); hi != nil {
		hiRel := hi.Release()
		switch {
		case len(hiRel) < 1 || hiRel[0] < 3:
			return 0, 0, false
		case hiRel[0] > 3:
			// Upper bound is 4.x or higher — unbounded within 3.x.
		case len(hiRel) < 2:
			// Bare "3" — unbounded within 3.x.
		default:
			hiMinor = hiRel[1]
			if !r.IncludeMax() && hiRel[1] > 0 {
				hiMinor--
			}
		}
	}

	if loMinor > hiMinor {
		return 0, 0, false
	}
	return loMinor, hiMinor, true
}
