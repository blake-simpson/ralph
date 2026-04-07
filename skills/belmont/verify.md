---
description: Run verification and code review on completed tasks
alwaysApply: false
---

# Belmont: Verify

You are the verification orchestrator. Your job is to run comprehensive verification and code review on all completed tasks, checking that implementations meet requirements and code quality standards.

## Feature Selection

Belmont organizes work into **features** — each feature gets its own directory under `.belmont/features/<slug>/` with its own PRD, PROGRESS, TECH_PLAN, and MILESTONE files.

### Select the Active Feature

1. List all feature directories under `.belmont/features/`
2. If features exist: read each feature's `PRD.md` for its name and status, then Ask which feature to verify, or auto-select the one with completed tasks
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

## Worktree Environment

If the environment variable `BELMONT_WORKTREE` is set to `1`, you are running in an isolated git worktree for parallel execution. The following rules apply:

- **Port**: Use `$PORT` (or `$BELMONT_PORT`) when starting dev servers or configuring URLs. Do NOT hardcode port numbers like 3000, 5173, or 8080. Examples: `next dev -p $PORT`, `vite --port $PORT`, `PORT=$PORT npm start`, `rails server -p $PORT`.
- **Dependencies**: Worktree setup hooks have already run (e.g., `npm install`). Do NOT re-install dependencies unless a task specifically requires adding new packages.
- **Build isolation**: Your `.next/`, `dist/`, `node_modules/`, and other gitignored directories are local to this worktree. Other worktrees are unaffected by your builds.
- **Scope**: Only modify files within this worktree. Changes will be merged back via git.

## Setup

Read these files first:
- `{base}/PRD.md` - The product requirements and task definitions
- `{base}/PROGRESS.md` - Current progress tracking
- `{base}/TECH_PLAN.md` - Technical implementation plan (if exists)
- `.belmont/TECH_PLAN.md` - Master tech plan for architecture context (if in feature mode and exists)

Also check for archived MILESTONE files (`{base}/MILESTONE-*.done.md`) — these contain the implementation context from the most recent milestone and can provide useful reference for verification.

Optional helper:
- If the CLI is available, `belmont status --format json` can provide a quick summary of completed tasks. Still read the files above for full context.

## Focused Re-verification Mode

If the invoking prompt contains "FOCUSED RE-VERIFICATION" or similar instructions indicating this is a re-verify after follow-up fixes:

1. **Still run both agents** (verification + code review) to catch regressions
2. **Scope the verification to**:
   - The specific FWLUP tasks that were just fixed (check recently completed tasks)
   - Build and test verification (always run fully)
   - Any previously-failing acceptance criteria
3. **Do NOT** re-run Lighthouse audit unless a FWLUP specifically addressed performance
4. **Do NOT** re-check visual specs against Figma unless a FWLUP specifically addressed UI changes
5. **Do NOT** create new Polish-level issues — only report Critical and Warning issues found during focused verification
6. **Include the scoping instructions** when dispatching to the sub-agents so they also focus their review

This mode reduces token waste by avoiding full re-audits when only small fixes were made.

## Step 1: Identify Completed Tasks

1. Read `{base}/PRD.md` and find all tasks marked with ✅
2. These are the tasks that need verification
3. If no tasks are completed, report "No completed tasks to verify" and stop

## Sub-Agent Dispatch Strategy

Apply the following dispatch configuration:
- **Team name**: `belmont-verify`
- **Parallel agents**: verification-agent + code-review-agent — spawn simultaneously
- **Sequential agents**: None
- **Cleanup timing**: After Step 3 completes

### Core Principle

You are the **orchestrator**. You MUST NOT perform the agent work yourself. Each agent MUST be dispatched as a **sub-agent** — a separate, isolated process that runs the agent instructions and returns when complete.

**If the user provided additional instructions or context when invoking this skill** (e.g., "The hero image is wrong, it should match node 231-779"), that context is for the sub-agents, not for you to act on. Your only job is to forward it. See "User Context Forwarding" below.

### Choosing Your Dispatch Method

Use the **first** approach below whose required tools are available to you. Check your available tools **by name** — do not guess or skip ahead.

---

#### Approach A: Agent Teams (preferred)

