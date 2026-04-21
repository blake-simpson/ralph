# AGENTS

This file provides guidance to Ai Agents when working with code in this repository.

## Notes
- When updating code, always ensure the README and docs/ are up to date with the new changes/paths etc.
  - The README covers the high-level overview, quick start, how it works, installation, and brief tables for skills/tools.
  - Detailed reference content lives in `docs/` (cli-commands, supported-tools, skills-reference, workflow, directory-structure, prd-format, agent-pipeline, updating, troubleshooting).
  - If a change affects both the README summary and a docs page, update both.
- When changing the Go code, always run the compiler after to test + rebuild the file

## Verify
- Try to verify your work after changes are made.
- If required, create a test directory and install to it to test your changes, symlinks, etc.

## Build & Run

```bash
# Development: compile without embedded files (dev mode)
go build ./cmd/belmont

# Development: run directly (requires --source for install)
go run ./cmd/belmont status --root /path/to/project
go run ./cmd/belmont install --source . --project /tmp/test-project --no-prompt

# Release build: compile with embedded skills/agents + version injection
./scripts/build.sh 0.2.0

# Or use the dev install script (builds + records source path)
./bin/install.sh --setup
```

**Important**: `go run` and plain `go build` do NOT embed skills/agents (they use the `!embed` build tag). The `install` command will fall back to source resolution (`--source` flag, `BELMONT_SOURCE` env, config file, or walking up from binary). Use `scripts/build.sh` to produce a release binary with embedded content.

There are no tests or linter configured. Verify changes by compiling (`go build ./cmd/belmont`) and manually testing commands.

## Skills Generation

Skills in `skills/belmont/` are generated from templates. **Do not edit generated files directly** — edit the source:

- **Shared content**: `skills/belmont/_partials/*.md` — reusable blocks with `{{variable}}` placeholders, inlined at build time via `<!-- @include ... -->`
- **Templates**: `skills/belmont/_src/*.md` — skill templates that include partials
- **Progressive-disclosure references**: `skills/belmont/_src/references/<skill>-<topic>.md` — detail loaded on demand by skills (NOT inlined). Named with the owning skill as a prefix. Skill bodies point at them via relative paths like `references/implement-milestone-template.md` so the same path resolves in every install target.
- **Generated output**: `skills/belmont/*.md` and `skills/belmont/references/*.md` — the files that get installed into projects

After editing partials or templates:

```bash
./scripts/generate-skills.sh          # Regenerate
./scripts/generate-skills.sh --check  # Verify generated files are up to date
```

Files without a `_src/` counterpart (`status.md`, `reset.md`) are edited directly.

The sub-agent dispatch strategy is shared via `skills/belmont/_partials/dispatch-strategy.md` and inlined at build time into orchestrator skills (implement, verify).

## Release Process

```bash
# 1. Prepare release (generates changelog, commits, tags)
./scripts/release.sh 0.2.0

# 2. Push to trigger GitHub Actions
git push origin main --tags

# GitHub Actions will:
#   - Cross-compile for darwin-amd64, darwin-arm64, linux-amd64, linux-arm64, windows-amd64
#   - Generate SHA-256 checksums
#   - Create a GitHub Release with all binaries
```

## Design Decisions

- **`--from`/`--to` is single-feature only**: Milestone range flags (`--from`, `--to`) are blocked in multi-feature mode (`--features`, `--all`) because milestone IDs (M1, M3, etc.) are local to each feature — the same ID means different things across features.
- **Ports: primary vs additional servers**: `PORT`/`BELMONT_PORT` is allocated by the Go CLI for the primary dev server (frameworks auto-detect it). All other servers (Storybook, Prisma Studio, etc.) must dynamically allocate their own free port at runtime — this is handled by agent instructions, not Go code. See `_partials/worktree-awareness.md`.
- **Unified state tracking**: PROGRESS.md is the single source of truth for all task/milestone state. PRD.md is a pure spec with no status markers. See "State Tracking" section below.

