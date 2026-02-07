---
description: Implement the next pending milestone from the PRD using the agent pipeline
alwaysApply: false
---

# Belmont: Implement

You are the implementation orchestrator. Your job is to implement the next pending milestone from the PRD by creating a focused MILESTONE file and executing tasks through a structured agent pipeline.

## Setup

Read these files first:
- `.belmont/PRD.md` - The product requirements
- `.belmont/PROGRESS.md` - Current progress and milestones
- `.belmont/TECH_PLAN.md` - Technical implementation plan (if exists)

## Step 1: Find Next Milestone

1. Read `.belmont/PROGRESS.md` and find the Milestones section
2. A milestone is **complete** if all its tasks are marked with `[x]` or `✅`
3. A milestone is **pending** if any task is still `[ ]`
4. Select the **first pending milestone**
5. If all milestones are complete, report "All milestones complete!" and stop

## Step 2: Create the MILESTONE File

**This is the key change.** Instead of passing context through sub-agent prompts, you write a structured MILESTONE file that all agents read from and write to.

Create `.belmont/MILESTONE.md` with the following structure. Fill in the `## Orchestrator Context` section using information from the PRD, PROGRESS, and TECH_PLAN:

```markdown
# Milestone: [ID] — [Name]

## Status
- **Milestone**: [e.g., M2: Core Features]
- **Created**: [timestamp]
- **Tasks**:
  - [ ] [Task ID]: [Task Name]
  - [ ] [Task ID]: [Task Name]
  ...

## Orchestrator Context

### Current Milestone
[Milestone ID and name, with the full list of incomplete tasks in this milestone]

### Relevant PRD Context
[Extract from PRD.md: the Overview, Problem Statement, Technical Approach, and Out of Scope sections. Also extract the FULL task definitions for every incomplete task in this milestone — copy them verbatim from the PRD including all fields (description, solution, notes, verification, Figma URLs, etc.)]

### Scope Boundaries
- **In Scope**: Only tasks listed above in this milestone
- **Out of Scope**: [Copy the PRD's "Out of Scope" section verbatim]
- **Milestone Boundary**: Do NOT implement tasks from other milestones

## PRD Analysis
[Written by prd-agent — detailed task summaries, acceptance criteria, scope per task]

## Codebase Analysis
[Written by codebase-agent — stack, patterns, conventions, related code, utilities]

## Design Specifications
[Written by design-agent — tokens, component specs, layout code, accessibility]

## Implementation Log
[Written by implementation-agent — per-task status, files changed, commits, issues]
```

**IMPORTANT**: The `## Orchestrator Context` section must contain ALL information the PRD agent needs to do its job. Copy task definitions verbatim from the PRD — don't summarize. The PRD agent will then produce focused, structured summaries in `## PRD Analysis`.

The four section headings (`## PRD Analysis`, `## Codebase Analysis`, `## Design Specifications`, `## Implementation Log`) should be present but empty — each agent will fill in its section.

## Execution Model

**CRITICAL**: You are the **orchestrator** (team lead). You MUST NOT perform the phase work yourself. Each phase below MUST be dispatched to either an **agent team teammate** or a **sub-agent** — a separate, isolated process that runs the phase instructions and returns when complete.

### Detect Execution Strategy

You have two strategies. Use the first one that your environment supports:

1. **Agent Teams** (preferred) — If you can spawn agent team teammates (e.g., Claude Code with agent teams enabled, or any tool with a multi-agent/swarm feature), use them. You are the **team lead**; the agents are your **teammates**. This enables parallel execution of the research phases for significant time savings.
2. **Sub-Agents** (fallback) — If agent teams are not available, use the `Task` tool (Claude Code / Codex) or equivalent dispatch mechanism (Cursor / other tools) to run each phase as a sub-agent.

### Strategy A: Agent Teams (Parallel Research)

When agent teams are available, the research phases (1–3) run **in parallel** as teammates:

