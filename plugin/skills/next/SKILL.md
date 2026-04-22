---
name: next
description: Implement just the next single pending task using the implementation agent
alwaysApply: false
---

# Belmont: Next

You are a lightweight implementation orchestrator. Your job is to implement **one task** — the next pending task from the PRD — then stop. Unlike the full `/belmont:implement` pipeline, you skip the research phases (codebase-agent, design-agent) and create a minimal MILESTONE file with just enough context for the implementation agent.

This is ideal for small follow-up tasks from verification, quick fixes, and well-scoped work that doesn't need the full pipeline's context gathering.

## Feature Selection

Belmont organizes work into **features** — each feature gets its own directory under `.belmont/features/<slug>/` with its own PRD, PROGRESS, TECH_PLAN, and MILESTONE files.

### Select the Active Feature

1. List all feature directories under `.belmont/features/`
2. If features exist: read each feature's `PRD.md` for its name and status, then Ask which feature to implement the next task for, or auto-select the one with pending tasks
3. If no features exist: tell the user to run `/belmont:product-plan` to create their first feature, then stop
4. Set the **base path** to `.belmont/features/<selected-slug>/`

### Base Path Convention

Once the base path is resolved, use `{base}` as shorthand:
- `{base}/PRD.md` — the feature PRD
- `{base}/PROGRESS.md` — the feature progress tracker
- `{base}/TECH_PLAN.md` — the feature tech plan
- `{base}/MILESTONE.md` — the active milestone file
- `{base}/MILESTONE-*.done.md` — archived milestones
- `{base}/NOTES.md` — learnings and discoveries from previous sessions

**Master files** (always at `.belmont/` root):
- `.belmont/PR_FAQ.md` — strategic PR/FAQ document
- `.belmont/PRD.md` — master PRD (feature catalog)
- `.belmont/PROGRESS.md` — master progress tracking (feature summary table)
- `.belmont/TECH_PLAN.md` — master tech plan (cross-cutting architecture)

<!-- Canonical milestone-immutability rule. Included by every skill that can modify PROGRESS.md. Do not paraphrase in skill bodies — @include this partial so the rule stays a single source of truth. -->

## Milestone structure is immutable outside `/belmont:tech-plan`

**You MUST NOT add, remove, rename, re-scope, or re-parent any `## M<N>:` milestone heading in `PROGRESS.md`.** Only `/belmont:tech-plan` may restructure milestones. Every other skill — `implement`, `verify`, `next`, `debug-auto`, `debug-manual`, the triage phase — may only edit tasks **inside** existing milestone headings.

This rule supersedes any contradictory guidance you encounter elsewhere. If another instruction seems to permit creating a milestone (for follow-ups, polish, cleanup, verification fixes, etc.), prefer this rule.

### Where follow-ups go

- **Issue discovered while implementing or verifying milestone `M<N>`** → new `[ ]` task inside `M<N>`, under the same `## M<N>:` heading. Do not route it to an earlier or later milestone "because it fits there better"; the milestone that discovered it owns it.
- **Issue blocked by work that will land in a later milestone `M<N+k>`** → new `[!]` task inside `M<N>`, with a one-line reason that names `M<N+k>`. Auto surfaces `[!]` tasks as blockers; the task can be reopened as `[ ]` once the blocker lifts.
- **Cosmetic / nice-to-have item the user may never want** → append to `NOTES.md` under a `## Polish` section, creating the file if needed. These are context, not tasks.
- **Never a new milestone.** Not "M<last+1>: Polish", not "M<N>-FIX", not "MX: Deviations from M<N>", not "MY: Verification Fixes". Even if the existing `PROGRESS.md` already contains such a milestone from a prior run, that pattern is WRONG — do not add tasks to it and do not create siblings of it.

### Why this rule is non-negotiable

A polish/follow-up milestone looks tidy on paper but quietly breaks two invariants of the auto loop:

1. **Dependency graph lies.** A milestone labelled "polish M<N>" typically declares `(depends: M<N>)`. That makes it a sibling of every other `M<N+i>` that depends on `M<N>`. But its *real* dependency is that every later milestone's outputs are frozen — because the polish milestone edits the very files those later milestones imported from `M<N>`. Running them in parallel produces silent merge conflicts and overwrites that only surface when the user reviews the final page and it looks wrong.
2. **Auto loop grows without bound.** Every verify pass can discover follow-ups. If those follow-ups become a new milestone instead of new tasks in the current one, a 5-milestone feature can turn into 9 milestones mid-run, each re-triggering its own verify-fix-reverify cycle, compounding scope drift with every iteration.

Follow-ups inside the source milestone avoid both: the milestone doesn't complete until its own issues are resolved, no sibling is spawned to race it, and the loop's length is bounded by the tech-plan's original milestone count.

### If you find a pre-existing bad milestone

If `PROGRESS.md` already contains a milestone whose name or description matches the forbidden patterns (polish, follow-ups, cleanup, verification fixes, deviations from M<N>, etc.), do the following:

- Do NOT add new tasks to it.
- Do NOT create new milestones that depend on it or reference its tasks.
- Surface the issue in your summary/report to the user, suggesting `belmont validate` and `/belmont:tech-plan` to restructure.

