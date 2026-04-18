package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juanbzz/pensa/internal/python"
	"github.com/juanbzz/pensa/internal/workspace"
	"github.com/spf13/cobra"
)

func newShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <package>",
		Short: "Show details about a package",
		Long:  "Shows detailed information about a specific package from the lock file.",
		Example: `  pensa show requests
  pensa show charset-normalizer`,
		Args: cobra.ExactArgs(1),
		RunE: runShow,
	}
}

func runShow(cmd *cobra.Command, args []string) error {
	lf, err := readLockFileFromCwd()
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	normalized := normalizeName(args[0])

	for _, p := range lf.Packages {
		if normalizeName(p.Name) != normalized {
			continue
		}

		fmt.Fprintf(w, "Name: %s\n", bold(p.Name))
		fmt.Fprintf(w, "Version: %s\n", p.Version)

		// Location: check site-packages.
		if loc := findPackageLocation(p.Name, p.Version); loc != "" {
			fmt.Fprintf(w, "Location: %s\n", loc)
		}

		// Requires (dependencies with constraints).
		fmt.Fprintf(w, "Requires:\n")
		if len(p.Dependencies) > 0 {
			depNames := make([]string, 0, len(p.Dependencies))
			for d := range p.Dependencies {
				depNames = append(depNames, d)
			}
			sort.Strings(depNames)
			for _, d := range depNames {
				constraint := p.Dependencies[d]
				if constraint != "" {
					fmt.Fprintf(w, "  * %s %s\n", d, constraint)
				} else {
					fmt.Fprintf(w, "  * %s\n", d)
				}
			}
		}

		// Required-by: find packages that depend on this one.
		fmt.Fprintf(w, "Required-by:\n")
		type revDep struct {
			name       string
			constraint string
		}
		var requiredBy []revDep
		for _, other := range lf.Packages {
			for dep, constraint := range other.Dependencies {
				if normalizeName(dep) == normalized {
					requiredBy = append(requiredBy, revDep{other.Name, constraint})
				}
			}
		}
		sort.Slice(requiredBy, func(i, j int) bool {
			return requiredBy[i].name < requiredBy[j].name
		})
		for _, r := range requiredBy {
			if r.constraint != "" {
				fmt.Fprintf(w, "  * %s %s\n", r.name, r.constraint)
			} else {
				fmt.Fprintf(w, "  * %s\n", r.name)
			}
		}

		return nil
	}

	return fmt.Errorf("package %q not found in lock file", args[0])
}

// findPackageLocation returns the site-packages path for an installed package.
func findPackageLocation(name, version string) string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	venvDir := dir
	if ws, _ := workspace.Discover(dir); ws != nil {
		venvDir = ws.Root
	}
	venvPath := filepath.Join(venvDir, ".venv")

	// Nothing to show if the venv doesn't exist.
	if !python.VenvExists(venvPath) {
		return ""
	}
	py, err := python.FromVenv(venvPath)
	if err != nil {
		return ""
	}

	siteDir := py.SitePackagesDir(venvPath)
	normalized := strings.ReplaceAll(strings.ToLower(name), "-", "_")
	distInfo := filepath.Join(siteDir, normalized+"-"+version+".dist-info")
	if _, err := os.Stat(distInfo); err == nil {
		return siteDir
	}
	return ""
}
