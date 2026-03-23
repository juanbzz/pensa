package installer

import (
	"crypto/sha256"
	"fmt"
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
}

// NewInstaller creates a new package installer.
func NewInstaller(client *index.PyPIClient, venvPath string, py *python.PythonInfo, cacheDir string) *Installer {
	return &Installer{
		client:   client,
		venvPath: venvPath,
		python:   py,
		cacheDir: cacheDir,
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

// InstallPackage downloads and installs a single package.
func (ins *Installer) InstallPackage(pkg lockfile.LockedPackage) error {
	// Find the best wheel filename from the lock file.
	wheelFile := bestWheelFromFiles(pkg.Files)
	if wheelFile == nil {
		return fmt.Errorf("no wheel found for %s %s", pkg.Name, pkg.Version)
	}

	// Get download URL from PyPI.
	info, err := ins.client.GetPackageInfo(pkg.Name)
	if err != nil {
		return fmt.Errorf("get package info: %w", err)
	}

	var downloadURL string
	for _, f := range info.Files {
		if f.Filename == wheelFile.File {
			downloadURL = f.URL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("download URL not found for %s", wheelFile.File)
	}

	// Download wheel to cache.
	wheelPath, err := ins.downloadWheel(wheelFile.File, downloadURL, wheelFile.Hash)
	if err != nil {
		return err
	}

	// Unpack into site-packages.
	sitePackages := ins.python.SitePackagesDir(ins.venvPath)
	if err := UnpackWheel(wheelPath, sitePackages); err != nil {
		return fmt.Errorf("unpack wheel: %w", err)
	}

	// Install entry points.
	distInfo, err := FindDistInfo(sitePackages, pkg.Name, pkg.Version)
	if err == nil {
		binDir := filepath.Join(ins.venvPath, "bin")
		pythonPath := filepath.Join(binDir, "python")
		InstallEntryPoints(distInfo, binDir, pythonPath)
	}

	return nil
}

func bestWheelFromFiles(files []lockfile.PackageFile) *lockfile.PackageFile {
	var best *lockfile.PackageFile
	for i, f := range files {
		if !strings.HasSuffix(f.File, ".whl") {
			continue
		}
		if best == nil {
			best = &files[i]
			continue
		}
		// Prefer py3-none-any wheels.
		if strings.Contains(f.File, "-py3-none-any") {
			best = &files[i]
		}
	}
	return best
}

func (ins *Installer) downloadWheel(filename, url, expectedHash string) (string, error) {
	wheelDir := filepath.Join(ins.cacheDir, "wheels")
	os.MkdirAll(wheelDir, 0755)

	wheelPath := filepath.Join(wheelDir, filename)

	// Check if already cached.
	if _, err := os.Stat(wheelPath); err == nil {
		return wheelPath, nil
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

	tmpPath := wheelPath + ".tmp"
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

	if err := os.Rename(tmpPath, wheelPath); err != nil {
		return "", err
	}

	return wheelPath, nil
}
