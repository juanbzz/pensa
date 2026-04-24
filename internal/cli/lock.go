package cli

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	// rpOverlapCache memoizes pythonRangesOverlap results keyed on the raw
	// package-level requires-python string. PyPI packages reuse a small set
	// of distinct values across many (pkg, version) pairs, so the cache hit
	// rate is high and the per-version filter is close to free.
	rpOverlapCache sync.Map // string → bool
}

func (p *indexProvider) Versions(pkg string) ([]version.Version, error) {
	info, err := p.client.GetPackageInfo(pkg)
	if err != nil {
		return nil, err
	}
	return filterCandidateVersions(info, p.requiresPython, p.requiresPythonAllowsProject), nil
}

// filterCandidateVersions applies the resolver-level version filter:
//   1. Drop non-stable versions (prereleases + dev releases).
//   2. Drop versions whose requires-python is incompatible with the
//      project's (upper bounds on the package side are stripped; see
//      requiresPythonAllowsProject).
//
// Fallback: if every version is filtered out, return allVersions
// unchanged. Rationale — a package that has ONLY prereleases (e.g.
// pre-1.0 libs) needs to be resolvable. Without the fallback, the
// solver would fail on "no versions" for such packages.
//
// Known gap: an exact pin on a prerelease (==1.0.0rc1) when stable
// versions exist will filter the rc1 out. The solver then fails
// because it can't find a version satisfying the pin. The fallback
// only fires when ALL versions are prereleases. An opt-in-prerelease
// flag would address it.
//
// Exposed as a package-level function (rather than an indexProvider
// method) so it can be unit-tested without constructing a full
// PyPIClient + resolution-cache chain.
func filterCandidateVersions(
	info *index.PackageInfo,
	requiresPython version.Constraint,
	requiresPythonAllowsProject func(pkgRequiresPython string) bool,
) []version.Version {
	allVersions := info.Versions()
	var compatible []version.Version
	for _, v := range allVersions {
		if !v.IsStable() {
			continue
		}
		if requiresPython != nil {
			rp := info.RequiresPythonFor(v)
			if rp != "" && !requiresPythonAllowsProject(rp) {
				continue
			}
		}
		compatible = append(compatible, v)
	}
	if len(compatible) == 0 {
		return allVersions
	}
	return compatible
}

