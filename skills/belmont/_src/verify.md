---
description: Run verification and code review on completed tasks
alwaysApply: false
---

# Belmont: Verify

You are the verification orchestrator. Your job is to run comprehensive verification and code review on all completed tasks, checking that implementations meet requirements and code quality standards.

<!-- @include feature-detection.md feature_action="Ask which feature to verify, or auto-select the one with completed tasks" -->

## Setup

Read these files first:
- `{base}/PRD.md` - The product requirements and task definitions
- `{base}/PROGRESS.md` - Current progress tracking
- `{base}/TECH_PLAN.md` - Technical implementation plan (if exists)
- `.belmont/TECH_PLAN.md` - Master tech plan for architecture context (if in feature mode and exists)

Also check for archived MILESTONE files (`{base}/MILESTONE-*.done.md`) — these contain the implementation context from the most recent milestone and can provide useful reference for verification.

Optional helper:
- If the CLI is available, `belmont status --format json` can provide a quick summary of completed tasks. Still read the files above for full context.

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

<!-- @include dispatch-strategy.md -->

## Step 2: Run Verification and Code Review

Use the dispatch method you selected above. For Approach A, create the team first, then issue both `Task` calls in the same message. For Approach B, issue both `Task` calls in the same message. For Approach C, execute inline sequentially.

Spawn these two sub-agents **simultaneously** (or sequentially if using Approach C):

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

<!-- @include identity-preamble.md agent_role="code review" agent_file="code-review-agent.md" -->
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
2. Categorize all issues found:
   - **Critical** - Must be fixed (blocking quality/functionality issues)
   - **Warnings** - Should be fixed (non-blocking but important)
   - **Suggestions** - Nice to have improvements

### Create Follow-up Tasks
If any issues were found by either agent:
1. Add new tasks to `{base}/PRD.md` for each critical or warning issue:
   ```markdown
   ### P0-X-FWLUP: [Issue Description] 🔵
   **Severity**: [Based on issue category]
   **Source**: [verification-agent / code-review-agent]

   **Task Description**:
   [Description of the issue and what needs to be fixed]

   **Solution**:
   [Recommended fix from the agent report]

   **Verification**:
   1. [Steps to verify the fix]
   ```
2. Add the follow-up tasks to a milestone in `{base}/PROGRESS.md`:
   - If a **pending** (⬜) milestone exists, add them to the last pending milestone
   - If **all milestones are complete** (✅), create a **new milestone** with the next sequential number (e.g., if M9 is the last, create `### ⬜ M10: Follow-ups`) and add them there
   - Follow-up tasks MUST live inside a milestone heading — never in a freestanding section outside the milestones structure
3. If critical issues were found, update the overall status to reflect this
4. If a new milestone was created, revert the overall status from `✅ Complete` to `🟡 In Progress`
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
- Suggestions: [count]

## Follow-up Tasks Created
[List of new FWLUP tasks added to PRD]

## Recommendations
[Any overall recommendations for the project]
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
3. **Create actionable follow-ups** - Issues should become trackable tasks
4. **Don't fix issues yourself** - Report them and create follow-up tasks
5. **Update tracking files** - Add follow-up tasks to both PRD.md and PROGRESS.md

Once done, prompt the user to "/clear" and then "/belmont:status", "/belmont:next", or "/belmont:implement"
   - If you are Codex, instead prompt: "/new" and then "belmont:status", "belmont:next", or "belmont:implement"
