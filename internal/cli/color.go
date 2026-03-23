package cli

import "github.com/fatih/color"

// Color functions for consistent output across commands.
// fatih/color auto-disables when output is not a TTY (piping, tests)
// and respects NO_COLOR env var.

var (
	green  = color.New(color.FgGreen).SprintFunc()
	blue   = color.New(color.FgBlue).SprintFunc()
	yellow = color.New(color.FgYellow).SprintFunc()
	red    = color.New(color.FgRed).SprintFunc()
	bold   = color.New(color.Bold).SprintFunc()
	dim    = color.New(color.Faint).SprintFunc()
)
