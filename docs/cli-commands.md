# CLI Commands

Belmont ships a small Go CLI (`belmont`) for status checks, file queries, and self-updating. Install via the [curl one-liner](../README.md#quick-start), or on Windows use `./bin/install.ps1` for a project-local helper.

## Usage

```bash
belmont install                         # Install skills/agents into current project
belmont update                          # Update to latest release
belmont update --check                  # Check for updates without installing
belmont status                          # View project progress
belmont status --format json            # Machine-readable status
belmont status --feature auth           # Feature-specific status
belmont tree --max-depth 3              # Project tree
belmont find --name PRD --type file     # Find files
belmont search --pattern "TECH_PLAN"    # Search file contents
belmont loop --feature auth              # Run feature loop (auto-detect tool)
belmont loop --feature auth --tool codex # Use specific tool
belmont loop --feature auth --from M2 --to M4  # Milestone range
belmont version                         # Show version, commit, build date
```

## How Skills Use the CLI

Skills prefer these helpers when available:
- `status` uses `belmont status` first
- `product-plan` and `tech-plan` may use `belmont tree`/`search` (or `find`) for quick structure/pattern checks
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
