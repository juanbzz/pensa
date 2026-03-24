# pensa v0.1.0

First release. A fast enough Python package and project manager, written in Go.

## Commands

16 commands: `new`, `add`, `remove`, `lock`, `update`, `install`, `sync`, `run`, `check`, `env`, `list`, `show`, `tree`, `build`, `publish`, `version`.

## Dependency management

- PubGrub dependency resolver
- Reads `[project]` (PEP 621) and `[tool.poetry]` formats
- PEP 735 dependency groups with `include-group` support
- Also reads Poetry `[tool.poetry.group.X]` as fallback
- Extras: `pensa add "requests[security]"`
- Pre-release versions filtered by default
- Incremental installs (skip what is already installed)
- Parallel downloads (4 concurrent)

## Lock file interop

- Native `pensa.lock` format with embedded download URLs
- Reads `uv.lock` and `poetry.lock` without re-resolving
- `pensa lock` respects existing pinned versions

## Build and publish

- PEP 517 build frontend (hatchling, poetry-core, setuptools)
- PEP 660 editable installs with project script support
- Publish to PyPI and TestPyPI with API token auth

## Workspaces

- `[tool.pensa.workspace]` for monorepo management
- Reads `[tool.uv.workspace]` for migration from uv
- Single lock file and venv at workspace root
- All members resolved together

## Platform support

- Platform-specific wheel selection
- macOS (arm64, x86_64), Linux (manylinux), Windows (amd64)
- Works with pyenv, asdf, mise, homebrew, conda

## UX

- Colored output, respects `NO_COLOR`
- Progress spinners during resolution and downloads
- Colored help output
- UNIX-style CLI design

## Known limitations

- TOML formatting not preserved (comments lost after edits)
- Workspace `add --package` not yet supported
- No private index support
- No `--json` output
