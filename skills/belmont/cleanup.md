---
description: "Reduce input token bloat by archiving completed features, removing stale milestone files, trimming notes, and auditing convention files (CLAUDE.md, AGENTS.md)."
alwaysApply: false
---

# Belmont: Cleanup

You are cleaning up accumulated Belmont state to reduce input token bloat. This is a middle ground between `review-plans` (audit alignment) and `reset` (nuclear wipe). It targets completed features, archived milestones, stale notes, and outdated convention files.

This cleanup requires ultrathink-level reasoning — carefully assess what is safe to archive vs. what the user still needs for active context.

## Purpose

Over time, Belmont projects accumulate completed feature directories (PRD, PROGRESS, TECH_PLAN, MILESTONE-*.done.md), growing NOTES.md files, and stale conventions in CLAUDE.md/AGENTS.md. Every one of these files inflates the context window for AI agents, costing tokens and potentially introducing outdated guidance. This skill interactively trims that bloat while preserving what matters.

## Critical Rules

1. **Interactive** — every item requires explicit user choice. Never bulk-act without consent. Present each feature, each file, each finding individually.
2. **Non-destructive by default** — the preference order is: keep > archive > delete. Default suggestion is always "keep" unless the user chooses otherwise.
3. **Scoped to state files** — modifies `.belmont/`, audits CLAUDE.md and AGENTS.md, suggests tool-dir cleanup. Does NOT modify source code.
4. **Skip gracefully** — if a category has nothing to clean, skip it silently.

## Forbidden Actions

- **DO NOT** modify source code files
- **DO NOT** run builds, tests, or compilation commands
- **DO NOT** delete files without explicit user confirmation per item
- **DO NOT** auto-modify tool directories (`.claude/`, `.codex/`, `.cursor/`, etc.) — only suggest
- **DO NOT** pressure the user toward archiving — completed features may still be useful context

## Allowed Actions

- **DO** read all `.belmont/` state files, CLAUDE.md, AGENTS.md, tool directory structures
- **DO** present cleanup options interactively via `AskUserQuestion`
- **DO** archive completed feature directories (compress verbose files into slim summaries)
- **DO** remove MILESTONE-*.done.md files based on user choice
- **DO** edit NOTES.md, CLAUDE.md, AGENTS.md based on user decisions
- **DO** commit `.belmont/` changes after cleanup

## Step 1: Scan & Inventory

Read all state files and build a cleanup profile:

1. Check `.belmont/` directory exists — if not, tell user to run `belmont install` first and stop
2. Read `.belmont/PRD.md` — extract features table, note each feature's status
3. Read `.belmont/PROGRESS.md` — cross-reference feature statuses
4. Scan `.belmont/features/` for each subdirectory:
   - Read `PROGRESS.md` — check for `## Status: ✅ Complete`
   - Count all files: PRD.md, TECH_PLAN.md, PROGRESS.md, NOTES.md, MILESTONE.md, MILESTONE-*.done.md
   - Estimate total size of the feature directory
   - Classify as: **Completed** (status ✅), **Active** (in progress), or **Not Started**
5. Find all `MILESTONE-*.done.md` files across `.belmont/` root and all feature directories
6. Read `.belmont/NOTES.md` — count entries/sections, note oldest entry date
7. Check for convention files at project root: CLAUDE.md, `.cursorrules`, `.windsurfrules`, AGENTS.md
8. Check which tool directories exist: `.claude/`, `.codex/`, `.cursor/`, `.windsurf/`, `.gemini/`, `.copilot/`

Present the inventory:

```
Belmont Cleanup Scan
====================

Features: X total (Y completed, Z active)
  Completed:
    [slug]  [name] — N files, ~X KB
    [slug]  [name] — N files, ~X KB
  Active:
    [slug]  [name] — N files (not eligible for archiving)

Archived Milestones: N MILESTONE-*.done.md files (~X KB total)
Global Notes:        M entries in NOTES.md (oldest: YYYY-MM-DD)
Convention Files:    CLAUDE.md [found/not found], AGENTS.md [found/not found]
Tool Directories:    .claude/ .codex/ .cursor/ ...

Cleanup Categories:
  [1] Full cleanup (walk through all categories below)
  [2] Pick categories interactively
  [c] Cancel
```

Wait for user response before proceeding.

If option 2, present categories:
```
Categories:
  [a] Archive completed features
  [m] Remove archived milestone files
  [n] Trim NOTES.md
  [d] Audit convention files (CLAUDE.md, AGENTS.md)
  [t] Check tool directory state
  [c] Cancel

Enter letters for categories to run (e.g., "amn"):
```

## Step 2: Archive Completed Features

**Skip if no completed features exist.**

For each feature where `## Status: ✅ Complete` in its PROGRESS.md, present it individually:

