# Feature Loop

The `belmont loop` command automates the full implementation cycle for a Belmont feature. Given a feature with a PRD and TECH_PLAN, it implements milestones, verifies them, fixes follow-up issues, and continues until the feature is complete.

## Prerequisites

- A feature directory at `.belmont/features/<slug>/` with `PRD.md`, `TECH_PLAN.md`, and `PROGRESS.md`
- The `belmont` CLI installed and on PATH
- At least one supported AI tool CLI installed

## Supported Tools

| Tool | CLI command | Auto-approve flag | Output format |
|------|------------|-------------------|---------------|
| **Claude Code** | `claude -p "<prompt>"` | `--permission-mode bypassPermissions` | `--output-format json` |
| **Codex** | `codex exec "<prompt>"` | `--dangerously-bypass-approvals-and-sandbox` | `--json` |
| **Gemini** | `gemini "<prompt>"` | `--yolo` | `--output-format json` |
| **Copilot** | `copilot -p "<prompt>"` | `--yolo` | text only |
| **Cursor** | `cursor agent -p "<prompt>"` | `--force` | `--output-format json` |

Windsurf has no headless CLI and is not supported for the loop.

## Usage

```bash
# Run all pending milestones for a feature
belmont loop --feature my-feature

# Run a specific milestone range
belmont loop --feature my-feature --from M2 --to M6
belmont loop --feature my-feature --from M3     # M3 through end
belmont loop --feature my-feature --to M4       # start through M4

# Use a specific AI tool (default: auto-detect)
belmont loop --feature my-feature --tool codex

# Control checkpoint policy
belmont loop --feature my-feature --policy milestone

# Set iteration limits
belmont loop --feature my-feature --max-iterations 30

# Specify project root
belmont loop --feature my-feature --root /path/to/project
```

## How It Works

The loop uses a layered decision system:

```
┌──────────────────────┐
│  Hard Guardrails     │  Go code, always runs first
│  (blockers, failures,│  Returns PAUSE/ERROR if triggered
│   stuck detection)   │
├──────────────────────┤
│  Smart Rules Engine  │  Deterministic rules, handles ~80% of cases
│  (work type aware,   │  Uses git diff classification + milestone state
│   milestone tracking)│
├──────────────────────┤
│  AI Decision Layer   │  Only for ambiguous cases, with rich context
│  (enhanced prompt,   │  Includes milestone verification state,
│   rules fallback)    │  work type, file counts, failure history
├──────────────────────┤
│  Execution Layer     │  Shells out to tool with skill prompt
└──────────────────────┘
```

### Hard Guardrails

These always run first:

1. **Blockers detected** → PAUSE for human intervention
2. **Consecutive failures** (default 3) → ERROR, stop the loop
3. **Stuck detection** (no state change after 2 iterations) → PAUSE

### Smart Rules Engine

After guardrails, deterministic rules handle the majority of decisions without an AI call:

1. **After IMPLEMENT_MILESTONE success → almost always VERIFY**
   - Skip only for: 0 files changed, pure docs, or non-critical config with ≤2 files
   - Frontend, backend, mixed, critical config: always VERIFY
2. **After VERIFY success + no follow-ups** → next undone milestone or COMPLETE
3. **After VERIFY success + follow-ups exist** → IMPLEMENT_NEXT
4. **After IMPLEMENT_NEXT success** → re-VERIFY
5. **After VERIFY failure (2+ times same milestone)** → delegate to AI for REPLAN/DEBUG
6. **After VERIFY failure (first time)** → IMPLEMENT_NEXT to fix issues
7. **After DEBUG success** → VERIFY
8. **All milestones done + verified + no follow-ups** → COMPLETE

The smart rules track per-milestone state (implemented, verified, verify failure count) and classify work type from git diffs (frontend, backend, config, docs, mixed, minimal).

### AI Decisions

The AI is only called for ambiguous cases the smart rules can't handle (e.g., repeated verification failures). It receives rich context:

- Per-milestone state: implemented, verified, verify failure count, work type, files changed
- Last 5 actions with success/failure, work type, and 500 chars of output
- Previous iteration output (last 1500 chars)
- Ambiguity reason explaining why the AI was called

