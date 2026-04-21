# Implement: Scope Guardrails

Read this before dispatching Phase 3 (implementation-agent) and whenever you are tempted to expand scope mid-milestone.

## Milestone Boundary (HARD RULE)

You may ONLY implement tasks that belong to the **current milestone** — the first pending milestone identified in Step 1. You MUST NOT:

- Implement tasks from future milestones, even if they seem easy or related
- "Get ahead" by starting work on the next milestone's tasks
- Add tasks to the current milestone that weren't already there

If you finish all tasks in the current milestone, **stop**. Report the milestone as complete. The user will invoke implement again for the next milestone.

## PRD Scope Boundary (HARD RULE)

ALL work must trace back to a specific task in `{base}/PRD.md`. You MUST NOT:

- Implement features, capabilities, or behaviors not described in the PRD
- Add "nice to have" improvements that aren't part of any task
- Refactor, restructure, or optimize code beyond what is required to complete the current task
- Create files, components, utilities, or endpoints that aren't needed by a task in the current milestone

If during implementation you discover something that **should** be done but **isn't in the PRD**, the correct action is:

1. Add it as a new `[ ]` task in the appropriate milestone in PROGRESS.md
2. Do NOT implement it now

## Scope Validation Checkpoint

The implementation agent (Phase 3) performs scope validation for each task before implementing it (see Step 0 in `implementation-agent.md`). As the orchestrator, verify before dispatching Phase 3:

1. All task IDs in the milestone exist in `{base}/PRD.md`
2. All tasks belong to the current milestone in `{base}/PROGRESS.md`
3. No tasks from other milestones have been included

If any check fails, STOP and report the issue rather than proceeding.