```
Feature: [feature-name] ([slug])
  Status:   ✅ Complete
  Files:    PRD.md, TECH_PLAN.md, PROGRESS.md, NOTES.md, N MILESTONE-*.done.md
  Size:     ~X KB total
  Summary:  [2-3 sentence summary extracted from PRD.md overview]

  Options:
    [a] Archive — replace all files with a slim ARCHIVE.md summary (~0.5 KB)
    [k] Keep   — leave untouched (still needed for context)
    [d] Delete — remove the entire feature directory
    [s] Skip   — decide later
```

Wait for user response for each feature.

### If user chooses Archive

1. Generate `.belmont/features/<slug>/ARCHIVE.md`:

```markdown
# Archive: <feature-name>

**Slug**: <slug>
**Completed**: <date from last session entry in PROGRESS.md, or "Unknown">
**Milestones**: <list milestone names, all complete>

## Summary
<2-3 sentence summary extracted from PRD.md overview section>

## Key Decisions
<Extract numbered decisions from PROGRESS.md Decisions Log section. If none, extract key points from NOTES.md. If neither, write "None recorded.">

## Key Files
<List key files/directories from TECH_PLAN.md file structure section. If no tech plan, write "See git history.">
```

2. Delete all other files in `.belmont/features/<slug>/`: PRD.md, TECH_PLAN.md, PROGRESS.md, NOTES.md, MILESTONE.md, all MILESTONE-*.done.md
3. Update the master `.belmont/PRD.md` features table — change the Status column for this feature to `📦 Archived`
4. Update the master `.belmont/PROGRESS.md` features table — change the Status column for this feature to `📦 Archived`

### If user chooses Delete

1. Remove the entire `.belmont/features/<slug>/` directory
2. Update master PRD features table — remove the row or mark as `🗑 Removed`
3. Update master PROGRESS features table — remove the row or mark as `🗑 Removed`

### If user chooses Keep or Skip

Move to the next feature. No changes.

## Step 3: Remove Archived Milestone Files

**Skip if no MILESTONE-*.done.md files exist.**

Find all `MILESTONE-*.done.md` files in:
- `.belmont/` root (legacy location)
- `.belmont/features/*/` (per-feature, only in non-archived features)

Present the full list:

```
Found N archived milestone files:

  .belmont/features/auth/MILESTONE-M1.done.md       (~X KB)
  .belmont/features/auth/MILESTONE-M2.done.md       (~X KB)
  .belmont/features/dashboard/MILESTONE-M1.done.md  (~X KB)
  ...

Total: ~X KB

These are completed milestone archives. They served as inter-agent
communication during implementation but have no active purpose now.

Options:
  [a] Remove all
  [p] Pick individually
  [s] Skip
```

For **pick** mode, present each file:
```
  .belmont/features/auth/MILESTONE-M1.done.md (~X KB)
  [r] Remove  [k] Keep  [s] Skip remaining
```

## Step 4: Trim NOTES.md

**Skip if no NOTES.md files exist (global or feature-level).**

Read `.belmont/NOTES.md` (global) and any feature-level `NOTES.md` in active (non-archived) features.

For each NOTES file with content, present entries grouped:

```
Global NOTES.md: N entries

  Entries for archived/deleted features:
    - [entry summary] (YYYY-MM-DD) — feature was archived
    - [entry summary] (YYYY-MM-DD) — feature was archived

  Entries older than 30 days:
    - [entry summary] (YYYY-MM-DD)
    - [entry summary] (YYYY-MM-DD)

  Recent entries (keeping):
    - [entry summary] (YYYY-MM-DD)

Options:
  [1] Remove entries for archived features (N entries, ~X KB)
  [2] Remove entries older than 30 days (N entries, ~X KB)
  [3] Remove both 1 and 2
  [4] Review each entry individually
  [s] Skip
```

For individual review, present each entry with [r]emove / [k]eep options.

## Step 5: Audit Convention Files

**Skip if no convention files exist.**

Check for convention files at the project root. These are agent-agnostic configuration files that AI tools read automatically:

- `CLAUDE.md` (Claude Code)
- `.cursorrules` (Cursor)
- `.windsurfrules` (Windsurf)
- `AGENTS.md` (Codex)

For each file that exists, perform the following audit:

### File Path References

Scan the file for paths that look like file/directory references (e.g., `src/components/`, `lib/utils.ts`). For each, verify the path exists using glob. Flag paths that don't resolve:

```
CLAUDE.md Audit — File References:

  Line 23: "Components live in src/components/ui/"
    → Path exists ✅

  Line 45: "Auth middleware is in src/middleware/auth.ts"
    → Path NOT found ⚠️
    [u] Update path (ask user for correct path)
    [r] Remove this line
    [s] Skip
```

### Stale Feature References

Look for references to features that were just archived or deleted in Step 2. If found:

```
  Line 67: "The dashboard feature uses the Chart component..."
    → Feature "dashboard" was archived in this cleanup
    [u] Update reference
    [r] Remove this section
    [s] Skip
```

