package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"pensa.sh/pensa/internal/cli"
)

func main() {
	// Inline so stop() can run before os.Exit — deferred funcs
	// are skipped on os.Exit, which would leak the signal handler.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := cli.ExecuteContext(ctx)
	stop()
	if err == nil {
		return
	}
	if errors.Is(err, context.Canceled) {
		// User asked us to stop (SIGINT/SIGTERM). Exit 130 matches
		// the POSIX convention of 128 + signal number for SIGINT.
		os.Exit(130)
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