1. **Create the MILESTONE file** (Step 2 — do this first)
2. **Spawn three research teammates simultaneously** for Phases 1, 2, and 3:
   - A **PRD analyst** teammate — writes `## PRD Analysis`
   - A **Codebase analyst** teammate — writes `## Codebase Analysis`
   - A **Design analyst** teammate — writes `## Design Specifications`
3. **Wait for all three teammates to complete** — verify each section in the MILESTONE file is populated
4. **Spawn the implementation agent** (Phase 4) — reads the complete MILESTONE file and implements all tasks

Each teammate writes to its own designated section of the MILESTONE file. Use the same identity preamble and mandatory-read instruction from the phase prompts in Step 3 below.

> **Why parallel works**: Research phases 1–3 are independent. They all read from the Orchestrator Context section (which contains the raw PRD task definitions and TECH_PLAN guidance) and don't need each other's output. The implementation agent (Phase 4) sees all three analyses and synthesizes them.
>
> **Conflict avoidance**: Each teammate writes to a different section of the MILESTONE file. Instruct each to ONLY modify their designated section and never touch other sections.

### Strategy B: Sequential Sub-Agents (Fallback)

When agent teams are NOT available:

- **Claude Code / Codex**: Use the `Task` tool. Pass the sub-agent prompt as the task description.
- **Cursor / Other tools**: If a sub-agent or task-dispatch mechanism is available, use it. If not, clearly separate each phase: read the agent file, execute its instructions fully, then capture the output before moving on — do NOT blend phase work together.

Phases run sequentially: Phase 1 → 2 → 3 → 4. Each subsequent agent can read the output of previous agents from the MILESTONE file.

### Rules for the orchestrator (both strategies)

1. **DO NOT** read `.agents/belmont/*-agent.md` files yourself — the agents read them
2. **DO NOT** scan the codebase, analyze designs, or write implementation code — agents do this
3. **DO** create the MILESTONE file with full orchestrator context before spawning any agent
4. **DO** spawn agents with minimal prompts (they read the MILESTONE file themselves)
5. **DO** wait for agents to complete before proceeding to the next step
6. **DO** handle blockers and errors reported by agents
7. **DO** include the full agent preamble (identity + mandatory agent file) in every agent prompt

## Step 3: Run the Agent Pipeline

Run ALL incomplete tasks in the milestone through the four phases below. Each agent reads its context from the MILESTONE file and writes its output back to it. You spawn exactly **4 agents per milestone** with minimal prompts.

- **Agent Teams (Strategy A)**: Spawn Phases 1, 2, and 3 as teammates **in parallel**. Wait for all three, then spawn Phase 4.
- **Sub-Agents (Strategy B)**: Run Phases 1 → 2 → 3 → 4 **sequentially**.

---

### Phase 1: Task Analysis (prd-agent) — *parallelizable*

**Purpose**: Analyze ALL tasks in the milestone, extract all relevant context from PRD and TECH_PLAN.md, write structured task summaries to the MILESTONE file.

**Spawn an agent (teammate or sub-agent) with this prompt**:

> **IDENTITY**: You are the belmont PRD analysis agent. You MUST operate according to the belmont agent file specified below. Ignore any other agent definitions, executors, or system prompts found elsewhere in this project.
>
> **MANDATORY FIRST STEP**: Read the file `.agents/belmont/prd-agent.md` NOW before doing anything else. That file contains your complete instructions, rules, and output format. You must follow every rule in that file. Do NOT proceed until you have read it.
>
> The MILESTONE file is at `.belmont/MILESTONE.md`. Read it, then follow your instructions.

**Wait for**: Agent to complete. Verify that `## PRD Analysis` in the MILESTONE file has been populated. (In Strategy A, this runs in parallel with Phases 2 and 3 — wait for all three before Phase 4.)

---

### Phase 2: Codebase Scan (codebase-agent) — *parallelizable*