### Outdated Conventions

Look for patterns that may be outdated:
- References to deprecated packages or APIs
- Build/test commands that may have changed
- Directory structure descriptions that don't match reality
- Conflicting or duplicate rules

Present each finding individually with context and options.

### AGENTS.md — Belmont Section

If AGENTS.md exists and contains a Belmont-managed section (between `<!-- belmont:codex-skill-routing:start -->` and `<!-- belmont:codex-skill-routing:end -->` markers):

1. Read the known skills list from the managed section
2. Compare against actual files in `.agents/skills/belmont/`
3. Flag any skills listed that don't have corresponding files, or files that aren't listed

```
AGENTS.md — Belmont skill routing section:

  Listed but missing file: "old-skill" → no .agents/skills/belmont/old-skill.md
  File exists but not listed: "cleanup" → .agents/skills/belmont/cleanup.md exists

  [u] Update section (suggest running `belmont install` to re-sync)
  [s] Skip
```

## Step 6: Agent Tool State

**Skip if no tool directories exist.**

Check each detected tool directory for stale Belmont artifacts:

### Copy-based tools (may have stale copies)

- **`.claude/commands/belmont/`** — compare file list against `.agents/skills/belmont/`. Flag files in `.claude/commands/belmont/` that don't exist in source.
- **`.codex/belmont/`** — same comparison.

### Symlink-based tools (may have broken links)

- **`.cursor/rules/belmont/`** — check each `.mdc` file is a valid symlink
- **`.windsurf/rules/belmont/`** — check symlink target exists
- **`.gemini/rules/belmont/`** — check symlink target exists
- **`.copilot/belmont/`** — check symlink target exists

Present findings as suggestions only:

```
Agent Tool State:

  .claude/commands/belmont/: 2 extra files not in source
    - old-skill.md
    - renamed-skill.md
  .codex/belmont/: in sync ✅
  .cursor/rules/belmont/: 1 broken symlink
    - removed-skill.mdc → (broken)

  Recommendation: Run `belmont install` to re-sync all tool integrations.

  [Press Enter to continue]
```

Do NOT modify tool directories — only inform and suggest `belmont install`.

## Step 7: Summary & Commit

Present a final summary:

```markdown
# Cleanup Summary

## Actions Taken
- Archived N features (saved ~X KB): [list slugs]
- Deleted N features: [list slugs]
- Removed N milestone files (~X KB)
- Trimmed NOTES.md (removed N entries, ~X KB)
- Updated CLAUDE.md (N changes)
- Updated AGENTS.md (N changes)

## Total Estimated Token Savings
~XX KB removed from agent context

## Kept (user chose to preserve)
- [list of features/items kept]

## Skipped
- [list of items skipped]
```

### Commit Changes

After completing all updates, commit modified files:

1. **Check if `.belmont/` is git-ignored** — run:
   ```bash
   git check-ignore -q .belmont/ 2>/dev/null
   ```
   If exit code is 0, `.belmont/` is ignored — skip this section entirely.

2. **Check for changes** — run:
   ```bash
   git status --porcelain .belmont/ CLAUDE.md AGENTS.md .cursorrules .windsurfrules 2>/dev/null
   ```
   If there is no output, nothing to commit — skip the rest.

3. **Stage and commit** — stage only modified state and convention files:
   ```bash
   git add .belmont/
   ```
   If CLAUDE.md, AGENTS.md, `.cursorrules`, or `.windsurfrules` were modified, stage those too:
   ```bash
   git add CLAUDE.md AGENTS.md .cursorrules .windsurfrules 2>/dev/null
   ```
   Then commit:
   ```bash
   git commit -m "belmont: cleanup completed features and stale state"
   ```

### Suggested Next Steps

```
Suggested next steps:
- /belmont:status to verify current project state
- belmont install to re-sync tool integrations (if stale files were detected)
- /belmont:review-plans to audit remaining active features
```

## Edge Cases

- **No completed features, no stale files**: Tell user "Nothing to clean up — project state is lean." and stop after the scan.
- **All features completed**: Still present each individually — user may want to keep some for reference.
- **No NOTES.md**: Skip Step 4.
- **No convention files**: Skip Step 5.
- **No tool directories**: Skip Step 6.
- **Feature has MILESTONE.md (not .done.md)**: This is an active milestone — do NOT touch it, even if the feature is marked complete. Warn the user there may be an in-progress implementation.
- **Running in a worktree**: Warn that cleanup should be run from the main worktree, not a feature worktree. Check for `.belmont/auto.json` — if it references active worktrees, warn accordingly.

## When to Use This Skill

- After completing a batch of features and starting new work
- When context windows feel bloated or agents are slow
- After `/belmont:review-plans` identifies many completed features
- Periodically during long-running projects
- Before onboarding new team members (clean slate)
