---
description: Run verification and code review on completed tasks
alwaysApply: false
---

# Belmont: Verify

You are the verification orchestrator. Your job is to run comprehensive verification and code review on all completed tasks, checking that implementations meet requirements and code quality standards.

<!-- @include feature-detection.md feature_action="Ask which feature to verify, or auto-select the one with completed tasks" -->

<!-- @include worktree-awareness.md -->

## Setup

Read these files first:
- `{base}/PRD.md` - The product requirements and task definitions
- `{base}/PROGRESS.md` - Current progress tracking
- `{base}/TECH_PLAN.md` - Technical implementation plan (if exists)
- `.belmont/TECH_PLAN.md` - Master tech plan for architecture context (if in feature mode and exists)
- `{base}/models.yaml` - Per-feature model tiers (if exists — see "Model Tiers" below)

Also check for archived MILESTONE files (`{base}/MILESTONE-*.done.md`) — these contain the implementation context from the most recent milestone and can provide useful reference for verification.

Optional helper:
- If the CLI is available, `belmont status --format json` can provide a quick summary of completed tasks. Still read the files above for full context.

## Model Tiers

Per-agent model tiers (low/medium/high) are defined in `{base}/models.yaml`. If that file is absent, each agent uses its frontmatter default (Sonnet for most) and you can skip the rest of this section.

<!-- @include tier-registry.md -->

<!-- @include tier-preflight.md -->

When dispatching the verification-agent and code-review-agent below, apply the tier overrides per `dispatch-strategy.md → Model Tier Overrides`. Specifically: if `models.yaml` lists `tiers.verification` or `tiers.code-review`, include `model: "<alias>"` in the corresponding Task call using the tier-registry mapping. Agents not listed inherit their frontmatter default — do NOT pass `model:` for those.

## Focused Re-verification Mode

If the invoking prompt contains "FOCUSED RE-VERIFICATION" or similar instructions indicating this is a re-verify after follow-up fixes:

1. **Still run both agents** (verification + code review) to catch regressions
2. **Scope the verification to**:
   - The specific follow-up tasks that were just fixed (check recently completed tasks)
   - Build and test verification (always run fully)
   - Any previously-failing acceptance criteria
3. **Do NOT** re-run Lighthouse audit unless a follow-up task specifically addressed performance
4. **Do NOT** re-check visual specs against design references unless a follow-up task specifically addressed UI changes. Still include the Visual Comparison Attestation in the report, noting that comparison was skipped per focused re-verification scope.
5. **Do NOT** create new Polish-level issues — only report Critical and Warning issues found during focused verification
6. **Include the scoping instructions** when dispatching to the sub-agents so they also focus their review

This mode reduces token waste by avoiding full re-audits when only small fixes were made.

## Step 1: Identify Completed Tasks

1. Read `{base}/PROGRESS.md` and find all tasks marked with `[x]` (done, not yet verified)
2. These are the tasks that need verification
3. If no tasks are marked `[x]`, report "No completed tasks to verify" and stop

## Step 1b: Gather Design References

Before spawning sub-agents, collect design references for the tasks being verified:

1. Read archived MILESTONE files (`{base}/MILESTONE-*.done.md`) — look for:
   - `## Design Specifications` section with a Figma Sources table (has `fileKey`, `nodeId` columns)
   - Embedded or linked reference images, screenshots, or mockups
2. Check `{base}/PRD.md` task definitions for `**Figma**:` fields or linked visual references
3. Check `{base}/TECH_PLAN.md` and `{base}/NOTES.md` for any visual specifications

Collect whatever you find — Figma `fileKey`/`nodeId` pairs, image paths, URLs. You will pass these to the verification agent in Step 2.

## Sub-Agent Dispatch Strategy

Apply the following dispatch configuration:
- **Team name**: `belmont-verify`
- **Parallel agents**: verification-agent + code-review-agent — spawn simultaneously
- **Sequential agents**: None
- **Cleanup timing**: After Step 3 completes

<!-- @include dispatch-strategy.md -->

## Step 2: Run Verification and Code Review

Use the dispatch method you selected above. For the **Agent Teams** method (Approach A), create the team first, then issue both `Task` calls in the same message. For the **Parallel Task** method (Approach B), issue both `Task` calls in the same message. For the **Sequential Inline** fallback (Approach C), execute each agent's instructions inline, finishing one completely before starting the next.

