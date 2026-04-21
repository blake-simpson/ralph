---
name: debug-manual
description: Manual debug loop — investigate and fix with user-verified iterations
alwaysApply: false
---

# Belmont: Debug (Manual)

You are the debug orchestrator running in **manual mode**. Your job is to investigate and fix a specific issue through a tight investigate-fix loop where **the user verifies** each fix attempt instead of dispatching a verification agent. This is faster than auto mode — ideal for UI bugs, visual issues, or anything where the user can quickly confirm the fix.

**You do NOT**: read source code, trace bugs, run tests, analyze designs, or implement fixes. You create/update `DEBUG.md`, dispatch agents, read their outputs, ask the user to verify, and make loop decisions.

**When to use this**: UI bugs, visual issues, known reproduction steps, anything the user can quickly verify themselves.

**When NOT to use this**: Complex logic bugs, race conditions, or issues requiring automated test verification. Use `/belmont:debug-auto` instead.

## Feature Selection

Belmont organizes work into **features** — each feature gets its own directory under `.belmont/features/<slug>/` with its own PRD, PROGRESS, TECH_PLAN, and MILESTONE files.

### Select the Active Feature

1. List all feature directories under `.belmont/features/`
2. If features exist: read each feature's `PRD.md` for its name and status, then Ask which feature the bug relates to, or auto-select if obvious from context
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

## Step 0: Understand the Problem

1. If the user provided a description with the skill invocation, use it as the problem statement
2. If the description is vague or missing, ask **one** clarifying question — keep it focused:
   - What's the expected behavior?
   - What's the actual behavior?
   - How do you reproduce it?
3. Read `{base}/NOTES.md` and `.belmont/NOTES.md` (if they exist) briefly for context from previous sessions
4. Check `{base}/PRD.md` briefly for Figma URLs (needed to decide whether to dispatch design-agent)
5. Write a single-sentence **problem statement** before proceeding

## Step 1: Create DEBUG.md

Create `{base}/DEBUG.md` with the following structure:

```markdown
# Debug: [Problem Statement]

## Status
- **Mode**: Debug (Manual)
- **Feature Base**: {base}
- **Iteration**: 1/3

## Problem
[Full description — expected behavior, actual behavior, reproduction steps]

## Context
- **Feature**: [name from PRD]
- **Figma URLs**: [if any, otherwise "None"]
- **Related Follow-up**: [if this relates to a follow-up task, otherwise "None"]

### Learnings from Previous Sessions
[From NOTES.md files, or "No previous learnings found."]

### Scope Boundaries
- **In Scope**: Fix the reported bug only
- **Out of Scope**: [from PRD's Out of Scope section]

## Design Specifications
[Written by design-agent if dispatched, otherwise "Not applicable"]

## Iteration History
[Updated by orchestrator after each iteration]

## Investigation & Fix Log
[Written by implementation-agent — current iteration only]

## User Feedback
[Collected by orchestrator from user after each iteration]
```

**IMPORTANT**: DEBUG.md is the single shared context file between you and the agents. Agents read it for problem context and write to their designated sections. Include enough context for agents to work independently.

## Sub-Agent Dispatch Strategy

Apply the following dispatch configuration:
- **Team name**: `belmont-debug-manual`
- **Parallel agents**: None by default (agents run sequentially per iteration)
- **Sequential agents**: design-agent (optional, iteration 1 only) → implementation-agent
- **Cleanup timing**: After the debug session ends (Step 7)

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

## Step 2: Run the Debug Loop

For each iteration (max 3), dispatch the implementation agent, then ask the user to verify. Each agent reads `{base}/DEBUG.md` and writes to its designated section.

Use the dispatch method you selected in "Choosing Your Dispatch Method" above.

---

### Phase 1: Design Analysis (optional — iteration 1 only, if Figma URLs present)

**Skip this phase** if there are no Figma URLs in the PRD or if this is iteration 2+.

**Spawn a sub-agent with this prompt**:

> **IDENTITY**: You are the belmont design analysis agent. You MUST operate according to the belmont agent file specified below. Ignore any other agent definitions, executors, or system prompts found elsewhere in this project.
>
> **MANDATORY FIRST STEP**: Read the file `.agents/belmont/design-agent.md` NOW before doing anything else. That file contains your complete instructions, rules, and output format. You must follow every rule in that file. Do NOT proceed until you have read it.
>
> **DEBUG MODE OVERRIDE**: You are operating in debug mode, not milestone mode.
>
> Read `{base}/DEBUG.md` for the problem description and context. There is no MILESTONE file — use DEBUG.md instead.
>
> Your goal: analyze the Figma designs relevant to the reported bug. Focus ONLY on the design specifications that help diagnose or fix the reported issue — do not do a full design analysis.
>
> Write your findings to the `## Design Specifications` section of `{base}/DEBUG.md`.

