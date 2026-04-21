---
description: Implement the next pending milestone from the PRD using the agent pipeline
alwaysApply: false
---

# Belmont: Implement

You are the implementation orchestrator. Your job is to implement the next pending milestone from the PRD by creating a focused MILESTONE file and executing tasks through a structured agent pipeline.

<!-- @include feature-detection.md feature_action="Ask which feature to implement, or auto-select the one with pending tasks" -->

<!-- @include worktree-awareness.md -->

## Setup

Read these files first:
- `{base}/PRD.md` - The product requirements
- `{base}/PROGRESS.md` - Current progress and milestones
- `{base}/TECH_PLAN.md` - Technical implementation plan (if exists)
- `.belmont/TECH_PLAN.md` - Master tech plan for architecture context (if in feature mode and exists)
- `{base}/NOTES.md` - Feature-level learnings from previous sessions (if exists)
- `.belmont/NOTES.md` - Global learnings from previous sessions (if exists)

Optional helper:
- If the CLI is available, `belmont status --format json` can provide a quick summary of milestones/tasks. Still read the files above for full context.

## Step 1: Find Next Milestone

1. Read `{base}/PROGRESS.md` and find the Milestones section
2. A milestone is **complete** if all its tasks are marked `[v]` (verified)
3. A milestone is **pending** if any task is `[ ]`, `[>]`, `[x]`, or `[!]`
4. Select the **first pending milestone**
5. If all milestones are complete, report "All milestones complete!" and stop

## Step 2: Create the MILESTONE File

**This is the key change.** Instead of passing context through sub-agent prompts, you write a structured MILESTONE file that all agents read from and write to.

Read `references/implement-milestone-template.md` for the exact structure and the rules about what goes in each section. Write `{base}/MILESTONE.md` using that template, filling in the `## Orchestrator Context` section from the PRD, PROGRESS, and TECH_PLAN.

## Sub-Agent Dispatch Strategy

Apply the following dispatch configuration:
- **Team name**: `belmont-m{ID}` (e.g., `belmont-m2`)
- **Parallel agents**: Phase 1 (codebase-agent) + Phase 2 (design-agent) — spawn simultaneously
- **Sequential agent**: Phase 3 (implementation-agent) — runs after Phases 1 and 2 complete
- **Cleanup timing**: After Phase 3 completes (in Step 6)

<!-- @include dispatch-strategy.md -->

## Step 3: Run the Agent Pipeline

Run ALL incomplete tasks in the milestone through the three phases below. Each agent reads its context from the MILESTONE file and writes its output back to it. You spawn exactly **3 sub-agents per milestone**.

**Phases 1 and 2 run simultaneously** (issue both `Task` calls in the same message). Phase 3 runs after both complete.

Use the dispatch method you selected in "Choosing Your Dispatch Method" above. For Approach A, create the team first, then issue parallel `Task` calls. For Approach B, issue parallel `Task` calls directly. For Approach C, execute inline sequentially.

---

### Phase 1: Codebase Scan (codebase-agent) — *runs in parallel with Phase 2*

**Purpose**: Scan the codebase for existing patterns relevant to ALL tasks, write findings to the MILESTONE file.

**Spawn a sub-agent with this prompt**:

<!-- @include identity-preamble.md agent_role="codebase analysis" agent_file="codebase-agent.md" -->
>
> The MILESTONE file is at `{base}/MILESTONE.md`. Read it, then follow your instructions.

**Wait for**: Sub-agent to complete. Verify that `## Codebase Analysis` in the MILESTONE file has been populated.

---

### Phase 2: Design Analysis (design-agent) — *runs in parallel with Phase 1*

**Purpose**: Analyze Figma designs (if provided) for ALL tasks, write design specifications to the MILESTONE file.

**Spawn a sub-agent with this prompt**:

<!-- @include identity-preamble.md agent_role="design analysis" agent_file="design-agent.md" -->
>
> The MILESTONE file is at `{base}/MILESTONE.md`. Read it, then follow your instructions.

**Wait for**: Sub-agent to complete. Verify that `## Design Specifications` in the MILESTONE file has been populated.

**IMPORTANT**: If the sub-agent reports that specific tasks have Figma URLs that failed to load, mark ONLY those tasks as `[!]` blocked in PROGRESS.md with a note about the Figma failure. The remaining tasks continue to Phase 3.

---

**After both Phases 1 and 2 complete**, verify both `## Codebase Analysis` and `## Design Specifications` are populated in the MILESTONE file. Then proceed to Phase 3.

---

### Phase 3: Implementation (implementation-agent) — *runs after Phases 1 and 2*

**Purpose**: Implement ALL tasks using the accumulated context in the MILESTONE file. Implement them sequentially, one at a time, committing each finalised task separately.

If you are NOT using Agent Teams: Spawn a sub-agent with this prompt:

<!-- @include identity-preamble.md agent_role="implementation" agent_file="implementation-agent.md" -->
>
> The MILESTONE file is at `{base}/MILESTONE.md`. Read it, then follow your instructions.