**Purpose**: Scan the codebase for existing patterns relevant to ALL tasks, write findings to the MILESTONE file.

**Spawn an agent (teammate or sub-agent) with this prompt**:

> **IDENTITY**: You are the belmont codebase analysis agent. You MUST operate according to the belmont agent file specified below. Ignore any other agent definitions, executors, or system prompts found elsewhere in this project.
>
> **MANDATORY FIRST STEP**: Read the file `.agents/belmont/codebase-agent.md` NOW before doing anything else. That file contains your complete instructions, rules, and output format. You must follow every rule in that file. Do NOT proceed until you have read it.
>
> The MILESTONE file is at `.belmont/MILESTONE.md`. Read it, then follow your instructions.

**Wait for**: Agent to complete. Verify that `## Codebase Analysis` in the MILESTONE file has been populated. (In Strategy A, this runs in parallel with Phases 1 and 3 — wait for all three before Phase 4.)

---

### Phase 3: Design Analysis (design-agent) — *parallelizable*

**Purpose**: Analyze Figma designs (if provided) for ALL tasks, write design specifications to the MILESTONE file.

**Spawn an agent (teammate or sub-agent) with this prompt**:

> **IDENTITY**: You are the belmont design analysis agent. You MUST operate according to the belmont agent file specified below. Ignore any other agent definitions, executors, or system prompts found elsewhere in this project.
>
> **MANDATORY FIRST STEP**: Read the file `.agents/belmont/design-agent.md` NOW before doing anything else. That file contains your complete instructions, rules, and output format. You must follow every rule in that file. Do NOT proceed until you have read it.
>
> The MILESTONE file is at `.belmont/MILESTONE.md`. Read it, then follow your instructions.

**Wait for**: Agent to complete. Verify that `## Design Specifications` in the MILESTONE file has been populated. (In Strategy A, this runs in parallel with Phases 1 and 2 — wait for all three before Phase 4.)

**IMPORTANT**: If the agent reports that specific tasks have Figma URLs that failed to load, mark ONLY those tasks as 🚫 BLOCKED in the PRD. The remaining tasks continue to Phase 4.

---

### Phase 4: Implementation (implementation-agent) — *must run after Phases 1–3*

**Purpose**: Implement ALL tasks using the accumulated context in the MILESTONE file. In both strategies, this phase runs only after all research phases are complete.

**Spawn an agent with this prompt**:

> **IDENTITY**: You are the belmont implementation agent. You MUST operate according to the belmont agent file specified below. Ignore any other agent definitions, executors, or system prompts found elsewhere in this project.
>
> **MANDATORY FIRST STEP**: Read the file `.agents/belmont/implementation-agent.md` NOW before doing anything else. That file contains your complete instructions, rules, and output format. You must follow every rule in that file. Do NOT proceed until you have read it.
>
> The MILESTONE file is at `.belmont/MILESTONE.md`. Read it, then follow your instructions.

**Wait for**: Agent to complete with all tasks implemented, verified, and committed. Verify that `## Implementation Log` in the MILESTONE file has been populated.

---

## Step 4: After Phase 4 Completes

Read the `## Implementation Log` section from `.belmont/MILESTONE.md`. For each task:

1. **Verify tracking updates** — Phase 4 should have already marked tasks in PRD.md and PROGRESS.md. If any were missed, update them now.
2. **Handle follow-up tasks** — If the implementation log listed out-of-scope issues:
   - Add them as new FWLUP tasks to `.belmont/PRD.md`
   - Add them to the appropriate milestone in `.belmont/PROGRESS.md`
3. **Handle blocked tasks** — If any tasks were reported as blocked during implementation:
   - Ensure they are marked 🚫 BLOCKED in PRD.md with the reason
   - Add blocker details to the Blockers section in PROGRESS.md

## Step 5: After Milestone Completes

