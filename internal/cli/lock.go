package cli

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/adrg/xdg"

	"pensa.sh/pensa/internal/config"
	"pensa.sh/pensa/internal/index"
	"pensa.sh/pensa/internal/lockfile"
	"pensa.sh/pensa/internal/pyproject"
	"pensa.sh/pensa/internal/resolve"
	"pensa.sh/pensa/internal/workspace"
	"pensa.sh/pensa/pkg/pep508"
	"pensa.sh/pensa/pkg/version"
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
	out := uiFromCmd(cmd)
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Check for workspace.
	ws, _ := workspace.Discover(dir)
	if ws != nil {
		if lockCurrentWorkspace(ws) {
			out.UpToDate("Lock file is up to date.")
			return nil
		}
		return runLockWorkspace(cmd.ErrOrStderr(), ws, lockOptions{})
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
		out.Warning("no dependencies to lock")
		return nil
	}

	// Skip resolution if lock file is current.
	if lockCurrent(pyprojectPath, dir) {
		out.UpToDate("Lock file is up to date.")
		return nil
	}

	return resolveAndLock(cmd.ErrOrStderr(), proj, pyprojectPath, lockOptions{})
}

// resolveAndLock runs the full resolve → lock pipeline.
// Shared between `lock`, `add`, `remove`, and `update` commands.
func resolveAndLock(w io.Writer, proj *pyproject.PyProject, pyprojectPath string, opts lockOptions) error {
	start := time.Now()

	cfg, err := config.New()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	groupedDeps, err := proj.ResolveAllDependencies()
	if err != nil {
		return fmt.Errorf("resolve dependencies: %w", err)
	}

	// Parse project's requires-python for version filtering.
	var requiresPython version.Constraint
	if proj.HasProjectSection() && proj.Project.RequiresPython != "" {
		requiresPython, _ = version.ParseConstraint(proj.Project.RequiresPython)
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

	resCache, err := index.NewResolutionCache(defaultCacheDir())
	if err != nil {
		return fmt.Errorf("resolution cache: %w", err)
	}
	cached := index.NewCachedClient(client, resCache)
	prefetchPackages(cached, resolverDeps, cfg.ConcurrentDownloads)

	baseProvider := &indexProvider{client: cached, requestedExtras: depExtras, prefetchSem: make(chan struct{}, cfg.ConcurrentDownloads), requiresPython: requiresPython}

	// Wrap provider to prefer locked versions unless upgrading.
	var solverProvider resolve.Provider = baseProvider
	if !opts.upgrade {
		lockPath, _ := lockfile.DetectLockFile(filepath.Dir(pyprojectPath))
		if lockPath != "" {
			if lf, err := lockfile.ReadLockFile(lockPath); err == nil {
				solverProvider = newLockedProvider(baseProvider, lf, opts.upgradePackages)
				prefetchLockedVersions(cached, lf, cfg.ConcurrentDownloads)
			}
		}
	}

	prefetcher := newPrefetchProvider(solverProvider, cached, cfg.ConcurrentDownloads)
	solver := resolve.NewSolver(prefetcher, proj.Name(), resolverDeps)

	var result *resolve.SolverResult
	if err := withSpinnerMsg(w, blue("Resolving dependencies..."), "", func() error {
		var solveErr error
		result, solveErr = solver.Solve()
		return solveErr
	}); err != nil {
		return fmt.Errorf("resolve: %w", err)
	}

	// Flush resolution cache to disk. Background goroutines may still be
	// running — Flush writes whatever is in memory at this point. Remaining
	// updates will land in next run's cache.
	if err := resCache.Flush(); err != nil {
		newUI(w, false, false).Warning(fmt.Sprintf("flush resolution cache: %s", err))
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
	resolveUI := newUI(w, false, false)
	resolveUI.Resolved(len(result.Decisions), elapsed)
	// Lock file write is implicit — no output needed.

	return nil
}

func newPyPIClient() (*index.PyPIClient, error) {
	cacheDir := defaultCacheDir()
	return index.NewPyPIClient(
		index.WithCache(index.NewCache(cacheDir)),
	), nil
}

var _ resolve.Provider = (*indexProvider)(nil)

// indexProvider bridges resolve.Provider ↔ index.PyPIClient.
type indexProvider struct {
	client          *index.CachedClient
	requestedExtras map[string][]string // normalized pkg name → requested extras
	prefetchSem     chan struct{}       // bounds background prefetch concurrency
	requiresPython  version.Constraint  // project's requires-python, nil if unset
}

func (p *indexProvider) Versions(pkg string) ([]version.Version, error) {
	info, err := p.client.GetPackageInfo(pkg)
	if err != nil {
		return nil, err
	}

	// Note: if info comes from the resolution cache (synthetic), it won't have
	// RequiresPython data. The requires-python filter only works with real
	// PackageInfo from PyPI. This is acceptable — the filter catches most cases
	// on packages fetched fresh (cold cache or first resolve). Cached packages
	// that were already resolved are likely compatible.

	allVersions := info.Versions()
	var compatible []version.Version
	for _, v := range allVersions {
		if !v.IsStable() {
			continue
		}
		// Note: requires-python filtering is NOT done here because
		// FilesForVersion() is O(files) per version and too expensive
		// for packages with hundreds of versions. Instead, the install
		// phase filters incompatible packages via compatibleWithPython().
		// The getLatestCompatibleVersion() in add.go handles picking
		// the right version for new deps.
		compatible = append(compatible, v)
	}
	if len(compatible) == 0 {
		return allVersions, nil
	}
	return compatible, nil
}

func hasRequiresPythonData(info *index.PackageInfo) bool {
	for _, f := range info.Files {
		if f.RequiresPython != "" {
			return true
		}
	}
	return false
}

// pythonRangesOverlap checks if a package's requires-python is compatible
// with the project's requires-python. The package must support ALL Python
// versions the project targets. E.g., project >=3.10 + package >=3.12 = incompatible
// because the package doesn't work on 3.10-3.11.
func pythonRangesOverlap(projectPython version.Constraint, pkgRequiresPython string) bool {
	pkgConstraint, err := version.ParseConstraint(pkgRequiresPython)
	if err != nil {
		return true // can't parse, don't filter
	}
	// Package must allow all versions the project supports.
	return pkgConstraint.AllowsAll(projectPython)
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

	// Background prefetch: warm the cache for discovered deps.
	// These are intentionally fire-and-forget — they're best-effort cache
	// warmers. Errors are ignored, and incomplete prefetches on fast exit
	// are harmless (the cache is populated on the next run). Concurrency
	// is bounded by prefetchSem.
	for _, d := range deps {
		go func(name string) {
			p.prefetchSem <- struct{}{}
			defer func() { <-p.prefetchSem }()
			p.client.GetPackageInfo(name)
		}(d.Pkg)
	}

	return deps, nil
}

// isExtrasOnly checks if a dependency is gated by an extras marker.
// NOTE: fragile text search on rendered marker string.
// Ideally we'd walk the marker AST for an "extra" node.
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

	wsUI := newUI(w, false, false)
	wsUI.Workspace(len(ws.Members))
	for _, m := range ws.Members {
		fmt.Fprintf(w, "  %s %s\n", dim("•"), m.Name)
	}

	// Collect deps from all members, inlining workspace member transitive deps.
	rawSources := ws.Project.WorkspaceSources()
	wsSources := make(map[string]bool, len(rawSources))
	for name := range rawSources {
		wsSources[normalizeName(name)] = true
	}

	// Gather all deps from all members.
	var allGroupedDeps []pyproject.GroupedDependency
	for _, member := range ws.Members {
		groupedDeps, err := member.Project.ResolveAllDependencies()
		if err != nil {
			return fmt.Errorf("resolve deps for %s: %w", member.Name, err)
		}
		allGroupedDeps = append(allGroupedDeps, groupedDeps...)
	}

	// Inline workspace member deps: when A depends on B (workspace member),
	// replace B with B's own dependencies so they get resolved from PyPI.
	expandedDeps := inlineWorkspaceDeps(ws, wsSources, allGroupedDeps)

	depGroups := make(map[string][]string)
	depExtras := make(map[string][]string)
	seen := make(map[string]bool)
	var resolverDeps []resolve.Dependency

	for _, gd := range expandedDeps {
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

	if len(resolverDeps) == 0 {
		fmt.Fprintln(w, yellow("No dependencies to lock."))
		return nil
	}

	cfg, err := config.New()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Parse workspace's requires-python for version filtering.
	var requiresPython version.Constraint
	if ws.Project.HasProjectSection() && ws.Project.Project.RequiresPython != "" {
		requiresPython, _ = version.ParseConstraint(ws.Project.Project.RequiresPython)
	}

	client, err := newPyPIClient()
	if err != nil {
		return err
	}

	resCache, err := index.NewResolutionCache(defaultCacheDir())
	if err != nil {
		return fmt.Errorf("resolution cache: %w", err)
	}
	cached := index.NewCachedClient(client, resCache)
	prefetchPackages(cached, resolverDeps, cfg.ConcurrentDownloads)

	baseProvider := &indexProvider{client: cached, requestedExtras: depExtras, prefetchSem: make(chan struct{}, cfg.ConcurrentDownloads), requiresPython: requiresPython}

	// Wrap provider to prefer locked versions unless upgrading.
	var solverProvider resolve.Provider = baseProvider
	if !opts.upgrade {
		lockPath, _ := lockfile.DetectLockFile(ws.Root)
		if lockPath != "" {
			if lf, err := lockfile.ReadLockFile(lockPath); err == nil {
				solverProvider = newLockedProvider(baseProvider, lf, opts.upgradePackages)
				prefetchLockedVersions(cached, lf, cfg.ConcurrentDownloads)
			}
		}
	}

	prefetcher := newPrefetchProvider(solverProvider, cached, cfg.ConcurrentDownloads)
	solver := resolve.NewSolver(prefetcher, ws.Project.Name(), resolverDeps)

	var result *resolve.SolverResult
	if err := withSpinnerMsg(w, blue("Resolving dependencies..."), "", func() error {
		var solveErr error
		result, solveErr = solver.Solve()
		return solveErr
	}); err != nil {
		return fmt.Errorf("resolve: %w", err)
	}

	// Flush resolution cache to disk. Background goroutines may still be
	// running — Flush writes whatever is in memory at this point. Remaining
	// updates will land in next run's cache.
	if err := resCache.Flush(); err != nil {
		newUI(w, false, false).Warning(fmt.Sprintf("flush resolution cache: %s", err))
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
	resolveUI := newUI(w, false, false)
	resolveUI.Resolved(len(result.Decisions), elapsed)
	// Lock file write is implicit — no output needed.

	return nil
}

// computeWorkspaceHash computes a combined content hash from all workspace members.
// Only dependency-relevant fields are included — changes to version, description,
// etc. won't trigger a re-resolve.
func computeWorkspaceHash(ws *workspace.Workspace) string {
	h := sha256.New()
	if proj, err := pyproject.ReadPyProject(filepath.Join(ws.Root, "pyproject.toml")); err == nil {
		h.Write([]byte(proj.DependencyHash()))
	}
	for _, m := range ws.Members {
		if proj, err := pyproject.ReadPyProject(filepath.Join(m.Path, "pyproject.toml")); err == nil {
			h.Write([]byte(proj.DependencyHash()))
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func defaultCacheDir() string {
	return filepath.Join(xdg.CacheHome, "pensa")
}

func prefetchPackages(client *index.CachedClient, deps []resolve.Dependency, concurrency int) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	for _, dep := range deps {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			client.GetPackageInfo(name)
		}(dep.Pkg)
	}
	wg.Wait()
}

func lockCurrent(pyprojectPath, dir string) bool {
	lockPath, _ := lockfile.DetectLockFile(dir)
	if lockPath == "" {
		return false
	}
	lf, err := lockfile.ReadLockFile(lockPath)
	if err != nil {
		return false
	}

	// Fast path: content hash match.
	hash := computeContentHash(pyprojectPath)
	if hash != "" && lf.Metadata.ContentHash != "" && hash == lf.Metadata.ContentHash {
		return true
	}

	// Slow path: structural validation.
	proj, err := pyproject.ReadPyProject(pyprojectPath)
	if err != nil {
		return false
	}
	deps, err := proj.ResolveAllDependencies()
	if err != nil {
		return false
	}

	requiresPython := ""
	if proj.HasProjectSection() && proj.Project.RequiresPython != "" {
		requiresPython = proj.Project.RequiresPython
	}

	reqs := groupedDepsToRequirements(deps)
	result := lockfile.Satisfies(lf, reqs, requiresPython)
	if result.Satisfied {
		// Update the content hash so the next run hits the fast path.
		lockfile.UpdateContentHash(lockPath, hash)
	}
	return result.Satisfied
}

func lockCurrentWorkspace(ws *workspace.Workspace) bool {
	lockPath, _ := lockfile.DetectLockFile(ws.Root)
	if lockPath == "" {
		return false
	}
	lf, err := lockfile.ReadLockFile(lockPath)
	if err != nil {
		return false
	}

	// Fast path: content hash match.
	hash := computeWorkspaceHash(ws)
	if hash != "" && lf.Metadata.ContentHash != "" && hash == lf.Metadata.ContentHash {
		return true
	}

	// Slow path: structural validation across all members.
	wsSources := make(map[string]bool)
	for name := range ws.Project.WorkspaceSources() {
		wsSources[normalizeName(name)] = true
	}

	var allDeps []pep508.Dependency
	seen := make(map[string]bool)
	for _, member := range ws.Members {
		groupedDeps, err := member.Project.ResolveAllDependencies()
		if err != nil {
			return false
		}
		for _, gd := range groupedDeps {
			name := normalizeName(gd.Dep.Name)
			if wsSources[name] || seen[name] {
				continue
			}
			seen[name] = true
			allDeps = append(allDeps, gd.Dep)
		}
	}

	requiresPython := ""
	if ws.Project.HasProjectSection() && ws.Project.Project.RequiresPython != "" {
		requiresPython = ws.Project.Project.RequiresPython
	}

	result := lockfile.Satisfies(lf, allDeps, requiresPython)
	if result.Satisfied {
		lockfile.UpdateContentHash(lockPath, hash)
	}
	return result.Satisfied
}

func groupedDepsToRequirements(deps []pyproject.GroupedDependency) []pep508.Dependency {
	seen := make(map[string]bool)
	var reqs []pep508.Dependency
	for _, gd := range deps {
		name := normalizeName(gd.Dep.Name)
		if seen[name] {
			continue
		}
		seen[name] = true
		reqs = append(reqs, gd.Dep)
	}
	return reqs
}

// inlineWorkspaceDeps expands workspace member dependencies into their
// transitive PyPI deps. When dep A is a workspace member, it's replaced
// with A's own dependencies. Handles chains (A → B → C) via BFS.
func inlineWorkspaceDeps(ws *workspace.Workspace, wsSources map[string]bool, deps []pyproject.GroupedDependency) []pyproject.GroupedDependency {
	var result []pyproject.GroupedDependency
	visited := make(map[string]bool)
	queue := make([]pyproject.GroupedDependency, len(deps))
	copy(queue, deps)

	for len(queue) > 0 {
		gd := queue[0]
		queue = queue[1:]

		normalized := normalizeName(gd.Dep.Name)
		if visited[normalized] {
			continue
		}
		visited[normalized] = true

		if wsSources[normalized] {
			// Workspace member — inline its deps instead.
			if target := ws.FindMember(gd.Dep.Name); target != nil {
				memberDeps, err := target.Project.ResolveAllDependencies()
				if err == nil {
					queue = append(queue, memberDeps...)
				}
			}
			continue
		}

		result = append(result, gd)
	}
	return result
}

func prefetchLockedVersions(client *index.CachedClient, lf *lockfile.LockFile, concurrency int) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	for _, pkg := range lf.Packages {
		ver, err := version.Parse(pkg.Version)
		if err != nil {
			continue
		}
		wg.Add(1)
		go func(name string, v version.Version) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			client.GetVersionDetail(name, v)
		}(pkg.Name, ver)
	}
	wg.Wait()
}

func computeContentHash(pyprojectPath string) string {
	proj, err := pyproject.ReadPyProject(pyprojectPath)
	if err != nil {
		return ""
	}
	return proj.DependencyHash()
}
