# CLI Commands

Belmont ships a small Go CLI (`belmont`) for status checks, automated feature implementation, and self-updating. Install via the [curl one-liner](../README.md#quick-start), or on Windows use `./bin/install.ps1` for a project-local helper.

## Usage

```bash
belmont install                         # Install skills/agents into current project
belmont update                          # Update to latest release
belmont update --check                  # Check for updates without installing
belmont status                          # View project progress
belmont status --format json            # Machine-readable status
belmont status --feature auth           # Feature-specific status
belmont auto --feature auth              # Run feature auto (auto-detect tool)
belmont auto --feature auth --tool codex # Use specific tool
belmont auto --feature auth --from M2 --to M4  # Milestone range
belmont auto --features auth,payments    # Run multiple features in parallel
belmont auto --all                       # Run all pending features in parallel
belmont auto --all --max-parallel 2      # Cap concurrent features
belmont sync                             # Sync master PROGRESS.md with feature states
belmont recover                          # List preserved worktrees from failed merges
belmont recover --list                   # Same as above
belmont recover --merge auth             # Retry merge for a preserved worktree
belmont recover --clean auth             # Delete worktree and branch
belmont recover --clean-all              # Clean all preserved worktrees
belmont version                         # Show version, commit, build date
# Note: "belmont loop" still works as an alias for "belmont auto"
# If a previous run was interrupted, auto detects stale branches and prompts to resume or restart
```

## Worktree Environment Variables

When `belmont auto` runs features or milestones in parallel worktrees, the following environment variables are automatically set for each worktree:

| Variable | Description |
|----------|-------------|
| `PORT` | Unique free port assigned to this worktree |
| `BELMONT_PORT` | Same value as `PORT` |
| `BELMONT_WORKTREE` | Set to `1` in worktree context |

Configure worktree lifecycle hooks via `.belmont/worktree.json`. See [Worktree Isolation](worktree-isolation.md) for full documentation.

## How Skills Use the CLI

Skills prefer these helpers when available:
- `status` uses `belmont status` first
- `implement`, `next`, `verify`, and `reset` may use `belmont status --format json` for summaries (still read `.belmont` files for full context)

## Windows

Build example (project-local helper):

```powershell
go build -o .belmont\\bin\\belmont.exe ./cmd/belmont
```

Helper install script:

```powershell
pwsh ./bin/install.ps1
```
