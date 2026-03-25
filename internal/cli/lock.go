package cli

import (
	"crypto/sha256"
	"fmt"
	"github.com/adrg/xdg"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juanbzz/pensa/internal/index"
	"github.com/juanbzz/pensa/internal/lockfile"
	"github.com/juanbzz/pensa/internal/pyproject"
	"github.com/juanbzz/pensa/internal/resolve"
	"github.com/juanbzz/pensa/internal/workspace"
	"github.com/juanbzz/pensa/pkg/pep508"
	"github.com/juanbzz/pensa/pkg/version"
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

	// Check for workspace.
	ws, _ := workspace.Discover(dir)
	if ws != nil {
		return runLockWorkspace(cmd.OutOrStdout(), ws, lockOptions{})
	}

	// Single project mode.
	pyprojectPath := filepath.Join(dir, "pyproject.toml")
	proj, err := pyproject.ReadPyProject(pyprojectPath)
	if err != nil {
		return fmt.Errorf("read pyproject.toml: %w", err)
	}

	allDeps, err := proj.ResolveAllDependencies()
	if err != nil {
		return fmt.Errorf("resolve dependencies: %w", err)
	}

	if len(allDeps) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), yellow("No dependencies to lock."))
		return nil
	}

	return resolveAndLock(cmd.OutOrStdout(), proj, pyprojectPath, lockOptions{})
}

// resolveAndLock runs the full resolve → lock pipeline.
// Shared between `lock`, `add`, `remove`, and `update` commands.
func resolveAndLock(w io.Writer, proj *pyproject.PyProject, pyprojectPath string, opts lockOptions) error {
	start := time.Now()

	groupedDeps, err := proj.ResolveAllDependencies()
	if err != nil {
		return fmt.Errorf("resolve dependencies: %w", err)
	}

	client, err := newPyPIClient()
	if err != nil {
		return err
	}

	// Build resolver deps (all groups resolved together) and track group membership + extras.
	depGroups := make(map[string][]string) // normalized name → groups
	depExtras := make(map[string][]string) // normalized name → requested extras
	seen := make(map[string]bool)
	var resolverDeps []resolve.Dependency

	for _, gd := range groupedDeps {
		normalized := normalizeName(gd.Dep.Name)
		depGroups[normalized] = append(depGroups[normalized], gd.Group)

		// Track requested extras.
		if len(gd.Dep.Extras) > 0 {
			depExtras[normalized] = append(depExtras[normalized], gd.Dep.Extras...)
		}

		if seen[normalized] {
			continue // already added to resolver
		}
		seen[normalized] = true

		constraint := gd.Dep.Constraint
		if constraint == nil {
			constraint = version.AnyConstraint()
		}
		resolverDeps = append(resolverDeps, resolve.Dependency{
			Pkg:        gd.Dep.Name,
			Constraint: constraint,
		})
	}

	baseProvider := &indexProvider{client: client, requestedExtras: depExtras}

	// Wrap provider to prefer locked versions unless upgrading.
	var solverProvider resolve.Provider = baseProvider
	if !opts.upgrade {
		lockPath, _ := lockfile.DetectLockFile(filepath.Dir(pyprojectPath))
		if lockPath != "" {
			if lf, err := lockfile.ReadLockFile(lockPath); err == nil {
				solverProvider = newLockedProvider(baseProvider, lf, opts.upgradePackages)
			}
		}
	}

	solver := resolve.NewSolver(solverProvider, proj.Name(), resolverDeps)

	var result *resolve.SolverResult
	if err := withSpinnerMsg(w, blue("Resolving dependencies..."), "", func() error {
		var solveErr error
		result, solveErr = solver.Solve()
		return solveErr
	}); err != nil {
		return fmt.Errorf("resolve: %w", err)
	}

	pythonVersions := ">=3.8"
	if proj.HasProjectSection() && proj.Project.RequiresPython != "" {
		pythonVersions = proj.Project.RequiresPython
	}

	contentHash := computeContentHash(pyprojectPath)

	lf, err := lockfile.BuildLockFile(result, client, pythonVersions, contentHash, depGroups)
	if err != nil {
		return fmt.Errorf("build lock file: %w", err)
	}

	pensaLockPath := filepath.Join(filepath.Dir(pyprojectPath), "pensa.lock")
	if err := lockfile.WritePensaLockFile(pensaLockPath, lf); err != nil {
		return fmt.Errorf("write lock file: %w", err)
	}

	elapsed := time.Since(start)
	fmt.Fprintf(w, "%s %d packages in %.1fs\n", green("Resolved"), len(result.Decisions), elapsed.Seconds())
	fmt.Fprintf(w, "%s pensa.lock\n", green("Wrote"))

	return nil
}

func newPyPIClient() (*index.PyPIClient, error) {
	cacheDir := defaultCacheDir()
	return index.NewPyPIClient(
		index.WithCache(index.NewCache(cacheDir)),
	), nil
}

// indexProvider bridges resolve.Provider ↔ index.PyPIClient.
type indexProvider struct {
	client          *index.PyPIClient
	requestedExtras map[string][]string // normalized pkg name → requested extras
}

func (p *indexProvider) Versions(pkg string) ([]version.Version, error) {
	info, err := p.client.GetPackageInfo(pkg)
	if err != nil {
		return nil, err
	}
	// Filter out pre-release versions by default.
	allVersions := info.Versions()
	var stable []version.Version
	for _, v := range allVersions {
		if v.IsStable() {
			stable = append(stable, v)
		}
	}
	// Fall back to all versions if no stable versions exist.
	if len(stable) == 0 {
		return allVersions, nil
	}
	return stable, nil
}

