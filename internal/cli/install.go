package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pensa.sh/pensa/internal/config"
	"pensa.sh/pensa/internal/installer"
	"pensa.sh/pensa/internal/lockfile"
	"pensa.sh/pensa/internal/python"
	"pensa.sh/pensa/internal/workspace"
	"pensa.sh/pensa/pkg/pep508"
	"pensa.sh/pensa/pkg/version"
	"github.com/spf13/cobra"
)

func newInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install dependencies from lock file",
		Long:  "Creates a virtual environment and installs all locked dependencies.",
		RunE:  runInstall,
	}
	cmd.Flags().Bool("no-root", false, "Do not install the project itself")
	cmd.Flags().Bool("no-dev", false, "Do not install dev dependencies")
	cmd.Flags().StringSlice("with", nil, "Include optional dependency groups")
	cmd.Flags().String("only", "", "Install only this dependency group")
	cmd.Flags().String("package", "", "Install only this workspace member's dependencies")
	return cmd
}

func runInstall(cmd *cobra.Command, args []string) error {
	noRoot, _ := cmd.Flags().GetBool("no-root")
	noDev, _ := cmd.Flags().GetBool("no-dev")
	withGroups, _ := cmd.Flags().GetStringSlice("with")
	onlyGroup, _ := cmd.Flags().GetString("only")
	pkgScope, _ := cmd.Flags().GetString("package")

	groups := resolveInstallGroups(noDev, withGroups, onlyGroup)
	return installFromLock(cmd.OutOrStdout(), !noRoot, groups, pkgScope)
}

// resolveInstallGroups determines which groups to install based on flags.
func resolveInstallGroups(noDev bool, withGroups []string, onlyGroup string) []string {
	if onlyGroup != "" {
		return []string{onlyGroup}
	}
	groups := []string{"main"}
	if !noDev {
		groups = append(groups, "dev")
	}
	groups = append(groups, withGroups...)
	return groups
}

// installFromLock reads the project's lock file and installs packages into a venv.
// If installRoot is true, also installs the project itself in editable mode.
// groups controls which dependency groups to install (nil = all).
// pkgScope, when non-empty, restricts the install to packages reachable
// from the named workspace member's [project].dependencies — used by
// `pensa install --package <member>`.
// Shared between `install`, `add`, `update`, and `remove` commands.
func installFromLock(w io.Writer, installRoot bool, groups []string, pkgScope string) error {
	start := time.Now()

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Check for workspace — use workspace root for lock file and venv.
	ws, _ := workspace.Discover(dir)
	rootDir := dir
	if ws != nil {
		rootDir = ws.Root
	}

	lockPath, _ := lockfile.DetectLockFile(rootDir)
	if lockPath == "" {
		return fmt.Errorf("no lock file found (run 'pensa lock' first)")
	}
	lf, err := lockfile.ReadLockFile(lockPath)
	if err != nil {
		return fmt.Errorf("read lock file: %w", err)
	}

	if len(lf.Packages) == 0 {
		fmt.Fprintf(w, "No packages to install.\n")
		return nil
	}

	// Restrict to one workspace member's dep closure when --package is set.
	var scopedMember *workspace.Member
	if pkgScope != "" {
		filtered, member, err := lockfileScopedToMember(w, lf, ws, pkgScope)
		if err != nil {
			return err
		}
		lf = filtered
		scopedMember = member
	}

	// Pick Python: prefer the existing venv's pyvenv.cfg (the source of truth
	// for what `pensa run python` will load from), fall back to host PATH when
	// we're creating a new venv.
	venvPath := filepath.Join(rootDir, ".venv")
	py, err := pickPython(w, venvPath)
	if err != nil {
		return err
	}

	// Create installer.
	cacheDir := defaultCacheDir()
	client, err := newPyPIClient()
	if err != nil {
		return err
	}

	ins := installer.NewInstaller(client, venvPath, py, cacheDir)

	siteDir := py.SitePackagesDir(venvPath)
	installed, _ := installer.InstalledPackages(siteDir)
	projectNames := localProjectNames(rootDir)

	cfg, _ := config.New()
	verbose := cfg != nil && cfg.Verbose
	out := newUI(w, verbose, cfg != nil && cfg.Quiet)

	toInstall, toRemove := planVenvChanges(lf, installed, groups, py, projectNames)

	if len(toInstall) == 0 && len(toRemove) == 0 {
		fmt.Fprintf(w, "%s\n", green("All packages up to date."))
		if installRoot {
			if err := installEditableProjects(w, ws, scopedMember, dir, venvPath, py); err != nil {
				return err
			}
		}
		return nil
	}

	for _, pkg := range toRemove {
		if err := installer.UninstallPackage(siteDir, pkg.name, pkg.version); err != nil {
			return fmt.Errorf("uninstall %s: %w", pkg.name, err)
		}
	}

	if len(toInstall) > 0 {
		results, err := downloadPackages(w, ins, toInstall)
		if err != nil {
			return err
		}
		for _, res := range results {
			if err := ins.InstallFromCache(res.pkg, res.wheelPath); err != nil {
				return fmt.Errorf("install %s: %w", res.pkg.Name, err)
			}
		}
	}

	elapsed := time.Since(start)
	if len(toInstall) > 0 {
		out.Installed(len(toInstall), elapsed)
	}
	if len(toRemove) > 0 {
		out.Uninstalled(len(toRemove), elapsed)
	}

	if verbose {
		for _, pkg := range toRemove {
			out.DiffRemove(pkg.name, pkg.version)
		}
		for _, pkg := range toInstall {
			out.DiffAdd(pkg.Name, pkg.Version)
		}
	}

	if installRoot {
		if err := installEditableProjects(w, ws, scopedMember, dir, venvPath, py); err != nil {
			return err
		}
	}

	return nil
}