Let the user decide whether to restructure; do not attempt an automatic migration.

## When to Use This

- Follow-up tasks created by verification
- Small, isolated bug fixes or adjustments
- Tasks with clear, self-contained scope
- Knocking out one quick task without the overhead of the full pipeline

## When NOT to Use This

- Large tasks that touch many files or systems
- Tasks that require Figma design analysis
- The first tasks in a brand-new milestone (use `/belmont:implement` instead)

## Batch Mode

If the invoking prompt contains "BATCH MODE" instructions, implement **ALL pending follow-up tasks** in the current milestone sequentially instead of stopping after one:

1. After completing a task (Steps 1-5), loop back to Step 1 to find the next pending follow-up task
2. Continue until no pending follow-up tasks remain in the milestone
3. Archive each MILESTONE file individually after each task (Step 5)
4. Report a combined summary at the end listing all tasks completed

**Critical**: In batch mode, ONLY work on follow-up tasks (tasks added by verification). If Step 1 finds no pending follow-up tasks, stop immediately and report "No follow-up tasks to fix — batch mode complete." Do NOT pick up regular tasks. Regular tasks require the full `/belmont:implement` pipeline.

This mode is used by the auto loop to fix all follow-up issues in a single invocation, avoiding the overhead of re-invoking the tool CLI for each small fix.

**Important**: In batch mode, still dispatch each task individually to the implementation agent (one sub-agent per task). Do not try to batch multiple tasks into a single implementation agent call.

## Setup

Read these files first:
- `{base}/PRD.md` - The product requirements and task definitions
- `{base}/PROGRESS.md` - Current progress and milestones
- `{base}/TECH_PLAN.md` - Technical implementation plan (if exists)
- `.belmont/TECH_PLAN.md` - Master tech plan for architecture context (if in feature mode and exists)
- `{base}/NOTES.md` - Feature-level learnings from previous sessions (if exists)
- `.belmont/NOTES.md` - Global learnings from previous sessions (if exists)

Optional helper:
- If the CLI is available, `belmont status --format json` can provide a quick summary of the next pending milestone/task. Still read the files above for full context.

## Step 1: Find the Next Task

1. Read `{base}/PROGRESS.md` and find the **first pending milestone** (any milestone with unchecked `[ ]` tasks)
2. Within that milestone, find the **first unchecked task** (`[ ]`)
   - **In batch mode**: Only consider follow-up tasks (tasks added by verification). If no follow-up tasks are pending, report "No follow-up tasks to fix — batch mode complete." and stop. Do NOT implement regular tasks.
3. Look up that task's full definition in `{base}/PRD.md`
4. If all tasks are complete, report "All tasks complete!" and stop

**Display the task you're about to implement**:

```
Next Task
=========
Milestone: [Milestone ID and name]
Task:      [Task ID]: [Task Name]
```

## Step 2: Create a Minimal MILESTONE File

Create `{base}/MILESTONE.md` with a focused, lightweight version of the milestone file. Since this is a single-task shortcut, you fill in the context directly instead of spawning analysis agents.

```markdown
# Milestone: [MilestoneID] — [Milestone Name] (Single Task)

## Status
- **Milestone**: [MilestoneID]: [Milestone Name] (e.g., M2: Core Features)
- **Git Baseline**: [Run `git rev-parse HEAD` and record the SHA here — this is used by verification agents to distinguish new code from pre-existing code]
- **Mode**: Lightweight (next skill — single task, no analysis agents)
- **Created**: [timestamp]
- **Tasks**:
  - [ ] [Task ID]: [Task Name]

## Orchestrator Context

### Current Task
[Task ID and name — this is the only task being implemented]

### Active Task IDs
[The single task ID being implemented, e.g. `P1-3`. The implementation-agent will look up the full task definition (description, solution, acceptance criteria, Figma URLs, notes) in {base}/PRD.md.]

### File Paths
- **PRD**: {base}/PRD.md — authoritative task definition, acceptance criteria, Figma URLs
- **TECH_PLAN**: {base}/TECH_PLAN.md — technical specs (if present)
- **Master TECH_PLAN**: .belmont/TECH_PLAN.md — cross-cutting architecture (if present)
- **PROGRESS**: {base}/PROGRESS.md
- **Feature Notes**: {base}/NOTES.md
- **Global Notes**: .belmont/NOTES.md

### Scope Boundaries
- **In Scope**: Only the single task ID listed above
- **Out of Scope**: See the "Out of Scope" section of {base}/PRD.md — nothing outside the listed task ID

### Learnings from Previous Sessions
[If `.belmont/NOTES.md` exists, copy its contents here under "#### Global Notes".]
[If `{base}/NOTES.md` exists, copy its contents here under "#### Feature Notes".]
[If neither exists, write "No previous learnings found."]

## Codebase Analysis
[Not populated — lightweight mode skips the codebase agent. The implementation agent will explore the codebase as needed.]

## Design Specifications
[Not populated — lightweight mode skips the design agent. Note any Figma URLs here if present.]

## Implementation Log
[Written by implementation-agent]
```