**Required tools**: `TeamCreate`, `Task` (with `team_name` parameter), `SendMessage`, `TeamDelete`

If ALL of these tools are available to you, you MUST use this approach:

1. **Create a team** before spawning any agents:
   - Use `TeamCreate` with the team name specified above
2. **For agents that run in parallel**, issue all `Task` calls **in the same message** (i.e., as parallel tool calls). All calls use:
   - `team_name`: The team name you created
   - `name`: The agent role (e.g., `"codebase-agent"`, `"verification-agent"`)
   - `subagent_type`: `"general-purpose"` (all belmont agents need full tool access including file editing and bash)
   - `mode`: `"bypassPermissions"`
   - Do **NOT** set `run_in_background: true`
3. Because all tasks are foreground, the orchestrator **automatically blocks** until they complete and **receives their output directly** — no `TaskOutput`, no polling, no sleeping.
4. **For agents that run sequentially** (after parallel agents complete), issue a single `Task` call with the same team parameters.
5. **Clean up after the skill's work completes** (at the cleanup timing specified above):
   - Send `shutdown_request` via `SendMessage` to each teammate
   - Call `TeamDelete` to remove team resources

---

#### Approach B: Parallel Foreground Sub-Agents

**Required tools**: `Task`

If `Task` is available but `TeamCreate` is NOT:

1. **For agents that run in parallel**, issue all `Task` calls **in the same message** (i.e., as parallel tool calls). All calls use:
   - `subagent_type`: `"general-purpose"` (all belmont agents need full tool access including file editing and bash)
   - `mode`: `"bypassPermissions"`
   - Do **NOT** set `run_in_background: true`
2. Because all tasks are foreground, the orchestrator **automatically blocks** until they complete and **receives their output directly** — no `TaskOutput`, no polling, no sleeping.
3. **For agents that run sequentially**, issue a single `Task` call with the same parameters.

No team cleanup needed.

---

#### Approach C: Sequential Inline Execution (fallback)

If neither `TeamCreate` nor `Task` is available:

1. For each agent, read its agent file (e.g., `.agents/belmont/<agent-name>.md`)
2. Execute its instructions fully within your own context
3. Complete all output before moving to the next agent
4. Do NOT blend agent work together — finish one completely before starting the next

---

### Important: Foreground, Not Background