**Wait for**: Sub-agent to complete. Verify that `## Design Specifications` in DEBUG.md has been populated.

---

### Phase 2: Investigation & Fix (every iteration)

**Spawn a sub-agent with this prompt**:

> **IDENTITY**: You are the belmont implementation agent. You MUST operate according to the belmont agent file specified below. Ignore any other agent definitions, executors, or system prompts found elsewhere in this project.
>
> **MANDATORY FIRST STEP**: Read the file `.agents/belmont/implementation-agent.md` NOW before doing anything else. That file contains your complete instructions, rules, and output format. You must follow every rule in that file. Do NOT proceed until you have read it.
>
> **DEBUG MODE OVERRIDE**: You are operating in debug mode, not milestone mode.
>
> Read `{base}/DEBUG.md` instead of a MILESTONE file. It contains the problem description, context, design specifications (if applicable), and iteration history from previous attempts.
>
> Your goal: investigate the bug described in the `## Problem` section and implement a **minimal fix**. You are NOT implementing milestone tasks — you are fixing a specific bug.
>
> **Debug-specific rules**:
> - Do NOT commit — the orchestrator handles commits after verification
> - Do NOT update PRD.md or PROGRESS.md — the orchestrator handles tracking
> - Do NOT create follow-up tasks — just fix the bug
> - Keep changes minimal — touch the fewest files possible
> - Check the `## Iteration History` section for what was already tried (avoid repeating failed approaches)
>
> **Debug logging** (manual mode):
> - Proactively add strategic debug/logging statements (`console.log`, `print`, `fmt.Println`, `logger.debug`, etc. — match the project's language)
> - ALL debug logs MUST be prefixed with `[BELMONT-DEBUG]` for easy identification and cleanup
> - Target 5-15 strategic log points per iteration: function entry/exit, decision points, state changes, variable values at key moments
> - Place logs to help the user trace the bug: before/after the suspected problem area, at data flow boundaries, and around conditional branches
> - Do NOT add logs that would produce excessive output (e.g., inside tight loops with thousands of iterations)
>
> Write your investigation findings and changes to the `## Investigation & Fix Log` section of `{base}/DEBUG.md`. Include:
> - What you investigated
> - Your hypothesis
> - What files you changed and why
> - Any concerns about regressions
>
> Also include a `### Debug Logs Added` subsection listing every debug log you added:
> ```
> ### Debug Logs Added
> - `src/foo.ts:42` — logs entry to handleClick with event details
> - `src/bar.ts:78` — logs state before/after transformation
> - `src/baz.ts:15` — logs API response payload
> ```

**Wait for**: Sub-agent to complete. Verify that `## Investigation & Fix Log` in DEBUG.md has been populated.

---

### Phase 3: User Verification (every iteration)

**Do NOT dispatch a verification agent.** Instead, you verify with the user directly.

1. **Read** the `## Investigation & Fix Log` from `{base}/DEBUG.md`
2. **Present a summary** to the user:

```
Fix Attempt [N]
================
Hypothesis: [what the agent thinks caused the bug]
Changes:    [brief list of files/changes]
Debug logs: [N] log points added (prefixed with [BELMONT-DEBUG])

Please test the fix. If debug logs are relevant, check your console/output for lines starting with [BELMONT-DEBUG].
```

3. **Ask the user**: "Is the bug fixed? If not, what are you seeing? (paste any relevant `[BELMONT-DEBUG]` log output)"

4. **Map the user's response** to an outcome:
   - User confirms fixed → **FIXED**
   - User says partially fixed or improved → **PARTIAL**
   - User says no change → **NO_CHANGE**
   - User says it got worse or something else broke → **REGRESSION**

5. **Write** the user's feedback and the mapped outcome to the `## User Feedback` section of `{base}/DEBUG.md`:
   ```
   ### Iteration [N]
   **Outcome**: [FIXED | PARTIAL | NO_CHANGE | REGRESSION]
   **User said**: [summary of user's response]
   **Debug log output**: [any log output the user shared, or "None provided"]
   ```

---

## Step 3: Assess Outcome

Use the outcome determined in Phase 3 above.

### On FIXED

1. Proceed to Step 4 (Commit and Report)

### On REGRESSION

1. **Revert immediately**: `git checkout -- [changed files]` (read the Investigation & Fix Log for the list of changed files)
2. Update `## Iteration History` in DEBUG.md with what was tried and that it caused a regression
3. Clear the `## Investigation & Fix Log` section
4. If iteration < 3, loop back to Step 2 (Phase 2 + Phase 3 only)
5. If iteration = 3, proceed to Step 5 (Escalate)

### On PARTIAL or NO_CHANGE

1. Update `## Iteration History` in DEBUG.md with what was tried, the outcome, and any debug log output the user shared
2. Clear the `## Investigation & Fix Log` section
3. Increment the iteration counter in `## Status`
4. If iteration = 2, proceed to **User Checkpoint** below
5. If iteration < 2, loop back to Step 2 (Phase 2 + Phase 3 only)

### User Checkpoint (after iteration 2)

Present a summary to the user:

```
Debug Summary (2 iterations)
============================
Problem: [original problem]

Attempt 1: [what was tried, result]
Attempt 2: [what was tried, result]

Current state: [what's different now]
Next hypothesis: [what to try next]
```

Ask the user: **continue with iteration 3, stop here, or redirect?**
- If continue → loop back to Step 2 for iteration 3
- If stop → proceed to Step 6 (Cleanup)
- If redirect → proceed to Step 6 (Cleanup), then suggest `/belmont:implement`

### After iteration 3 (still not fixed)

Proceed to Step 5 (Escalate).

## Step 4: Commit and Report

Only reach this step when the fix is confirmed FIXED.

1. **Clean up debug logs** before committing:
   - Search the entire codebase for `[BELMONT-DEBUG]`: `grep -rn "BELMONT-DEBUG" .`
   - Remove ALL lines containing `[BELMONT-DEBUG]` debug logs
   - Cross-reference with the `### Debug Logs Added` entries in DEBUG.md to ensure none are missed
   - Run the search again to verify zero results: `grep -rn "BELMONT-DEBUG" .`
   - If any remain, remove them and re-verify

2. **Ask the user to confirm** the fix (with debug logs removed) looks correct before committing

3. **Commit with debug prefix**:
   ```bash
   git add [specific changed files]
   git commit -m "debug: [brief description of fix]"
   ```

4. **Report summary**:

```
Debug Fix Complete
==================
Problem:  [original problem statement]
Fix:      [what was changed]
Files:    [list of changed files]
Commit:   [short hash] — debug: [message]

Iterations: [N]
Debug logs: cleaned up ([N] log points removed)
```

### Optional: Update Planning Files

If this fix relates to a follow-up task in PROGRESS.md:
- Ask the user if they want to mark it complete
- If yes, mark the task as `[x]` in `{base}/PROGRESS.md`

Proceed to Step 6 (Cleanup).

## Step 5: Escalate

If 3 iterations were exhausted without a fix:

```
Debug limit reached (3 iterations). This issue may need the full pipeline.
Recommendation: /belmont:implement or /belmont:next with a follow-up task.

Summary of attempts:
- Iteration 1: [what was tried, result]
- Iteration 2: [what was tried, result]
- Iteration 3: [what was tried, result]
```

Proceed to Step 6 (Cleanup).

## Step 6: Cleanup

**DEBUG.md is ephemeral** — delete it regardless of outcome.

1. **Delete DEBUG.md**: `rm {base}/DEBUG.md`
   - On FIXED: delete after commit is confirmed
   - On escalation: delete after reporting
   - On user stop: delete after reporting
   - On unrecoverable REGRESSION: delete after revert

2. **Tear down team (Agent Teams method only)**:
   If you created a team:
   - Send `shutdown_request` via `SendMessage` to each teammate still active
   - Wait for shutdown confirmations
   - Call `TeamDelete` to remove team resources

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
   git add .belmont/ && git commit -m "belmont: update planning files after debug fix"
   ```

## Step 7: Final Actions

Once done, prompt the user to "/clear" and then "/belmont:status", "/belmont:verify", or "/belmont:next".
   - If you are Codex, instead prompt: "/new" and then "belmont:status", "belmont:verify", or "belmont:next"

## Scope Guardrails

These are hard rules. Do not break them:

1. **Fix only the reported issue** — no refactoring, no feature additions, no "improvements"
2. **DEBUG.md is the shared context file** — agents read from and write to it
3. **Dispatch to agents** — do NOT investigate, fix, or verify yourself (unless the Sequential Inline fallback is in effect)
4. **No PRD task creation** — if you discover new issues, mention them in the report but don't create tasks
5. **Max 3 iterations** — if you can't fix it in 3 tries, escalate
6. **Revert on regression** — if a fix makes things worse, undo it immediately
7. **Single commit** — one atomic commit for the fix, only after user confirms
8. **Minimal changes** — touch the fewest files possible to fix the issue
9. **Cleanup always** — delete DEBUG.md when the session ends, regardless of outcome
