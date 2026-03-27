package installer

import (
	"crypto/sha256"
	"fmt"
	"github.com/juanbzz/pensa/internal/build"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/juanbzz/pensa/internal/index"
	"github.com/juanbzz/pensa/internal/lockfile"
	"github.com/juanbzz/pensa/internal/python"
)

// Installer downloads and installs packages into a venv.
type Installer struct {
	client   *index.PyPIClient
	venvPath string
	python   *python.PythonInfo
	cacheDir string
	platform *PlatformTags
}

// NewInstaller creates a new package installer.
func NewInstaller(client *index.PyPIClient, venvPath string, py *python.PythonInfo, cacheDir string) *Installer {
	return &Installer{
		client:   client,
		venvPath: venvPath,
		python:   py,
		cacheDir: cacheDir,
		platform: NewPlatformTags(py),
	}
}

// Install installs all packages from a lock file.
func (ins *Installer) Install(lf *lockfile.LockFile) error {
	for _, pkg := range lf.Packages {
		if err := ins.InstallPackage(pkg); err != nil {
			return fmt.Errorf("install %s: %w", pkg.Name, err)
		}
	}
	return nil
}

// InstallPackage downloads and installs a single package (download + unpack).
func (ins *Installer) InstallPackage(pkg lockfile.LockedPackage) error {
	wheelPath, err := ins.ResolvePackage(pkg)
	if err != nil {
		return err
	}
	return ins.InstallFromCache(pkg, wheelPath)
}

// ResolvePackage returns a path to an installable wheel for the given package.
// Tries wheel first, then cached built wheel, then builds from sdist if needed.
func (ins *Installer) ResolvePackage(pkg lockfile.LockedPackage) (string, error) {
	// Try wheel.
	wheelFile := bestWheelFromFiles(pkg.Files, ins.platform)
	if wheelFile != nil {
		url, err := ins.resolveURL(pkg.Name, wheelFile)
		if err != nil {
			return "", fmt.Errorf("resolve wheel URL: %w", err)
		}
		return ins.downloadFile(wheelFile.File, url, wheelFile.Hash, "wheels")
	}

	// Try cached built wheel.
	if cached := ins.findCachedWheel(pkg); cached != "" {
		return cached, nil
	}

	// Build from sdist.
	return ins.buildFromSdist(pkg)
}

func (ins *Installer) buildFromSdist(pkg lockfile.LockedPackage) (string, error) {
	sdistFile := bestSdistFromFiles(pkg.Files)
	if sdistFile == nil {
		return "", fmt.Errorf("no sdist found for %s", pkg.Name)
	}

	url, err := ins.resolveURL(pkg.Name, sdistFile)
	if err != nil {
		return "", err
	}

	sdistPath, err := ins.downloadFile(sdistFile.File, url, sdistFile.Hash, "sdists")
	if err != nil {
		return "", fmt.Errorf("download sdist: %w", err)
	}

	buildDir := filepath.Join(ins.cacheDir, "built")
	os.MkdirAll(buildDir, 0755)

	return build.BuildFromSdist(build.SdistBuildOptions{
		Name:      pkg.Name,
		Version:   pkg.Version,
		SdistPath: sdistPath,
		OutputDir: buildDir,
		Python:    ins.python,
	})
}

// InstallFromCache installs a wheel into site-packages via hard-link from
// a pre-extracted global cache. Falls back to direct extraction if caching fails.
func (ins *Installer) InstallFromCache(pkg lockfile.LockedPackage, wheelPath string) error {
	sitePackages := ins.python.SitePackagesDir(ins.venvPath)

	// Try: extract to cache once, then hard-link to site-packages.
	if extractDir, err := extractToCache(wheelPath, ins.cacheDir); err == nil {
		if err := linkToSitePackages(extractDir, sitePackages); err != nil {
			// Hard-link failed — fall back to direct extraction.
			if err := UnpackWheel(wheelPath, sitePackages); err != nil {
				return fmt.Errorf("unpack wheel: %w", err)
			}
		}
	} else {
		// Cache extraction failed — fall back to direct extraction.
		if err := UnpackWheel(wheelPath, sitePackages); err != nil {
			return fmt.Errorf("unpack wheel: %w", err)
		}
	}

	distInfo, err := FindDistInfo(sitePackages, pkg.Name, pkg.Version)
	if err == nil {
		binDir := filepath.Join(ins.venvPath, "bin")
		pythonPath := filepath.Join(binDir, "python")
		InstallEntryPoints(distInfo, binDir, pythonPath)
	}

	return nil
}

func bestWheelFromFiles(files []lockfile.PackageFile, plat *PlatformTags) *lockfile.PackageFile {
	var best *lockfile.PackageFile
	bestScore := -1
	for i, f := range files {
		if !strings.HasSuffix(f.File, ".whl") {
			continue
		}
		score := plat.Score(f.File)
		if score < 0 {
			continue
		}
		if bestScore < 0 || score < bestScore {
			bestScore = score
			best = &files[i]
		}
	}
	return best
}

func (ins *Installer) downloadFile(filename, url, expectedHash, subdir string) (string, error) {
	dir := filepath.Join(ins.cacheDir, subdir)
	os.MkdirAll(dir, 0755)

	filePath := filepath.Join(dir, filename)

	// Check if already cached.
	if _, err := os.Stat(filePath); err == nil {
		return filePath, nil
	}

	// Download.
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", filename, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: status %d", filename, resp.StatusCode)
	}

	tmpPath := filePath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", err
	}

	h := sha256.New()
	w := io.MultiWriter(f, h)

	if _, err := io.Copy(w, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return "", err
	}
	f.Close()

	// Verify hash if provided.
	if expectedHash != "" {
		actual := fmt.Sprintf("sha256:%x", h.Sum(nil))
		if actual != expectedHash {
			os.Remove(tmpPath)
			return "", fmt.Errorf("hash mismatch for %s: got %s, want %s", filename, actual, expectedHash)
		}
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		return "", err
	}

	return filePath, nil
}

func (ins *Installer) findCachedWheel(pkg lockfile.LockedPackage) string {
	pattern := filepath.Join(ins.cacheDir, "built", pkg.Name+"-"+pkg.Version+"-*.whl")
	matches, _ := filepath.Glob(pattern)
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

func (ins *Installer) resolveURL(pkgName string, file *lockfile.PackageFile) (string, error) {
	if file.URL != "" {
		return file.URL, nil
	}

	info, err := ins.client.GetPackageInfo(pkgName)
	if err != nil {
		return "", fmt.Errorf("get package info: %w", err)
	}

	for _, f := range info.Files {
		if f.Filename == file.File {
			return f.URL, nil
		}
	}
	return "", fmt.Errorf("download URL not found for %s", file.File)
}

func bestSdistFromFiles(files []lockfile.PackageFile) *lockfile.PackageFile {
	for i, f := range files {
		if strings.HasSuffix(f.File, ".tar.gz") {
			return &files[i]
		}

	}
	for i, f := range files {
		if strings.HasSuffix(f.File, ".zip") {
			return &files[i]
		}
	}
	return nil
}
