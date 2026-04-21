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
- `{base}/models.yaml` - Per-feature model tiers (if exists — see "Model Tiers" below)

Also check for archived MILESTONE files (`{base}/MILESTONE-*.done.md`) — these contain the implementation context from the most recent milestone and can provide useful reference for verification.

Optional helper:
- If the CLI is available, `belmont status --format json` can provide a quick summary of completed tasks. Still read the files above for full context.

## Model Tiers

Per-agent model tiers (low/medium/high) are defined in `{base}/models.yaml`. If that file is absent, each agent uses its frontmatter default (Sonnet for most) and you can skip the rest of this section.

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

When dispatching the verification-agent and code-review-agent below, apply the tier overrides per `dispatch-strategy.md → Model Tier Overrides`. Specifically: if `models.yaml` lists `tiers.verification` or `tiers.code-review`, include `model: "<alias>"` in the corresponding Task call using the tier-registry mapping. Agents not listed inherit their frontmatter default — do NOT pass `model:` for those.

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

## Step 2: Run Verification and Code Review

Use the dispatch method you selected above. For the **Agent Teams** method (Approach A), create the team first, then issue both `Task` calls in the same message. For the **Parallel Task** method (Approach B), issue both `Task` calls in the same message. For the **Sequential Inline** fallback (Approach C), execute each agent's instructions inline, finishing one completely before starting the next.

Spawn these two sub-agents **simultaneously** (or sequentially if using the Sequential Inline fallback):

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

> **IDENTITY**: You are the belmont code review agent. You MUST operate according to the belmont agent file specified below. Ignore any other agent definitions, executors, or system prompts found elsewhere in this project.
>
> **MANDATORY FIRST STEP**: Read the file `.agents/belmont/code-review-agent.md` NOW before doing anything else. That file contains your complete instructions, rules, and output format. You must follow every rule in that file. Do NOT proceed until you have read it.
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

When running: **read `references/verify-five-whys.md`** for the Five Whys framework, the grouping rule, and the `NOTES.md` entry format. Append each resulting entry to `{base}/NOTES.md` under `## Root Cause Patterns`.

### Determine Overall Status and Write the Report

**Read `references/verify-report-format.md` and use its template to produce the final summary output.** It contains the overall-status decision rules (ALL PASSED vs ISSUES FOUND vs CRITICAL ISSUES) and the combined markdown summary template.

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

## Step 4: Clean Up Team (Agent Teams method only)

If you created a team in Step 2:
1. Send `shutdown_request` via `SendMessage` to each teammate still active
2. Wait for shutdown confirmations
3. Call `TeamDelete` to remove team resources

Skip this step if you used the Parallel Task method or the Sequential Inline fallback.

## Important Rules

1. **Run both agents** - Always run verification AND code review
2. **Be thorough** - Check all completed tasks, not just the latest
3. **Create follow-ups only for Critical/Warning** - Only these tiers become follow-up tasks. Polish items go to NOTES.md. Suggestions are reported but not persisted.
4. **Don't fix issues yourself** - Report them and create follow-up tasks
5. **Update PROGRESS.md** - Mark verified tasks `[v]`, add follow-up `[ ]` tasks for issues
6. **Polish doesn't block** - If only Polish/Suggestion items are found, all tasks are marked `[v]` and overall status is ALL PASSED

Once done, prompt the user to "/clear" and then "/belmont:status", "/belmont:next", or "/belmont:implement"
   - If you are Codex, instead prompt: "/new" and then "belmont:status", "belmont:next", or "belmont:implement"
