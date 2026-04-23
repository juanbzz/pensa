package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// Workspace edge cases not covered by workspace_test.go: error paths,
// fallback behaviors, lookup misses, and the auxiliary getters.

// A workspace member directory that doesn't exist should produce a
// clear error rather than silently continuing.
func TestWorkspaceEdge_MissingMemberDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "ws"
version = "0.1.0"

[tool.pensa.workspace]
members = ["apps/missing"]
`), 0644)

	_, err := Discover(dir)
	if err == nil {
		t.Error("expected error when member dir is missing")
	}
}

// Malformed member pyproject.toml → Discover returns an error.
func TestWorkspaceEdge_MalformedMemberPyproject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "ws"
version = "0.1.0"

[tool.pensa.workspace]
members = ["apps/api"]
`), 0644)

	apiDir := filepath.Join(dir, "apps", "api")
	os.MkdirAll(apiDir, 0755)
	// Malformed TOML — unterminated string.
	os.WriteFile(filepath.Join(apiDir, "pyproject.toml"),
		[]byte(`[project]`+"\n"+`name = "unterminated`), 0644)

	_, err := Discover(dir)
	if err == nil {
		t.Error("expected error on malformed member pyproject.toml")
	}
}

// Member without a [project] section → name falls back to the
// directory's basename.
func TestWorkspaceEdge_MemberWithoutProjectSectionUsesDirName(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "ws"
version = "0.1.0"

[tool.pensa.workspace]
members = ["apps/no-project-section"]
`), 0644)

	memberDir := filepath.Join(dir, "apps", "no-project-section")
	os.MkdirAll(memberDir, 0755)
	// No [project] section, only an unrelated section.
	os.WriteFile(filepath.Join(memberDir, "pyproject.toml"), []byte(`
[tool.black]
line-length = 88
`), 0644)

	ws, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws == nil || len(ws.Members) != 1 {
		t.Fatalf("expected one member, got %v", ws)
	}
	if ws.Members[0].Name != "no-project-section" {
		t.Errorf("member name = %q; want dirname fallback 'no-project-section'",
			ws.Members[0].Name)
	}
}

// FindMember on an unknown name returns nil (not an error).
func TestWorkspaceEdge_FindMemberUnknown(t *testing.T) {
	ws := &Workspace{
		Members: []Member{{Name: "present"}},
	}
	if got := ws.FindMember("absent"); got != nil {
		t.Errorf("FindMember(absent) = %+v; want nil", got)
	}
}

// MemberForDir with a path that isn't any member returns nil.
func TestWorkspaceEdge_MemberForDirUnknown(t *testing.T) {
	ws := &Workspace{
		Members: []Member{{Name: "m", Path: "/present"}},
	}
	if got := ws.MemberForDir("/absent"); got != nil {
		t.Errorf("MemberForDir(/absent) = %+v; want nil", got)
	}
}

// Empty members list in config → not a workspace (IsWorkspaceRoot
// returns false). Covered by Discover returning nil for this case.
func TestWorkspaceEdge_EmptyMembersListIsNotWorkspace(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "not-a-workspace"
version = "0.1.0"

[tool.pensa.workspace]
members = []
`), 0644)

	ws, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws != nil {
		t.Error("empty members list should not be treated as a workspace")
	}
}

// pensa.workspace takes precedence over uv.workspace when both are present.
func TestWorkspaceEdge_PensaWorkspaceTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "ws"
version = "0.1.0"

[tool.pensa.workspace]
members = ["pensa-path"]

[tool.uv.workspace]
members = ["uv-path"]
`), 0644)

	// Both member dirs must exist (Discover reads each).
	for _, p := range []string{"pensa-path", "uv-path"} {
		memberDir := filepath.Join(dir, p)
		os.MkdirAll(memberDir, 0755)
		os.WriteFile(filepath.Join(memberDir, "pyproject.toml"), []byte(`
[project]
name = "`+p+`"
version = "0.1.0"
`), 0644)
	}

	ws, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws == nil {
		t.Fatal("expected workspace")
	}
	// Pensa config wins — only pensa-path member.
	if len(ws.Members) != 1 {
		t.Fatalf("expected 1 member (pensa takes precedence), got %d", len(ws.Members))
	}
	if ws.Members[0].Name != "pensa-path" {
		t.Errorf("expected pensa-path, got %q", ws.Members[0].Name)
	}
}

// MemberNames produces a comma-separated list in member order.
func TestWorkspaceEdge_MemberNamesFormatting(t *testing.T) {
	ws := &Workspace{
		Members: []Member{
			{Name: "alpha"},
			{Name: "beta"},
			{Name: "gamma"},
		},
	}
	if got := ws.MemberNames(); got != "alpha, beta, gamma" {
		t.Errorf("MemberNames = %q; want 'alpha, beta, gamma'", got)
	}
}

// VenvPath returns <root>/.venv.
func TestWorkspaceEdge_VenvPath(t *testing.T) {
	ws := &Workspace{Root: "/tmp/ws"}
	if got := ws.VenvPath(); got != "/tmp/ws/.venv" {
		t.Errorf("VenvPath = %q; want /tmp/ws/.venv", got)
	}
}
