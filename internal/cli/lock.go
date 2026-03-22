package cli

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juanbzz/goetry/internal/index"
	"github.com/juanbzz/goetry/internal/lockfile"
	"github.com/juanbzz/goetry/internal/pyproject"
	"github.com/juanbzz/goetry/internal/resolve"
	"github.com/juanbzz/goetry/pkg/pep508"
	"github.com/juanbzz/goetry/pkg/version"
	"github.com/spf13/cobra"
)

func newLockCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lock",
		Short: "Lock the project dependencies",
		Long:  "Reads pyproject.toml, resolves all dependencies, and writes poetry.lock.",
		RunE:  runLock,
	}
}

func runLock(cmd *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	pyprojectPath := filepath.Join(dir, "pyproject.toml")
	proj, err := pyproject.ReadPyProject(pyprojectPath)
	if err != nil {
		return fmt.Errorf("read pyproject.toml: %w", err)
	}

	deps, err := proj.ResolveDependencies()
	if err != nil {
		return fmt.Errorf("resolve dependencies: %w", err)
	}

	if len(deps) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No dependencies to lock.")
		return nil
	}

	return resolveAndLock(cmd.OutOrStdout(), proj, pyprojectPath)
}

// resolveAndLock runs the full resolve → lock pipeline.
// Shared between `lock` and `add` commands.
func resolveAndLock(w io.Writer, proj *pyproject.PyProject, pyprojectPath string) error {
	start := time.Now()

	deps, err := proj.ResolveDependencies()
	if err != nil {
		return fmt.Errorf("resolve dependencies: %w", err)
	}

	client, err := newPyPIClient()
	if err != nil {
		return err
	}

	resolverDeps := make([]resolve.Dependency, 0, len(deps))
	for _, d := range deps {
		constraint := d.Constraint
		if constraint == nil {
			constraint = version.AnyConstraint()
		}
		resolverDeps = append(resolverDeps, resolve.Dependency{
			Pkg:        d.Name,
			Constraint: constraint,
		})
	}

	provider := &indexProvider{client: client}
	solver := resolve.NewSolver(provider, proj.Name(), resolverDeps)

	fmt.Fprintf(w, "Resolving dependencies...\n")

	result, err := solver.Solve()
	if err != nil {
		return fmt.Errorf("resolve: %w", err)
	}

	pythonVersions := ">=3.8"
	if proj.HasProjectSection() && proj.Project.RequiresPython != "" {
		pythonVersions = proj.Project.RequiresPython
	}

	contentHash := computeContentHash(pyprojectPath)

	lf, err := lockfile.BuildLockFile(result, client, pythonVersions, contentHash)
	if err != nil {
		return fmt.Errorf("build lock file: %w", err)
	}

	lockPath := filepath.Join(filepath.Dir(pyprojectPath), "poetry.lock")
	if err := lockfile.WriteLockFile(lockPath, lf); err != nil {
		return fmt.Errorf("write lock file: %w", err)
	}

	elapsed := time.Since(start)
	fmt.Fprintf(w, "Resolved %d packages in %.1fs\n", len(result.Decisions), elapsed.Seconds())
	fmt.Fprintf(w, "Wrote poetry.lock\n")

	return nil
}

func newPyPIClient() (*index.PyPIClient, error) {
	cacheDir, err := defaultCacheDir()
	if err != nil {
		return nil, err
	}
	return index.NewPyPIClient(
		index.WithCache(index.NewCache(cacheDir)),
	), nil
}

// indexProvider bridges resolve.Provider ↔ index.PyPIClient.
type indexProvider struct {
	client *index.PyPIClient
}

func (p *indexProvider) Versions(pkg string) ([]version.Version, error) {
	info, err := p.client.GetPackageInfo(pkg)
	if err != nil {
		return nil, err
	}
	return info.Versions(), nil
}

func (p *indexProvider) Dependencies(pkg string, ver version.Version) ([]resolve.Dependency, error) {
	detail, err := p.client.GetVersionDetail(pkg, ver)
	if err != nil {
		return nil, err
	}

	var deps []resolve.Dependency
	for _, d := range detail.Dependencies {
		if isExtrasOnly(d) {
			continue
		}
		constraint := d.Constraint
		if constraint == nil {
			constraint = version.AnyConstraint()
		}
		deps = append(deps, resolve.Dependency{
			Pkg:        d.Name,
			Constraint: constraint,
		})
	}
	return deps, nil
}

func isExtrasOnly(d pep508.Dependency) bool {
	if d.Markers == nil {
		return false
	}
	return strings.Contains(d.Markers.String(), "extra ==")
}

func defaultCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(home, ".cache", "goetry"), nil
}

func computeContentHash(pyprojectPath string) string {
	data, err := os.ReadFile(pyprojectPath)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}
