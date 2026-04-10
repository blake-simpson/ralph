---
description: "Review alignment between planning documents (PR/FAQ, PRDs, Tech Plans) and the codebase. Use when you want to detect drift, resolve conflicts, or ensure plans match reality."
alwaysApply: false
---

# Belmont: Review Plans

You are reviewing the alignment between planning documents and the codebase. This is an interactive audit that walks through each layer of the document hierarchy, finds discrepancies, and resolves them on the spot.

This review requires ultrathink-level reasoning — carefully trace dependencies across document layers and detect subtle drift between plans and implementation.

## Purpose

Belmont's planning workflow creates a layered document hierarchy (PR/FAQ → Master PRD/Tech Plan → Feature PRDs/Tech Plans → Tasks/Milestones). Over time, implementations deviate from plans, new features appear unplanned, and documents fall out of sync. This review closes that gap.

## Critical Rules

1. **Planning-only** — this skill modifies planning documents (PRDs, Tech Plans, PROGRESS, NOTES). It does NOT modify source code.
2. **Interactive** — every finding is presented to the user with resolution options. Never auto-resolve.
3. **Non-destructive** — edits to planning docs use Edit (preserve existing content, modify specific sections). Never overwrite entire files.
4. **Skip gracefully** — if a layer's source documents don't exist, skip that layer and note it in the summary.

## Forbidden Actions

- **DO NOT** modify any source code files
- **DO NOT** run builds, tests, or any compilation commands
- **DO NOT** create new feature directories — only suggest it as a follow-up
- **DO NOT** auto-resolve findings without user input
- **DO NOT** delete planning documents

## Allowed Actions

- **DO** read all `.belmont/` planning documents
- **DO** read the codebase structure (tree, glob, key files referenced in tech plans)
- **DO** present findings interactively via `AskUserQuestion`
- **DO** edit PRDs, Tech Plans, PROGRESS, and NOTES files based on user decisions
- **DO** create follow-up tasks in feature PRDs and PROGRESS files

## Step 1: Load All Planning Documents

Read all `.belmont/` files and build an inventory:

1. Check for `.belmont/` directory — if it doesn't exist, tell the user to run `belmont install` first and stop
2. Read `.belmont/PR_FAQ.md` — note if it exists and has content
3. Read `.belmont/PRD.md` — the master PRD (feature catalog). Extract the features table
4. Read `.belmont/TECH_PLAN.md` — the master tech plan
5. Scan `.belmont/features/` for subdirectories
6. For each feature directory, read:
   - `PRD.md` (feature requirements + tasks)
   - `TECH_PLAN.md` (feature technical plan)
   - `PROGRESS.md` (milestones + task tracking)
   - `NOTES.md` (if it exists)

If no features exist (no subdirectories under `.belmont/features/`), tell the user:

```
No features found. Run /belmont:product-plan to create your first feature.
```

Then stop.

Report what was found:

```
Loading planning documents...

  PR/FAQ:           ✅ Found / ⚠ Not found
  Master PRD:       ✅ Found (X features listed) / ⚠ Not found
  Master Tech Plan: ✅ Found / ⚠ Not found
  Features:         X directories found
    - <slug>: PRD ✅  Tech Plan ✅  Progress ✅  Notes ✅
    - <slug>: PRD ✅  Tech Plan ⚠  Progress ✅  Notes ⚠
    ...
```

## Step 2: Layer 1 Review — Strategic (PR/FAQ ↔ Masters)

**Skip this layer if PR/FAQ doesn't exist.** Note "Skipping Layer 1 — no PR/FAQ found" and move on.

Compare the PR/FAQ against the master PRD and master tech plan:

1. **PR/FAQ backlog vs master PRD features table**
   - Find features/items in PR/FAQ product backlog that are NOT in the master PRD features table → **Gap**
   - Find features in master PRD that are NOT mentioned in PR/FAQ → **Unplanned**

2. **Vision alignment**
   - Compare PR/FAQ problem statement and solution approach against master PRD overview → **Conflict** if they disagree

3. **Tech plan alignment**
   - Check if master tech plan's architecture supports the PR/FAQ's stated solution approach → **Drift** if misaligned

For each finding, present it to the user with resolution options (see Interactive Resolution below).

If no findings: report "Layer 1: PR/FAQ and master documents are aligned ✅" and move on.

## Step 3: Layer 2 Review — Feature Plans ↔ Masters

For each feature listed in the master PRD features table:

1. **Feature directory exists?**
   - If no directory under `.belmont/features/<slug>/` → **Gap** (feature planned but no detailed PRD)

2. **Feature PRD vs master PRD entry**
   - Compare feature PRD scope/description against master PRD entry for that feature → **Drift** if scope expanded beyond master description
   - Check status consistency — if master table says "Complete" but feature PROGRESS.md has `[ ]` or `[>]` tasks → **Conflict**

