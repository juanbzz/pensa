package index

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

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
	Name      string                     `msgpack:"n"`
	Versions  []string                   `msgpack:"vs"`
	Deps      map[string]ResolutionEntry `msgpack:"ds"` // version string → entry
	PEP658    bool                       `msgpack:"p"`  // whether PEP 658 metadata is available
	WheelURLs map[string]string          `msgpack:"wu"` // version → best wheel URL (PEP 658 only)
}

// ResolutionCache provides fast binary cache for resolver metadata.
type ResolutionCache struct {
	dir string
	mem sync.Map // string → *ResolutionPackage
}

func NewResolutionCache(cacheDir string) *ResolutionCache {
	dir := filepath.Join(cacheDir, "resolution")
	os.MkdirAll(dir, 0755)
	return &ResolutionCache{dir: dir}
}

func (rc *ResolutionCache) Get(name string) (*ResolutionPackage, error) {
	if v, ok := rc.mem.Load(name); ok {
		return v.(*ResolutionPackage), nil
	}

	path := filepath.Join(rc.dir, name+".msgpack")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pkg ResolutionPackage
	if err := msgpack.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}
	rc.mem.Store(name, &pkg)
	return &pkg, nil
}

func (rc *ResolutionCache) Put(pkg *ResolutionPackage) error {
	rc.mem.Store(pkg.Name, pkg)
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

// ToPackageInfo reconstructs a minimal PackageInfo from cached data.
// The result is sufficient for Versions() and BestWheel() during resolution.
func (rp *ResolutionPackage) ToPackageInfo() *PackageInfo {
	info := &PackageInfo{Name: rp.Name}
	safeName := strings.ReplaceAll(rp.Name, "-", "_")
	for _, vs := range rp.Versions {
		filename := safeName + "-" + vs + "-py3-none-any.whl"
		fi := FileInfo{Filename: filename}
		if rp.PEP658 {
			fi.CoreMetadata = true
			if url, ok := rp.WheelURLs[vs]; ok {
				fi.URL = url
			}
		}
		info.Files = append(info.Files, fi)
	}
	return info
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

	// Store best wheel URLs for PEP 658 packages.
	var wheelURLs map[string]string
	if pep658 {
		wheelURLs = make(map[string]string)
		for _, v := range versions {
			if wheel := info.BestWheel(v); wheel != nil {
				wheelURLs[v.String()] = wheel.URL
			}
		}
	}

	return &ResolutionPackage{
		Name:      info.Name,
		Versions:  vs,
		Deps:      make(map[string]ResolutionEntry),
		PEP658:    pep658,
		WheelURLs: wheelURLs,
	}
}
