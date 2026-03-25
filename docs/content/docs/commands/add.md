---
title: add
weight: 2
---

## Usage

```bash
pensa add <package>
pensa add <package>@<constraint>
pensa add <package> -G <group>
```

## Description

Adds a dependency to `pyproject.toml`, resolves it, updates the lock file, and installs it.

## Examples

```bash
pensa add requests
pensa add httpx@^0.27
pensa add "requests[security]"
pensa add pytest -G dev
```

## Flags

| Flag | Description |
|------|-------------|
| `-G`, `--group` | Add to a dependency group (e.g., `dev`, `test`) |

## Behavior

1. Queries PyPI for the latest version matching the constraint (or latest stable if none given)
2. Adds the dependency to `[project.dependencies]` (or `[dependency-groups]` with `-G`)
3. Resolves all dependencies together
4. Writes the lock file
5. Installs the new package into the virtual environment
