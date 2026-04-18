package cli

import (
	"fmt"
	"sort"

	"pensa.sh/pensa/internal/lockfile"
	"github.com/spf13/cobra"
)

func newTreeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tree [package]",
		Short: "Show the dependency tree",
		Long:  "Displays packages and their dependencies as a tree.",
		Example: `  pensa tree
  pensa tree requests
  pensa tree --top-level`,
		Args: cobra.MaximumNArgs(1),
		RunE: runTree,
	}
	cmd.Flags().BoolP("top-level", "T", false, "Show only top-level dependency trees")
	return cmd
}

func runTree(cmd *cobra.Command, args []string) error {
	lf, err := readLockFileFromCwd()
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	byName := buildPackageIndex(lf.Packages)

	// Single package tree.
	if len(args) == 1 {
		normalized := normalizeName(args[0])
		pkg, ok := byName[normalized]
		if !ok {
			return fmt.Errorf("package %q not found in poetry.lock", args[0])
		}
		fmt.Fprintf(w, "%s %s %s\n", pkg.Name, pkg.Version, pkg.Description)
		printDeps(w, pkg, byName, "", make(map[string]bool))
		return nil
	}

	roots := lf.Packages

	topLevel, _ := cmd.Flags().GetBool("top-level")
	if topLevel {
		roots, err = filterTopLevel(roots)
		if err != nil {
			return err
		}
	}

	sortPackages(roots)

	for _, pkg := range roots {
		fmt.Fprintf(w, "%s %s %s\n", pkg.Name, pkg.Version, pkg.Description)
		printDeps(w, pkg, byName, "", make(map[string]bool))
	}
	return nil
}

// printDeps recursively prints the dependency tree with box-drawing characters.
func printDeps(w interface{ Write([]byte) (int, error) }, pkg lockfile.LockedPackage, byName map[string]lockfile.LockedPackage, prefix string, seen map[string]bool) {
	depNames := make([]string, 0, len(pkg.Dependencies))
	for d := range pkg.Dependencies {
		depNames = append(depNames, d)
	}
	sort.Strings(depNames)

	for i, depName := range depNames {
		constraint := pkg.Dependencies[depName]
		isLast := i == len(depNames)-1

		connector := "├──"
		childPrefix := "│   "
		if isLast {
			connector = "└──"
			childPrefix = "    "
		}

		normalized := normalizeName(depName)
		if child, ok := byName[normalized]; ok {
			fmt.Fprintf(w, "%s%s %s %s\n", prefix, connector, child.Name, child.Version)
			if !seen[normalized] {
				seen[normalized] = true
				printDeps(w, child, byName, prefix+childPrefix, seen)
				delete(seen, normalized)
			}
		} else {
			fmt.Fprintf(w, "%s%s %s %s\n", prefix, connector, depName, constraint)
		}
	}
}
