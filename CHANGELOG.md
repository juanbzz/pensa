# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-03-24

First release. A Python package and project manager written in Go.

### Added

- 16 commands: `new`, `add`, `remove`, `lock`, `update`, `install`, `sync`, `run`, `list`, `show`, `tree`, `check`, `env`, `build`, `publish`, `version`
- PubGrub dependency resolver
- Read `pyproject.toml` in both PEP 621 (uv) and Poetry formats
- Read `pensa.lock`, `uv.lock`, and `poetry.lock` without re-resolution
- Native `pensa.lock` format with embedded download URLs (installs without querying PyPI)
- Dependency groups via PEP 735 and Poetry format (PEP 735 takes precedence)
- Extras support (`pensa add "requests[security]"`)
- Pre-release version filtering by default, with fallback if no stable release exists
- Platform-specific wheel selection (macOS, Linux manylinux, Windows)
- Parallel downloads (4 concurrent)
- Incremental installs (scan site-packages, skip what's current)
- Editable installs via PEP 660
- Project scripts from `[project.scripts]` installed as CLI commands
- Exact venv sync via `pensa sync` (install missing + remove extras)
- Workspace support via `[tool.pensa.workspace]` with `members` glob patterns
- Multi-project resolution into a single root lock file
- Workspace-aware `lock` and `install` commands
- Build sdist and wheel via PEP 517 backends (hatchling, poetry-core, setuptools)
- Publish to PyPI with token authentication
- Colored output (respects `NO_COLOR`)
- Progress spinners for resolution and downloads
- Formatted tables for `list` output
- Python discovery via pyenv, asdf, mise, homebrew, and conda

[Unreleased]: https://github.com/juanbzz/pensa/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/juanbzz/pensa/releases/tag/v0.1.0
