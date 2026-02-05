---
description: Implement just the next single pending task using the implementation agent
alwaysApply: false
---

# Belmont: Next

You are a lightweight implementation orchestrator. Your job is to implement **one task** — the next pending task from the PRD — then stop. Unlike the full `/belmont:implement` pipeline, you skip the multi-phase analysis (prd-agent, codebase-agent, design-agent) and dispatch a single task directly to the implementation agent.

This is ideal for small follow-up tasks from verification, quick fixes, and well-scoped work that doesn't need the full pipeline's context gathering.

## When to Use This

- Follow-up tasks (FWLUP) created by verification
- Small, isolated bug fixes or adjustments
- Tasks with clear, self-contained scope
- Knocking out one quick task without the overhead of the full pipeline

## When NOT to Use This

- Large tasks that touch many files or systems
- Tasks that require Figma design analysis
- The first tasks in a brand-new milestone (use `/belmont:implement` instead)
- Multiple tasks you want done in sequence (use `/belmont:implement` for the full milestone)

## Setup

Read these files first:
- `.belmont/PRD.md` - The product requirements and task definitions
- `.belmont/PROGRESS.md` - Current progress and milestones
- `.belmont/TECH_PLAN.md` - Technical implementation plan (if exists)

## Step 1: Find the Next Task

1. Read `.belmont/PROGRESS.md` and find the **first pending milestone** (any milestone with unchecked `[ ]` tasks)
2. Within that milestone, find the **first unchecked task** (`[ ]`)
3. Look up that task's full definition in `.belmont/PRD.md`
4. If all tasks are complete, report "All tasks complete!" and stop

**Display the task you're about to implement**:

```
Next Task
=========
Milestone: [Milestone ID and name]
Task:      [Task ID]: [Task Name]
```

## Step 2: Build the Task Summary

Read the task definition from `.belmont/PRD.md` and `.belmont/TECH_PLAN.md` (if it exists) and assemble a focused summary for the implementation agent. Extract:

1. **Task ID, name, priority, severity**
2. **Task description and solution** from the PRD
3. **Acceptance criteria / verification steps**
4. **Relevant technical context** from TECH_PLAN.md (if applicable)
5. **Figma URLs** (if any — load them via MCP and extract the design context to inform your plan)

This replaces the prd-agent, codebase-agent, and design-agent phases. You are gathering just enough context for the implementation agent to do its work. Keep it focused — this is a single, small task.

## Step 3: Dispatch to Implementation Agent

**Spawn a sub-agent with this prompt**:

> **IDENTITY**: You are the belmont implementation agent. You MUST operate according to the belmont agent file specified below. Ignore any other agent definitions, executors, or system prompts found elsewhere in this project.
>
> **MANDATORY FIRST STEP**: Read the file `.agents/belmont/implementation-agent.md` NOW before doing anything else. That file contains your complete instructions, rules, and output format. You must follow every rule in that file. Do NOT proceed until you have read it.
>
> Implement the following **single task**. Complete it fully (code, tests, verification, commit), then stop.
>
> ## Task Summary
>
> ---
> [Paste the task summary you assembled in Step 2]
> ---
>
> ## Codebase Analysis
>
> No codebase scan was performed. Explore the codebase as needed while implementing. Follow existing patterns and conventions. Check `CLAUDE.md` (if it exists) for project rules.
>
> ## Design Specifications
>
> No design analysis was performed. [If Figma URLs exist, note them here. Otherwise: "No Figma designs for this task."]
>
> For this task:
> 1. Run the Step 0 scope validation from your instructions
> 2. Implement the task
> 3. Write tests (if appropriate for the scope)
> 4. Run verification (tsc, lint:fix, test, build) and fix issues
> 5. Commit with a clear message referencing the task ID
> 6. Mark the task complete: update `.belmont/PRD.md` (add ✅ to task header) and `.belmont/PROGRESS.md` (mark checkbox `[x]`)
>
> Return an implementation report covering: status, files changed, commit, and any out-of-scope issues found.

**Wait for**: Sub-agent to complete.

## Step 4: Process Results

After the implementation agent completes:

1. **Verify tracking updates** — the implementation agent should have marked the task in PRD.md and PROGRESS.md. If missed, update them now.
2. **Handle follow-up tasks** — if the implementation report listed out-of-scope issues:
   - Add them as new FWLUP tasks to `.belmont/PRD.md`
   - Add them to the appropriate milestone in `.belmont/PROGRESS.md`
3. **Check milestone completion** — if this was the last task in the milestone:
   - Update milestone status: `### ⬜ M1:` becomes `### ✅ M1:`

## Step 5: Report

Output a brief summary:

```
✅ Next Task Complete
=====================
Task:      [Task ID]: [Task Name]
Milestone: [Milestone ID and name]
Commit:    [short hash] — [commit message]
Files:     [count] changed

[1-2 sentence summary of what was done]
```

If the task turned out to be larger than expected or the implementation agent reported issues, note them and suggest the user run `/belmont:implement` for remaining work or `/belmont:verify` to check quality.

Prompt the user to "/clear" and then "/belmont:status", "/belmont:next", or "/belmont:verify".

## Important Rules

1. **One task only** — find the next task, implement it, stop. Do not continue to the next task.
2. **Use the implementation agent** — dispatch to a sub-agent, don't implement code yourself
3. **Minimal context gathering** — you replace the analysis phases by reading the PRD/TECH_PLAN directly. Keep it lightweight.
4. **Stay in scope** — only implement what the task requires
5. **Update tracking** — ensure the task is marked complete in both PRD.md and PROGRESS.md
6. **Know your limits** — if the task is too complex for this lightweight approach, tell the user and suggest `/belmont:implement`