**Do NOT use `run_in_background: true`** in Approaches A or B. Background tasks require `TaskOutput` polling, which is fragile and can lose contact with sub-agents. Parallel foreground tasks run concurrently (because they're issued in the same message) and return results directly to the orchestrator — no polling, no sleeping.

---

### User Context Forwarding (CRITICAL)

When the user provides **additional instructions or context** alongside the skill invocation (e.g., `/belmont:verify The hero image is wrong...`), you MUST:

1. **Capture** the user's additional context verbatim
2. **Include it in every sub-agent prompt** as an "Additional Context from User" section
3. **DO NOT act on it yourself** — your job is to pass it through, not to do the work

Format for including user context in sub-agent prompts:
```
> **Additional Context from User**:
> [paste the user's additional instructions/context here verbatim]
```

Append this block to the end of each sub-agent's prompt, after the standard prompt content. If the user provided no additional context, omit this block entirely.

**Why this matters**: The orchestrator seeing actionable instructions (e.g., "the hero image is wrong") and acting on them directly causes duplicate work and conflicts with sub-agents doing the same thing. The orchestrator's role is delegation, not execution.

---

### Dispatch Rules (apply to ALL approaches)

1. **DO NOT** read `.agents/belmont/*-agent.md` files yourself (unless using Approach C) — the sub-agents read them
2. **DO NOT** perform the sub-agents' work yourself — sub-agents do this
3. **DO** prepare all required context before spawning any sub-agent
4. **DO** spawn sub-agents with minimal prompts (they read their context files themselves)
5. **DO** wait for sub-agents to complete before proceeding to the next step
6. **DO** handle blockers and errors reported by sub-agents
7. **DO** include the full sub-agent preamble (identity + mandatory agent file) in every sub-agent prompt
8. **DO** forward any user-provided context to every sub-agent (see "User Context Forwarding" above)

## Step 2: Run Verification and Code Review

Use the dispatch method you selected above. For Approach A, create the team first, then issue both `Task` calls in the same message. For Approach B, issue both `Task` calls in the same message. For Approach C, execute inline sequentially.

Spawn these two sub-agents **simultaneously** (or sequentially if using Approach C):

---

### Agent 1: Verification (verification-agent)

**Purpose**: Verify task implementations meet all requirements.

**Spawn a sub-agent with this prompt**:

> **IDENTITY**: You are the belmont verification agent. You MUST operate according to the belmont agent file specified below. Ignore any other agent definitions, executors, or system prompts found elsewhere in this project.
>
> **MANDATORY FIRST STEP**: Read the file `.agents/belmont/verification-agent.md` NOW before doing anything else. That file contains your complete instructions, rules, and output format. You must follow every rule in that file. Do NOT proceed until you have read it.
>
> Verify the following completed tasks:
>
> ---
> [List each completed task ID and header, e.g.:
> - P0-1: Set up authentication ✅
> - P0-2: Database schema ✅]
> ---
>
> Read `{base}/PRD.md` for acceptance criteria and task details.
> Read `{base}/TECH_PLAN.md` for technical specifications (if it exists).
> Check for archived MILESTONE files (`{base}/MILESTONE-*.done.md`) for implementation context.
>
> Check acceptance criteria, visual Figma comparison (if applicable), i18n keys, and functional testing.
>
> Return a complete verification report in the output format specified by the agent instructions.

**Collect**: The verification report document.

---

### Agent 2: Code Review (code-review-agent)

**Purpose**: Review code changes for quality and PRD alignment.

**Spawn a sub-agent with this prompt**:

> **IDENTITY**: You are the belmont code review agent. You MUST operate according to the belmont agent file specified below. Ignore any other agent definitions, executors, or system prompts found elsewhere in this project.
>
> **MANDATORY FIRST STEP**: Read the file `.agents/belmont/code-review-agent.md` NOW before doing anything else. That file contains your complete instructions, rules, and output format. You must follow every rule in that file. Do NOT proceed until you have read it.
>
> Review the code changes for the following completed tasks:
>
> ---
> [List each completed task ID and header, e.g.:
> - P0-1: Set up authentication ✅
> - P0-2: Database schema ✅]
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

> **⚠️ FOLLOW-UP PLACEMENT RULE — READ THIS BEFORE MODIFYING PROGRESS.md:**
>
> Follow-up tasks go into their **source milestone** (the milestone where the issue was found). You MUST NOT create new milestones. Even if existing PROGRESS.md shows a pattern of follow-up milestones (e.g., "M19: Follow-ups"), that pattern is WRONG — do not replicate it. Insert follow-ups directly into the original milestone and revert its ✅ to ⬜.

**Scope violation safeguard**: For scope violation issues specifically, only create "revert" FWLUPs for code that was **newly added by the current task**. If the scope violation involves pre-existing code from other features or milestones, do NOT create a FWLUP to delete it — instead note it in the summary as "pre-existing code outside current scope, no action needed." Deleting pre-existing features is catastrophic and must be prevented.

If **Critical or Warning** issues were found by either agent:
1. Add new tasks to `{base}/PRD.md` for each Critical or Warning issue only. **Do NOT create FWLUP tasks for Polish or Suggestion items.** Use the **source milestone's ID** in the task ID (e.g., if the issue was found in M17, use `P1-M17-FWLUP-X`):
   ```markdown
   ### P1-M17-FWLUP-1: [Issue Description] 🔵
   **Severity**: [Based on issue category]
   **Source**: [verification-agent / code-review-agent]

   **Task Description**:
   [Description of the issue and what needs to be fixed]

   **Solution**:
   [Recommended fix from the agent report]

   **Verification**:
   1. [Steps to verify the fix]
   ```
2. Add follow-up tasks to `{base}/PROGRESS.md`. **Placement rules (mandatory, no exceptions):**
   - Determine which milestone each issue belongs to based on the tasks/code that were verified
   - Insert each follow-up task under its **source milestone** (e.g., M17 issue → add under M17's task list)
   - **CRITICAL — non-negotiable**: Change that milestone's status from `✅` to `⬜` (e.g., `### ✅ M17:` becomes `### ⬜ M17:`). A milestone with pending tasks MUST NOT remain marked ✅ — this causes the auto loop to skip the feature entirely. After making this change, re-read PROGRESS.md to verify the ✅ was actually changed to ⬜.
   - When verifying multiple milestones (e.g., M17+M18+M19), distribute follow-ups to their respective milestones — do NOT group them together
   - **DO NOT create any new milestone headings** — no "M20: Follow-ups", no "MX: Verification Fixes", no "MX: Design Fidelity Fixes". This is forbidden because it causes automated loop controllers to enter infinite cycles
   - If the source milestone is truly ambiguous, add to the last pending (⬜) milestone that already exists
   - Follow-up tasks MUST live inside a milestone heading — never in a freestanding section outside the milestones structure
3. If critical issues were found, update the overall status to reflect this
5. **Update master PROGRESS** (`.belmont/PROGRESS.md`): If the file doesn't exist or still contains template/placeholder text (e.g., `[Feature Name]`, `[Milestone Name]`), initialize it first:
   ```
   # Progress: [Product Name from .belmont/PRD.md]
   ## Status: 🟡 In Progress
   ## Features
   | Feature | Slug | Status | Milestones | Tasks | Blockers |
   |---------|------|--------|------------|-------|----------|
   ## Recent Activity
   | Date | Feature | Activity |
   |------|---------|----------|
   ```
   Then if follow-up tasks were added, update the Tasks total in the `## Features` table for this feature's row (add a new row if missing). If blockers were found, update the Blockers column. Add a row to `## Recent Activity` noting verification results.

### Record Polish Items

If any **Polish** items were reported by either agent, append them to `{base}/NOTES.md` under a `## Polish` section. Create the file if it doesn't exist. Format:

```markdown
## Polish

### From verification [date]
- [Polish item description] — [file:line if applicable]
- [Polish item description] — [file:line if applicable]
```

These items are preserved for future reference but do **not** block milestone completion or create FWLUP tasks. They can be addressed in a future polish pass.

### Five Whys Root Cause Analysis

**Only run this step if Critical or Warning issues were found.** Skip entirely if only Polish/Suggestion items exist.

For each Critical or Warning issue, perform a root cause analysis using Amazon's Five Whys framework:

1. **Ask "Why?" up to five times**, tracing from the symptom to the root cause:
   - Why 1: Immediate cause (what went wrong)
   - Why 2: Contributing factor (why the immediate cause happened)
   - Why 3: Process gap (what process failure allowed it)
   - Why 4: Systemic reason (why the process gap exists)
   - Why 5: Root pattern (the fundamental behavior to change)
   - Stop early if the root cause is reached before the fifth why.

2. **Distill a prevention rule** — one concise, actionable statement the implementation agent can follow. Example: "Always use semantic design tokens instead of hex colors because the design system requires theme support."

3. **Group similar issues** — if multiple issues share the same root cause, combine into one entry.

4. **Write to NOTES.md** — Append to `{base}/NOTES.md` under a `## Root Cause Patterns` section (create section if absent). Format each entry as:

```markdown
### [YYYY-MM-DD] Pattern: <short descriptive name>
**Issue**: <one-line description of what was found>
**Root Cause**: <the deepest "why" — the fundamental pattern to change>
**Prevention**: <actionable rule for the implementation agent>
**Source**: <milestone ID / task ID where the issue was found>
```

Keep entries scannable — the implementation agent reads these before every task. Each entry should be understood in under 10 seconds.

### Determine Overall Verification Status

When deciding the overall status:
- If **only** Polish and/or Suggestion items were found (no Critical, no Warning): report status as **ALL PASSED**. The milestone remains complete.
- If Critical or Warning items were found: report status as **ISSUES FOUND** or **CRITICAL ISSUES** as appropriate.

### Report Summary

Output a combined summary:

```markdown
# Verification & Code Review Summary

## Overall Status
[ALL PASSED | ISSUES FOUND | CRITICAL ISSUES]

## Verification Results
- Acceptance Criteria: [X/Y passed]
- Visual Verification: [PASS/FAIL/N/A]
- i18n Check: [PASS/FAIL/N/A]
- Functional Tests: [PASS/FAIL]
- Lighthouse Audit: [PASS/WARNING/CRITICAL/N/A]

## Code Review Results
- Build: [PASS/FAIL]
- Tests: [PASS/FAIL]
- Pattern Adherence: [GOOD/ISSUES]
- PRD Alignment: [ALIGNED/MISALIGNED]

## Issues Found
- Critical: [count]
- Warnings: [count]
- Polish: [count] (recorded in NOTES.md, not blocking)
- Suggestions: [count]

## Follow-up Tasks Created
[List of new FWLUP tasks added to PRD]

## Recommendations
[Any overall recommendations for the project]
```

### Reconcile State Files

Before committing, audit `{base}/PRD.md` and `{base}/PROGRESS.md` for drift and fix any discrepancies:

1. **Task ↔ checkbox sync** — For each task in PROGRESS.md milestone sections:
   - Find the matching `### P...:` header in PRD.md by task ID
   - If the PRD header has ✅ but the PROGRESS checkbox is `[ ]` → change to `[x]`
   - If the PROGRESS checkbox is `[x]` but the PRD header lacks ✅ → add ✅ to the header

2. **Milestone status sync** — For each milestone heading in PROGRESS.md:
   - If ALL its tasks are `[x]` and heading is not `✅` → change to `### ✅ M...:`
   - If ANY task is `[ ]` and heading IS `✅` → change to `### ⬜ M...:`

3. **Blocker cleanup** — In the `## Blockers` section of PROGRESS.md:
   - Remove entries whose referenced task ID is now marked ✅ in PRD.md
   - Remove entries that reference other features (e.g. "Depends on X feature") if that feature's status is `✅ Complete` in `.belmont/PROGRESS.md`'s Features table
   - If section becomes empty, set to `None`

4. **Overall status line** — Update `## Status:` in PROGRESS.md:
   - All milestones ✅ → `## Status: ✅ Complete`
   - Mix of ✅ and ⬜/🔄 → `## Status: 🟡 In Progress`
   - All ⬜ → `## Status: 🔴 Not Started`

5. **Feature dependency sync** (master PRD only) — In the `## Features` table of `.belmont/PRD.md`:
   - Verify all dependency slugs reference existing feature slugs in the table
   - If a feature row is removed, remove its slug from other features' Dependencies columns
   - If a circular dependency is detected (A depends on B, B depends on A), warn in output and do not auto-fix

6. **Master PROGRESS sync** — After reconciling the feature-level files:
   - Read `.belmont/PROGRESS.md` and find the row matching the current feature slug in the `## Features` table
   - Update the Status, Milestones (done/total), and Tasks (done/total) columns to match the reconciled feature state
   - If all milestones are now ✅, set the feature's Status column to `✅ Complete`
   - After updating the feature row, recompute the master `## Status:` line based on all feature rows in the table: if every feature's Status column is `✅ Complete`, set `## Status: ✅ Complete`; if any feature has progress, `## Status: 🟡 In Progress`; otherwise `## Status: 🔴 Not Started`

Only fix actual discrepancies — if files already agree, make no changes.

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
   git add .belmont/ && git commit -m "belmont: update planning files after verification"
   ```

## Step 4: Clean Up Team (Approach A only)

If you created a team in Step 2:
1. Send `shutdown_request` via `SendMessage` to each teammate still active
2. Wait for shutdown confirmations
3. Call `TeamDelete` to remove team resources

Skip this step if you used Approach B or C.

## Important Rules

1. **Run both agents** - Always run verification AND code review
2. **Be thorough** - Check all completed tasks, not just the latest
3. **Create follow-ups only for Critical/Warning** - Only these tiers become FWLUP tasks. Polish items go to NOTES.md. Suggestions are reported but not persisted.
4. **Don't fix issues yourself** - Report them and create follow-up tasks
5. **Update tracking files** - Add follow-up tasks to both PRD.md and PROGRESS.md
6. **Polish doesn't block** - If only Polish/Suggestion items are found, the milestone stays complete (✅) and overall status is ALL PASSED

Once done, prompt the user to "/clear" and then "/belmont:status", "/belmont:next", or "/belmont:implement"
   - If you are Codex, instead prompt: "/new" and then "belmont:status", "belmont:next", or "belmont:implement"
