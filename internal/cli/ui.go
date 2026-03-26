package cli

import (
	"fmt"
	"io"
	"time"
)

// ui holds output configuration for the session.
// All output goes to w (typically os.Stderr for side-effect commands).
type ui struct {
	w       io.Writer
	verbose bool
	quiet   bool
}

func newUI(w io.Writer, verbose, quiet bool) *ui {
	return &ui{w: w, verbose: verbose, quiet: quiet}
}

// --- Phase summaries (shown unless quiet) ---

func (u *ui) Resolved(count int, elapsed time.Duration) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, "%s %s packages in %s\n",
		green("Resolved"), bold(fmt.Sprintf("%d", count)), formatDuration(elapsed))
}

func (u *ui) Installed(count int, elapsed time.Duration) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, "%s %s packages in %s\n",
		green("Installed"), bold(fmt.Sprintf("%d", count)), formatDuration(elapsed))
}

func (u *ui) Wrote(filename string) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, "%s %s\n", green("Wrote"), filename)
}

func (u *ui) UpToDate(msg string) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, "%s\n", green(msg))
}

// --- Action verbs (add/remove/update) ---

func (u *ui) Added(name, version string) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, "%s %s %s\n", green("Added"), bold(name), dim("v"+version))
}

func (u *ui) Removed(name, version string) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, "%s %s %s\n", red("Removed"), bold(name), dim("v"+version))
}

func (u *ui) Updated(name, from, to string) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, "%s %s %s -> %s\n",
		green("Updated"), bold(name), dim("v"+from), dim("v"+to))
}

// --- Verbose-only: per-package diff lines ---

func (u *ui) DiffAdd(name, version string) {
	if !u.verbose {
		return
	}
	fmt.Fprintf(u.w, " %s %s %s\n", green("+"), bold(name), dim(version))
}

func (u *ui) DiffRemove(name, version string) {
	if !u.verbose {
		return
	}
	fmt.Fprintf(u.w, " %s %s %s\n", red("-"), bold(name), dim(version))
}

// --- Info messages ---

func (u *ui) Info(msg string) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, "%s\n", msg)
}

func (u *ui) Infof(format string, args ...interface{}) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, format+"\n", args...)
}

func (u *ui) Workspace(memberCount int) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, "%s workspace with %d members\n", blue("Locking"), memberCount)
}

func (u *ui) Member(name string) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, "  %s %s\n", dim("•"), name)
}

// --- Diagnostics (never suppressed) ---

func (u *ui) Error(msg string) {
	fmt.Fprintf(u.w, "%s %s\n", red(bold("error:")), msg)
}

func (u *ui) Warning(msg string) {
	fmt.Fprintf(u.w, "%s %s\n", yellow(bold("warning:")), msg)
}

func (u *ui) Hint(msg string) {
	fmt.Fprintf(u.w, "%s %s\n", cyan(bold("hint:")), msg)
}

// --- Timing ---

func formatDuration(d time.Duration) string {
	switch {
	case d >= time.Minute:
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	case d >= time.Second:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
}
