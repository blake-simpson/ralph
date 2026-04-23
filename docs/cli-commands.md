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
belmont status --color always           # Force ANSI-coloured markers (auto|always|never; auto honors NO_COLOR + TTY)
belmont auto --feature auth              # Run feature auto (auto-detect tool)
belmont auto --feature auth --tool codex # Use specific tool
belmont auto --feature auth --from M2 --to M4  # Milestone range
belmont auto --features auth,payments    # Run multiple features in parallel
belmont auto --all                       # Run all pending features in parallel
belmont auto --all --max-parallel 2      # Cap concurrent features
belmont reverify --feature my-feature     # Re-verify all completed milestones
belmont reverify --feature my-feature --from M3 --to M10  # Re-verify specific range
belmont reverify --feature my-feature --tool codex  # Use specific tool
belmont sync                             # Sync master PROGRESS.md with feature states (explicit only, no longer auto-hooked)
belmont recover                          # List preserved worktrees from failed merges
belmont recover --list                   # Same as above
belmont recover --merge auth             # Retry merge for a preserved worktree
belmont recover --clean auth             # Delete worktree and branch
belmont recover --clean-all              # Clean all preserved worktrees
belmont steer --message "pin all axes"   # Inject instructions into an in-flight auto run
belmont steer --milestone M5 --file fix.md   # Scope to one milestone, read from file
belmont steer -                          # Read steering text from stdin
belmont steer                            # Opens $EDITOR when a TTY is attached
belmont validate                         # Lint PROGRESS.md for milestone-structure violations
belmont validate --feature about         # Scope lint to one feature
belmont version                         # Show version, commit, build date
# Note: "belmont loop" still works as an alias for "belmont auto"
# If a previous run was interrupted, auto detects stale branches and prompts to resume or restart
```

## Milestone-structure validation

`belmont validate` lints `PROGRESS.md` for milestone-structure violations — the class of bug documented in [`knowledge/cross-cutting/milestone-immutability.md`](../knowledge/cross-cutting/milestone-immutability.md). It detects two patterns:

- **Polish / follow-up milestone names.** Milestones whose name matches `polish`, `follow-ups`, `cleanup`, `verification fixes`, `deviations from M<N>`, `from M<N> implementation`, `fwlup(s)`. These violate the rule that follow-ups stay in the milestone that discovered them.
- **Cross-milestone task IDs.** Task IDs like `P3-FWLUP-M2-1` that embed a milestone number should live under that milestone; when they're found under a different one, the milestone structure is lying about ownership and parallel merges will collide.

```bash
belmont validate                            # Scan every feature
belmont validate --feature about            # One feature
belmont validate --format json              # Machine-readable output
```

Exit code `1` on violations. `belmont auto` runs this lint at startup; interactive runs get a `[y/N]` override prompt, non-interactive runs abort. Restructure via `/belmont:tech-plan` before rerunning.

## Steering a running auto run

`belmont steer` is the way to hand new instructions to an `auto` run that's
already in progress — headless agent invocations never see stdin, so typing
into the terminal does nothing. The command appends a pending entry to
`STEERING.md` inside each active worktree (or the master feature directory
for non-parallel runs). Before the next agent phase fires, the auto loop
reads any matching entries and prepends them to the agent's prompt as a
high-priority block (higher than `NOTES.md`).

Lifecycle:

- Consumed entries are **dropped from disk** — they don't accumulate inside
  `STEERING.md`. When the last pending entry is consumed the file is
  deleted, so agents that explore `.belmont/features/<slug>/` never
  re-read steering text that's already been injected into the prompt.
- The durable audit trail lives in the auto run's stderr stream — look for
  `[feature][milestone]: [STEERING] injected N instruction(s) — "…"`
  lines with their timestamps.

Rules:

- Only works while `belmont auto` has an active `.belmont/auto.json`.
  Manual CLI sessions are steered by typing directly into the running
  terminal.
- With no `--milestone`, writes to every active worktree for the feature.
- With `--milestone M5`, writes only to that milestone's worktree.
- Exactly one input source is required: `--message "text"`, `--file PATH`,
  `-` (stdin), or no source with `$EDITOR` set and a TTY attached.
- `copyBelmontStateToWorktree` preserves `STEERING.md` across the
  resume-time state refresh, so steering you drop before resuming a
  preserved worktree survives.

## Worktree Environment Variables

When `belmont auto` runs features or milestones in parallel worktrees, the following environment variables are automatically set for each worktree:

| Variable | Description |
|----------|-------------|
| `PORT` | Unique free port assigned to this worktree |
| `BELMONT_PORT` | Same value as `PORT` |
| `BELMONT_WORKTREE` | Set to `1` in worktree context |

Dependencies are auto-installed by detecting your lock file (e.g., `package-lock.json` → `npm install`). Configure custom worktree lifecycle hooks via `.belmont/worktree.json`. See [Worktree Isolation](worktree-isolation.md) for full documentation.

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
