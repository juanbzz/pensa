package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juanbzz/pensa/internal/lockfile"
	"github.com/juanbzz/pensa/internal/pyproject"
	"github.com/juanbzz/pensa/internal/workspace"
	"github.com/juanbzz/pensa/pkg/pep508"
	"github.com/spf13/cobra"
)

func newRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove [packages...]",
		Short: "Remove dependencies from the project",
		Long:  "Removes one or more packages from pyproject.toml, re-locks, and re-installs.",
		Example: `  pensa remove requests
  pensa remove requests flask
  pensa remove pytest -G dev`,
		Args: cobra.MinimumNArgs(1),
		RunE: runRemove,
	}
	cmd.Flags().StringP("group", "G", "", "Dependency group (e.g., dev, test)")
	cmd.Flags().String("package", "", "Target workspace member (by name)")
	return cmd
}

func runRemove(cmd *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	pkgFlag, _ := cmd.Flags().GetString("package")
	group, _ := cmd.Flags().GetString("group")
	out := uiFromCmd(cmd)

	// Resolve workspace + target member.
	ws, _ := workspace.Discover(dir)
	member, err := resolveTargetMember(ws, pkgFlag, dir)
	if err != nil {
		return err
	}

	var pyprojectPath string
	var proj *pyproject.PyProject
	if member != nil {
		pyprojectPath = filepath.Join(member.Path, "pyproject.toml")
		proj = member.Project
	} else {
		pyprojectPath = filepath.Join(dir, "pyproject.toml")
		proj, err = pyproject.ReadPyProject(pyprojectPath)
		if err != nil {
			return fmt.Errorf("read pyproject.toml: %w", err)
		}
	}

	// Read lock file to get current versions for removed packages.
	removeLockDir := dir
	if ws != nil {
		removeLockDir = ws.Root
	}
	lockedVersions := make(map[string]string)
	if lockPath, _ := lockfile.DetectLockFile(removeLockDir); lockPath != "" {
		if lf, err := lockfile.ReadLockFile(lockPath); err == nil {
			for _, p := range lf.Packages {
				lockedVersions[normalizeName(p.Name)] = p.Version
			}
		}
	}

	var removedPkgs []string
	for _, arg := range args {
		name := pep508.NormalizeName(arg)
		if group != "" {
			if err := removeFromDependencyGroup(proj, name, group); err != nil {
				return err
			}
		} else {
			if err := removeFromProject(proj, name); err != nil {
				return err
			}
		}
		removedPkgs = append(removedPkgs, name)
	}

	if err := pyproject.WritePyProject(pyprojectPath, proj); err != nil {
		return fmt.Errorf("write pyproject.toml: %w", err)
	}

	// Re-lock: entire workspace or single project.
	if ws != nil {
		if member != nil {
			member.Project, _ = pyproject.ReadPyProject(pyprojectPath)
		}
		if err := runLockWorkspace(os.Stderr, ws, lockOptions{}); err != nil {
			return err
		}
	} else {
		proj, err = pyproject.ReadPyProject(pyprojectPath)
		if err != nil {
			return fmt.Errorf("re-read pyproject.toml: %w", err)
		}

		allDeps, err := proj.ResolveAllDependencies()
		if err != nil {
			return fmt.Errorf("resolve dependencies: %w", err)
		}
		if len(allDeps) == 0 {
			for _, name := range []string{"pensa.lock", "uv.lock", "poetry.lock"} {
				os.Remove(filepath.Join(dir, name))
			}
			out.Info("No dependencies remaining.")
			return nil
		}

		if err := resolveAndLock(os.Stderr, proj, pyprojectPath, lockOptions{}); err != nil {
			return err
		}
	}

	if err := installFromLock(os.Stderr, true, nil); err != nil {
		return err
	}

	// Show what was removed.
	for _, name := range removedPkgs {
		out.DiffRemove(name, lockedVersions[name])
	}

	return nil
}

// removeFromProject removes a dependency from the appropriate section of pyproject.toml.
func removeFromProject(proj *pyproject.PyProject, name string) error {
	normalized := normalizeName(name)

	if proj.HasProjectSection() && len(proj.Project.Dependencies) > 0 {
		for i, existing := range proj.Project.Dependencies {
			dep, err := pep508.Parse(existing)
			if err != nil {
				continue
			}
			if normalizeName(dep.Name) == normalized {
				proj.Project.Dependencies = append(proj.Project.Dependencies[:i], proj.Project.Dependencies[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("%q is not a dependency in [project.dependencies]", name)
	}

	if proj.HasPoetrySection() && len(proj.Tool.Poetry.Dependencies) > 0 {
		for key := range proj.Tool.Poetry.Dependencies {
			if normalizeName(key) == normalized && normalizeName(key) != "python" {
				delete(proj.Tool.Poetry.Dependencies, key)
				return nil
			}
		}
		return fmt.Errorf("%q is not a dependency in [tool.poetry.dependencies]", name)
	}

	return fmt.Errorf("%q is not a dependency", name)
}

// removeFromDependencyGroup removes a dependency from a PEP 735 or Poetry group.
// Tries PEP 735 [dependency-groups] first, falls back to Poetry groups.
func removeFromDependencyGroup(proj *pyproject.PyProject, name, group string) error {
	normalized := normalizeName(name)

	// Try PEP 735 first.
	if proj.DependencyGroups != nil {
		entries, ok := proj.DependencyGroups[group]
		if ok {
			for i, entry := range entries {
				if s, ok := entry.(string); ok {
					d, err := pep508.Parse(s)
					if err == nil && normalizeName(d.Name) == normalized {
						proj.DependencyGroups[group] = append(entries[:i], entries[i+1:]...)
						return nil
					}
				}
			}
			return fmt.Errorf("%q is not a dependency in group %q", name, group)
		}
	}

	// Fall back to Poetry groups.
	return removeFromGroup(proj, name, group)
}

// removeFromGroup removes a dependency from a Poetry named group.
func removeFromGroup(proj *pyproject.PyProject, name, group string) error {
	normalized := normalizeName(name)

	if !proj.HasPoetrySection() || proj.Tool.Poetry.Groups == nil {
		return fmt.Errorf("%q is not a dependency in group %q", name, group)
	}

	g, ok := proj.Tool.Poetry.Groups[group]
	if !ok || len(g.Dependencies) == 0 {
		return fmt.Errorf("%q is not a dependency in group %q", name, group)
	}

	for key := range g.Dependencies {
		if normalizeName(key) == normalized {
			delete(g.Dependencies, key)
			proj.Tool.Poetry.Groups[group] = g
			return nil
		}
	}

	return fmt.Errorf("%q is not a dependency in group %q", name, group)
}