func (p *indexProvider) Dependencies(pkg string, ver version.Version) ([]resolve.Dependency, error) {
	detail, err := p.client.GetVersionDetail(pkg, ver)
	if err != nil {
		return nil, err
	}

	// Get requested extras for this package.
	extras := p.requestedExtras[normalizeName(pkg)]

	var deps []resolve.Dependency
	for _, d := range detail.Dependencies {
		if isExtrasOnly(d) {
			// Include this dep only if its extra was requested.
			if !isRequestedExtra(d, extras) {
				continue
			}
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

// isRequestedExtra checks if a dep's extra marker matches any of the requested extras.
// Marker format: `extra == 'security'` or `extra == "security"`
func isRequestedExtra(d pep508.Dependency, requestedExtras []string) bool {
	if d.Markers == nil || len(requestedExtras) == 0 {
		return false
	}
	markerStr := d.Markers.String()
	for _, extra := range requestedExtras {
		if strings.Contains(markerStr, "extra == '"+extra+"'") ||
			strings.Contains(markerStr, `extra == "`+extra+`"`) {
			return true
		}
	}
	return false
}

// runLockWorkspace locks all workspace members together into a single lock file.
func runLockWorkspace(w io.Writer, ws *workspace.Workspace, opts lockOptions) error {
	start := time.Now()

	fmt.Fprintf(w, "%s workspace with %d members\n", blue("Locking"), len(ws.Members))
	for _, m := range ws.Members {
		fmt.Fprintf(w, "  %s %s\n", dim("•"), m.Name)
	}

	// Collect deps from all members.
	depGroups := make(map[string][]string)
	depExtras := make(map[string][]string)
	seen := make(map[string]bool)
	var resolverDeps []resolve.Dependency

	for _, member := range ws.Members {
		groupedDeps, err := member.Project.ResolveAllDependencies()
		if err != nil {
			return fmt.Errorf("resolve deps for %s: %w", member.Name, err)
		}

		for _, gd := range groupedDeps {
			normalized := normalizeName(gd.Dep.Name)
			depGroups[normalized] = append(depGroups[normalized], gd.Group)
			if len(gd.Dep.Extras) > 0 {
				depExtras[normalized] = append(depExtras[normalized], gd.Dep.Extras...)
			}

			if seen[normalized] {
				continue
			}
			seen[normalized] = true

			constraint := gd.Dep.Constraint
			if constraint == nil {
				constraint = version.AnyConstraint()
			}
			resolverDeps = append(resolverDeps, resolve.Dependency{
				Pkg:        gd.Dep.Name,
				Constraint: constraint,
			})
		}
	}

	if len(resolverDeps) == 0 {
		fmt.Fprintln(w, yellow("No dependencies to lock."))
		return nil
	}

	client, err := newPyPIClient()
	if err != nil {
		return err
	}

	baseProvider := &indexProvider{client: client, requestedExtras: depExtras}

	// Wrap provider to prefer locked versions unless upgrading.
	var solverProvider resolve.Provider = baseProvider
	if !opts.upgrade {
		lockPath, _ := lockfile.DetectLockFile(ws.Root)
		if lockPath != "" {
			if lf, err := lockfile.ReadLockFile(lockPath); err == nil {
				solverProvider = newLockedProvider(baseProvider, lf, opts.upgradePackages)
			}
		}
	}

	solver := resolve.NewSolver(solverProvider, ws.Project.Name(), resolverDeps)

	var result *resolve.SolverResult
	if err := withSpinnerMsg(w, blue("Resolving dependencies..."), "", func() error {
		var solveErr error
		result, solveErr = solver.Solve()
		return solveErr
	}); err != nil {
		return fmt.Errorf("resolve: %w", err)
	}

	pythonVersions := ">=3.8"
	if ws.Project.HasProjectSection() && ws.Project.Project.RequiresPython != "" {
		pythonVersions = ws.Project.Project.RequiresPython
	}

	// Compute content hash from all members' pyproject.toml files.
	contentHash := computeWorkspaceHash(ws)

	lf, err := lockfile.BuildLockFile(result, client, pythonVersions, contentHash, depGroups)
	if err != nil {
		return fmt.Errorf("build lock file: %w", err)
	}

	pensaLockPath := ws.LockFilePath()
	if err := lockfile.WritePensaLockFile(pensaLockPath, lf); err != nil {
		return fmt.Errorf("write lock file: %w", err)
	}

	elapsed := time.Since(start)
	fmt.Fprintf(w, "%s %d packages in %.1fs\n", green("Resolved"), len(result.Decisions), elapsed.Seconds())
	fmt.Fprintf(w, "%s pensa.lock\n", green("Wrote"))

	return nil
}

// computeWorkspaceHash computes a combined content hash from all workspace members.
func computeWorkspaceHash(ws *workspace.Workspace) string {
	h := sha256.New()
	// Include root pyproject.
	if data, err := os.ReadFile(filepath.Join(ws.Root, "pyproject.toml")); err == nil {
		h.Write(data)
	}
	// Include each member's pyproject.
	for _, m := range ws.Members {
		if data, err := os.ReadFile(filepath.Join(m.Path, "pyproject.toml")); err == nil {
			h.Write(data)
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func defaultCacheDir() string {
	return filepath.Join(xdg.CacheHome, "pensa")
}

func computeContentHash(pyprojectPath string) string {
	data, err := os.ReadFile(pyprojectPath)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}
