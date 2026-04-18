package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pensa.sh/pensa/internal/lockfile"
	"pensa.sh/pensa/internal/pyproject"
	"pensa.sh/pensa/internal/workspace"
	"github.com/spf13/cobra"
)

func newWhyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "why <package>",
		Short: "Explain why a package is installed",
		Long:  "Shows why a package is in the lock file — whether it's a direct dependency or pulled in transitively, and by whom.",
		Example: `  pensa why certifi
  pensa why url-normalize`,
		Args: cobra.ExactArgs(1),
		RunE: runWhy,
	}
}

func runWhy(cmd *cobra.Command, args []string) error {
	lf, err := readLockFileFromCwd()
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	target := normalizeName(args[0])

	// Check the package exists in the lock.
	idx := buildPackageIndex(lf.Packages)
	pkg, ok := idx[target]
	if !ok {
		return fmt.Errorf("package %q not found in lock file", args[0])
	}

	// Check if it's a direct dependency.
	directNames, err := getDirectDepNames()
	if err != nil {
		return err
	}

	if directNames[target] {
		fmt.Fprintf(w, "%s is a direct dependency.\n", bold(pkg.Name))
		return nil
	}

	// Build reverse dep index: who depends on this package?
	reverseDeps := buildReverseDeps(lf)

	// Find the chain from a direct dep down to the target.
	chain := findDepChain(target, reverseDeps, directNames)
	if len(chain) == 0 {
		fmt.Fprintf(w, "%s is a transitive dependency (chain unknown).\n", bold(pkg.Name))
		return nil
	}

	// Print the chain as a tree.
	// chain is [direct_dep, ..., parent_of_target, target]
	topPkg := idx[chain[0]]
	fmt.Fprintf(w, "%s %s\n", bold(topPkg.Name), topPkg.Version)
	for i := 1; i < len(chain); i++ {
		indent := ""
		for j := 0; j < i-1; j++ {
			indent += "    "
		}
		connector := "└── "
		name := chain[i]
		// Show the constraint from the parent.
		parentPkg := idx[chain[i-1]]
		constraint := parentPkg.Dependencies[name]
		if constraint == "" {
			// Try original case name.
			for depName, c := range parentPkg.Dependencies {
				if normalizeName(depName) == name {
					constraint = c
					break
				}
			}
		}
		if constraint != "" {
			fmt.Fprintf(w, "%s%s%s %s\n", indent, connector, bold(name), constraint)
		} else {
			fmt.Fprintf(w, "%s%s%s\n", indent, connector, bold(name))
		}
	}

	return nil
}

// getDirectDepNames returns a set of normalized direct dependency names.
func getDirectDepNames() (map[string]bool, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	// Check workspace.
	if ws, _ := workspace.Discover(dir); ws != nil {
		names := make(map[string]bool)
		for _, m := range ws.Members {
			deps, err := m.Project.ResolveAllDependencies()
			if err != nil {
				continue
			}
			for _, d := range deps {
				names[normalizeName(d.Dep.Name)] = true
			}
		}
		return names, nil
	}

	pp, err := pyproject.ReadPyProject(filepath.Join(dir, "pyproject.toml"))
	if err != nil {
		return nil, err
	}
	deps, err := pp.ResolveAllDependencies()
	if err != nil {
		return nil, err
	}
	names := make(map[string]bool, len(deps))
	for _, d := range deps {
		names[normalizeName(d.Dep.Name)] = true
	}
	return names, nil
}

// buildReverseDeps builds a map from package name → list of packages that depend on it.
// Includes both regular dependencies and extras dependencies.
func buildReverseDeps(lf *lockfile.LockFile) map[string][]string {
	rev := make(map[string][]string)
	seen := make(map[string]map[string]bool) // prevent duplicates

	addRev := func(depName, parentName string) {
		dep := normalizeName(depName)
		if seen[dep] == nil {
			seen[dep] = make(map[string]bool)
		}
		if !seen[dep][parentName] {
			seen[dep][parentName] = true
			rev[dep] = append(rev[dep], parentName)
		}
	}

	for _, pkg := range lf.Packages {
		pkgName := normalizeName(pkg.Name)
		for dep := range pkg.Dependencies {
			addRev(dep, pkgName)
		}
		// Also include extras deps (e.g., black[d] → aiohttp).
		for _, extraDeps := range pkg.Extras {
			for _, entry := range extraDeps {
				// Extras entries are like "aiohttp (>=3.10)" — extract name.
				name := entry
				if i := strings.Index(entry, " "); i > 0 {
					name = entry[:i]
				}
				if i := strings.Index(name, "("); i > 0 {
					name = name[:i]
				}
				name = strings.TrimSpace(name)
				if name != "" {
					addRev(name, pkgName)
				}
			}
		}
	}
	for k := range rev {
		sort.Strings(rev[k])
	}
	return rev
}

// findDepChain finds a path from a direct dep to the target via BFS on reverse deps.
// Returns the chain as [direct_dep, ..., parent, target].
func findDepChain(target string, reverseDeps map[string][]string, directDeps map[string]bool) []string {
	type node struct {
		name string
		path []string
	}

	visited := make(map[string]bool)
	queue := []node{{name: target, path: []string{target}}}
	visited[target] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		parents := reverseDeps[current.name]
		for _, parent := range parents {
			if visited[parent] {
				continue
			}
			newPath := make([]string, len(current.path)+1)
			copy(newPath[1:], current.path)
			newPath[0] = parent

			if directDeps[parent] {
				return newPath // found a path from direct dep to target
			}

			visited[parent] = true
			queue = append(queue, node{name: parent, path: newPath})
		}
	}

	return nil
}
