package cli

import (
	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags.
var Version = "dev"

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "goetry",
		Short: "A fast Python package manager",
		Long:  "Goetry is a fast Python package manager written in Go, inspired by Poetry's UX.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(newVersionCmd())

	return cmd
}

func Execute() error {
	return newRootCmd().Execute()
}