## State Tracking

All task and milestone state lives in PROGRESS.md. PRD.md is a pure specification document with no status markers.

### Task states (PROGRESS.md checkboxes)

| Marker | State | Meaning |
|--------|-------|---------|
| `[ ]` | todo | Not started |
| `[>]` | in_progress | Currently being worked on |
| `[x]` | done | Implemented, not yet verified |
| `[v]` | verified | Implemented and passed verification |
| `[!]` | blocked | Cannot proceed |

### Milestone status: always computed from tasks
- All `[v]` → verified
- All `[x]` or `[v]` → done (needs verification)
- Any `[>]` → in progress
- Any `[!]` → has blockers
- All `[ ]` → not started

### Feature/master status: computed from milestones

### Key rules
- **No emoji on milestone headers** — milestone status is computed, not stored. Headers are `### M1: Name`.
- **No `## Blockers` section** — blocked tasks are `[!]` checkboxes. The Go CLI counts them directly.
- **No `## Status:` line** — overall status is computed from task states.
- **Follow-up tasks** are plain `[ ]` entries added to the relevant milestone (no special FWLUP format).
- **Reverify** finds milestones with `[x]` (done, not verified) tasks and re-verifies them. On success: `[x]` → `[v]`.
- **Master PRD.md** is a living global document (vision, constraints, cross-cutting decisions). No features table. Actively curated — edit/remove stale info.
- **Master TECH_PLAN.md** is a living global document for cross-cutting architecture. Same active curation.
- **Master PROGRESS.md** has the features table: `| Feature | Slug | Priority | Dependencies | Status | Milestones | Tasks |`. Status/Milestones/Tasks are computed. Priority and Dependencies are manually set during planning.

### Go CLI state parsing
- `parseMilestones()` reads milestone headers and task checkboxes from PROGRESS.md, building `milestone` structs with embedded `[]task` slices
- `flattenTasks()` extracts tasks from milestones for flat task lists
- `milestoneAllDone(m)`, `milestoneAllVerified(m)`, `milestoneHasBlockers(m)`, `milestoneNotStarted(m)` — computed helpers, used throughout decision logic
- Task state enum: `taskTodo`, `taskInProgress`, `taskDone`, `taskVerified`, `taskBlocked`
- `blockedTaskCount()` / `blockedTaskNames()` replace old blocker section parsing
- `parseMasterDeps()` reads Priority + Dependencies from master PROGRESS.md features table (not from master PRD.md)

## Architecture

Belmont is an agent-agnostic AI coding toolkit. It installs markdown-based **skills** (workflow prompts) and **agents** (sub-agent instructions) into any AI coding tool's project directory.

### Key directories

- `cmd/belmont/main.go` — Single-file Go CLI. All logic lives here (status parsing, installer, updater). No external dependencies.
- `cmd/belmont/embed.go` — `//go:embed` directives for release builds (build tag: `embed`). Embeds `skills/`, `agents/`, and `prompts/` into the binary.
- `cmd/belmont/embed_dev.go` — Empty embed vars for dev builds (build tag: `!embed`). Allows `go run` without embedded content.
- `skills/belmont/` — Skill markdown files (product-plan, tech-plan, implement, next, verify, status, reset). These are the source-of-truth copied/linked into target projects.
- `skills/belmont/_partials/` — Shared content blocks used by skill templates (identity-preamble, forbidden-actions, progress-template, dispatch-strategy).
- `skills/belmont/_src/` — Skill template files with `@include` directives. Processed by `generate-skills.sh` to produce `skills/belmont/*.md`.
- `skills/belmont/_src/references/` — Progressive-disclosure detail files (`<skill>-<topic>.md`). Copied verbatim to `skills/belmont/references/` by `generate-skills.sh`. Skill bodies point at them via relative `references/*.md` paths rather than inlining the content, so the detail only loads when the skill actually needs it. Prefix-matched per skill into `plugin/skills/<name>/references/` by `generate-plugin.sh`.
- `agents/belmont/` — Agent instruction markdown files (codebase-agent, design-agent, implementation-agent, verification-agent, code-review-agent, reconciliation-agent). Copied into target projects.
- `prompts/belmont/` — AI prompt templates used by the CLI (e.g. `ai-decision.md`, `post-verify-triage.md`). Loaded via Go `text/template` with dynamic context injection. Embedded in release builds.
- `scripts/build.sh` — Regenerates skills from templates, copies skills/agents/prompts into `cmd/belmont/`, builds with `-tags embed` and ldflags version injection, then cleans up.
- `scripts/release.sh` — Regenerates skills, verifies build, generates CHANGELOG entry, commits, creates annotated git tag.
- `scripts/generate-skills.sh` — Generates skill files from `_src/` templates + `_partials/`. Supports `--check` to verify files are up to date.
- `.github/workflows/release.yml` — GitHub Actions: cross-compile on tag push, create GitHub Release with binaries.
- `install.sh` (root) — Public curl-pipe-sh installer for end users.
- `bin/install.sh` / `bin/install.ps1` — Developer bootstrap scripts that build from source.
- `docs/` — Reference documentation (cli-commands, supported-tools, skills-reference, workflow, directory-structure, prd-format, agent-pipeline, updating, troubleshooting).

