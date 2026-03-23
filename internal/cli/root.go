package cli

import (
	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags.
var Version = "dev"

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pensa",
		Short: "A fast enough Python package and project manager, written in Go",
		Long:  "A fast enough Python package and project manager, written in Go.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newNewCmd())
	cmd.AddCommand(newLockCmd())
	cmd.AddCommand(newAddCmd())
	cmd.AddCommand(newInstallCmd())
	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newRemoveCmd())
	cmd.AddCommand(newCheckCmd())
	cmd.AddCommand(newEnvCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newShowCmd())
	cmd.AddCommand(newTreeCmd())

	return cmd
}

func Execute() error {
	return newRootCmd().Execute()
}