Spawn these two sub-agents **simultaneously** (or sequentially if using the Sequential Inline fallback):

---

### Agent 1: Verification (verification-agent)

**Purpose**: Verify task implementations meet all requirements.

**Spawn a sub-agent with this prompt**:

<!-- @include identity-preamble.md agent_role="verification" agent_file="verification-agent.md" -->
>
> Verify the following completed tasks:
>
> ---
> [List each completed task ID and header, e.g.:
> - P0-1: Set up authentication [x]
> - P0-2: Database schema [x]]
> ---
>
> Read `{base}/PRD.md` for acceptance criteria and task details.
> Read `{base}/TECH_PLAN.md` for technical specifications (if it exists).
> Check for archived MILESTONE files (`{base}/MILESTONE-*.done.md`) for implementation context.
>
> Check acceptance criteria, visual design comparison, i18n keys, and functional testing.
>
> **Design References for Visual Verification**:
> [List whatever you found in Step 1b. For each task with references, list them:
> - Task [ID]: Figma fileKey=`xxx`, nodeId=`yyy`
> - Task [ID]: Reference screenshot at [path or URL]
> - Task [ID]: No visual reference found
> If no MILESTONE files or references were found, write: "No design references found in archived MILESTONE files or PRD."]
>
> **Visual Verification**: For any task with visual output, you MUST use Playwright MCP to take screenshots and verify the implementation. If design references are listed above, you MUST load them — call `mcp__plugin_figma_figma__get_screenshot` for Figma references, Read for local images, WebFetch for URLs — and perform structured side-by-side comparison (layout, spacing, typography, colors, component shapes, alignment). Include the Visual Comparison Attestation in your report. Do NOT silently skip available design references.
>
> Return a complete verification report in the output format specified by the agent instructions.

**Collect**: The verification report document.

---

### Agent 2: Code Review (code-review-agent)

**Purpose**: Review code changes for quality and PRD alignment.

**Spawn a sub-agent with this prompt**:

<!-- @include identity-preamble.md agent_role="code review" agent_file="code-review-agent.md" -->
>
> Review the code changes for the following completed tasks:
>
> ---
> [List each completed task ID and header, e.g.:
> - P0-1: Set up authentication [x]
> - P0-2: Database schema [x]]
> ---
>
> Read `{base}/PRD.md` for task details and planned solution.
> Read `{base}/TECH_PLAN.md` for technical specifications (if it exists).
> Check for archived MILESTONE files (`{base}/MILESTONE-*.done.md`) for implementation context.
>
> Detect the project's package manager (check for `pnpm-lock.yaml`, `yarn.lock`, `bun.lockb`/`bun.lock`, or `package-lock.json`; also check the `packageManager` field in `package.json`). Use the detected package manager to run build and test commands (e.g. `pnpm run build`, `yarn run build`, etc. — default to `npm` if unsure). Review code quality, pattern adherence, and PRD alignment.
>
> Return a complete code review report in the output format specified by the agent instructions.

**Collect**: The code review report document.

---

## Step 3: Process Results

After both agents complete:

### Combine Reports
1. Merge the verification report and code review report
2. Categorize all issues found into **four tiers**:
   - **Critical** — Must fix (broken functionality, security, failing tests, visual design mismatches)
   - **Warning** — Should fix (missing error handling, pattern violations, missing tests, i18n gaps)
   - **Polish** — Minor improvements that do NOT affect functionality (aria-labels, code style, docs, minor a11y notes, small spacing tweaks). These do NOT block the milestone.
   - **Suggestions** — Informational only (refactoring ideas, alternative approaches). Not tracked.

### Create Follow-up Tasks

> **FOLLOW-UP PLACEMENT RULE — READ THIS BEFORE MODIFYING PROGRESS.md:**
>
> Follow-up tasks go into their **source milestone** (the milestone where the issue was found). You MUST NOT create new milestones. Even if existing PROGRESS.md shows a pattern of follow-up milestones (e.g., "M19: Follow-ups"), that pattern is WRONG — do not replicate it. Insert follow-ups directly into the original milestone as new `[ ]` tasks.

**Scope violation safeguard**: For scope violation issues specifically, only create "revert" follow-up tasks for code that was **newly added by the current task**. If the scope violation involves pre-existing code from other features or milestones, do NOT create a follow-up task to delete it — instead note it in the summary as "pre-existing code outside current scope, no action needed." Deleting pre-existing features is catastrophic and must be prevented.

