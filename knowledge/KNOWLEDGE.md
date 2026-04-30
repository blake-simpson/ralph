# Belmont Knowledge Index

Curated operational knowledge for working on Belmont itself. Scan the table below, open only the entries that match what you're touching. Every entry is self-contained — one read per entry is sufficient; there are no required chain lookups.

## How to use

- **Before non-trivial architectural work** (skills, CLI, parallel mode, state model, agent coordination), scan this index. Entries are grouped by domain; match the "Read when you're about to…" column to what you're doing.
- **Open only relevant entries.** If you're changing `runScopeGuard`, you need `auto-mode/scope-guard-runtime.md` — you don't need `cross-cutting/port-isolation.md`. Skip files that don't match.
- **Amend, don't append.** When you make a change that updates an entry's truth, edit the entry in place and add one line to its `Revisions` footer. Do not add a new dated block at the top.
- **Cross-domain chronology comes from git log.** `git log --oneline -- knowledge/` gives you the full history of decisions across every topic. There is no global decision log file; it would bloat unbounded and duplicate git.
- **Every entry follows the same skeleton** so you can jump to the section you need: `Why this matters` → `Invariant` → `How it's enforced` → `Failure mode if you break it` → `Don't re-do` → `Evidence` → `Revisions`. Cross-cutting entries additionally carry a `Domains:` header line.

## When to read which entry

### Auto-mode specific

| Entry | Read when you're about to... |
|---|---|
| [auto-mode/scope-guard-runtime.md](auto-mode/scope-guard-runtime.md) | change `runScopeGuard` / `diffScopeViolations` / `rebuildAfterScopeGuard`, propose a new runtime enforcement for parallel mode, or weaken any post-phase check |
| [auto-mode/parallel-wave-orchestration.md](auto-mode/parallel-wave-orchestration.md) | change `runWaveParallel`, `runMilestoneInWorktree`, `copyBelmontStateToWorktree`, `gracefulShutdown`, merge sequencing, or the live-status overlay |
| [auto-mode/verify-evidence.md](auto-mode/verify-evidence.md) | change `runEvidenceCheck`, `taskHasCommit`, how verify marks `[v]`, or design a new evidence contract |
| [auto-mode/clean-tree-preflight.md](auto-mode/clean-tree-preflight.md) | touch `requireCleanWorkingTree`, `commitBelmontUpdate`, `belmontManagedPaths`, the `--allow-dirty` / `--no-commit` flags, or weaken auto's startup contract that the working tree must be clean |

### Cross-cutting (multiple domains)

| Entry | Domains | Read when you're about to... |
|---|---|---|
| [cross-cutting/milestone-immutability.md](cross-cutting/milestone-immutability.md) | skills, state, auto-mode | edit any skill that writes PROGRESS.md, add a new milestone, or design how follow-ups / polish / fixes are routed |
| [cross-cutting/port-isolation.md](cross-cutting/port-isolation.md) | cli, skills, agents | touch `buildWorktreeEnv`, `worktree-awareness.md`, or any agent logic that starts a server or probes a URL |
| [cross-cutting/steering.md](cross-cutting/steering.md) | cli, skills, auto-mode | change `belmont steer`, `consumePendingSteering`, STEERING.md lifecycle, or have another guard inject its own correction |

### Meta

| Entry | Read when... |
|---|---|
| [meta/validated-runs.md](meta/validated-runs.md) | proposing "let's simplify" or "we don't need X anymore" for any guard — cross-reference against the preserved proof branches before touching load-bearing code |

## Maintenance rules

- **Per-topic, not per-session.** Amend existing entries rather than appending dated blocks.
- **Keep each entry under ~200 lines.** Split if it balloons. Two related topics with clear boundaries are better than one overstuffed file.
- **`Revisions` footer per amendment.** One line: `YYYY-MM-DD — what changed`.
- **New topic → new file + new row in this index.** Do not inline entry content into KNOWLEDGE.md.
- **Two entries covering the same ground → merge, delete the loser.** Then update this index.
- **If you change an entry, confirm the routing-table "Read when..." still matches.** A stale trigger is worse than no trigger.
