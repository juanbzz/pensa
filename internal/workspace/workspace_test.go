package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscover_PensaWorkspace(t *testing.T) {
	dir := t.TempDir()

	// Create workspace root.
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "my-workspace"
version = "0.1.0"

[tool.pensa.workspace]
members = ["apps/api", "apps/worker"]
`), 0644)

	// Create members.
	os.MkdirAll(filepath.Join(dir, "apps", "api"), 0755)
	os.WriteFile(filepath.Join(dir, "apps", "api", "pyproject.toml"), []byte(`
[project]
name = "my-api"
version = "0.1.0"
dependencies = ["flask>=3.0"]
`), 0644)

	os.MkdirAll(filepath.Join(dir, "apps", "worker"), 0755)
	os.WriteFile(filepath.Join(dir, "apps", "worker", "pyproject.toml"), []byte(`
[project]
name = "my-worker"
version = "0.1.0"
dependencies = ["celery>=5.0"]
`), 0644)

	ws, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws == nil {
		t.Fatal("should discover workspace")
	}

	if ws.Root != dir {
		t.Errorf("root = %q, want %q", ws.Root, dir)
	}
	if len(ws.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(ws.Members))
	}

	api := ws.FindMember("my-api")
	if api == nil {
		t.Error("should find my-api member")
	}
	worker := ws.FindMember("my-worker")
	if worker == nil {
		t.Error("should find my-worker member")
	}
}

func TestDiscover_UVWorkspace(t *testing.T) {
	dir := t.TempDir()

	// Create workspace root with uv format.
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "uv-workspace"
version = "0.1.0"

[tool.uv.workspace]
members = ["packages/lib"]
`), 0644)

	os.MkdirAll(filepath.Join(dir, "packages", "lib"), 0755)
	os.WriteFile(filepath.Join(dir, "packages", "lib", "pyproject.toml"), []byte(`
[project]
name = "my-lib"
version = "0.1.0"
`), 0644)

	ws, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws == nil {
		t.Fatal("should discover uv workspace")
	}
	if len(ws.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(ws.Members))
	}
	if ws.Members[0].Name != "my-lib" {
		t.Errorf("member name = %q", ws.Members[0].Name)
	}
}

func TestDiscover_FromMemberDir(t *testing.T) {
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
	os.WriteFile(filepath.Join(apiDir, "pyproject.toml"), []byte(`
[project]
name = "api"
version = "0.1.0"
`), 0644)

	// Discover from member directory — should walk up and find root.
	ws, err := Discover(apiDir)
	if err != nil {
		t.Fatal(err)
	}
	if ws == nil {
		t.Fatal("should discover workspace from member dir")
	}
	if ws.Root != dir {
		t.Errorf("root = %q, want %q", ws.Root, dir)
	}
}

func TestDiscover_NoWorkspace(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "single-project"
version = "0.1.0"
`), 0644)

	ws, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws != nil {
		t.Error("should return nil for non-workspace project")
	}
}

func TestWorkspace_LockFilePath(t *testing.T) {
	ws := &Workspace{Root: "/tmp/myworkspace"}
	if ws.LockFilePath() != "/tmp/myworkspace/pensa.lock" {
		t.Errorf("lock path = %q", ws.LockFilePath())
	}
}
