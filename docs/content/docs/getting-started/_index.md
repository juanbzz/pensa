---
title: Getting Started
weight: 1
---

## Install

Homebrew:

```bash
brew install pensa-sh/tap/pensa
```

Or download a binary from [GitHub Releases](https://github.com/juanbzz/pensa/releases).

## Quick start

Create a new project, add a dependency, and run it:

```bash
pensa new myproject
cd myproject
pensa add requests
pensa run python -c "import requests; print(requests.__version__)"
```

This creates a project with a `pyproject.toml`, resolves dependencies against PyPI, creates a virtual environment, installs the package, and runs a command inside the venv.

## Existing projects

pensa reads existing `pyproject.toml` files in both PEP 621 (uv) and Poetry formats. It also reads `uv.lock` and `poetry.lock` without re-resolving.

```bash
cd your-existing-project
pensa install
pensa run python app.py
```

## What's next

- [Commands](../commands) — reference for all commands
- [Concepts](../concepts) — how pensa works under the hood
