package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"pensa.sh/pensa/internal/python"
	"pensa.sh/pensa/internal/workspace"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [flags] [--] command [args...]",
		Short: "Run a command inside the project's virtualenv",
		Long: `Syncs the virtualenv, then executes the given command with the venv's Python and PATH.

Use -- to separate pensa flags from the command and its arguments:
  pensa run --no-sync -- python script.py
  pensa run --package backend -- uvicorn --reload

Without flags, -- is not needed:
  pensa run python script.py`,
		Args: cobra.MinimumNArgs(1),
		RunE: runRun,
	}
	cmd.Flags().Bool("no-sync", false, "Skip venv sync before running")
	cmd.Flags().String("package", "", "Target workspace member (accepted for uv compat)")
	cmd.Flags().SetInterspersed(false) // stop parsing flags after first positional arg
	return cmd
}

func runRun(cmd *cobra.Command, args []string) error {
	noSync, _ := cmd.Flags().GetBool("no-sync")
	// --package is accepted for uv Taskfile compat but not used —
	// the venv is shared across workspace members.

	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Determine venv path (workspace root or project dir).
	venvDir := dir
	if ws, _ := workspace.Discover(dir); ws != nil {
		venvDir = ws.Root
	}
	venvPath := filepath.Join(venvDir, ".venv")

	// Auto-sync: ensure venv is up to date.
	if !noSync {
		// Output to stderr so it doesn't pollute command stdout.
		if err := installFromLock(os.Stderr, true, nil); err != nil {
			return fmt.Errorf("sync: %w", err)
		}
	} else if !python.VenvExists(venvPath) {
		return fmt.Errorf("no virtualenv found at %s (run 'pensa install' or remove --no-sync)", venvPath)
	}

	// Exec the command in the venv.
	binDir := filepath.Join(venvPath, "bin")

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