3. **Feature tech plan vs master tech plan**
   - Check if feature tech plan follows master tech plan's architecture decisions → **Drift** if it diverges
   - If feature has no tech plan → **Gap** (note it, ask if one should be created)

4. **Orphaned features**
   - Feature directories that exist under `.belmont/features/` but aren't listed in master PRD → **Unplanned**

For each finding, present interactively. If no findings: report "Layer 2: Feature plans align with master documents ✅"

## Step 4: Layer 3 Review — Tasks & Milestones ↔ Feature Plans

For each feature with both a PRD and PROGRESS file:

1. **Task consistency**
   - Tasks in PRD's task list but NOT in any PROGRESS milestone → **Gap** (task defined but not scheduled)
   - Tasks in PROGRESS milestones but NOT in PRD → **Unplanned** (orphaned tasks)

2. **Task definition alignment**
   - Verify each task in PROGRESS.md has a matching task definition (### heading) in PRD.md → **Gap** if missing
   - Task exists in PRD.md but not in any PROGRESS milestone → **Gap** (task defined but not scheduled)

3. **Stale status**
   - Tasks marked `[>]` (in progress) but no recent session history entries → **Stale** (potentially abandoned)
   - Tasks marked `[!]` (blocked) where the blocking condition may no longer apply → **Stale**

4. **Milestone dependencies**

   PROGRESS files may include dependency annotations on milestone headings: `### M3: Feature X (depends: M1)`. When any milestone has `(depends: ...)`, the file uses **explicit dependency mode** — `belmont auto` will run independent milestones in parallel via git worktrees. Validate:

   - **Dangling references** — `(depends: M5)` but M5 doesn't exist in the PROGRESS file → **Conflict**. Suggest removing the reference or adding the missing milestone.
   - **Circular dependencies** — M2 depends on M3 and M3 depends on M2 (direct or transitive cycles) → **Conflict**. List the cycle and suggest which dependency to remove based on the milestone descriptions and natural ordering.
   - **Over-constrained chains** — every milestone depends on the previous one in a strict serial chain (M1 → M2 → M3 → M4 → ...) when some could plausibly run in parallel → **Drift** (from optimal parallelism). Compare milestone descriptions: if two milestones touch independent areas (e.g., separate features, backend vs frontend, different pages), suggest removing the dependency between them to enable parallel execution.
   - **Under-constrained dependencies** — milestones that clearly share resources or build on each other's output but have no declared dependency → **Gap**. For example, if M3's tasks reference files or APIs created in M2 but M3 doesn't declare `(depends: M2)`, flag it. Suggest adding the dependency.
   - **Completed milestone still depended on** — a milestone whose tasks are all `[v]` that is listed as a dependency is fine (done milestones satisfy deps). But if a milestone has all tasks `[!]` (blocked/skipped) and other milestones depend on it → **Conflict**. The dependents can never proceed. Suggest either unblocking the dependency or removing it from dependents.
   - **Mixed mode inconsistency** — some milestones have `(depends: ...)` and others don't. Milestones without explicit deps go into wave 1 (run first). Flag if a milestone without deps clearly should depend on another milestone → **Gap**. Conversely, flag if the only milestone with deps is the final one and the rest would all run in parallel (likely under-specified).

   When suggesting fixes, provide the exact corrected milestone heading. For example:
   - Remove dangling: `### M3: Feature X` (drop the invalid dep)
   - Add missing: `### M3: Feature X (depends: M1, M2)` (add the dep)
   - Break serial chain: `### M3: Dashboard (depends: M1)` (change from M2 to M1 since M2 and M3 are independent)

For each finding, present interactively. If no findings: report "Layer 3: Tasks and milestones are consistent ✅"

## Step 5: Layer 4 Review — Codebase ↔ Plans

Read the codebase structure to compare against plans:

1. **Get codebase structure**
   - Use glob/read on key directories

2. **Tech plan file structure**
   - For each feature with a tech plan that specifies file paths or directory structure:
     - Check if those files/directories exist → **Gap** if planned files are missing
     - Check if implementations match what was planned → **Drift** if different

3. **Completed work vs pending tasks**
   - Look for implementations that suggest completed work for tasks still marked pending → **Stale**
   - Check key files referenced in tech plans for substantial implementation

4. **Unplanned code**
   - Look for significant code/patterns that don't correspond to any planned feature → **Unplanned**
   - Scope this to major directories and patterns — this is NOT a full code audit

For each finding, present interactively. If no findings: report "Layer 4: Codebase aligns with plans ✅"

## Interactive Resolution

For each finding, present it using `AskUserQuestion` with context and resolution options.

### Finding Format

Present each finding with:
- **Type**: Gap / Drift / Conflict / Stale / Unplanned
- **Layer**: Which layer it was found in
- **Details**: What the discrepancy is, with quotes from both sides
- **Location**: Which files are involved

### Resolution Options by Type

**Gap** (something planned but missing):
1. Create follow-up task — add to feature PRD as `P0-X-REVIEW: [description]` with source annotation, add to PROGRESS milestone
2. Update upstream document — modify the source document to remove the planned item
3. Skip — note as skipped in summary

**Drift** (something changed from plan):
1. Update plan to match reality — edit the planning document to reflect current state
2. Create task to realign code — add follow-up task to bring code back to plan
3. Add note as intentional — append to NOTES.md marking this as a deliberate deviation
4. Skip

**Conflict** (two documents disagree):
1. Keep document A's version — update doc B to match
2. Keep document B's version — update doc A to match
3. Rewrite both — update both documents with a new agreed-upon version (ask user what it should say)
4. Skip

**Stale** (information is outdated):
1. Update status to reflect reality — fix the status markers/completion state
2. Skip

**Unplanned** (something exists that was never planned):
1. Add to PRD as new task/feature — create an entry in the appropriate PRD
2. Add to notes as discovery — append to NOTES.md
3. Skip

### When Updating Documents

- **Creating tasks**: Add to feature PRD under Tasks section as `P0-X-REVIEW: [description]` with `**Source**: Review audit [date]`. Add the task to the appropriate PROGRESS milestone.
- **Updating PRDs/Tech Plans**: Use Edit to modify specific sections. Preserve all existing content not being changed.
- **Adding notes**: Append to the appropriate NOTES.md (feature-level or `.belmont/NOTES.md`). Use the date-headed format from the note skill.

## Step 6: Summary & Next Steps

After all findings are resolved (or skipped), output a summary:

```markdown
# Review Summary

## Documents Reviewed
- PR/FAQ: [yes/no/not found]
- Master PRD: [X features listed]
- Master Tech Plan: [yes/no/not found]
- Feature PRDs: [X/Y reviewed]
- Feature Tech Plans: [X/Y reviewed]

## Findings
- Gaps: X found, Y resolved
- Drift: X found, Y resolved
- Conflicts: X found, Y resolved
- Stale: X found, Y resolved
- Unplanned: X found, Y resolved

## Actions Taken
- [List of document modifications made]

## Skipped
- [List of findings the user chose to skip]
```

If zero findings across all layers:

```
All documents are aligned! No discrepancies found.

Suggested next steps:
- /belmont:implement to continue building
- /belmont:status for a progress overview
```

If actions were taken, suggest relevant follow-ups:

```
Suggested next steps:
- /belmont:product-plan to update PRDs with new scope
- /belmont:tech-plan to update tech plans
- /belmont:implement to address follow-up tasks
- /belmont:next to quickly fix individual review tasks
- /belmont:cleanup to archive completed features and reduce token bloat
```

### Validate State Consistency

Before committing, verify PROGRESS.md is internally consistent:

1. **Task ↔ definition sync** — For each task in PROGRESS.md milestone sections:
   - Verify a matching `### P...:` task definition exists in PRD.md → flag missing definitions
   - PRD.md has NO status markers — it is a pure spec document

2. **State validity** — Check that task states use valid markers: `[ ]`, `[>]`, `[x]`, `[v]`, `[!]`
   - Flag any tasks with old-style markers (emoji, `[DONE]`, etc.)

3. **Milestone consistency** — Milestone status is computed from tasks, not stored:
   - Milestone headers should NOT have emoji status markers (✅/⬜)
   - If old-format headers are found, remove the emoji prefix

Only fix actual issues — if files are already consistent, make no changes.

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
   git add .belmont/ && git commit -m "belmont: update planning files after review"
   ```

## Edge Cases

- **No PR/FAQ**: Skip Layer 1, start at Layer 2
- **No master tech plan**: Skip tech plan comparisons in Layers 1-2, still check feature PRDs and Layer 3-4
- **No features**: Tell user to run `/belmont:product-plan` first, stop
- **Feature has PRD but no tech plan**: Note as a Gap finding, ask if one should be created
- **Feature has no PROGRESS file**: Note as a Gap, skip Layer 3 for that feature
- **Zero findings in a layer**: Skip that layer's interactive section, note "aligned" in summary
- **Large number of findings**: Process them layer by layer. Do not overwhelm the user — group related findings where possible

## When to Use This Skill

- After implementation sessions to check for drift
- Before major milestones to ensure plans are still accurate
- Periodically during active development
- After significant codebase changes
- When onboarding to understand document state
