package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
)

// withSpinner runs fn while showing a spinner with the given message.
// If output is not a TTY (piped, tests), it prints the message and runs without a spinner.
func withSpinner(w io.Writer, message string, fn func() error) error {
	// Check if w is a TTY (only show spinner for interactive terminals).
	if f, ok := w.(*os.File); !ok || !isatty.IsTerminal(f.Fd()) {
		fmt.Fprintf(w, "%s\n", message)
		return fn()
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Writer = w.(*os.File)
	s.Suffix = " " + message
	s.Color("blue")
	s.Start()

	err := fn()

	s.Stop()
	// Clear the spinner line.
	fmt.Fprintf(w, "\r\033[K")

	return err
}

// withSpinnerMsg runs fn while showing a spinner, then prints a final message on success.
func withSpinnerMsg(w io.Writer, spinMsg string, doneMsg string, fn func() error) error {
	err := withSpinner(w, spinMsg, fn)
	if err != nil {
		return err
	}
	if doneMsg != "" {
		fmt.Fprintf(w, "%s\n", doneMsg)
	}
	return nil
}

// isTerminal checks if w is a file descriptor pointing to a terminal.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd())
}

// downloadSpinner shows a spinner during parallel downloads.
// It takes a count and returns a stop function.
func downloadSpinner(w io.Writer, count int) func() {
	msg := fmt.Sprintf("Downloading %d packages...", count)

	if !isTerminal(w) {
		fmt.Fprintf(w, "%s\n", blue(msg))
		return func() {}
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Writer = w.(*os.File)
	s.Suffix = " " + color.BlueString(msg)
	s.Color("blue")
	s.Start()

	return func() {
		s.Stop()
		fmt.Fprintf(w, "\r\033[K")
	}
}
