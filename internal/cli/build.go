package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juanbzz/pensa/internal/build"
	"github.com/juanbzz/pensa/internal/pyproject"
	"github.com/spf13/cobra"
)

func newBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build the package",
		Long:  "Builds sdist and wheel archives using the project's PEP 517 build backend.",
		Example: `  pensa build
  pensa build --wheel
  pensa build --sdist
  pensa build -o out/`,
		Args: cobra.NoArgs,
		RunE: runBuild,
	}
	cmd.Flags().Bool("wheel", false, "Build wheel only")
	cmd.Flags().Bool("sdist", false, "Build sdist only")
	cmd.Flags().StringP("output", "o", "dist", "Output directory")
	return cmd
}

func runBuild(cmd *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	w := cmd.OutOrStdout()

	// Read project info for display.
	proj, err := pyproject.ReadPyProject(filepath.Join(dir, "pyproject.toml"))
	if err != nil {
		return fmt.Errorf("read pyproject.toml: %w", err)
	}

	name := proj.Name()
	ver := proj.Version()
	if name == "" {
		name = filepath.Base(dir)
	}
	if ver == "" {
		ver = "0.0.0"
	}

	wheelOnly, _ := cmd.Flags().GetBool("wheel")
	sdistOnly, _ := cmd.Flags().GetBool("sdist")
	outputDir, _ := cmd.Flags().GetString("output")

	// Default: build both.
	buildWheel := true
	buildSdist := true
	if wheelOnly {
		buildSdist = false
	}
	if sdistOnly {
		buildWheel = false
	}

	opts := build.Options{
		ProjectDir: dir,
		OutputDir:  outputDir,
		Wheel:      buildWheel,
		Sdist:      buildSdist,
	}

	var result *build.Result
	spinMsg := fmt.Sprintf("%s %s %s", blue("Building"), bold(name), dim("("+ver+")"))
	if err := withSpinner(w, spinMsg, func() error {
		var buildErr error
		result, buildErr = build.Build(opts)
		return buildErr
	}); err != nil {
		return err
	}

	for _, f := range result.Files {
		fmt.Fprintf(w, "  %s %s\n", green("Built"), filepath.Base(f))
	}

	return nil
}
