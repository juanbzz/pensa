package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juanbzz/pensa/internal/pyproject"
)

// Workspace represents a multi-project workspace.
type Workspace struct {
	Root    string   // absolute path to workspace root directory
	Project *pyproject.PyProject // root pyproject.toml
	Members []Member // workspace member packages
}

// Member represents a single workspace member package.
type Member struct {
	Name    string               // package name from pyproject [project.name]
	Path    string               // absolute path to member directory
	Project *pyproject.PyProject // parsed pyproject.toml
}

// Discover finds a workspace root starting from the given directory.
// Walks up the directory tree looking for a pyproject.toml with workspace config.
// Returns nil if not in a workspace (single project mode).
func Discover(startDir string) (*Workspace, error) {
	absStart, err := filepath.Abs(startDir)
	if err != nil {
		return nil, err
	}

	dir := absStart
	for {
		pyprojectPath := filepath.Join(dir, "pyproject.toml")
		if _, err := os.Stat(pyprojectPath); err == nil {
			proj, err := pyproject.ReadPyProject(pyprojectPath)
			if err == nil && proj.IsWorkspaceRoot() {
				return loadWorkspace(dir, proj)
			}
		}

		// Walk up to parent.
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root.
			break
		}
		dir = parent
	}

	return nil, nil
}

// loadWorkspace parses all workspace members from the root.
func loadWorkspace(rootDir string, rootProj *pyproject.PyProject) (*Workspace, error) {
	ws := &Workspace{
		Root:    rootDir,
		Project: rootProj,
	}

	memberPaths := rootProj.WorkspaceMembers()

	for _, memberPath := range memberPaths {
		absPath := filepath.Join(rootDir, memberPath)
		pyprojectPath := filepath.Join(absPath, "pyproject.toml")

		memberProj, err := pyproject.ReadPyProject(pyprojectPath)
		if err != nil {
			return nil, fmt.Errorf("read workspace member %s: %w", memberPath, err)
		}

		name := memberProj.Name()
		if name == "" {
			name = filepath.Base(absPath)
		}

		ws.Members = append(ws.Members, Member{
			Name:    name,
			Path:    absPath,
			Project: memberProj,
		})
	}

	return ws, nil
}

// FindMember returns the member with the given name, or nil.
func (ws *Workspace) FindMember(name string) *Member {
	for i, m := range ws.Members {
		if m.Name == name {
			return &ws.Members[i]
		}
	}
	return nil
}

// MemberForDir returns the member whose path matches dir, or nil.
func (ws *Workspace) MemberForDir(dir string) *Member {
	for i, m := range ws.Members {
		if m.Path == dir {
			return &ws.Members[i]
		}
	}
	return nil
}

// MemberNames returns a comma-separated list of member names.
func (ws *Workspace) MemberNames() string {
	names := make([]string, len(ws.Members))
	for i, m := range ws.Members {
		names[i] = m.Name
	}
	return strings.Join(names, ", ")
}

// LockFilePath returns the path to the workspace lock file.
func (ws *Workspace) LockFilePath() string {
	return filepath.Join(ws.Root, "pensa.lock")
}

// VenvPath returns the path to the workspace venv.
func (ws *Workspace) VenvPath() string {
	return filepath.Join(ws.Root, ".venv")
}
