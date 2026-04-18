package lockfile

import (
	"pensa.sh/pensa/pkg/pep508"
	"pensa.sh/pensa/pkg/version"
)

// SatisfiesResult describes whether a lock file satisfies a set of requirements.
type SatisfiesResult struct {
	Satisfied bool
	Reason    string // empty when satisfied; describes why not otherwise
}

// Satisfies checks whether lf still satisfies the given direct requirements
// without needing re-resolution. Requirements are the parsed direct
// dependencies from pyproject.toml (all groups, deduplicated).
func Satisfies(lf *LockFile, requirements []pep508.Dependency, requiresPython string) SatisfiesResult {
	// Check requires-python hasn't changed.
	if requiresPython != lf.Metadata.PythonVersions {
		return SatisfiesResult{Reason: "requires-python changed"}
	}

	// Build lookup: normalized name → LockedPackage.
	locked := make(map[string]*LockedPackage, len(lf.Packages))
	for i := range lf.Packages {
		locked[pep508.NormalizeName(lf.Packages[i].Name)] = &lf.Packages[i]
	}

	// Every direct requirement must have a locked package whose version
	// falls within the current constraint.
	for _, req := range requirements {
		name := pep508.NormalizeName(req.Name)
		pkg, ok := locked[name]
		if !ok {
			return SatisfiesResult{Reason: "new dependency: " + req.Name}
		}

		if req.Constraint == nil || req.Constraint.IsAny() {
			continue
		}

		ver, err := version.Parse(pkg.Version)
		if err != nil {
			return SatisfiesResult{Reason: "unparseable locked version: " + pkg.Version}
		}

		if !req.Constraint.Allows(ver) {
			return SatisfiesResult{
				Reason: req.Name + " " + req.Constraint.String() +
					" not satisfied by locked " + pkg.Version,
			}
		}
	}

	return SatisfiesResult{Satisfied: true}
}
