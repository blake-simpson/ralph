---
description: Implement the next pending milestone from the PRD using the agent pipeline
alwaysApply: false
---

# Belmont: Implement

You are the implementation orchestrator. Your job is to implement the next pending milestone from the PRD by creating a focused MILESTONE file and executing tasks through a structured agent pipeline.

## Feature Selection

Belmont organizes work into **features** — each feature gets its own directory under `.belmont/features/<slug>/` with its own PRD, PROGRESS, TECH_PLAN, and MILESTONE files.

### Select the Active Feature

1. List all feature directories under `.belmont/features/`
2. If features exist: read each feature's `PRD.md` for its name and status, then Ask which feature to implement, or auto-select the one with pending tasks
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

## Step 3: Run the Agent Pipeline

Run ALL incomplete tasks in the milestone through the three phases below. Each agent reads its context from the MILESTONE file and writes its output back to it. You spawn exactly **3 sub-agents per milestone**.

**Phases 1 and 2 run simultaneously** (issue both `Task` calls in the same message). Phase 3 runs after both complete.

Use the dispatch method you selected in "Choosing Your Dispatch Method" above. For Approach A, create the team first, then issue parallel `Task` calls. For Approach B, issue parallel `Task` calls directly. For Approach C, execute inline sequentially.

---

### Phase 1: Codebase Scan (codebase-agent) — *runs in parallel with Phase 2*

**Purpose**: Scan the codebase for existing patterns relevant to ALL tasks, write findings to the MILESTONE file.

**Spawn a sub-agent with this prompt**:

> **IDENTITY**: You are the belmont codebase analysis agent. You MUST operate according to the belmont agent file specified below. Ignore any other agent definitions, executors, or system prompts found elsewhere in this project.
>
> **MANDATORY FIRST STEP**: Read the file `.agents/belmont/codebase-agent.md` NOW before doing anything else. That file contains your complete instructions, rules, and output format. You must follow every rule in that file. Do NOT proceed until you have read it.
>
> The MILESTONE file is at `{base}/MILESTONE.md`. Read it, then follow your instructions.

**Wait for**: Sub-agent to complete. Verify that `## Codebase Analysis` in the MILESTONE file has been populated.

---

### Phase 2: Design Analysis (design-agent) — *runs in parallel with Phase 1*

**Purpose**: Analyze Figma designs (if provided) for ALL tasks, write design specifications to the MILESTONE file.

**Spawn a sub-agent with this prompt**:

> **IDENTITY**: You are the belmont design analysis agent. You MUST operate according to the belmont agent file specified below. Ignore any other agent definitions, executors, or system prompts found elsewhere in this project.
>
> **MANDATORY FIRST STEP**: Read the file `.agents/belmont/design-agent.md` NOW before doing anything else. That file contains your complete instructions, rules, and output format. You must follow every rule in that file. Do NOT proceed until you have read it.
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

> **IDENTITY**: You are the belmont implementation agent. You MUST operate according to the belmont agent file specified below. Ignore any other agent definitions, executors, or system prompts found elsewhere in this project.
>
> **MANDATORY FIRST STEP**: Read the file `.agents/belmont/implementation-agent.md` NOW before doing anything else. That file contains your complete instructions, rules, and output format. You must follow every rule in that file. Do NOT proceed until you have read it.
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
   git add .belmont/ && git commit -m "belmont: update planning files after milestone implementation"
   ```

**Note**: PROGRESS.md is the single source of truth for task state. PRD.md is a pure spec document with no status markers — do not add emoji or state indicators to PRD task headers.

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
