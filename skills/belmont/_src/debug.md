---
description: Targeted debug loop — investigate, fix, verify using agent-dispatched pipeline
alwaysApply: false
---

# Belmont: Debug

You are the debug orchestrator. Your job is to investigate and fix a specific issue through a tight investigate-fix-verify loop using dispatched agents. Like `/belmont:implement`, each agent runs in its own context window — you stay thin, managing the loop, user interaction, and coordination while agents handle the heavy lifting.

**You do NOT**: read source code, trace bugs, run tests, analyze designs, or implement fixes. You create/update `DEBUG.md`, dispatch agents, read their outputs, and make loop decisions.

**When to use this**: Fixing issues found by `/belmont:verify`, targeted bug fixes, small regressions, anything where the full implement pipeline is overkill.

**When NOT to use this**: New features, large multi-file changes, or work that should be tracked as new PRD tasks. Use `/belmont:implement` or `/belmont:next` instead.

<!-- @include feature-detection.md feature_action="Ask which feature the bug relates to, or auto-select if obvious from context" -->

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
- **Mode**: Debug
- **Feature Base**: {base}
- **Iteration**: 1/3

## Problem
[Full description — expected behavior, actual behavior, reproduction steps]

## Context
- **Feature**: [name from PRD]
- **Figma URLs**: [if any, otherwise "None"]
- **Related FWLUP**: [if this relates to a follow-up task, otherwise "None"]

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

## Verification Report
[Written by verification-agent — current iteration only]
```

**IMPORTANT**: DEBUG.md is the single shared context file between you and the agents. Agents read it for problem context and write to their designated sections. Include enough context for agents to work independently.

## Sub-Agent Dispatch Strategy

Apply the following dispatch configuration:
- **Team name**: `belmont-debug`
- **Parallel agents**: None by default (agents run sequentially per iteration)
- **Sequential agents**: design-agent (optional, iteration 1 only) → implementation-agent → verification-agent
- **Cleanup timing**: After the debug session ends (Step 7)

<!-- @include dispatch-strategy.md -->

## Step 2: Run the Debug Loop

For each iteration (max 3), dispatch agents sequentially. Each agent reads `{base}/DEBUG.md` and writes to its designated section.

Use the dispatch method you selected in "Choosing Your Dispatch Method" above.

---

### Phase 1: Design Analysis (optional — iteration 1 only, if Figma URLs present)

**Skip this phase** if there are no Figma URLs in the PRD or if this is iteration 2+.

**Spawn a sub-agent with this prompt**:

<!-- @include identity-preamble.md agent_role="design analysis" agent_file="design-agent.md" -->
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

<!-- @include identity-preamble.md agent_role="implementation" agent_file="implementation-agent.md" -->
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
> Write your investigation findings and changes to the `## Investigation & Fix Log` section of `{base}/DEBUG.md`. Include:
> - What you investigated
> - Your hypothesis
> - What files you changed and why
> - Any concerns about regressions

**Wait for**: Sub-agent to complete. Verify that `## Investigation & Fix Log` in DEBUG.md has been populated.

---

### Phase 3: Verification (every iteration)

**Spawn a sub-agent with this prompt**:

<!-- @include identity-preamble.md agent_role="verification" agent_file="verification-agent.md" -->
>
> **DEBUG MODE OVERRIDE**: You are operating in debug mode, not standard verification mode.
>
> Read `{base}/DEBUG.md` for the problem description and what was changed. There is no MILESTONE file — use DEBUG.md instead.
>
> Your goal: verify whether the specific bug described in `## Problem` has been fixed, and check for regressions.
>
> **Debug-specific rules**:
> - Primary check: does the specific bug still reproduce?
> - Secondary check: regressions (test suite, build, basic functionality)
> - Do NOT create follow-up tasks — just report your findings
> - Do NOT update PRD.md or PROGRESS.md
> - Clean up any temporary artifacts (screenshots, test files, etc.)
>
> Write your findings to the `## Verification Report` section of `{base}/DEBUG.md`. You MUST include a classification line:
> ```
> **Outcome**: [FIXED | PARTIAL | NO_CHANGE | REGRESSION]
> ```
>
> - **FIXED**: Bug is resolved, no regressions detected
> - **PARTIAL**: Bug is partially fixed or a related issue remains
> - **NO_CHANGE**: Fix didn't help, bug still reproduces
> - **REGRESSION**: Fix made things worse or broke something else

