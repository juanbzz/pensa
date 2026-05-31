# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.1] - 2026-05-31

### Fixed

- `pensa add <pkg>` followed by lock no longer fails with `no versions of X match >=<latest>,<4.0` when the Simple-API cache has gone stale. Add now invalidates pensa's resolution cache for every package it touches, so the lock step sees the same fresh version listing add just picked.
- Editable install of setuptools-backed local packages (`build-backend = "setuptools.build_meta"`) now actually installs the project: the editable `.pth`, finder module, and `dist-info` end up in the venv and `import <yourproject>` works. Previously setuptools' egg_info chatter leaked into a pip requirement; third-party deps installed but the local package didn't.
- abi3 wheels (`cp3MIN-abi3`) are no longer routed past the install pre-filter into the sdist build path. The scorer already recognized them; the install gate now does too. Affects packages like `psutil`.
- Building a wheel from an sdist whose pyproject declares `setuptools.build_meta` but omits `wheel` from `build-system.requires` no longer fails with `error: invalid command 'bdist_wheel'`. Pensa now seeds `wheel` into the isolated build env when the backend is setuptools — same as pip/build/uv.

## [0.3.0] - 2026-05-04

### Added

- `pensa why <pkg>` shows the reverse-dependency chain explaining why a package is in the lock.
- `pensa lock --upgrade` and `pensa lock --upgrade-package <name>` to selectively re-resolve.
- Partial-group locking: `pensa lock --only group` (or `--with`/`--without`) scopes the lock to that group set, persisted across subsequent `add`/`remove`/`update`.
- `pensa lock` is cancellable: Ctrl+C exits cleanly without leaving partial lock files.

### Changed

- **Breaking:** Go module path moved from `github.com/juanbzz/pensa` to `pensa.sh/pensa`. `go install pensa.sh/pensa/cmd/pensa@latest`.
- Source published to Codeberg (`codeberg.org/juanbz/pensa`) and GitHub (`github.com/juanbzz/pensa`).
- `pensa add` and `pensa remove` keep pyproject.toml edits to single-line diffs and add a trailing comma to the prior last entry on insert.
- User-facing strings refer to "lock file" generically rather than `poetry.lock`.

### Fixed

- abi3 wheels built for older Python versions are now matched (e.g. `cp39-abi3` on Python 3.12).
- Sdists are no longer silently skipped when no compatible wheel is available.
- universal2 wheels: license metadata handled; falls back to sdist when the universal wheel fails.
- `zope.interface` and `zope-interface` are treated as the same package.
- Workspace: `pensa install --package <member>` actually scopes to that member.
- Workspace: `pensa update` walks members instead of bailing at the root.
- Workspace: `pensa add` and `pensa remove` aggregate top-level deps across all members.
- Workspace: `pensa check` is workspace-aware (hash + wording).
- Lock: transitive deps are tagged with their parent's group.
- Lock: platform-mismatched deps dropped at lock time, not only at install.
- Install: converges the venv when group flags filter the lock (removes packages that are no longer in scope).
- Install: reads `pyvenv.cfg` to find the venv's Python instead of the host Python.
- Sync: skips packages incompatible with the current Python or platform.
- Build: surfaces the build backend's actual error message; CPython traceback frames are dropped.
- Resolve: handles unbounded-upper `requires-python`, deps whose python-only marker can't apply, and union-covers-everything cases.
- Resolver: defensive upper bounds on Python version are stripped so libraries don't block newer Pythons.
- Resolver: `Union.AllowsAll` subset check; `anyConstraint.Difference` handles `Union`.
- Marker-based extras filtering walks the AST instead of substring-matching.
- `pensa run` parses Cobra flags correctly when running scripts.
- Solver-failure output names the most-conflicted packages when the iteration cap is hit, gets a footer, and stops double-prefixing.
- Multi-line diagnostic continuation lines align under the message body.

### Performance

- Install hard-links wheels from the global cache instead of copying, with a copy fallback across filesystems.
- Resolver re-uses prior-run picks when re-locking.
- Resolver keeps handled clauses across backtracks and learns version-range constraints instead of per-version atoms.
- Resolution cache uses copy-on-write to avoid blocking writers behind readers.
- Background prefetches are drained before flushing the resolution cache so warm runs stay warm.

## [0.2.0] - 2026-03-27

### Added

- Workspace per-member commands, e.g., `pensa add flask --package backend` 
- Workspace inter-package dependencies. Handles A→B→C chains via BFS.
- Workspace sources, e.g., `[tool.pensa.sources]` with `workspace = true`. Tested with `[tool.uv.sources]` compat. Pensa sources take priority.
- Auto-sync on `pensa run`. Output to stderr. `--no-sync` to skip.
- PEP 508 specifiers in `pensa add`.
- Global flags `-v`/`--verbose`, `-q`/`--quiet`, `--color`, `--no-color`. Env vars: `PENSA_VERBOSE`, `PENSA_QUIET`, `PENSA_COLOR`.
- Configurable download concurrency. Defaults to 50, override via `PENSA_CONCURRENT_DOWNLOADS`.
- Structural lock validation.
- HTTP conditional requests.
- Speculative version prefetch. Prefetches next 10 versions during solving to hide backtracking latency.

### Changed

- Feedback UI standardized.
- Resolution cache batched writes. `Put()` is in-memory only, single `Flush()` after solving. CPU dropped from 12s to 0.7s on medium-sized workspace.
- PackageInfo served from resolution cache. Avoids parsing large JSON Simple API responses on warm runs.
- Use `goccy/go-json`. Drop-in 2-3x faster JSON parser.
- Download concurrency increased from 8 to 50 workers.

### Fixed

- requires-python filtering.
- Skip incompatible packages on install.
- `pensa show`/`list`/`tree` in workspaces.

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

[Unreleased]: https://codeberg.org/juanbz/pensa/compare/v0.3.0...HEAD
[0.3.0]: https://codeberg.org/juanbz/pensa/compare/v0.2.0...v0.3.0
[0.2.0]: https://codeberg.org/juanbz/pensa/compare/v0.1.0...v0.2.0
[0.1.0]: https://codeberg.org/juanbz/pensa/releases/tag/v0.1.0