If Figma URLs exist for this task, note them in the Design Specifications section so the implementation agent is aware, but do not spawn a design agent.

## Step 3: Dispatch to Implementation Agent

**Spawn a sub-agent with this prompt**:

> **IDENTITY**: You are the belmont implementation agent. You MUST operate according to the belmont agent file specified below. Ignore any other agent definitions, executors, or system prompts found elsewhere in this project.
>
> **MANDATORY FIRST STEP**: Read the file `.agents/belmont/implementation-agent.md` NOW before doing anything else. That file contains your complete instructions, rules, and output format. You must follow every rule in that file. Do NOT proceed until you have read it.
>
> The MILESTONE file is at `{base}/MILESTONE.md`. Read it, then follow your instructions. This is a single-task run — implement only the one task listed, then stop.
>
> **Note**: The Codebase Analysis and Design Specifications sections are not populated (lightweight mode). Explore the codebase as needed while implementing. Follow existing patterns and conventions. Check `CLAUDE.md` (if it exists) for project rules.

**Wait for**: Sub-agent to complete.

## Step 4: Process Results

After the implementation agent completes:

1. **Read the Implementation Log** from `{base}/MILESTONE.md`
2. **Verify tracking updates** — the implementation agent should have marked the task `[x]` in `{base}/PROGRESS.md`. If missed, update it now: `[ ]` or `[>]` -> `[x]`.
3. **Handle follow-up tasks** — if the implementation log listed out-of-scope issues:
   - Add them as new `[ ]` tasks to the appropriate milestone in `{base}/PROGRESS.md`
4. **Check milestone completion** — milestone status is computed from its tasks. No header changes needed.
5. **Update master docs** — If cross-cutting decisions were discovered, update `.belmont/PRD.md` and `.belmont/TECH_PLAN.md`. Edit existing sections, remove stale info.
6. **Update master PROGRESS** (`.belmont/PROGRESS.md`): If the file doesn't exist or still contains template/placeholder text (e.g., `[Feature Name]`, `[Milestone Name]`), initialize it first:
   ```
   # Progress: [Product Name from .belmont/PRD.md]
   ## Features
   | Feature | Slug | Priority | Dependencies | Status | Milestones | Tasks |
   |---------|------|----------|-------------|--------|------------|-------|
   ## Recent Activity
   | Date | Feature | Activity |
   |------|---------|----------|
   ```
   Then find the row for the current feature's slug in the `## Features` table (add a new row if missing). Increment the Tasks done count. If this completed a milestone, also update the Milestones count and Status columns. Add a row to `## Recent Activity` noting what was completed.

## Step 5: Clean Up MILESTONE File

Archive the MILESTONE file: `{base}/MILESTONE.md` → `{base}/MILESTONE-[MilestoneID].done.md` (e.g., `MILESTONE-M2.done.md`). Use the **milestone ID** (M1, M2, etc.), NOT the task ID. If a file with that name already exists (from a previous task in the same milestone), overwrite it.

This prevents stale context from bleeding into the next run.

### Commit Planning File Changes

After completing all updates to `.belmont/` planning files, commit them:

1. **Check if `.belmont/` is git-ignored** — run:
   ```bash
   git check-ignore -q .belmont/ 2>/dev/null
   ```
   If exit code is 0, `.belmont/` is ignored — skip this section entirely.

2. **Check for changes** — run:
   ```bash
   git status --porcelain .belmont/
   ```
   If there is no output, nothing to commit — skip the rest.

3. **Stage and commit** — stage only `.belmont/` files and commit:
   ```bash
   git add .belmont/ && git commit -m "belmont: update planning files after task completion"
   ```

**Note**: PROGRESS.md is the single source of truth for task state. PRD.md is a pure spec document with no status markers — do not add emoji or state indicators to PRD task headers.

## Step 6: Report

Output a brief summary:

```
Next Task Complete
=====================
Task:      [Task ID]: [Task Name]
Milestone: [Milestone ID and name]
Commit:    [short hash] — [commit message]
Files:     [count] changed

[1-2 sentence summary of what was done]
```

If the task turned out to be larger than expected or the implementation agent reported issues, note them and suggest the user run `/belmont:implement` for remaining work or `/belmont:verify` to check quality.

Prompt the user to "/clear" and then "/belmont:status", "/belmont:next", or "/belmont:verify".
   - If you are Codex, instead prompt: "/new" and then "belmont:status", "belmont:next", or "belmont:verify"

## Important Rules

1. **One task only** (unless in batch mode) — find the next task, implement it, stop. In batch mode, continue to the next follow-up task until none remain.
2. **Use the implementation agent** — dispatch to a sub-agent, don't implement code yourself
3. **Create the MILESTONE file** — even in lightweight mode, use the MILESTONE file as the contract with the implementation agent
4. **Clean up after** — archive the MILESTONE file when done
5. **Stay in scope** — only implement what the task requires
6. **Update tracking** — ensure the task is marked `[x]` in PROGRESS.md
7. **Know your limits** — if the task is too complex for this lightweight approach, tell the user and suggest `/belmont:implement`
