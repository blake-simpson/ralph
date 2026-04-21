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
- `{base}/models.yaml` - Per-feature model tiers (if exists — see "Model Tiers" below)

Optional helper:
- If the CLI is available, `belmont status --format json` can provide a quick summary of milestones/tasks. Still read the files above for full context.

## Model Tiers

Per-agent model tiers (low/medium/high) are defined in `{base}/models.yaml`. If that file is absent, each agent uses its frontmatter default (Sonnet for most, Opus for reconciliation) and you can skip the rest of this section.

### Model Tier Registry

Belmont uses three user-facing tiers — `low`, `medium`, `high` — which map to concrete model identifiers per AI CLI. When you need to pass a model override explicitly (see `dispatch-strategy.md` Model Tier Overrides or `tier-preflight.md`), translate via this table.

| Tier   | Claude  | Codex          | Gemini                | Cursor             | Copilot              |
|--------|---------|----------------|-----------------------|--------------------|----------------------|
| low    | haiku   | gpt-5.4-mini   | gemini-2.5-flash-lite | sonnet-4           | haiku-4.5            |
| medium | sonnet  | gpt-5.3-codex  | gemini-2.5-flash      | sonnet-4-thinking  | claude-sonnet-4.5    |
| high   | opus    | gpt-5.4        | gemini-2.5-pro        | gpt-5              | gpt-5.4              |

The canonical source is the `modelTiers` map in `cmd/belmont/main.go`. If this table drifts from the Go registry, the Go registry wins — file an issue and update this partial. `scripts/generate-skills.sh --check` is the place to add a drift guard.

### Model Tier Preflight (non-Claude CLIs)

Non-Claude CLIs (Codex, Gemini, Cursor, Copilot) run the entire skill in a single top-level session at whichever model the session was started with — there's no sub-agent dispatch to override mid-session. Before doing any heavy work, compare the **required tier** for the current skill to the **session's current model** and surface a warning if they diverge. Do NOT block execution; let the user decide.

**Workflow at start-of-skill (non-Claude only)**:

1. **Read** `.belmont/features/<slug>/models.yaml`. If absent, skip this preflight (defaults apply).
2. **Determine the required tier for this skill**:
   - `implement` → `tiers.implementation`
   - `verify` → `tiers.verification`
   - `code-review` (if applicable) → `tiers.code-review`
   - others → skip preflight unless the skill specifies its own tier.
3. **Map the required tier to a model ID for the current CLI** using `tier-registry.md`.
4. **Compare to the session's current model**:
   - Codex: run `/model` or check session settings.
   - Gemini: check `/model`.
   - Cursor: check `/model`.
   - Copilot: check `/model`.
5. **If they diverge**, print this warning block before doing any further work:

   ```
   ⚠ Model tier mismatch
   models.yaml says this phase should run at <tier> (<expected-model-id>).
   Your session is currently on <current-model-id>.
   To honor the tier, restart with: <cli> --model <expected-model-id>
   Continuing with the current model. Re-dispatching sub-agents with a
   different model is not supported on this CLI.
   ```

6. **Proceed with the skill**. The warning is informational; it never blocks execution.

**Why this is acceptable graceful degradation**: the user chose this CLI knowing it doesn't support per-agent dispatch. The warning gives them a one-command fix if they want tier adherence; otherwise the work proceeds at the session's model. Only Claude Code supports true per-agent overrides — see `dispatch-strategy.md` Model Tier Overrides for that path.

When dispatching sub-agents (Step 3 below), apply the tier overrides per `dispatch-strategy.md → Model Tier Overrides`. Specifically: for each Task call, if the corresponding agent has an entry in `models.yaml` `tiers:`, include `model: "<alias>"` in the Task call using the tier-registry mapping. Agents not listed in `models.yaml` inherit their frontmatter default — do NOT pass `model:` for those.

## Step 1: Find Next Milestone

1. Read `{base}/PROGRESS.md` and find the Milestones section
2. A milestone is **complete** if all its tasks are marked `[v]` (verified)
3. A milestone is **pending** if any task is `[ ]`, `[>]`, `[x]`, or `[!]`
4. Select the **first pending milestone**
5. If all milestones are complete, report "All milestones complete!" and stop

## Step 2: Create the MILESTONE File

Write a structured MILESTONE file that all agents read from and write to. The MILESTONE file is a **coordination document**: it names the active tasks and points sub-agents at the canonical PRD and TECH_PLAN, which each sub-agent reads directly.

**Read `references/implement-milestone-template.md` for the exact structure, then write `{base}/MILESTONE.md` using that template.** Fill in the `## Orchestrator Context` section using information from PROGRESS.md and the user's invocation context.

**Mandatory rules while writing MILESTONE** (these MUST be observed every run — they are not in the reference file because they can never be skipped):

- **Do NOT copy PRD or TECH_PLAN content into MILESTONE.** The pointers in `### File Paths` are enough. Duplicating content wastes context across every sub-agent invocation.
- **`### Active Task IDs` lists IDs only** (e.g. `P0-1, P0-2`). The PRD holds the full definitions.
- **The three sub-agent-written sections (`## Codebase Analysis`, `## Design Specifications`, `## Implementation Log`) remain the source of truth for downstream agents** — they ARE written into MILESTONE and ARE read by Phase 3. Only the PRD/TECH_PLAN content is externalised; the sub-agent hand-off data stays inside MILESTONE. Leave these three headings present but empty; each agent will fill in its section.

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
   - Do **NOT** set `run_in_background: true` — foreground parallel tasks return results directly; background tasks require `TaskOutput` polling which is fragile and can lose contact with sub-agents.