// requiresPythonAllowsProject returns whether a package's requires-python
// (after stripping defensive upper bounds) allows the project's target
// Python range. Result is cached by the raw requires-python string to
// avoid re-parsing identical values across a package's versions.
func (p *indexProvider) requiresPythonAllowsProject(pkgRequiresPython string) bool {
	if v, ok := p.rpOverlapCache.Load(pkgRequiresPython); ok {
		return v.(bool)
	}
	ok := pythonRangesOverlap(p.requiresPython, pkgRequiresPython)
	p.rpOverlapCache.Store(pkgRequiresPython, ok)
	return ok
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
// with the project's requires-python. The package's lower bound must
// allow the project's supported Python range.
//
// Upper bounds in the PACKAGE's requires-python are stripped before the
// check. A package declaring `python<3.13` or `python<4`
// is almost always making a defensive claim — it hasn't been tested on
// newer Python, not that it's known to break. Honoring those upper bounds
// causes the resolver to reject otherwise-compatible packages whenever
// the project targets a Python version past the author's test matrix,
// triggering backtracking cascades. The project's own requires-python
// (projectPython) is NOT stripped — users declaring their own upper
// bound know what they mean.
func pythonRangesOverlap(projectPython version.Constraint, pkgRequiresPython string) bool {
	pkgConstraint, err := version.ParseConstraint(pkgRequiresPython)
	if err != nil {
		return true // can't parse, don't filter
	}
	pkgConstraint = version.StripUpperBound(pkgConstraint)
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
		} else if !markerKeepsDep(d.Markers, p.requiresPython) {
			// Marker is python-only and unsatisfiable across the
			// project's requires-python range — skip. Prevents
			// per-package self-contradiction when a package ships
			// marker-split dep declarations (e.g. stripe 11.0.0
			// has typing-extensions<=4.2.0 for Python<3.7 AND
			// typing-extensions>=4.5.0 for Python>=3.7).
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

// Preferred: the base provider has no lockfile context. Preferences
// come from the lockedProvider wrapper.
func (p *indexProvider) Preferred(pkg string) (version.Version, bool) {
	return version.Version{}, false
}

// DependenciesIfCached returns deps for (pkg, ver) only when the
// version detail is already in the client's cache (in-memory sync.Map
// or disk-backed resolution cache). Never triggers a network fetch.
// Used by the solver's range-batching to widen base clauses across
// cached neighbors.
func (p *indexProvider) DependenciesIfCached(pkg string, ver version.Version) ([]resolve.Dependency, bool) {
	detail, ok := p.client.VersionDetailIfCached(pkg, ver)
	if !ok || detail == nil {
		return nil, false
	}
	extras := p.requestedExtras[normalizeName(pkg)]
	var deps []resolve.Dependency
	for _, d := range detail.Dependencies {
		if isExtrasOnly(d) {
			if !isRequestedExtra(d, extras) {
				continue
			}
		} else if !markerKeepsDep(d.Markers, p.requiresPython) {
			continue
		}
		constraint := d.Constraint
		if constraint == nil {
			constraint = version.AnyConstraint()
		}
		deps = append(deps, resolve.Dependency{Pkg: d.Name, Constraint: constraint})
	}
	return deps, true
}

// isExtrasOnly reports whether a dependency is gated by an extras
// marker — i.e., the dep's marker references the `extra` variable
// anywhere in its AST. When true, the dep should only be included if
// the user has requested a matching extra.
//
// Walks the Marker AST rather than searching the rendered string.
// A text-search approach misfires on string literals that happen
// to contain "extra ==" and can't distinguish `extra == X` from
// `extra != X`.
func isExtrasOnly(d pep508.Dependency) bool {
	return markerMentionsExtra(d.Markers)
}

// isRequestedExtra reports whether a dep's marker evaluates TRUE for
// any of the requested extras. Uses Marker.Evaluate with env.Extra
// set to each requested extra in turn — so operators other than
// literal `extra == 'X'` (e.g., `extra != 'X'`) are handled correctly.
//
// Non-extra environment fields are set to wide-permissive values so
// that platform/Python-version gates in the same marker don't cause
// us to drop a dep that would apply at install time. Install-time
// filtering (compatibleWithPython) handles the narrow case.
func isRequestedExtra(d pep508.Dependency, requestedExtras []string) bool {
	if d.Markers == nil || len(requestedExtras) == 0 {
		return false
	}
	for _, extra := range requestedExtras {
		env := permissiveEnv()
		env.Extra = extra
		if d.Markers.Evaluate(env) {
			return true
		}
	}
	return false
}

// markerMentionsExtra returns true if the marker AST has any
// CompareMarker referencing the `extra` variable.
func markerMentionsExtra(m pep508.Marker) bool {
	switch v := m.(type) {
	case nil:
		return false
	case pep508.AnyMarker:
		return false
	case *pep508.CompareMarker:
		return v.Var == "extra"
	case *pep508.AndMarker:
		return markerMentionsExtra(v.Left) || markerMentionsExtra(v.Right)
	case *pep508.OrMarker:
		return markerMentionsExtra(v.Left) || markerMentionsExtra(v.Right)
	}
	return false
}

// permissiveEnv returns an Environment where platform/Python fields
// are set to values that let most markers evaluate TRUE. Used at
// resolve time to decide extras inclusion without prematurely
// filtering out deps that would apply at install time on a different
// Python or platform.
func permissiveEnv() pep508.Environment {
	return pep508.Environment{
		PythonVersion:                "3.99",
		PythonFullVersion:            "3.99.0",
		OSName:                       "posix",
		SysPlatform:                  "linux",
		PlatformRelease:              "",
		PlatformSystem:               "Linux",
		PlatformVersion:              "",
		PlatformMachine:              "x86_64",
		PlatformPythonImplementation: "CPython",
		ImplementationName:           "cpython",
		ImplementationVersion:        "3.99.0",
	}
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
