package index

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/juanbzz/pensa/pkg/pep508"
	"github.com/vmihailenco/msgpack/v5"
)

// ResolutionEntry stores what the resolver needs for a single package version.
type ResolutionEntry struct {
	Version      string   `msgpack:"v"`
	Dependencies []string `msgpack:"d"` // PEP 508 dependency strings
}

// ResolutionPackage stores all resolver-relevant data for a package.
type ResolutionPackage struct {
	Name     string                       `msgpack:"n"`
	Versions []string                     `msgpack:"vs"`
	Deps     map[string]ResolutionEntry   `msgpack:"ds"` // version string → entry
	PEP658   bool                         `msgpack:"p"`  // whether PEP 658 metadata is available
}

// ResolutionCache provides fast binary cache for resolver metadata.
type ResolutionCache struct {
	dir string
}

func NewResolutionCache(cacheDir string) *ResolutionCache {
	dir := filepath.Join(cacheDir, "resolution")
	os.MkdirAll(dir, 0755)
	return &ResolutionCache{dir: dir}
}

func (rc *ResolutionCache) Get(name string) (*ResolutionPackage, error) {
	path := filepath.Join(rc.dir, name+".msgpack")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pkg ResolutionPackage
	if err := msgpack.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}
	return &pkg, nil
}

func (rc *ResolutionCache) Put(pkg *ResolutionPackage) error {
	data, err := msgpack.Marshal(pkg)
	if err != nil {
		return err
	}
	path := filepath.Join(rc.dir, pkg.Name+".msgpack")
	return os.WriteFile(path, data, 0644)
}

// ToVersionDetail converts a ResolutionEntry back to a VersionDetail.
func (re *ResolutionEntry) ToVersionDetail(name string) *VersionDetail {
	detail := &VersionDetail{
		Name: name,
	}
	for _, depStr := range re.Dependencies {
		dep, err := pep508.Parse(depStr)
		if err == nil {
			detail.Dependencies = append(detail.Dependencies, dep)
		}
	}
	return detail
}

// FromVersionDetail creates a ResolutionEntry from a VersionDetail.
func FromVersionDetail(detail *VersionDetail) ResolutionEntry {
	deps := make([]string, 0, len(detail.Dependencies))
	for _, d := range detail.Dependencies {
		deps = append(deps, formatDep(d))
	}
	return ResolutionEntry{
		Version:      detail.Version.String(),
		Dependencies: deps,
	}
}

func formatDep(d pep508.Dependency) string {
	s := d.Name
	if len(d.Extras) > 0 {
		s += "[" + strings.Join(d.Extras, ",") + "]"
	}
	if d.Constraint != nil && !d.Constraint.IsAny() {
		s += " " + d.Constraint.String()
	}
	if d.Markers != nil {
		s += " ; " + d.Markers.String()
	}
	return s
}

// FromPackageInfo creates a ResolutionPackage with version list from PackageInfo.
func FromPackageInfo(info *PackageInfo) *ResolutionPackage {
	versions := info.Versions()
	vs := make([]string, len(versions))
	for i, v := range versions {
		vs[i] = v.String()
	}

	// Check if any wheel has PEP 658 core-metadata.
	pep658 := false
	for _, f := range info.Files {
		if f.CoreMetadata {
			pep658 = true
			break
		}
	}

	return &ResolutionPackage{
		Name:     info.Name,
		Versions: vs,
		Deps:     make(map[string]ResolutionEntry),
		PEP658:   pep658,
	}
}
