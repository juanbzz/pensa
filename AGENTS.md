# Agents

## Issue tracking: beads

This project uses [beads](https://github.com/steveyegge/beads) (`bd`) for issue tracking. The database lives in `.beads/` and is initialized in stealth mode (no git hooks, no auto-commit).

```bash
# List open issues
bd list

# Create a new issue
bd create "title" -t bug -p 1 -d "description"

# Show an issue
bd show <id>

# Find ready work (no open blockers)
bd ready

# Close an issue
bd close <id> "resolution note"
```

Issue IDs use the `goetry-` prefix (historical — kept during the rename to pensa).

`bd --help` for the full surface.
