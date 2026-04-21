# Verify: Follow-up Tasks

Use this when you need to record Critical or Warning issues found during verification. Governs follow-up task placement in PROGRESS.md.

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
