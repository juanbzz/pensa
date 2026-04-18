package cli

import (
	"fmt"
	"io"
	"sync"

	"pensa.sh/pensa/internal/config"
	"pensa.sh/pensa/internal/installer"
	"pensa.sh/pensa/internal/lockfile"
	"golang.org/x/sync/errgroup"
)

// downloadResult pairs a locked package with its resolved wheel path.
type downloadResult struct {
	pkg       lockfile.LockedPackage
	wheelPath string
}

// downloadPackages downloads wheels for all packages in parallel,
// returning the local paths. Shows a spinner during download.
func downloadPackages(w io.Writer, ins *installer.Installer, pkgs []lockfile.LockedPackage) ([]downloadResult, error) {
	cfg, _ := config.New()

	stop := downloadSpinner(w, len(pkgs))

	var mu sync.Mutex
	var results []downloadResult

	g := new(errgroup.Group)
	downloadLimit := 50
	if cfg != nil && cfg.ConcurrentDownloads > 0 {
		downloadLimit = cfg.ConcurrentDownloads
	}
	g.SetLimit(downloadLimit)

	for _, pkg := range pkgs {
		pkg := pkg
		g.Go(func() error {
			path, err := ins.ResolvePackage(pkg)
			if err != nil {
				return fmt.Errorf("download %s: %w", pkg.Name, err)
			}
			mu.Lock()
			results = append(results, downloadResult{pkg, path})
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		stop()
		return nil, err
	}
	stop()

	return results, nil
}