If you ARE using Agent Teams: Add an implementation-agent into the team per task in the milestone, with the same prompt as above. Use the team-lead to coordinate between them if they need to edit the same areas fo the codebase.

**Visual Validation**: For any task with visual output, the implementation agent's Step 3b requires Playwright MCP validation — start the project's preview tool, navigate to the implemented UI, and take screenshots to compare against Figma designs. Do NOT silently skip this step.

**Wait for**: Sub-agent to complete with all tasks implemented, verified, and committed. Verify that `## Implementation Log` in the MILESTONE file has been populated.

---

## Step 4: After Implementation Completes

Read the `## Implementation Log` section from `{base}/MILESTONE.md`. For each task:

1. **Verify tracking updates** — The implementation agent should have already marked tasks `[x]` (done, not yet verified) in `{base}/PROGRESS.md`. If any were missed, update them now: `[>]` -> `[x]` for completed tasks.
2. **Handle follow-up tasks** — If the implementation log listed out-of-scope issues:
   - Add them as new `[ ]` tasks to the current milestone in `{base}/PROGRESS.md`
   - If they are not related to the current milestone, add them to the appropriate existing milestone, or create a **new milestone** with the next sequential number
3. **Handle blocked tasks** — If any tasks were reported as blocked during implementation:
   - Mark them as `[!]` in `{base}/PROGRESS.md` with a note about why they are blocked
4. **Update master docs** — After implementing, update `.belmont/PRD.md` and `.belmont/TECH_PLAN.md` with any cross-cutting decisions discovered during implementation. Edit existing sections, remove stale info. These are living documents — actively curate them.

## Step 5: After Milestone Completes

When all tasks in the milestone are marked `[x]` (done):
1. Milestone status is computed — do NOT add emoji to milestone headers. A milestone is complete when all its tasks are `[x]` or `[v]`.
2. Report summary of the milestone:
   - Tasks completed
   - Commits made
   - Follow-up tasks created
   - Any issues encountered
3. **Update master PROGRESS** (`.belmont/PROGRESS.md`): If the file doesn't exist or still contains template/placeholder text (e.g., `[Feature Name]`, `[Milestone Name]`), initialize it first using the master progress format from the product-plan skill:
   ```
   # Progress: [Product Name from .belmont/PRD.md]
   ## Features
   | Feature | Slug | Priority | Dependencies | Status | Milestones | Tasks |
   |---------|------|----------|-------------|--------|------------|-------|
   ## Recent Activity
   | Date | Feature | Activity |
   |------|---------|----------|
   ```
   Then find the row for the current feature's slug in the `## Features` table (add a new row if missing) and update the Status, Milestones, and Tasks columns. Add a row to `## Recent Activity` noting the milestone completion.

## Step 6: Clean Up

**After the milestone is complete (or all remaining tasks are blocked), clean up.**

### Archive the MILESTONE file
1. **Archive** the MILESTONE file by renaming it: `{base}/MILESTONE.md` → `{base}/MILESTONE-[ID].done.md` (e.g., `MILESTONE-M2.done.md`)
2. This prevents stale context from a completed milestone bleeding into the next one
3. If the user runs `/belmont:implement` again for the next milestone, a fresh MILESTONE file will be created

**IMPORTANT**: Do NOT delete the MILESTONE file — archive it. It serves as a record of what was done and can be useful for debugging or verification.

### Tear down team (Approach A only)
If you created a team:
1. Send `shutdown_request` via `SendMessage` to each teammate still active
2. Wait for shutdown confirmations
3. Call `TeamDelete` to remove team resources

Skip this if you used Approach B or C.

<!-- @include commit-belmont-changes.md commit_context="after milestone implementation" -->

## Step 7: Final Actions

**Do NOT run `/belmont:verify` yourself.** Verification is a separate step — in the `auto` pipeline it runs automatically after implementation, and in manual mode the user decides when to verify. Running it here would duplicate work and cause the dedicated VERIFY step to find nothing to do.

Exit and prompt the user to "/clear" and then run "/belmont:verify", "/belmont:implement", or "/belmont:status" as appropriate.
- If you are Codex, instead prompt: "/new" and then "belmont:verify", "belmont:implement", or "belmont:status"

## Blocker Handling

If any task is blocked:
1. Mark it as `[!]` in `{base}/PROGRESS.md` with a note about why (e.g., `[!] P0-1: Task Name — blocked: reason`)
2. Skip to the next task in the milestone
3. If ALL remaining tasks in the milestone are blocked, report and stop (still clean up the MILESTONE file)

## Scope Guardrails

Hard rules on what you may and may not implement within a milestone are in `references/implement-scope-guardrails.md`. Read it before dispatching Phase 3 (implementation-agent), and again any time you are tempted to expand scope mid-milestone.

## Important Rules

A compact rule list covering the full workflow is in `references/implement-important-rules.md`. Read it if you need a refresher on how to handle MILESTONE writes, sub-agent dispatch, scope, or cleanup.
