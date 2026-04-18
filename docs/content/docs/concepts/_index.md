---
title: Concepts
weight: 3
---

## How pensa works

pensa is a single Go binary that manages Python projects. It handles dependency resolution, virtual environments, package installation, and building — without requiring Go or any runtime dependencies on the user's machine.

## Design principles

- **UNIX philosophy** — each command does one thing well. `list`, `show`, and `tree` are separate commands, not modes of one command.
- **CLI Guidelines** — designed around [clig.dev](https://clig.dev/) for accessible, predictable behavior.
- **No configuration needed** — pensa reads your existing `pyproject.toml` and lock files. No migration required.
- **Independent** — not backed by a company. Open source, community-driven.

## Compatibility

pensa reads:

- `pyproject.toml` in both PEP 621 (uv, pip) and Poetry formats
- `pensa.lock`, `uv.lock`, and `poetry.lock` without re-resolving
- Python installations from pyenv, asdf, mise, homebrew, and conda

## PEP compliance

| PEP | What | Status |
|-----|------|--------|
| 440 | Version parsing and comparison | Done |
| 508 | Dependency specifiers | Done |
| 503 | Simple Repository API (PyPI) | Done |
| 621 | pyproject.toml metadata | Done |
| 405 | Virtual environments | Done |
| 427 | Wheel format | Done |
| 517 | Build system interface | Done |
| 660 | Editable installs | Done |
| 735 | Dependency groups | Done |