3. Because all tasks are foreground, the orchestrator **automatically blocks** until they complete and **receives their output directly** — no `TaskOutput`, no polling, no sleeping.
4. **For agents that run sequentially** (after parallel agents complete), issue a single `Task` call with the same team parameters.
5. **Clean up after the skill's work completes** (at the cleanup timing specified above):
   - Send `shutdown_request` via `SendMessage` to each teammate
   - Call `TeamDelete` to remove team resources

#### Approach B: Parallel Foreground Sub-Agents

**Required tools**: `Task`

If `Task` is available but `TeamCreate` is NOT:

1. **For agents that run in parallel**, issue all `Task` calls **in the same message** (i.e., as parallel tool calls). All calls use:
   - `subagent_type`: `"general-purpose"` (all belmont agents need full tool access including file editing and bash)
   - `mode`: `"bypassPermissions"`
   - Do **NOT** set `run_in_background: true` — foreground parallel tasks return results directly; background tasks require `TaskOutput` polling which is fragile and can lose contact with sub-agents.
2. Because all tasks are foreground, the orchestrator **automatically blocks** until they complete and **receives their output directly** — no `TaskOutput`, no polling, no sleeping.
3. **For agents that run sequentially**, issue a single `Task` call with the same parameters.

No team cleanup needed.

#### Approach C: Sequential Inline Execution (fallback)

If neither `TeamCreate` nor `Task` is available:

1. For each agent, read its agent file (e.g., `.agents/belmont/<agent-name>.md`)
2. Execute its instructions fully within your own context
3. Complete all output before moving to the next agent
4. Do NOT blend agent work together — finish one completely before starting the next

### Model Tier Overrides (Claude Code only)

Each Belmont agent has a default model in its frontmatter (`model: sonnet` / `model: opus`). When running on Claude Code with Approach A or B, you can override that default per-dispatch via the Task tool's `model:` parameter — this takes precedence over frontmatter.

**When to pass `model:`**: read `.belmont/features/<slug>/models.yaml` at start-of-skill (if it exists) and translate each agent's tier into the appropriate model alias for this session:

- `low` → `haiku`
- `medium` → `sonnet`
- `high` → `opus`

Then include `model: "<alias>"` in the Task call for each agent whose tier appears in `models.yaml`. Agents not listed in `models.yaml` inherit their frontmatter default — do NOT pass `model:` for those.

Example (Approach A):
```
Task(team_name: "...", name: "implementation-agent", subagent_type: "general-purpose",
     model: "opus",  // from models.yaml: tiers.implementation = high
     mode: "bypassPermissions", prompt: "...")
```

**If `models.yaml` is absent**, omit `model:` entirely — agent frontmatter defaults apply.

**Non-Claude CLIs** (Codex, Gemini, Cursor, Copilot): they don't have a Task-tool-style sub-agent dispatch, so mid-session model override is impossible. Use the preflight partial (`tier-preflight.md`) instead, which surfaces a warning if the session model doesn't match the tier the skill expects.

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

Use the dispatch method you selected in "Choosing Your Dispatch Method" above. For the **Agent Teams** method (Approach A), create the team first, then issue parallel `Task` calls. For the **Parallel Task** method (Approach B), issue parallel `Task` calls directly. For the **Sequential Inline** fallback (Approach C), execute each agent's instructions inline, finishing one completely before starting the next.

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

### Tear down team (Agent Teams method only)
If you created a team:
1. Send `shutdown_request` via `SendMessage` to each teammate still active
2. Wait for shutdown confirmations
3. Call `TeamDelete` to remove team resources

Skip this if you used the Parallel Task method or the Sequential Inline fallback.

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

### Milestone Boundary (HARD RULE)

You may ONLY implement tasks that belong to the **current milestone** — the first pending milestone identified in Step 1. You MUST NOT:

- Implement tasks from future milestones, even if they seem easy or related
- "Get ahead" by starting work on the next milestone's tasks
- Add tasks to the current milestone that weren't already there

If you finish all tasks in the current milestone, **stop**. Report the milestone as complete. The user will invoke implement again for the next milestone.

### PRD Scope Boundary (HARD RULE)

ALL work must trace back to a specific task in `{base}/PRD.md`. You MUST NOT:

- Implement features, capabilities, or behaviors not described in the PRD
- Add "nice to have" improvements that aren't part of any task
- Refactor, restructure, or optimize code beyond what is required to complete the current task
- Create files, components, utilities, or endpoints that aren't needed by a task in the current milestone

If during implementation you discover something that **should** be done but **isn't in the PRD**, the correct action is:

1. Add it as a new `[ ]` task in the appropriate milestone in PROGRESS.md
2. Do NOT implement it now

### Scope Validation Checkpoint

The implementation agent (Phase 3) performs scope validation for each task before implementing it (see Step 0 in `implementation-agent.md`). As the orchestrator, verify before dispatching Phase 3:

1. All task IDs in the milestone exist in `{base}/PRD.md`
2. All tasks belong to the current milestone in `{base}/PROGRESS.md`
3. No tasks from other milestones have been included

If any check fails, STOP and report the issue rather than proceeding.

## Important Rules

1. **Create the MILESTONE file first** - Write it with active task IDs and file-path pointers to PRD/TECH_PLAN before spawning any agent. Do NOT copy PRD/TECH_PLAN content verbatim.
2. **MILESTONE is the coordination hub** - It lists active tasks and points sub-agents at the PRD/TECH_PLAN, which they read directly. Sub-agents fetch their own task definitions and technical specs from the canonical files.
3. **Minimal agent prompts** - Agents read from the MILESTONE file (and the PRD/TECH_PLAN it points at), not from your prompt

Additional operational rules (phase ordering, cleanup, blocker handling, quality gates) are in `references/implement-important-rules.md`. Read it for the full operational checklist.
