---
description: Reset belmont state files (PRD, PROGRESS, TECH_PLAN) to start fresh
alwaysApply: false
---

# Belmont: Reset

You are resetting the belmont state directory so the user can start a new planning session from scratch.

## CRITICAL RULES

1. **NEVER** clear files without explicit user confirmation.
2. **ONLY** modify files in the `.belmont/` directory.
3. Do NOT touch `.agents/`, source code, or any other files.

## Step 1: Read Current State

Read the following files (if they exist) and collect a summary:

- `.belmont/PR_FAQ.md` — Check if it exists and has real content
- `.belmont/PRD.md` — Extract the product/feature name, check if it's a master feature catalog
- `.belmont/TECH_PLAN.md` — Check if it exists and has content (master tech plan)
- `.belmont/features/` — Scan for feature subdirectories. For each, read its PRD.md for name and PROGRESS.md for task counts.

Optional helper:
- If the CLI is available, `belmont status --format json` can provide a quick task/milestone summary. Still check for MILESTONE files, TECH_PLAN, PR_FAQ, and features/ existence.

If `.belmont/` does not exist or contains only empty templates and no features, tell the user there is nothing to reset and stop.

## Step 2: Present Options

Present a clear summary and options:

```
⚠️  Reset Belmont State
========================

Product: [product name from master PRD]
PR/FAQ:       [Has content / Empty]
Master PRD:   [Has content / Empty]
Master Tech:  [Exists / Does not exist]

Features:
  [slug]  [feature name] — [X] tasks ([Y] complete)
  [slug]  [feature name] — [X] tasks ([Y] complete)
  ...

Options:
  [1] Reset a specific feature (delete its directory contents, preserve masters)
  [2] Reset ALL features (clear all feature dirs, preserve masters)
  [3] Full reset (everything, including masters and PR_FAQ)
  [c] Cancel

⚠️  This cannot be undone.
```

**Wait for the user's response.** Do NOT proceed until you receive a reply.

## Step 3: Handle Response

**Option 1 — Reset specific feature:**
1. Ask which feature to reset (by slug or number)
2. Delete all files in `.belmont/features/<slug>/` (PRD.md, PROGRESS.md, TECH_PLAN.md, MILESTONE.md, MILESTONE-*.done.md)
3. Remove the feature directory
4. Update the master PRD features table to remove/mark the feature
5. Report what was reset

**Option 2 — Reset ALL features:**
1. Delete all subdirectories under `.belmont/features/`
2. Reset `.belmont/PRD.md` to the master template (keep product name, clear features table)
3. Delete `.belmont/TECH_PLAN.md` (master tech plan)
4. Delete any root-level MILESTONE files
5. Preserve `.belmont/PR_FAQ.md`
6. Report what was reset

**Option 3 — Full reset:**
1. Delete all subdirectories under `.belmont/features/`
2. Reset `.belmont/PR_FAQ.md` to template text: `"Run /belmont:working-backwards to create your PR/FAQ document.\n"`
3. Reset `.belmont/PRD.md` to template text: `"Run the /belmont:product-plan skill to create a plan for your feature.\n"`
4. Delete `.belmont/TECH_PLAN.md`
5. Delete `.belmont/MILESTONE.md` (if exists at root)
6. Delete all `.belmont/MILESTONE-*.done.md` (if any exist at root)
7. Report what was reset

**Option c — Cancel:**
Report: `Cancelled. No files were changed.`

## Important Rules

1. Always show the summary BEFORE asking for confirmation
2. Never proceed without an explicit confirmation from the user
3. Respect the user's choice of scope (single feature, all features, or full)
4. After clearing, prompt the user toward `/belmont:working-backwards` or `/belmont:product-plan`