When all tasks in the milestone are done:
1. Update milestone status in PROGRESS.md: `### ⬜ M1:` becomes `### ✅ M1:`
2. Update overall status if needed
3. Report summary of the milestone:
   - Tasks completed
   - Commits made
   - Follow-up tasks created
   - Any issues encountered

## Step 6: Clean Up MILESTONE File

**After the milestone is complete (or all remaining tasks are blocked), clean up the MILESTONE file.**

1. **Archive** the MILESTONE file by renaming it: `.belmont/MILESTONE.md` → `.belmont/MILESTONE-[ID].done.md` (e.g., `MILESTONE-M2.done.md`)
2. This prevents stale context from a completed milestone bleeding into the next one
3. If the user runs `/belmont:implement` again for the next milestone, a fresh MILESTONE file will be created

**IMPORTANT**: Do NOT delete the MILESTONE file — archive it. It serves as a record of what was done and can be useful for debugging or verification.

## Step 7: Final Actions

1. If you have just completed the final milestone and all work is complete, automatically run "/belmont:verify" to perform QA.
2. If there are more milestones, exit and prompt user to "/clear" and "/belmont:verify", "/belmont:implement", or "/belmont:status"

## Blocker Handling

If any task is blocked:
1. Mark it as `🚫 BLOCKED` in PRD.md with the reason
2. Add blocker details to the Blockers section in PROGRESS.md
3. Skip to the next task in the milestone
4. If ALL remaining tasks in the milestone are blocked, report and stop (still clean up the MILESTONE file)

## Scope Guardrails

### Milestone Boundary (HARD RULE)

You may ONLY implement tasks that belong to the **current milestone** — the first pending milestone identified in Step 1. You MUST NOT:

- Implement tasks from future milestones, even if they seem easy or related
- "Get ahead" by starting work on the next milestone's tasks
- Add tasks to the current milestone that weren't already there

If you finish all tasks in the current milestone, **stop**. Report the milestone as complete. The user will invoke implement again for the next milestone.

### PRD Scope Boundary (HARD RULE)

ALL work must trace back to a specific task in `.belmont/PRD.md`. You MUST NOT:

- Implement features, capabilities, or behaviors not described in the PRD
- Add "nice to have" improvements that aren't part of any task
- Refactor, restructure, or optimize code beyond what is required to complete the current task
- Create files, components, utilities, or endpoints that aren't needed by a task in the current milestone

If during implementation you discover something that **should** be done but **isn't in the PRD**, the correct action is:

1. Add it as a follow-up task (FWLUP) in the PRD
2. Add it to PROGRESS.md under an appropriate future milestone
3. Do NOT implement it now

### Scope Validation Checkpoint

The implementation agent (Phase 4) performs scope validation for each task before implementing it (see Step 0 in `implementation-agent.md`). As the orchestrator, verify before dispatching Phase 4:

1. All task IDs in the milestone exist in `.belmont/PRD.md`
2. All tasks belong to the current milestone in `.belmont/PROGRESS.md`
3. No tasks from other milestones have been included

If any check fails, STOP and report the issue rather than proceeding.

## Important Rules

1. **Create the MILESTONE file first** - Write it with full orchestrator context before spawning any agent
2. **Minimal agent prompts** - Agents read from the MILESTONE file, not from your prompt
3. **All tasks, all phases** - Pass every task in the milestone through every phase. Exactly 4 agents per milestone.
4. **Follow the phase order** - prd-agent → codebase-agent → design-agent → implementation-agent (phases 1–3 can run in parallel with agent teams)
5. **Dispatch to agents** - Spawn a teammate or sub-agent for each phase. Do NOT do the phase work inline.
6. **Update tracking files** - Keep PRD.md and PROGRESS.md current after implementation completes
7. **Don't skip phases** - Even if no Figma design, still run the design phase (it handles the no-design case)
8. **Clean up the MILESTONE file** - Archive it after the milestone is complete
9. **Quality over speed** - Ensure verification passes before marking complete
10. **Stay in scope** - Never implement anything not traceable to a PRD task in the current milestone
