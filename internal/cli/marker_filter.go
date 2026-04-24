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