// lockfileScopedToMember returns a copy of lf containing only the
// packages reachable from the named workspace member's direct
// dependencies, plus a pointer to the resolved member so the caller
// can route editable installs to that member alone. Powers
// `pensa install --package <member>`.
//
// Member name lookup is PEP 503-normalized: pgmarketing-backend,
// pgmarketing_backend, and PGMarketing.Backend all match the same
// member. Roots that are declared in the member's pyproject but
// missing from the lock produce a warning so a stale lock is
// surfaced rather than silently filtered out.
func lockfileScopedToMember(
	w io.Writer,
	lf *lockfile.LockFile,
	ws *workspace.Workspace,
	pkgScope string,
) (*lockfile.LockFile, *workspace.Member, error) {
	if ws == nil {
		return nil, nil, fmt.Errorf("--package %q: not in a workspace", pkgScope)
	}
	target := pep508.NormalizeName(pkgScope)
	var member *workspace.Member
	for i := range ws.Members {
		if pep508.NormalizeName(ws.Members[i].Name) == target {
			member = &ws.Members[i]
			break
		}
	}
	if member == nil {
		names := make([]string, 0, len(ws.Members))
		for _, m := range ws.Members {
			names = append(names, m.Name)
		}
		return nil, nil, fmt.Errorf("--package %q: unknown workspace member (available: %v)", pkgScope, names)
	}

	deps, err := member.Project.ResolveAllDependencies()
	if err != nil {
		return nil, nil, fmt.Errorf("read %s deps: %w", member.Name, err)
	}
	roots := make([]string, 0, len(deps))
	for _, gd := range deps {
		roots = append(roots, normalizeName(gd.Dep.Name))
	}

	adj := make(map[string][]string, len(lf.Packages))
	for _, pkg := range lf.Packages {
		children := make([]string, 0, len(pkg.Dependencies))
		for depName := range pkg.Dependencies {
			children = append(children, normalizeName(depName))
		}
		adj[normalizeName(pkg.Name)] = children
	}

	for _, r := range roots {
		if _, present := adj[r]; !present {
			fmt.Fprintf(w, "warning: %s declares dep %q but it isn't in the lockfile (run 'pensa lock')\n", member.Name, r)
		}
	}

	reach := map[string]bool{}
	queue := append([]string{}, roots...)
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if reach[curr] {
			continue
		}
		reach[curr] = true
		queue = append(queue, adj[curr]...)
	}

	scoped := &lockfile.LockFile{Metadata: lf.Metadata}
	for _, pkg := range lf.Packages {
		if reach[normalizeName(pkg.Name)] {
			scoped.Packages = append(scoped.Packages, pkg)
		}
	}
	return scoped, member, nil
}

