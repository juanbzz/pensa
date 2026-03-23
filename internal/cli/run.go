package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/juanbzz/pensa/internal/python"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "run [command] [args...]",
		Short:              "Run a command inside the project's virtualenv",
		Long:               "Executes the given command with the virtualenv's Python and PATH.",
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: true,
		RunE:               runRun,
	}
}

func runRun(cmd *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	venvPath := filepath.Join(dir, ".venv")
	if !python.VenvExists(venvPath) {
		return fmt.Errorf("no virtualenv found at %s (run 'pensa install' first)", venvPath)
	}

	binDir := filepath.Join(venvPath, "bin")

	// Build the command.
	command := args[0]
	commandArgs := args[1:]

	// Check if the command exists in the venv bin dir.
	venvCmd := filepath.Join(binDir, command)
	if _, err := os.Stat(venvCmd); err == nil {
		command = venvCmd
	}

	c := exec.Command(command, commandArgs...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	// Set up environment with venv bin dir prepended to PATH.
	env := os.Environ()
	for i, e := range env {
		if len(e) > 5 && e[:5] == "PATH=" {
			env[i] = "PATH=" + binDir + string(os.PathListSeparator) + e[5:]
		}
	}
	env = append(env, "VIRTUAL_ENV="+venvPath)
	c.Env = env

	if err := c.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}

	return nil
}