### How the installer works

`belmont install` syncs skills and agents into a target project. In release binaries, content is extracted from the embedded filesystem. In dev builds, it reads from a source directory.

1. **Embedded mode** (release binary, no `--source`): extracts from `embed.FS`
2. **Source mode** (`--source` flag or `BELMONT_SOURCE` env): reads from filesystem
3. Wires each detected AI tool to those canonical locations (symlinks for Cursor/Windsurf/Gemini/Copilot, copies for Claude Code/Codex)
4. For Codex installs, adds/updates a marked section in `AGENTS.md` that routes `belmont:<skill>` requests to local files
5. Removes legacy Belmont-managed root `SKILLS.md` (if present from older installs)
6. Creates `.belmont/` state directory with PRD.md and PROGRESS.md templates
7. Cleans stale files — if a skill was renamed/removed in source, the old file is deleted from the target

Source resolution order (source mode only): `--source` flag > `BELMONT_SOURCE` env > `~/.config/belmont/config.json` > walk up from CLI binary location.

### CLI commands

The Go CLI (`cmd/belmont/main.go`) provides: `install`, `update`, `status`, `auto` (alias: `loop`), `reverify`, `sync`, `recover`, `version`. All commands support `--format json` for machine-readable output. The `status` command parses `.belmont/PROGRESS.md` to extract tasks, milestones, and computed statuses. PRD.md is only read for the feature name. When `auto` is running, `status` reads live state from active worktrees via `.belmont/auto.json`. The `auto` command automates end-to-end feature implementation by shelling out to AI tool CLIs (Claude Code, Codex, Gemini, Copilot, Cursor) in headless mode. It supports milestone dependencies with `(depends: M1)` syntax in PROGRESS.md, enabling parallel execution via git worktrees when milestones are independent. Each worktree gets isolated `.belmont/` state (copy-based, not symlinked) so AI agents commit state changes as part of their feature branch. Each worktree is assigned a unique `PORT`/`BELMONT_PORT` env var for dev server isolation, and `.belmont/worktree.json` provides user-configurable setup/teardown hooks (e.g., `npm install`). The `reverify` command finds milestones with `[x]` tasks (done but not verified) and runs verification on each sequentially. On success tasks are marked `[v]`; on failure, new `[ ]` follow-up tasks are added. Supports `--from`/`--to` range filtering and `--tool` to specify the AI tool. The `sync` command updates the master `.belmont/PROGRESS.md` feature table to match computed feature-level states (explicit command only). The `recover` command manages preserved worktrees from failed merges — listing, retrying merges with improved error handling, or cleaning up. The `update` command self-updates by downloading the latest release from GitHub, and auto-migrates old state format to unified tracking.
