package cli

import (
	"fmt"
	"sort"

	"github.com/juanbzz/goetry/internal/lockfile"
	"github.com/spf13/cobra"
)

func newShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <package>",
		Short: "Show details about a package",
		Long:  "Shows detailed information about a specific package from the lock file.",
		Example: `  goetry show requests
  goetry show charset-normalizer`,
		Args: cobra.ExactArgs(1),
		RunE: runShow,
	}
}

func runShow(cmd *cobra.Command, args []string) error {
	lf, err := readLockFileFromCwd()
	if err != nil {
		return err
	}

	return showPackageDetail(cmd.OutOrStdout(), lf, args[0])
}

// showPackageDetail prints detailed info for a single package.
func showPackageDetail(w interface{ Write([]byte) (int, error) }, lf *lockfile.LockFile, name string) error {
	normalized := normalizeName(name)

	for _, p := range lf.Packages {
		if normalizeName(p.Name) == normalized {
			fmt.Fprintf(w, " name        : %s\n", p.Name)
			fmt.Fprintf(w, " version     : %s\n", p.Version)
			fmt.Fprintf(w, " description : %s\n", p.Description)

			if len(p.Dependencies) > 0 {
				fmt.Fprintf(w, "\n dependencies\n")
				depNames := make([]string, 0, len(p.Dependencies))
				for d := range p.Dependencies {
					depNames = append(depNames, d)
				}
				sort.Strings(depNames)
				for _, d := range depNames {
					fmt.Fprintf(w, "  - %s %s\n", d, p.Dependencies[d])
				}
			}
			return nil
		}
	}

	return fmt.Errorf("package %q not found in poetry.lock", name)
}
