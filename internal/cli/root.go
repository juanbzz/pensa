package cli

import (
	"github.com/fatih/color"
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

	// Color the help template.
	heading := color.New(color.Bold, color.FgBlue).SprintFunc()
	cmdName := color.New(color.FgGreen).SprintFunc()
	flagStyle := color.New(color.FgYellow).SprintFunc()
	cobra.AddTemplateFunc("heading", heading)
	cobra.AddTemplateFunc("cmdName", cmdName)
	cobra.AddTemplateFunc("flagStyle", flagStyle)
	cmd.SetUsageTemplate(`{{heading "Usage:"}}
  {{.UseLine}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

{{heading "Aliases:"}}
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

{{heading "Examples:"}}
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

{{heading "Available Commands:"}}{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{cmdName (rpad .Name .NamePadding)}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{heading "Flags:"}}
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

{{heading "Global Flags:"}}
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

{{heading "Additional help topics:"}}{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{flagStyle .CommandPath}} [command] --help" for more information about a command.{{end}}
`)

	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newNewCmd())
	cmd.AddCommand(newLockCmd())
	cmd.AddCommand(newAddCmd())
	cmd.AddCommand(newInstallCmd())
	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newRemoveCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newSyncCmd())
	cmd.AddCommand(newBuildCmd())
	cmd.AddCommand(newPublishCmd())
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
