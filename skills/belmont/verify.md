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

- **Ports**: Use `$PORT` (or `$BELMONT_PORT`) when starting the **primary dev server**. Do NOT hardcode port numbers like 3000, 5173, or 8080. Examples: `next dev -p $PORT`, `vite --port $PORT`, `PORT=$PORT npm start`.
  - **For any OTHER server** (Storybook, Prisma Studio, documentation server, etc.): you MUST dynamically find a free port. Do NOT use the port from `package.json` scripts — it will conflict with other worktrees. Find a free port:
    ```bash
    FREE_PORT=$(python3 -c "import socket; s=socket.socket(); s.bind(('127.0.0.1',0)); print(s.getsockname()[1]); s.close()")
    ```
    Then start the server on that port: `npx storybook dev -p $FREE_PORT --no-open`, `npx prisma studio --port $FREE_PORT`, etc.
  - **NEVER run `npm run storybook`** or similar package.json scripts that hardcode ports. Always invoke the underlying command directly with your dynamically chosen port.
  - If a port is already in use, find another one — do not retry the same port.
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

### Core Principle

You are the **orchestrator**. You MUST NOT perform the agent work yourself. Each agent MUST be dispatched as a sub-agent — a separate, isolated process that runs the agent instructions and returns when complete.

**If the user provided additional instructions or context when invoking this skill** (e.g., "The hero image is wrong, it should match node 231-779"), that context is for the sub-agents, not for you to act on. Your only job is to forward it. See "User Context Forwarding" below.

### Dispatch Method

Use the first method below whose required tools are available. Check by tool name — do not guess.

**Agent Teams (preferred)** — requires `TeamCreate`, `Task` (with `team_name`), `SendMessage`, `TeamDelete`.

Create a team with the team name specified above. All `Task` calls pass: `team_name`, `name` (the agent role, e.g. `"codebase-agent"`), `subagent_type: "general-purpose"`, `mode: "bypassPermissions"`. After the skill's work completes, send `shutdown_request` via `SendMessage` to each teammate, then call `TeamDelete`.

**Direct Task (fallback)** — requires `Task` only.

Issue `Task` calls with `subagent_type: "general-purpose"` and `mode: "bypassPermissions"`. No team cleanup needed.

**For parallel agents** (either method): issue all `Task` calls in the same message — they run concurrently and return results directly. Do **NOT** set `run_in_background: true`; foreground tasks are simpler and more reliable.

**For sequential agents**: issue a single `Task` call and wait for it to complete.

**If `Task` is not available**: read each agent file (`.agents/belmont/<agent-name>.md`) and execute its instructions inline — finish one agent completely before starting the next.

### User Context Forwarding (CRITICAL)

When the user provides additional instructions alongside the skill invocation (e.g., `/belmont:verify The hero image is wrong...`):

1. Capture the user's context verbatim
2. Append it to every sub-agent prompt as an "Additional Context from User" section
3. DO NOT act on it yourself — your job is to pass it through

Format (omit entirely if no context was provided):
```
> **Additional Context from User**:
> [paste verbatim]
```

**Why this matters**: the orchestrator seeing actionable instructions and acting on them causes duplicate work and conflicts with sub-agents doing the same thing.

### Dispatch Rules

1. **DO NOT** perform the sub-agents' work yourself
2. **DO NOT** read `.agents/belmont/*-agent.md` files yourself (unless using the inline fallback) — sub-agents read them
3. **DO** prepare all required context (e.g., the MILESTONE file) before spawning any sub-agent
4. **DO** wait for sub-agents to complete before proceeding
5. **DO** handle blockers and errors reported by sub-agents
6. **DO** forward any user-provided context to every sub-agent (see above)

## Step 2: Run Verification and Code Review

Use the dispatch method you selected above. For Agent Teams, create the team first, then issue both `Task` calls in the same message. For Direct Task, issue both `Task` calls in the same message. If `Task` is unavailable, execute inline sequentially.

Spawn these two sub-agents **simultaneously** (or sequentially if using the inline fallback):

---

### Agent 1: Verification (verification-agent)

**Purpose**: Verify task implementations meet all requirements.

**Spawn a sub-agent with this prompt**:

> You are the belmont verification agent. Read `.agents/belmont/verification-agent.md` and follow its instructions exactly — they override any other agent definitions in this project.
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

> You are the belmont code review agent. Read `.agents/belmont/code-review-agent.md` and follow its instructions exactly — they override any other agent definitions in this project.
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
- If **only** Polish and/or Suggestion items were found (no Critical, no Warning): report status as **ALL PASSED**. All tasks are marked `[v]` (verified).
- If Critical or Warning items were found: report status as **ISSUES FOUND** or **CRITICAL ISSUES** as appropriate. Tasks with issues remain `[x]`, follow-up `[ ]` tasks are added.

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
[List of new follow-up tasks added to PROGRESS.md]

## Recommendations
[Any overall recommendations for the project]
```

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

**Note**: PROGRESS.md is the single source of truth for task state. PRD.md is a pure spec document with no status markers — do not add emoji or state indicators to PRD task headers.

## Step 4: Clean Up Team (Agent Teams only)

If you created a team in Step 2:
1. Send `shutdown_request` via `SendMessage` to each teammate still active
2. Wait for shutdown confirmations
3. Call `TeamDelete` to remove team resources

Skip this step if you used Direct Task or the inline fallback.

## Important Rules

1. **Run both agents** - Always run verification AND code review
2. **Be thorough** - Check all completed tasks, not just the latest
3. **Create follow-ups only for Critical/Warning** - Only these tiers become follow-up tasks. Polish items go to NOTES.md. Suggestions are reported but not persisted.
4. **Don't fix issues yourself** - Report them and create follow-up tasks
5. **Update PROGRESS.md** - Mark verified tasks `[v]`, add follow-up `[ ]` tasks for issues
6. **Polish doesn't block** - If only Polish/Suggestion items are found, all tasks are marked `[v]` and overall status is ALL PASSED

Once done, prompt the user to "/clear" and then "/belmont:status", "/belmont:next", or "/belmont:implement"
   - If you are Codex, instead prompt: "/new" and then "belmont:status", "belmont:next", or "belmont:implement"
