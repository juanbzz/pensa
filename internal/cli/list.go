package cli

import (
	"fmt"

	"github.com/juanbzz/pensa/internal/lockfile"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List packages from the lock file",
		Long:  "Lists all packages from the lock file in a table.",
		Example: `  pensa list
  pensa list --top-level`,
		Args: cobra.NoArgs,
		RunE: runList,
	}
	cmd.Flags().BoolP("top-level", "T", false, "Show only top-level dependencies")
	return cmd
}

func runList(cmd *cobra.Command, args []string) error {
	lf, err := readLockFileFromCwd()
	if err != nil {
		return err
	}

	pkgs := lf.Packages

	topLevel, _ := cmd.Flags().GetBool("top-level")
	if topLevel {
		pkgs, err = filterTopLevel(pkgs)
		if err != nil {
			return err
		}
	}

	sortPackages(pkgs)
	printPackageTable(cmd.OutOrStdout(), pkgs)
	return nil
}

// printPackageTable prints packages as an aligned table: name version description.
func printPackageTable(w interface{ Write([]byte) (int, error) }, pkgs []lockfile.LockedPackage) {
	if len(pkgs) == 0 {
		fmt.Fprintf(w, "No packages found.\n")
		return
	}

	nameW, verW := 0, 0
	for _, p := range pkgs {
		if len(p.Name) > nameW {
			nameW = len(p.Name)
		}
		if len(p.Version) > verW {
			verW = len(p.Version)
		}
	}

	for _, p := range pkgs {
		fmt.Fprintf(w, "%-*s  %-*s  %s\n", nameW, p.Name, verW, p.Version, p.Description)
	}
}
