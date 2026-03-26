//go:build integration

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestWorkspaceMember_AddWithPackageFlag(t *testing.T) {
	assert := is.New(t)
	dir := setupWorkspace(t)
	chdir(t, dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"add", "idna", "--package", "test-api"})

	err := cmd.Execute()
	assert.NoErr(err)

	out := buf.String()
	assert.True(strings.Contains(out, "Adding"))
	assert.True(strings.Contains(out, "idna"))

	// Dep should be in api's pyproject, not root's.
	apiData, _ := os.ReadFile(filepath.Join(dir, "apps", "api", "pyproject.toml"))
	assert.True(strings.Contains(string(apiData), "idna"))

	rootData, _ := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	assert.True(!strings.Contains(string(rootData), "idna"))

	// Lock file should exist at workspace root.
	_, err = os.Stat(filepath.Join(dir, "pensa.lock"))
	assert.NoErr(err)

	// Lock should contain the new dep.
	lockData, _ := os.ReadFile(filepath.Join(dir, "pensa.lock"))
	assert.True(strings.Contains(string(lockData), "idna"))
}

func TestWorkspaceMember_AddFromMemberDir(t *testing.T) {
	assert := is.New(t)
	dir := setupWorkspace(t)
	chdir(t, filepath.Join(dir, "apps", "worker"))

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"add", "idna"})

	err := cmd.Execute()
	assert.NoErr(err)

	// Dep should be in worker's pyproject (cwd-based detection).
	workerData, _ := os.ReadFile(filepath.Join(dir, "apps", "worker", "pyproject.toml"))
	assert.True(strings.Contains(string(workerData), "idna"))

	// NOT in api's pyproject.
	apiData, _ := os.ReadFile(filepath.Join(dir, "apps", "api", "pyproject.toml"))
	assert.True(!strings.Contains(string(apiData), "idna"))
}

func TestWorkspaceMember_AddAtRootWithoutFlag_Errors(t *testing.T) {
	assert := is.New(t)
	dir := setupWorkspace(t)
	chdir(t, dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"add", "idna"})

	err := cmd.Execute()
	assert.True(err != nil)
	assert.True(strings.Contains(err.Error(), "use --package"))
}

func TestWorkspaceMember_AddUnknownMember_Errors(t *testing.T) {
	assert := is.New(t)
	dir := setupWorkspace(t)
	chdir(t, dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"add", "idna", "--package", "nonexistent"})

	err := cmd.Execute()
	assert.True(err != nil)
	assert.True(strings.Contains(err.Error(), "not found"))
	assert.True(strings.Contains(err.Error(), "test-api"))
	assert.True(strings.Contains(err.Error(), "test-worker"))
}

func TestWorkspaceMember_RemoveWithPackageFlag(t *testing.T) {
	assert := is.New(t)
	dir := setupWorkspace(t)
	chdir(t, dir)

	// First add a dep.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"add", "idna", "--package", "test-api"})
	err := cmd.Execute()
	assert.NoErr(err)

	// Verify it's there.
	apiData, _ := os.ReadFile(filepath.Join(dir, "apps", "api", "pyproject.toml"))
	assert.True(strings.Contains(string(apiData), "idna"))

	// Now remove it.
	cmd = newRootCmd()
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"remove", "idna", "--package", "test-api"})
	err = cmd.Execute()
	assert.NoErr(err)

	out := buf.String()
	assert.True(strings.Contains(out, "Removing"))
	assert.True(strings.Contains(out, "idna"))

	// Should be gone from api's pyproject.
	apiData, _ = os.ReadFile(filepath.Join(dir, "apps", "api", "pyproject.toml"))
	assert.True(!strings.Contains(string(apiData), "idna"))
}

func TestWorkspaceMember_RemoveFromMemberDir(t *testing.T) {
	assert := is.New(t)
	dir := setupWorkspace(t)

	// Add from root with --package.
	chdir(t, dir)
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"add", "idna", "--package", "test-worker"})
	err := cmd.Execute()
	assert.NoErr(err)

	// Remove from member dir (cwd-based).
	chdir(t, filepath.Join(dir, "apps", "worker"))
	cmd = newRootCmd()
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"remove", "idna"})
	err = cmd.Execute()
	assert.NoErr(err)

	workerData, _ := os.ReadFile(filepath.Join(dir, "apps", "worker", "pyproject.toml"))
	assert.True(!strings.Contains(string(workerData), "idna"))
}

