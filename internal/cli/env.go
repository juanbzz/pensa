package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juanbzz/pensa/internal/python"
	"github.com/spf13/cobra"
)

func newEnvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Show the virtualenv path",
		Long:  "Prints the path to the project's virtual environment. Use -v for more detail.",
		Example: `  pensa env
  pensa env -v`,
		Args: cobra.NoArgs,
		RunE: runEnv,
	}
	cmd.Flags().BoolP("verbose", "v", false, "Show Python version and executable path")
	return cmd
}

func runEnv(cmd *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	venvPath := filepath.Join(dir, ".venv")
	if !python.VenvExists(venvPath) {
		return fmt.Errorf("no virtualenv found. Run \"pensa install\" to create one.")
	}

	w := cmd.OutOrStdout()
	verbose, _ := cmd.Flags().GetBool("verbose")

	if !verbose {
		fmt.Fprintln(w, venvPath)
		return nil
	}

	fmt.Fprintf(w, "Path:       %s\n", venvPath)

	py, err := python.Discover()
	if err != nil {
		fmt.Fprintf(w, "Python:     unknown\n")
		fmt.Fprintf(w, "Executable: unknown\n")
		return nil
	}

	fmt.Fprintf(w, "Python:     %s\n", py.Version)
	fmt.Fprintf(w, "Executable: %s\n", filepath.Join(venvPath, "bin", "python"))

	return nil
}
