package cli

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
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
	start := time.Now()

	// Find pyproject.toml.
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	pyprojectPath := filepath.Join(dir, "pyproject.toml")
	proj, err := pyproject.ReadPyProject(pyprojectPath)
	if err != nil {
		return fmt.Errorf("read pyproject.toml: %w", err)
	}

	// Extract dependencies.
	deps, err := proj.ResolveDependencies()
	if err != nil {
		return fmt.Errorf("resolve dependencies: %w", err)
	}

	if len(deps) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No dependencies to lock.")
		return nil
	}

	// Create PyPI client with cache.
	cacheDir, err := defaultCacheDir()
	if err != nil {
		return err
	}
	client := index.NewPyPIClient(
		index.WithCache(index.NewCache(cacheDir)),
	)

	// Convert pep508 deps to resolver deps.
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

	// Create provider and solver.
	provider := &indexProvider{client: client}
	solver := resolve.NewSolver(provider, proj.Name(), resolverDeps)

	fmt.Fprintf(cmd.OutOrStdout(), "Resolving dependencies...\n")

	result, err := solver.Solve()
	if err != nil {
		return fmt.Errorf("resolve: %w", err)
	}

	// Build lock file.
	pythonVersions := ">=3.8"
	if proj.HasProjectSection() && proj.Project.RequiresPython != "" {
		pythonVersions = proj.Project.RequiresPython
	}

	contentHash := computeContentHash(pyprojectPath)

	lf, err := lockfile.BuildLockFile(result, client, pythonVersions, contentHash)
	if err != nil {
		return fmt.Errorf("build lock file: %w", err)
	}

	lockPath := filepath.Join(dir, "poetry.lock")
	if err := lockfile.WriteLockFile(lockPath, lf); err != nil {
		return fmt.Errorf("write lock file: %w", err)
	}

	elapsed := time.Since(start)
	fmt.Fprintf(cmd.OutOrStdout(), "Resolved %d packages in %.1fs\n", len(result.Decisions), elapsed.Seconds())
	fmt.Fprintf(cmd.OutOrStdout(), "Wrote poetry.lock\n")

	return nil
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
		// Skip extras-only dependencies.
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
	s := d.Markers.String()
	return containsExtraMarker(s)
}

func containsExtraMarker(s string) bool {
	return len(s) > 0 && (contains(s, "extra ==") || contains(s, "extra =="))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
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
