package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juanbzz/pensa/internal/python"
	"github.com/spf13/cobra"
)

func newNewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new [directory]",
		Short: "Create a new Python project",
		Long:  "Scaffolds a new Python project with pyproject.toml, README.md, .gitignore, and main.py.",
		Example: `  pensa new
  pensa new myproject
  pensa new --name custom-name`,
		Args: cobra.MaximumNArgs(1),
		RunE: runNew,
	}
	cmd.Flags().String("name", "", "Project name (defaults to directory name)")
	return cmd
}

func runNew(cmd *cobra.Command, args []string) error {
	w := cmd.OutOrStdout()

	// Determine target directory.
	var targetDir string
	var err error
	if len(args) == 1 {
		targetDir, err = filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("resolve path: %w", err)
		}
	} else {
		targetDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
	}

	// Check if target dir exists and is non-empty.
	if len(args) == 1 {
		if err := checkTargetDir(targetDir); err != nil {
			return err
		}
	}

	// Check for existing pyproject.toml.
	if _, err := os.Stat(filepath.Join(targetDir, "pyproject.toml")); err == nil {
		return fmt.Errorf("pyproject.toml already exists in %s", targetDir)
	}

	// Determine project name.
	nameFlag, _ := cmd.Flags().GetString("name")
	projectName := nameFlag
	if projectName == "" {
		projectName = filepath.Base(targetDir)
	}
	if projectName == "/" || projectName == "." {
		return fmt.Errorf("cannot infer project name, use --name")
	}
	projectName = strings.ToLower(strings.ReplaceAll(projectName, " ", "-"))

	// Detect Python version for requires-python.
	requiresPython := detectRequiresPython()

	// Create target directory if needed.
	if len(args) == 1 {
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}
	}

	// Write files.
	files := map[string]string{
		"pyproject.toml": generatePyproject(projectName, requiresPython),
		"README.md":      fmt.Sprintf("# %s\n", projectName),
		"main.py":        generateMain(projectName),
		".gitignore":     pythonGitignore,
	}

	for name, content := range files {
		path := filepath.Join(targetDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}

	fmt.Fprintf(w, "%s project %s in %s\n", green("Created"), bold(projectName), targetDir)
	return nil
}

// checkTargetDir verifies the target directory is safe to scaffold into.
func checkTargetDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		// Doesn't exist yet — fine, we'll create it.
		return nil
	}
	if !info.IsDir() {
		return fmt.Errorf("%s exists and is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read directory: %w", err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("directory %s already exists and is not empty", filepath.Base(dir))
	}
	return nil
}

// detectRequiresPython returns a requires-python string based on the system Python.
func detectRequiresPython() string {
	py, err := python.Discover()
	if err != nil {
		return ">=3.8"
	}
	return fmt.Sprintf(">=%d.%d", py.Major, py.Minor)
}

func generatePyproject(name, requiresPython string) string {
	return fmt.Sprintf(`[project]
name = %q
version = "0.1.0"
description = ""
requires-python = %q
dependencies = []

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`, name, requiresPython)
}

func generateMain(name string) string {
	return fmt.Sprintf(`def main():
    print("Hello from %s!")

if __name__ == "__main__":
    main()
`, name)
}

const pythonGitignore = `# Python
__pycache__/
*.py[cod]
*$py.class
*.so
*.egg-info/
dist/
build/
*.egg

# Virtual environments
.venv/
venv/

# IDE
.idea/
.vscode/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db
`
