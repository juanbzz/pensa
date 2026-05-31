package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"pensa.sh/pensa/internal/lockfile"
	"pensa.sh/pensa/internal/pyproject"
	"pensa.sh/pensa/internal/workspace"
	"pensa.sh/pensa/pkg/version"
)

// OutdatedEntry is one row of `pensa outdated` output.
type OutdatedEntry struct {
	Name    string `json:"name"`
	Current string `json:"current"`
	Latest  string `json:"latest"`
	Level   string `json:"level"` // "major" | "minor" | "patch"
}

// errOutdatedFound is returned by runOutdated when any package is
// behind its latest compatible version. cobra translates a non-nil
// error from RunE into a non-zero exit code; we silence the error
// printing so the user sees only the rendered table or JSON.
var errOutdatedFound = errors.New("packages are outdated")

func newOutdatedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "outdated",
		Short: "Show packages with newer versions available",
		Long: `Compare lock-file versions against the latest stable compatible version
on PyPI. By default lists only top-level dependencies; use --all to include
transitive deps. Exits non-zero when any package is behind, so CI can gate
on the result.`,
		Example: `  pensa outdated
  pensa outdated --all
  pensa outdated --json`,
		Args: cobra.NoArgs,
		RunE: runOutdated,
	}
	cmd.Flags().Bool("all", false, "Include transitive dependencies (default: top-level only)")
	cmd.Flags().Bool("json", false, "Emit JSON instead of the default table")
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	return cmd
}

func runOutdated(cmd *cobra.Command, args []string) error {
	lf, err := readLockFileFromCwd()
	if err != nil {
		return err
	}

	all, _ := cmd.Flags().GetBool("all")
	jsonOut, _ := cmd.Flags().GetBool("json")

	pkgs := lf.Packages
	if !all {
		pkgs, err = filterTopLevel(pkgs)
		if err != nil {
			return err
		}
	}
	sortPackages(pkgs)

	requiresPython, err := projectRequiresPython()
	if err != nil {
		return err
	}

	client, err := newPyPIClient()
	if err != nil {
		return err
	}
	lookup := func(name string) (version.Version, error) {
		return getLatestCompatibleVersion(client, name, requiresPython)
	}

	entries := findOutdated(pkgs, lookup)

	if jsonOut {
		writeOutdatedJSON(cmd.OutOrStdout(), entries)
	} else {
		writeOutdatedTable(cmd.OutOrStdout(), entries)
	}

	if len(entries) > 0 {
		return errOutdatedFound
	}
	return nil
}

// findOutdated walks pkgs and returns entries whose latest compatible
// version is newer than the locked one. Skips on parse failure or
// lookup error so a partial PyPI outage doesn't kill the report.
func findOutdated(pkgs []lockfile.LockedPackage, latest func(name string) (version.Version, error)) []OutdatedEntry {
	var out []OutdatedEntry
	for _, p := range pkgs {
		cur, err := version.Parse(p.Version)
		if err != nil {
			continue
		}
		lat, err := latest(p.Name)
		if err != nil {
			continue
		}
		if version.Compare(lat, cur) <= 0 {
			continue
		}
		out = append(out, OutdatedEntry{
			Name:    p.Name,
			Current: p.Version,
			Latest:  lat.String(),
			Level:   bumpLevel(cur, lat),
		})
	}
	return out
}

// bumpLevel returns "major", "minor", or "patch" — the highest version
// component where cur and latest differ. Used to colour or sort entries
// by severity in clients that want it; the JSON shape exposes it
// directly.
func bumpLevel(cur, latest version.Version) string {
	if latest.Major() != cur.Major() {
		return "major"
	}
	if latest.Minor() != cur.Minor() {
		return "minor"
	}
	return "patch"
}

// projectRequiresPython reads the project's requires-python (or, in a
// workspace, intersects all members') so the latest-version lookup
// respects what the project can actually run.
func projectRequiresPython() (version.Constraint, error) {
	dir, err := workspaceOrCwd()
	if err != nil {
		return nil, err
	}
	pp, err := pyproject.ReadPyProject(dir + "/pyproject.toml")
	if err != nil {
		// Workspace roots may not declare [project]; that's OK — fall
		// through to no constraint. requires-python is a hint, not a
		// hard requirement for outdated reporting.
		return nil, nil
	}
	if pp.Project == nil || pp.Project.RequiresPython == "" {
		return nil, nil
	}
	c, _ := version.ParseConstraint(pp.Project.RequiresPython)
	return c, nil
}

func workspaceOrCwd() (string, error) {
	dir, err := getCwd()
	if err != nil {
		return "", err
	}
	if ws, _ := workspace.Discover(dir); ws != nil {
		return ws.Root, nil
	}
	return dir, nil
}

// getCwd is a thin wrapper for testability; os.Getwd is what callers
// elsewhere in this package use directly, but isolating it here keeps
// the read-pyproject path one fewer thing to mock.
func getCwd() (string, error) {
	return os.Getwd()
}

func writeOutdatedTable(w io.Writer, entries []OutdatedEntry) {
	if len(entries) == 0 {
		fmt.Fprintln(w, "All top-level dependencies are up to date.")
		return
	}

	nameW, curW, latW, lvlW := len("Package"), len("Current"), len("Latest"), len("Level")
	for _, e := range entries {
		if n := len(e.Name); n > nameW {
			nameW = n
		}
		if n := len(e.Current); n > curW {
			curW = n
		}
		if n := len(e.Latest); n > latW {
			latW = n
		}
		if n := len(e.Level); n > lvlW {
			lvlW = n
		}
	}

	row := func(a, b, c, d string) {
		fmt.Fprintf(w, "%-*s  %-*s  %-*s  %-*s\n", nameW, a, curW, b, latW, c, lvlW, d)
	}
	row("Package", "Current", "Latest", "Level")
	row(strings.Repeat("-", nameW), strings.Repeat("-", curW), strings.Repeat("-", latW), strings.Repeat("-", lvlW))
	// Sort by level severity (major > minor > patch) then by name so
	// the most-impactful upgrades land at the top.
	sort.SliceStable(entries, func(i, j int) bool {
		if severity(entries[i].Level) != severity(entries[j].Level) {
			return severity(entries[i].Level) > severity(entries[j].Level)
		}
		return normalizeName(entries[i].Name) < normalizeName(entries[j].Name)
	})
	for _, e := range entries {
		row(e.Name, e.Current, e.Latest, e.Level)
	}
}

func severity(level string) int {
	switch level {
	case "major":
		return 3
	case "minor":
		return 2
	case "patch":
		return 1
	}
	return 0
}

func writeOutdatedJSON(w io.Writer, entries []OutdatedEntry) {
	if entries == nil {
		entries = []OutdatedEntry{} // emit `[]`, not `null`
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(entries)
}