// setupUVWorkspace creates a workspace using [tool.uv.workspace] format,
// matching the real-world pgm monorepo pattern.
func setupUVWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "mymonorepo"
version = "0.1.0"
requires-python = ">=3.10"

[tool.uv.workspace]
members = ["apps/backend", "apps/pipeline"]

[tool.uv.sources]
mymonorepo-backend = { workspace = true }
mymonorepo-pipeline = { workspace = true }
`), 0644)

	backendDir := filepath.Join(dir, "apps", "backend")
	os.MkdirAll(backendDir, 0755)
	os.WriteFile(filepath.Join(backendDir, "pyproject.toml"), []byte(`
[project]
name = "mymonorepo-backend"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["six>=1.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)
	os.MkdirAll(filepath.Join(backendDir, "mymonorepo_backend"), 0755)
	os.WriteFile(filepath.Join(backendDir, "mymonorepo_backend", "__init__.py"), []byte(""), 0644)

	pipelineDir := filepath.Join(dir, "apps", "pipeline")
	os.MkdirAll(pipelineDir, 0755)
	os.WriteFile(filepath.Join(pipelineDir, "pyproject.toml"), []byte(`
[project]
name = "mymonorepo-pipeline"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["certifi>=2023.0.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)
	os.MkdirAll(filepath.Join(pipelineDir, "mymonorepo_pipeline"), 0755)
	os.WriteFile(filepath.Join(pipelineDir, "mymonorepo_pipeline", "__init__.py"), []byte(""), 0644)

	return dir
}

func TestWorkspaceMember_UVFormat_AddWithPackageFlag(t *testing.T) {
	assert := is.New(t)
	dir := setupUVWorkspace(t)
	chdir(t, dir)

	// Add to backend member using --package.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"add", "idna", "--package", "mymonorepo-backend"})
	err := cmd.Execute()
	assert.NoErr(err)

	// Dep should be in backend's pyproject.
	backendData, _ := os.ReadFile(filepath.Join(dir, "apps", "backend", "pyproject.toml"))
	assert.True(strings.Contains(string(backendData), "idna"))

	// NOT in pipeline's pyproject.
	pipelineData, _ := os.ReadFile(filepath.Join(dir, "apps", "pipeline", "pyproject.toml"))
	assert.True(!strings.Contains(string(pipelineData), "idna"))

	// Lock file at workspace root should have all deps.
	lockData, _ := os.ReadFile(filepath.Join(dir, "pensa.lock"))
	lockContent := string(lockData)
	assert.True(strings.Contains(lockContent, "idna"))    // newly added
	assert.True(strings.Contains(lockContent, "six"))      // backend's existing dep
	assert.True(strings.Contains(lockContent, "certifi"))  // pipeline's dep

	// Workspace sources should be skipped (not in lock as PyPI packages).
	assert.True(!strings.Contains(lockContent, "mymonorepo-backend"))
	assert.True(!strings.Contains(lockContent, "mymonorepo-pipeline"))
}

func TestWorkspaceMember_UVFormat_AddFromMemberDir(t *testing.T) {
	assert := is.New(t)
	dir := setupUVWorkspace(t)

	// cd into pipeline member and add without --package.
	chdir(t, filepath.Join(dir, "apps", "pipeline"))

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"add", "idna"})
	err := cmd.Execute()
	assert.NoErr(err)

	// Dep should be in pipeline's pyproject (cwd-based).
	pipelineData, _ := os.ReadFile(filepath.Join(dir, "apps", "pipeline", "pyproject.toml"))
	assert.True(strings.Contains(string(pipelineData), "idna"))

	// NOT in backend's pyproject.
	backendData, _ := os.ReadFile(filepath.Join(dir, "apps", "backend", "pyproject.toml"))
	assert.True(!strings.Contains(string(backendData), "idna"))
}

func TestWorkspaceMember_UVFormat_RemoveWithPackageFlag(t *testing.T) {
	assert := is.New(t)
	dir := setupUVWorkspace(t)
	chdir(t, dir)

	// Add then remove.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"add", "idna", "--package", "mymonorepo-pipeline"})
	err := cmd.Execute()
	assert.NoErr(err)

	cmd = newRootCmd()
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"remove", "idna", "--package", "mymonorepo-pipeline"})
	err = cmd.Execute()
	assert.NoErr(err)

	pipelineData, _ := os.ReadFile(filepath.Join(dir, "apps", "pipeline", "pyproject.toml"))
	assert.True(!strings.Contains(string(pipelineData), "idna"))
}

func TestWorkspaceMember_UVFormat_RootWithoutFlag_Errors(t *testing.T) {
	assert := is.New(t)
	dir := setupUVWorkspace(t)
	chdir(t, dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"add", "idna"})

	err := cmd.Execute()
	assert.True(err != nil)
	assert.True(strings.Contains(err.Error(), "use --package"))
	assert.True(strings.Contains(err.Error(), "mymonorepo-backend"))
	assert.True(strings.Contains(err.Error(), "mymonorepo-pipeline"))
}

// setupPensaWorkspace creates a workspace using [tool.pensa.workspace] +
// [tool.pensa.sources] — pensa's native format.
func setupPensaWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "pensa-mono"
version = "0.1.0"
requires-python = ">=3.10"

[tool.pensa.workspace]
members = ["services/api", "services/worker"]

[tool.pensa.sources]
pensa-mono-api = { workspace = true }
pensa-mono-worker = { workspace = true }
`), 0644)

	apiDir := filepath.Join(dir, "services", "api")
	os.MkdirAll(apiDir, 0755)
	os.WriteFile(filepath.Join(apiDir, "pyproject.toml"), []byte(`
[project]
name = "pensa-mono-api"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["six>=1.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)
	os.MkdirAll(filepath.Join(apiDir, "pensa_mono_api"), 0755)
	os.WriteFile(filepath.Join(apiDir, "pensa_mono_api", "__init__.py"), []byte(""), 0644)

	workerDir := filepath.Join(dir, "services", "worker")
	os.MkdirAll(workerDir, 0755)
	os.WriteFile(filepath.Join(workerDir, "pyproject.toml"), []byte(`
[project]
name = "pensa-mono-worker"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["certifi>=2023.0.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)
	os.MkdirAll(filepath.Join(workerDir, "pensa_mono_worker"), 0755)
	os.WriteFile(filepath.Join(workerDir, "pensa_mono_worker", "__init__.py"), []byte(""), 0644)

	return dir
}

func TestWorkspaceMember_PensaFormat_Lock(t *testing.T) {
	assert := is.New(t)
	dir := setupPensaWorkspace(t)
	chdir(t, dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	err := cmd.Execute()
	assert.NoErr(err)

	out := buf.String()
	assert.True(strings.Contains(out, "workspace"))
	assert.True(strings.Contains(out, "2 members"))

	// Lock at workspace root.
	lockData, _ := os.ReadFile(filepath.Join(dir, "pensa.lock"))
	lockContent := string(lockData)
	assert.True(strings.Contains(lockContent, "six"))
	assert.True(strings.Contains(lockContent, "certifi"))

	// Workspace sources skipped.
	assert.True(!strings.Contains(lockContent, "pensa-mono-api"))
	assert.True(!strings.Contains(lockContent, "pensa-mono-worker"))
}

func TestWorkspaceMember_PensaFormat_AddWithPackageFlag(t *testing.T) {
	assert := is.New(t)
	dir := setupPensaWorkspace(t)
	chdir(t, dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"add", "idna", "--package", "pensa-mono-api"})
	err := cmd.Execute()
	assert.NoErr(err)

	// Dep in api's pyproject.
	apiData, _ := os.ReadFile(filepath.Join(dir, "services", "api", "pyproject.toml"))
	assert.True(strings.Contains(string(apiData), "idna"))

	// NOT in worker's pyproject.
	workerData, _ := os.ReadFile(filepath.Join(dir, "services", "worker", "pyproject.toml"))
	assert.True(!strings.Contains(string(workerData), "idna"))

	// Lock has all deps including new one.
	lockData, _ := os.ReadFile(filepath.Join(dir, "pensa.lock"))
	lockContent := string(lockData)
	assert.True(strings.Contains(lockContent, "idna"))
	assert.True(strings.Contains(lockContent, "six"))
	assert.True(strings.Contains(lockContent, "certifi"))
}

func TestWorkspaceMember_PensaFormat_AddFromMemberDir(t *testing.T) {
	assert := is.New(t)
	dir := setupPensaWorkspace(t)
	chdir(t, filepath.Join(dir, "services", "worker"))

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"add", "idna"})
	err := cmd.Execute()
	assert.NoErr(err)

	// Dep in worker's pyproject (cwd-based).
	workerData, _ := os.ReadFile(filepath.Join(dir, "services", "worker", "pyproject.toml"))
	assert.True(strings.Contains(string(workerData), "idna"))

	// NOT in api's.
	apiData, _ := os.ReadFile(filepath.Join(dir, "services", "api", "pyproject.toml"))
	assert.True(!strings.Contains(string(apiData), "idna"))
}

func TestWorkspaceMember_PensaFormat_SourcesSkipped(t *testing.T) {
	assert := is.New(t)
	dir := setupPensaWorkspace(t)
	chdir(t, dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	err := cmd.Execute()
	assert.NoErr(err)

	// Read every package name in the lock file.
	lockData, _ := os.ReadFile(filepath.Join(dir, "pensa.lock"))
	lockContent := string(lockData)

	// Workspace member names must NOT appear as resolved packages.
	for _, name := range []string{"pensa-mono-api", "pensa-mono-worker"} {
		needle := "name = \"" + name + "\""
		assert.True(!strings.Contains(lockContent, needle))
	}

	// But real PyPI packages must be there.
	assert.True(strings.Contains(lockContent, "name = \"six\""))
	assert.True(strings.Contains(lockContent, "name = \"certifi\""))
}

func TestWorkspaceMember_PensaFormat_PensaSourcesTakePriority(t *testing.T) {
	assert := is.New(t)
	dir := t.TempDir()

	// Both [tool.pensa.sources] and [tool.uv.sources] defined.
	// Pensa sources list "priority-api" as workspace.
	// UV sources list "priority-api" as NOT workspace (missing).
	// Pensa should take priority → skip "priority-api" from PyPI.
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "priority-test"
version = "0.1.0"
requires-python = ">=3.10"

[tool.pensa.workspace]
members = ["lib"]

[tool.pensa.sources]
priority-lib = { workspace = true }

[tool.uv.sources]
`), 0644)

	libDir := filepath.Join(dir, "lib")
	os.MkdirAll(libDir, 0755)
	os.WriteFile(filepath.Join(libDir, "pyproject.toml"), []byte(`
[project]
name = "priority-lib"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["six>=1.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)
	os.MkdirAll(filepath.Join(libDir, "priority_lib"), 0755)
	os.WriteFile(filepath.Join(libDir, "priority_lib", "__init__.py"), []byte(""), 0644)

	chdir(t, dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	err := cmd.Execute()
	assert.NoErr(err)

	lockData, _ := os.ReadFile(filepath.Join(dir, "pensa.lock"))
	lockContent := string(lockData)

	// priority-lib should be skipped (pensa sources say workspace=true).
	assert.True(!strings.Contains(lockContent, "priority-lib"))
	// six should be resolved.
	assert.True(strings.Contains(lockContent, "six"))
}

// setupInterDepWorkspace creates a workspace where member A depends on member B.
// api depends on ["my-lib" (workspace), "six" (PyPI)]
// lib depends on ["certifi" (PyPI)]
// Transitive: api → my-lib → certifi should all be in the lock.
func setupInterDepWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "interdep-workspace"
version = "0.1.0"
requires-python = ">=3.10"

[tool.pensa.workspace]
members = ["services/api", "libs/core"]

[tool.pensa.sources]
interdep-lib = { workspace = true }
`), 0644)

	apiDir := filepath.Join(dir, "services", "api")
	os.MkdirAll(apiDir, 0755)
	os.WriteFile(filepath.Join(apiDir, "pyproject.toml"), []byte(`
[project]
name = "interdep-api"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["interdep-lib", "six>=1.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)
	os.MkdirAll(filepath.Join(apiDir, "interdep_api"), 0755)
	os.WriteFile(filepath.Join(apiDir, "interdep_api", "__init__.py"), []byte(""), 0644)

	libDir := filepath.Join(dir, "libs", "core")
	os.MkdirAll(libDir, 0755)
	os.WriteFile(filepath.Join(libDir, "pyproject.toml"), []byte(`
[project]
name = "interdep-lib"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["certifi>=2023.0.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)
	os.MkdirAll(filepath.Join(libDir, "interdep_lib"), 0755)
	os.WriteFile(filepath.Join(libDir, "interdep_lib", "__init__.py"), []byte(""), 0644)

	return dir
}

func TestWorkspaceMember_InterDeps_TransitiveDepsInLock(t *testing.T) {
	assert := is.New(t)
	dir := setupInterDepWorkspace(t)
	chdir(t, dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	err := cmd.Execute()
	assert.NoErr(err)

	lockData, _ := os.ReadFile(filepath.Join(dir, "pensa.lock"))
	lockContent := string(lockData)

	// six: direct dep of api.
	assert.True(strings.Contains(lockContent, "name = \"six\""))
	// certifi: transitive dep via api → interdep-lib → certifi.
	assert.True(strings.Contains(lockContent, "name = \"certifi\""))
}

func TestWorkspaceMember_InterDeps_MemberNotInLock(t *testing.T) {
	assert := is.New(t)
	dir := setupInterDepWorkspace(t)
	chdir(t, dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	err := cmd.Execute()
	assert.NoErr(err)

	lockData, _ := os.ReadFile(filepath.Join(dir, "pensa.lock"))
	lockContent := string(lockData)

	// Workspace member should NOT be in lock as a PyPI package.
	assert.True(!strings.Contains(lockContent, "name = \"interdep-lib\""))
	assert.True(!strings.Contains(lockContent, "name = \"interdep-api\""))
}

func TestWorkspaceMember_InterDeps_InstallWorks(t *testing.T) {
	assert := is.New(t)
	dir := setupInterDepWorkspace(t)
	chdir(t, dir)

	// Lock first.
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	err := cmd.Execute()
	assert.NoErr(err)

	// Install.
	cmd = newRootCmd()
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"install"})
	err = cmd.Execute()
	assert.NoErr(err)

	out := buf.String()
	// Should mention installing both members in editable mode.
	assert.True(strings.Contains(out, "interdep-api") || strings.Contains(out, "interdep-lib") || strings.Contains(out, "up to date"))
}

// setupChainWorkspace creates A → B → C chain:
// app depends on ["mid-lib" (workspace)]
// mid-lib depends on ["base-lib" (workspace)]
// base-lib depends on ["idna" (PyPI)]
func setupChainWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`
[project]
name = "chain-workspace"
version = "0.1.0"
requires-python = ">=3.10"

[tool.pensa.workspace]
members = ["app", "libs/mid", "libs/base"]

[tool.pensa.sources]
chain-mid = { workspace = true }
chain-base = { workspace = true }
`), 0644)

	appDir := filepath.Join(dir, "app")
	os.MkdirAll(appDir, 0755)
	os.WriteFile(filepath.Join(appDir, "pyproject.toml"), []byte(`
[project]
name = "chain-app"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["chain-mid", "six>=1.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)
	os.MkdirAll(filepath.Join(appDir, "chain_app"), 0755)
	os.WriteFile(filepath.Join(appDir, "chain_app", "__init__.py"), []byte(""), 0644)

	midDir := filepath.Join(dir, "libs", "mid")
	os.MkdirAll(midDir, 0755)
	os.WriteFile(filepath.Join(midDir, "pyproject.toml"), []byte(`
[project]
name = "chain-mid"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["chain-base", "certifi>=2023.0.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)
	os.MkdirAll(filepath.Join(midDir, "chain_mid"), 0755)
	os.WriteFile(filepath.Join(midDir, "chain_mid", "__init__.py"), []byte(""), 0644)

	baseDir := filepath.Join(dir, "libs", "base")
	os.MkdirAll(baseDir, 0755)
	os.WriteFile(filepath.Join(baseDir, "pyproject.toml"), []byte(`
[project]
name = "chain-base"
version = "0.1.0"
requires-python = ">=3.10"
dependencies = ["idna>=3.0"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
`), 0644)
	os.MkdirAll(filepath.Join(baseDir, "chain_base"), 0755)
	os.WriteFile(filepath.Join(baseDir, "chain_base", "__init__.py"), []byte(""), 0644)

	return dir
}

func TestWorkspaceMember_InterDeps_Chain(t *testing.T) {
	assert := is.New(t)
	dir := setupChainWorkspace(t)
	chdir(t, dir)

	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"lock"})
	err := cmd.Execute()
	assert.NoErr(err)

	lockData, _ := os.ReadFile(filepath.Join(dir, "pensa.lock"))
	lockContent := string(lockData)

	// Direct dep of app.
	assert.True(strings.Contains(lockContent, "name = \"six\""))
	// Transitive via app → chain-mid.
	assert.True(strings.Contains(lockContent, "name = \"certifi\""))
	// Transitive via app → chain-mid → chain-base.
	assert.True(strings.Contains(lockContent, "name = \"idna\""))

	// No workspace members in lock.
	assert.True(!strings.Contains(lockContent, "name = \"chain-app\""))
	assert.True(!strings.Contains(lockContent, "name = \"chain-mid\""))
	assert.True(!strings.Contains(lockContent, "name = \"chain-base\""))
}
