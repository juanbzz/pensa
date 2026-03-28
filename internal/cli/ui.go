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
// Minimal color: bold on counts only, plain text otherwise.

func (u *ui) Resolved(count int, elapsed time.Duration) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, "Resolved %s in %s\n",
		bold(pluralPkgs(count)), formatDuration(elapsed))
}

func (u *ui) Installed(count int, elapsed time.Duration) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, "Installed %s in %s\n",
		bold(pluralPkgs(count)), formatDuration(elapsed))
}

func (u *ui) Uninstalled(count int, elapsed time.Duration) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, "Uninstalled %s in %s\n",
		bold(pluralPkgs(count)), formatDuration(elapsed))
}

func (u *ui) UpToDate(msg string) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, "%s\n", msg)
}

// --- Diff lines (shown unless quiet) ---
// Color only on +/- prefix. Package name bold, version plain.

func (u *ui) DiffAdd(name, version string) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, " %s %s==%s\n", green("+"), bold(name), version)
}

func (u *ui) DiffRemove(name, version string) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, " %s %s==%s\n", red("-"), bold(name), version)
}

// --- Info messages ---

func (u *ui) Info(msg string) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, "%s\n", msg)
}

func (u *ui) Infof(format string, args ...any) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, format+"\n", args...)
}

func (u *ui) Workspace(memberCount int) {
	if u.quiet {
		return
	}
	fmt.Fprintf(u.w, "Locking workspace with %d members\n", memberCount)
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

// --- Helpers ---

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

func pluralPkgs(n int) string {
	if n == 1 {
		return "1 package"
	}
	return fmt.Sprintf("%d packages", n)
}
