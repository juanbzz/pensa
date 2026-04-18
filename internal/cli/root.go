package cli

import (
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"pensa.sh/pensa/internal/config"
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

	// Global flags.
	cmd.PersistentFlags().BoolP("verbose", "v", false, "Show per-package details")
	cmd.PersistentFlags().BoolP("quiet", "q", false, "Suppress all output except errors")
	cmd.PersistentFlags().Bool("color", false, "Force color output")
	cmd.PersistentFlags().Bool("no-color", false, "Disable color output")

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
	cmd.AddCommand(newWhyCmd())

	return cmd
}

func Execute() error {
	return newRootCmd().Execute()
}

// uiFromCmd creates a ui instance from cobra command flags + config.
// Status output goes to stderr for side-effect commands.
func uiFromCmd(cmd *cobra.Command) *ui {
	cfg, _ := config.New()

	verbose, _ := cmd.Flags().GetBool("verbose")
	quiet, _ := cmd.Flags().GetBool("quiet")
	forceColor, _ := cmd.Flags().GetBool("color")
	noColor, _ := cmd.Flags().GetBool("no-color")

	// Merge: flag > env var.
	if cfg != nil {
		if !verbose && cfg.Verbose {
			verbose = true
		}
		if !quiet && cfg.Quiet {
			quiet = true
		}
	}

	// Color control: --no-color > --color > NO_COLOR > PENSA_COLOR > auto.
	if noColor {
		color.NoColor = true
	} else if forceColor {
		color.NoColor = false
	} else if cfg != nil && cfg.Color == "never" {
		color.NoColor = true
	} else if cfg != nil && cfg.Color == "always" {
		color.NoColor = false
	}
	// "auto" is the default — fatih/color handles TTY detection.

	return newUI(os.Stderr, verbose, quiet)
}
