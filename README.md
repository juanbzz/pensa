# pensa

A fast enough Python package and project manager, written in Go.

## Why pensa?

Python packaging tools are increasingly owned by a few companies. pensa aims to add diversity as an independent alternative.

pensa began as a Poetry port to Go but found its own direction by following the [CLI Guidelines](https://clig.dev/) and UNIX principles. It provides focused, accessible commands and works with your existing project files without changes.

### What it does

- Resolves dependencies, manages virtual environments, builds and publishes packages
- Reads `pyproject.toml` in both uv (PEP 621) and Poetry formats
- Reads `pensa.lock`, `uv.lock`, and `poetry.lock` without re-resolving
- Dependency groups with PEP 735, extras support, platform-specific wheel selection
- Workspaces for monorepo management with single lock file
- Editable installs with project scripts (PEP 660)
- Build and publish to PyPI via PEP 517 backends (hatchling, poetry-core, setuptools)
- Works with pyenv, asdf, mise, homebrew, and conda

### What makes it different

- Written in Go
- Single binary, no runtime dependencies
- Parallel downloads, incremental installs
- Lock file with embedded download URLs (installs without querying PyPI)
- Transparent workspace discovery (no special subcommands)
- Pre-release versions filtered by default
- Designed around [clig.dev](https://clig.dev/) for accessible, predictable CLI behavior

## Install

Homebrew:

```
brew install pensa-sh/tap/pensa
```

Or download a binary from [GitHub Releases](https://github.com/juanbzz/pensa/releases).

## Getting started

```
pensa new myproject
cd myproject
pensa add requests
pensa run python -c "import requests; print(requests.__version__)"
```

This creates a project, adds a dependency, resolves it against PyPI, creates a virtual environment, installs the package, and runs a command inside the venv.

## Documentation

For guides, command reference, and configuration details, visit [pensa.sh](https://pensa.sh).

## Contributing

Issues and pull requests are welcome at [github.com/juanbzz/pensa](https://github.com/juanbzz/pensa).

## License

MIT
