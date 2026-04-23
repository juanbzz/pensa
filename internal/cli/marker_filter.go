package cli

import (
	"fmt"

	"pensa.sh/pensa/pkg/pep508"
	"pensa.sh/pensa/pkg/version"
)

// markerKeepsDep reports whether a dep with marker m should be kept
// in the transitive graph given the project's requires-python range.
//
// Returns true (keep) in all cases EXCEPT: the marker references only
// python_version / python_full_version AND every sampled Python
// version inside the project's range makes the marker evaluate false.
// Mixed or non-python markers are always kept — install-time marker
// evaluation handles final exclusion and we can't know platform or
// implementation values at resolve time.
func markerKeepsDep(m pep508.Marker, pyReq version.Constraint) bool {
	if m == nil {
		return true
	}
	if _, ok := m.(pep508.AnyMarker); ok {
		return true
	}
	if pyReq == nil {
		return true
	}
	if !markerIsPythonOnly(m) {
		return true
	}
	for _, py := range pythonSamplePoints(pyReq) {
		env := permissiveEnv()
		env.PythonVersion = py
		env.PythonFullVersion = py + ".0"
		if m.Evaluate(env) {
			return true
		}
	}
	return false
}

// markerIsPythonOnly reports whether every CompareMarker in the AST
// references python_version or python_full_version. Returns false
// for AnyMarker or nil (neither classifies as "python-only" — those
// are handled separately by markerKeepsDep).
func markerIsPythonOnly(m pep508.Marker) bool {
	switch v := m.(type) {
	case nil:
		return false
	case pep508.AnyMarker:
		return false
	case *pep508.CompareMarker:
		return v.Var == "python_version" || v.Var == "python_full_version"
	case *pep508.AndMarker:
		return markerIsPythonOnly(v.Left) && markerIsPythonOnly(v.Right)
	case *pep508.OrMarker:
		return markerIsPythonOnly(v.Left) && markerIsPythonOnly(v.Right)
	}
	return false
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
// Python 3.x range constraint. Returns (lo, hi, true) when the
// constraint is a simple Range over Python 3.x with both bounds
// concrete; otherwise (_, _, false) signalling the caller should fall
// back to a default sweep.
func pythonIntegerBounds(c version.Constraint) (int, int, bool) {
	r, ok := c.(*version.Range)
	if !ok {
		return 0, 0, false
	}
	lo, hi := r.Min(), r.Max()
	if lo == nil || hi == nil {
		return 0, 0, false
	}
	loRel := lo.Release()
	hiRel := hi.Release()
	if len(loRel) < 2 || len(hiRel) < 1 {
		return 0, 0, false
	}
	if loRel[0] != 3 || hiRel[0] < 3 {
		return 0, 0, false
	}
	loMinor := loRel[1]
	if !r.IncludeMin() {
		loMinor++
	}
	var hiMinor int
	if hiRel[0] > 3 {
		// Upper bound is 4.x or higher — treat as unbounded within 3.x.
		hiMinor = 99
	} else {
		if len(hiRel) < 2 {
			// Bare "3" without a minor — treat as unbounded.
			hiMinor = 99
		} else {
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
