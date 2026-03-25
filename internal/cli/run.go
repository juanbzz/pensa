package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/juanbzz/pensa/internal/lockfile"
	"github.com/juanbzz/pensa/internal/python"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "run [command] [args...]",
		Short:              "Run a command inside the project's virtualenv",
		Long:               "Executes the given command with the virtualenv's Python and PATH.\nAuto-syncs the venv if it's out of date. Use --no-sync to skip.",
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: true,
		RunE:               runRun,
	}
}

func runRun(cmd *cobra.Command, args []string) error {
	// Parse --no-sync manually since DisableFlagParsing is true.
	noSync := false
	runArgs := args
	for i, a := range args {
		if a == "--no-sync" {
			noSync = true
			runArgs = append(args[:i], args[i+1:]...)
			break
		}
		if a == "--" {
			break
		}
	}

	if len(runArgs) == 0 {
		return fmt.Errorf("no command specified")
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	venvPath := filepath.Join(dir, ".venv")

	if !noSync {
		if err := autoSync(dir, venvPath); err != nil {
			return err
		}
	} else if !python.VenvExists(venvPath) {
		return fmt.Errorf("no virtualenv found at %s (run 'pensa install' first)", venvPath)
	}

	binDir := filepath.Join(venvPath, "bin")

	// Build the command.
	command := runArgs[0]
	commandArgs := runArgs[1:]

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

const syncMarker = ".pensa-sync"

func autoSync(dir, venvPath string) error {
	lockPath, _ := lockfile.DetectLockFile(dir)
	if lockPath == "" {
		return fmt.Errorf("no lock file found (run 'pensa lock' first)")
	}

	// Check if venv exists and is up to date via marker file.
	if python.VenvExists(venvPath) && !needsSync(lockPath, venvPath) {
		return nil
	}

	fmt.Fprintf(os.Stderr, "pensa: syncing venv...\n")
	return installFromLock(os.Stderr, true, nil)
}

func needsSync(lockPath, venvPath string) bool {
	markerPath := filepath.Join(venvPath, syncMarker)

	markerInfo, err := os.Stat(markerPath)
	if err != nil {
		return true // no marker = needs sync
	}

	lockInfo, err := os.Stat(lockPath)
	if err != nil {
		return true
	}

	return lockInfo.ModTime().After(markerInfo.ModTime())
}

func writeSyncMarker(venvPath string) {
	path := filepath.Join(venvPath, syncMarker)
	os.WriteFile(path, []byte{}, 0644)
}
