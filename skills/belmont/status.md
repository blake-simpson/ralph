---
description: Show current status of belmont tasks and milestones
alwaysApply: false
---

# Belmont: Status

Read the current project state and produce a formatted status report.

## Fast Path (Preferred)

If available, use the global CLI first:

```bash
belmont status
```

If the command fails, fall back to the manual steps below.

## Modes

### Feature Listing Mode (default)

When no specific feature is requested:

1. Read `.belmont/PR_FAQ.md` — check if it exists and has content
2. Read `.belmont/PRD.md` — the master PRD (feature catalog)
3. Scan `.belmont/features/` for subdirectories
4. For each feature directory, read its `PRD.md` for the feature name and `PROGRESS.md` for task counts
5. Produce a **feature listing report** (see format below)

If no features exist yet, tell the user to run `/belmont:product-plan` to create their first feature.

### Single Feature Mode

If a specific feature is requested (user says "show status for auth" or similar):

1. Set base path to `.belmont/features/<slug>/`
2. Read `{base}/PRD.md`, `{base}/PROGRESS.md`, check `{base}/TECH_PLAN.md`, check `{base}/NOTES.md` and `.belmont/NOTES.md`
3. Produce the standard status report (see format below)

## Files to Read

1. `{base}/PRD.md` - Task definitions and completion status
2. `{base}/PROGRESS.md` - Milestones and session history
3. `{base}/TECH_PLAN.md` - Check if it exists and has content
4. `{base}/NOTES.md` - Check if feature-level notes exist
5. `.belmont/NOTES.md` - Check if global notes exist

If `.belmont/` directory doesn't exist, tell the user to run `belmont install` first.

## Feature Listing Report Format

When in feature listing mode:

```
Belmont Status
==============

Product: [Extract from master PRD title, or "Unnamed Product"]

PR/FAQ: [Written / Not written (run /belmont:working-backwards)]
Master Tech Plan: [Ready / Not written]

Features:
  [status] [slug]  [feature name]  [X/Y tasks done]
  [status] [slug]  [feature name]  [X/Y tasks done]
  ...

Use /belmont:status with a feature name for details.
Use /belmont:product-plan to add a new feature.

Legend: [v] verified  [x] done  [>] in progress  [!] blocked  [ ] todo
```

Feature status indicators:
- [v] All tasks verified
- [x] All tasks complete
- [>] In progress
- [ ] Not started

## Standard Status Report Format

Produce a report following this exact format:

```
Belmont Status
==============

Feature: [Extract from PRD title]

Tech Plan: [Ready / Not written (run /belmont:tech-plan to create)]
Notes:     [Has notes / -- None]

Status: [Not Started | In Progress | Complete | Verified]

Tasks: X verified, Y done, Z in progress, W blocked, V pending (of N total)

  [v] P0-1: [Task name]
  [v] P0-2: [Task name]
  [x] P1-1: [Task name]
  [>] P1-2: [Task name]
  [!] P1-3: [Task name]
  [ ] P2-1: [Task name]
  [ ] P2-2: [Task name]

Milestones: (status computed from tasks)
  [v] M1: [Milestone name]       (all tasks verified)
  [>] M2: [Milestone name]       (3/5 tasks done)
  [ ] M3: [Milestone name]       (not started)

Blocked Tasks:
  - [!] P1-3: [Task name] — [reason if noted]

Next Milestone:
  - [Milestone ID] - [Milestone name]
Next Individual Task:
  - [Task ID] - [Task name]

Recent Activity:
---
Last completed: [Task ID] - [Task name]
Recent decisions:
  - [Last 3 decisions from Decisions Log]

Legend: [v] verified  [x] done  [>] in progress  [!] blocked  [ ] todo
```

## How to Determine Status

### Task Status (from PROGRESS.md checkboxes)
- **Verified [v]**: `[v]` in PROGRESS.md
- **Done [x]**: `[x]` in PROGRESS.md (implemented, not yet verified)
- **In Progress [>]**: `[>]` in PROGRESS.md
- **Blocked [!]**: `[!]` in PROGRESS.md
- **Todo [ ]**: `[ ]` in PROGRESS.md

PROGRESS.md is the single source of truth for task state. PRD.md is a pure spec with no status markers.

### Overall Status (computed from tasks)
- **Not Started**: All tasks are `[ ]`
- **In Progress**: Mix of states
- **Complete**: All tasks are `[x]` or `[v]`
- **Verified**: All tasks are `[v]`

### Milestone Status (computed from tasks)
Milestone status is computed from its tasks — no markers on milestone headers. A milestone is verified when all its tasks are `[v]`.

### Task Priority Order
- Tasks are sorted by priority: P0 first, then P1, P2, P3
- Within same priority, by task number

## Rules

- **DO NOT** modify any files - this is read-only
- **DO NOT** run `git status` or otherwise inspect git. Belmont status is independent of git.
- **DO NOT** scan the codebase. Just use the progress + PRD files for info.
- **DO** read relevant files (PRD for task definitions, PROGRESS for task state)
- **DO** show all tasks with their current status from PROGRESS.md
- **DO** show milestones with computed status from their tasks
- **DO** show blocked tasks (marked [!]) if any exist
- **DO** show recent decisions from the Decisions Log
- **DO** truncate long task names (max ~55 characters)
