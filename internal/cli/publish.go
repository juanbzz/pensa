package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pensa.sh/pensa/internal/build"
	"pensa.sh/pensa/internal/publish"
	"pensa.sh/pensa/internal/pyproject"
	"github.com/spf13/cobra"
)

func newPublishCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish the package to PyPI",
		Long:  "Uploads distribution files from dist/ to a Python package index.",
		Example: `  pensa publish
  pensa publish --repository testpypi
  pensa publish --token pypi-abc123
  pensa publish --build`,
		Args: cobra.NoArgs,
		RunE: runPublish,
	}
	cmd.Flags().String("token", "", "PyPI API token (or set PENSA_PYPI_TOKEN)")
	cmd.Flags().String("repository", "pypi", "Repository: pypi, testpypi, or a URL")
	cmd.Flags().Bool("build", false, "Build before publishing")
	return cmd
}

func runPublish(cmd *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	w := cmd.OutOrStdout()

	// Resolve token.
	token, _ := cmd.Flags().GetString("token")
	if token == "" {
		token = os.Getenv("PENSA_PYPI_TOKEN")
	}

	repository, _ := cmd.Flags().GetString("repository")
	shouldBuild, _ := cmd.Flags().GetBool("build")

	// Build first if requested.
	if shouldBuild {
		proj, err := pyproject.ReadPyProject(filepath.Join(dir, "pyproject.toml"))
		if err != nil {
			return fmt.Errorf("read pyproject.toml: %w", err)
		}
		name := proj.Name()
		ver := proj.Version()
		fmt.Fprintf(w, "%s %s %s\n", blue("Building"), bold(name), dim("("+ver+")"))

		result, err := build.Build(build.Options{
			ProjectDir: dir,
			OutputDir:  filepath.Join(dir, "dist"),
			Wheel:      true,
			Sdist:      true,
		})
		if err != nil {
			return fmt.Errorf("build: %w", err)
		}
		for _, f := range result.Files {
			fmt.Fprintf(w, "  %s %s\n", green("Built"), filepath.Base(f))
		}
	}

	// Find distribution files.
	distDir := filepath.Join(dir, "dist")
	files, err := findDistFiles(distDir)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return fmt.Errorf("no distribution files found in dist/. Run \"pensa build\" first")
	}

	fmt.Fprintf(w, "%s to %s\n", blue("Publishing"), bold(repository))

	for _, f := range files {
		fmt.Fprintf(w, "  %s %s\n", green("Uploading"), filepath.Base(f))
	}

	if err := publish.Publish(publish.Options{
		Files:      files,
		Repository: repository,
		Token:      token,
	}); err != nil {
		return err
	}

	fmt.Fprintf(w, "%s\n", green("Published successfully."))
	return nil
}

// findDistFiles returns all .whl and .tar.gz files in a directory.
func findDistFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dist/: %w", err)
	}

	var files []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".whl") || strings.HasSuffix(name, ".tar.gz") {
			files = append(files, filepath.Join(dir, name))
		}
	}
	return files, nil
}