// compatibleWithPython checks whether a locked package is compatible with
// the current Python interpreter. Skips packages whose python-versions
// constraint excludes the current Python, or that have wheels but none
// matching the current CPython version.
//
// Upper bounds in the package's python-versions constraint are stripped
// before checking. Most `python<X.Y` declarations are
// defensive ("untested on newer") rather than hard incompatibilities;
// rejecting an install because of one is usually a worse outcome than
// letting the user run and discover any actual breakage themselves.
func compatibleWithPython(pkg lockfile.LockedPackage, py *python.PythonInfo) bool {
	pyVer, err := version.Parse(fmt.Sprintf("%d.%d.%d", py.Major, py.Minor, py.Patch))
	if err != nil {
		return true // can't parse, don't skip
	}

	// Check python-versions constraint (lower bound only).
	if pkg.PythonVersions != "" {
		constraint, err := version.ParseConstraint(pkg.PythonVersions)
		if err == nil {
			constraint = version.StripUpperBound(constraint)
			if !constraint.Allows(pyVer) {
				return false
			}
		}
	}

	// Check that we have at least one installable artifact for this platform.
	if !canInstallOnPlatform(pkg.Files, py) {
		return false
	}

	return true
}

// canInstallOnPlatform reports whether a package has any artifact we can
// use on the current platform: a wheel whose tags match, or a source
// distribution we can build. Returns false only when the lock lists
// wheels that all target other platforms AND no sdist is available
// (e.g. pywin32 on macOS) — those packages are legitimately skipped at
// install time.
//
// Returning true for sdist-only or sdist-plus-incompatible-wheel cases
// lets the installer attempt buildFromSdist and surface any real build
// failure (missing compiler, missing system library) as an actionable
// error, rather than a silent omission.
func canInstallOnPlatform(files []lockfile.PackageFile, py *python.PythonInfo) bool {
	cpTag := fmt.Sprintf("cp%d%d", py.Major, py.Minor)
	hasWheel := false
	hasSdist := false

	for _, f := range files {
		if strings.HasSuffix(f.File, ".tar.gz") || strings.HasSuffix(f.File, ".zip") {
			hasSdist = true
			continue
		}
		if !strings.HasSuffix(f.File, ".whl") {
			continue
		}
		hasWheel = true

		// Universal wheels always match.
		if strings.Contains(f.File, "-py3-none-any") || strings.Contains(f.File, "-py2.py3-none-any") {
			return true
		}

		// Check Python version tag.
		if !strings.Contains(f.File, cpTag) && !strings.Contains(f.File, "-py3-") {
			continue
		}

		// Check platform tag — skip wheels for other platforms.
		if !wheelMatchesPlatform(f.File, py.GOOS) {
			continue
		}

		return true
	}

	// No matching wheel. Fall back to sdist if we have one; otherwise
	// accept an empty file list (downstream will fail with a clear
	// "no sdist found for X" error instead of silently skipping).
	return hasSdist || !hasWheel
}

// wheelMatchesPlatform checks if a wheel filename is compatible with the
// target OS (a runtime.GOOS value). Wheel filenames end with
// {python}-{abi}-{platform}.whl. Taking goos as an argument rather than
// reading runtime.GOOS lets callers (and tests) ask about a platform
// other than the host's.
func wheelMatchesPlatform(filename, goos string) bool {
	// Platform-independent.
	if strings.Contains(filename, "-any.whl") {
		return true
	}

	switch {
	case strings.Contains(filename, "macosx") || strings.Contains(filename, "darwin"):
		return goos == "darwin"
	case strings.Contains(filename, "manylinux") || strings.Contains(filename, "musllinux") || strings.Contains(filename, "linux"):
		return goos == "linux"
	case strings.Contains(filename, "win32") || strings.Contains(filename, "win_amd64") || strings.Contains(filename, "win_arm64"):
		return goos == "windows"
	}

	return true // unknown platform tag, don't skip
}