The AI responds with a JSON object specifying the action, reason, and optional milestone ID. If the AI call fails, the loop falls back to the legacy deterministic rules engine.

### Action Types

| Action | Description |
|--------|-------------|
| IMPLEMENT_MILESTONE | Implement next incomplete milestone |
| IMPLEMENT_NEXT | Fix follow-up tasks or issues found during verification |
| VERIFY | Run verification on completed milestones |
| DEBUG | Run automated debugging when verification keeps failing |
| REPLAN | Re-run tech planning when current approach has systemic issues |
| SKIP_MILESTONE | Skip a milestone blocked by external factors |
| COMPLETE | All work in scope is done and verified |
| PAUSE | Stop for human intervention |
| ERROR | Unrecoverable failure, stop the loop |

DEBUG, REPLAN, and SKIP_MILESTONE are only available via AI decisions (not the smart rules or rules fallback). DEBUG triggers `/belmont:debug-auto`, REPLAN triggers `/belmont:tech-plan`, and SKIP_MILESTONE marks the milestone done in PROGRESS.md directly (no tool call).

### Tool Auto-Detection

When `--tool` is not specified, the loop checks `$PATH` for supported CLIs in priority order: `claude`, `codex`, `gemini`, `copilot`, `cursor`. The first one found is used. If none are found, the loop exits with a helpful error message.

### Execution Layer

Each action shells out to the selected tool CLI in headless mode:

| Action | Prompt sent to tool |
|--------|-------------------|
| IMPLEMENT_MILESTONE | `/belmont:implement --feature <slug>` |
| IMPLEMENT_NEXT | `/belmont:next --feature <slug>` |
| VERIFY | `/belmont:verify --feature <slug>` |
| DEBUG | `/belmont:debug-auto --feature <slug>` |
| REPLAN | `/belmont:tech-plan --feature <slug>` |

## Checkpoint Policies

| Policy | Behavior |
|--------|----------|
| `autonomous` (default) | Only pauses on blockers, errors, or stuck detection |
| `milestone` | Pauses before each new milestone, after verification, and for REPLAN/SKIP_MILESTONE |
| `every_action` | Human approves every step |

## Safety Guardrails

- **Max iterations**: Hard cap (default 20) prevents runaway loops
- **Consecutive failures**: 3 failures in a row → stop
- **Stuck detection**: Same state after 2 successful iterations → pause

## CLI Options

| Option | Default | Description |
|--------|---------|-------------|
| `--feature <slug>` | (required) | Feature slug to implement |
| `--from <milestone>` | | Start from milestone (e.g. M2) |
| `--to <milestone>` | | End at milestone (e.g. M6) |
| `--tool <name>` | auto-detect | CLI tool: claude, codex, gemini, copilot, cursor |
| `--policy <policy>` | `autonomous` | Checkpoint policy |
| `--max-iterations <n>` | `20` | Maximum loop iterations |
| `--max-failures <n>` | `3` | Consecutive failures before stopping |
| `--root <path>` | `.` | Project root directory |

## Architecture

```
┌─────────────────────────────────┐
│     belmont loop CLI            │  belmont loop --feature <slug>
├─────────────────────────────────┤
│     Loop Controller             │  Main loop, error handling
├─────────────────────────────────┤
│     Hard Guardrails             │  Blockers, failures, stuck detection
├─────────────────────────────────┤
│     Smart Rules Engine          │  Work-type-aware deterministic decisions
├─────────────────────────────────┤
│     AI Decision Layer           │  Ambiguous cases only, enhanced context
├─────────────────────────────────┤
│     Multi-tool Executor         │  Shells out to claude/codex/gemini/copilot/cursor
├─────────────────────────────────┤
│     State Reader                │  Calls buildStatus() directly (no subprocess)
└─────────────────────────────────┘
```

The state reader reuses the existing Go `buildStatus()` function directly — no subprocess needed. The smart rules engine handles ~80% of decisions deterministically using git diff classification and per-milestone tracking. The AI decision layer is only called for ambiguous cases, with rich context including work type, verification history, and failure patterns. The legacy rules engine serves as automatic fallback if the AI call fails. The executor builds the appropriate CLI command for whichever tool is selected.