If **all tasks pass verification** (no Critical or Warning issues):
1. Mark each verified task as `[v]` in `{base}/PROGRESS.md` (change `[x]` to `[v]`)

If **Critical or Warning** issues were found by either agent:
1. For tasks that passed: mark as `[v]` in `{base}/PROGRESS.md`
2. For tasks with issues: leave as `[x]` and add new `[ ]` follow-up tasks to the same milestone. These are plain tasks, not specially tagged:
   ```
   - [ ] P1-M17-FIX-1: [Issue Description]
   ```
3. Add follow-up tasks to `{base}/PROGRESS.md`. **Placement rules (mandatory, no exceptions):**
   - Determine which milestone each issue belongs to based on the tasks/code that were verified
   - Insert each follow-up task under its **source milestone** as a new `[ ]` task
   - When verifying multiple milestones (e.g., M17+M18+M19), distribute follow-ups to their respective milestones — do NOT group them together
   - **DO NOT create any new milestone headings** — no "M20: Follow-ups", no "MX: Verification Fixes", no "MX: Design Fidelity Fixes". This is forbidden because it causes automated loop controllers to enter infinite cycles
   - If the source milestone is truly ambiguous, add to the last milestone that has pending tasks
   - Follow-up tasks MUST live inside a milestone heading — never in a freestanding section outside the milestones structure
4. **Update master PROGRESS** (`.belmont/PROGRESS.md`): If the file doesn't exist or still contains template/placeholder text (e.g., `[Feature Name]`, `[Milestone Name]`), initialize it first:
   ```
   # Progress: [Product Name from .belmont/PRD.md]
   ## Features
   | Feature | Slug | Priority | Dependencies | Status | Milestones | Tasks |
   |---------|------|----------|-------------|--------|------------|-------|
   ## Recent Activity
   | Date | Feature | Activity |
   |------|---------|----------|
   ```
   Then if follow-up tasks were added, update the Tasks total in the `## Features` table for this feature's row (add a new row if missing). Add a row to `## Recent Activity` noting verification results.

### Record Polish Items

If any **Polish** items were reported by either agent, append them to `{base}/NOTES.md` under a `## Polish` section. Create the file if it doesn't exist. Format:

```markdown
## Polish

### From verification [date]
- [Polish item description] — [file:line if applicable]
- [Polish item description] — [file:line if applicable]
```

These items are preserved for future reference but do **not** block milestone completion or create follow-up tasks. They can be addressed in a future polish pass.

### Five Whys Root Cause Analysis

**Only run this step if Critical or Warning issues were found.** Skip entirely if only Polish/Suggestion items exist.

When running: **read `references/verify-five-whys.md`** for the Five Whys framework, the grouping rule, and the `NOTES.md` entry format. Append each resulting entry to `{base}/NOTES.md` under `## Root Cause Patterns`.

### Determine Overall Status and Write the Report

**Read `references/verify-report-format.md` and use its template to produce the final summary output.** It contains the overall-status decision rules (ALL PASSED vs ISSUES FOUND vs CRITICAL ISSUES) and the combined markdown summary template.

<!-- @include commit-belmont-changes.md commit_context="after verification" -->

## Step 4: Clean Up Team (Agent Teams method only)

If you created a team in Step 2:
1. Send `shutdown_request` via `SendMessage` to each teammate still active
2. Wait for shutdown confirmations
3. Call `TeamDelete` to remove team resources

Skip this step if you used the Parallel Task method or the Sequential Inline fallback.

## Important Rules

1. **Run both agents** - Always run verification AND code review
2. **Be thorough** - Check all completed tasks, not just the latest
3. **Create follow-ups only for Critical/Warning** - Only these tiers become follow-up tasks. Polish items go to NOTES.md. Suggestions are reported but not persisted.
4. **Don't fix issues yourself** - Report them and create follow-up tasks
5. **Update PROGRESS.md** - Mark verified tasks `[v]`, add follow-up `[ ]` tasks for issues
6. **Polish doesn't block** - If only Polish/Suggestion items are found, all tasks are marked `[v]` and overall status is ALL PASSED

Once done, prompt the user to "/clear" and then "/belmont:status", "/belmont:next", or "/belmont:implement"
   - If you are Codex, instead prompt: "/new" and then "belmont:status", "belmont:next", or "belmont:implement"