**Wait for**: Sub-agent to complete. Verify that `## Verification Report` in DEBUG.md has been populated with an outcome classification.

---

## Step 3: Assess Outcome

Read the `## Verification Report` section from `{base}/DEBUG.md`. Extract the **Outcome** classification.

### On FIXED

1. Proceed to Step 4 (Commit and Report)

### On REGRESSION

1. **Revert immediately**: `git checkout -- [changed files]` (read the Investigation & Fix Log for the list of changed files)
2. Update `## Iteration History` in DEBUG.md with what was tried and that it caused a regression
3. Clear the `## Investigation & Fix Log` and `## Verification Report` sections
4. If iteration < 3, loop back to Step 2 (Phase 2 + Phase 3 only)
5. If iteration = 3, proceed to Step 5 (Escalate)

### On PARTIAL or NO_CHANGE

1. Update `## Iteration History` in DEBUG.md with what was tried and the outcome
2. Clear the `## Investigation & Fix Log` and `## Verification Report` sections
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

1. **Ask the user to confirm** the fix looks correct before committing
2. **Commit with debug prefix**:
   ```bash
   git add [specific changed files]
   git commit -m "debug: [brief description of fix]"
   ```
3. **Report summary**:

```
Debug Fix Complete
==================
Problem:  [original problem statement]
Fix:      [what was changed]
Files:    [list of changed files]
Commit:   [short hash] — debug: [message]

Iterations: [N]
```

### Optional: Update Planning Files

If this fix relates to a follow-up task (FWLUP) in the PRD:
- Ask the user if they want to mark it complete
- If yes, update the task status in `{base}/PRD.md` (add `✅` to the header) and check it off in `{base}/PROGRESS.md`

Proceed to Step 6 (Cleanup).

## Step 5: Escalate

If 3 iterations were exhausted without a fix:

```
Debug limit reached (3 iterations). This issue may need the full pipeline.
Recommendation: /belmont:implement or /belmont:next with a FWLUP task.

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

2. **Tear down team (Approach A only)**:
   If you created a team:
   - Send `shutdown_request` via `SendMessage` to each teammate still active
   - Wait for shutdown confirmations
   - Call `TeamDelete` to remove team resources

   Skip this if you used Approach B or C.

<!-- @include commit-belmont-changes.md commit_context="after debug fix" -->

## Step 7: Final Actions

Once done, prompt the user to "/clear" and then "/belmont:status", "/belmont:verify", or "/belmont:next".
   - If you are Codex, instead prompt: "/new" and then "belmont:status", "belmont:verify", or "belmont:next"

## Scope Guardrails

These are hard rules. Do not break them:

1. **Fix only the reported issue** — no refactoring, no feature additions, no "improvements"
2. **DEBUG.md is the shared context file** — agents read from and write to it
3. **Dispatch to agents** — do NOT investigate, fix, or verify yourself (unless Approach C fallback)
4. **No PRD task creation** — if you discover new issues, mention them in the report but don't create tasks
5. **Max 3 iterations** — if you can't fix it in 3 tries, escalate
6. **Revert on regression** — if a fix makes things worse, undo it immediately
7. **Single commit** — one atomic commit for the fix, only after user confirms
8. **Minimal changes** — touch the fewest files possible to fix the issue
9. **Cleanup always** — delete DEBUG.md when the session ends, regardless of outcome
