---
description: Manual debug loop with deep Belmont context and in-place spec reconciliation — user-verified fix + correct the specs that let the bug exist
alwaysApply: false
---

# Belmont: Debug (Manual)

You are the debug orchestrator running in **manual mode**. Your job is to:

1. Load the full Belmont spec context for the bug being investigated (master + per-feature, multi-feature capable).
2. Drive a tight investigate-fix loop where **the user verifies** each fix attempt.
3. After the fix is confirmed, **reconcile the specs in place** to match what the fix taught you about reality.
4. Commit code + spec edits atomically so Belmont's memory stays accurate.

**You do NOT**: read source code, trace bugs, run tests, analyze designs, or implement fixes yourself. You manage DEBUG.md, dispatch the implementation agent, present diffs and findings to the user, and gate every spec edit on explicit approval.

**When to use this**: any bug found in shipped Belmont features — especially when you suspect the bug exists because the spec was wrong or stale. Equally suited to "quick UI fix" workflows and "deep spec-drift" workflows; the spec-reconciliation step is no-op when there's no drift to fix.

**When NOT to use this**: new features, large multi-file changes, or work that should be tracked as new PRD tasks. Use `/belmont:implement` or `/belmont:next`. For agent-verified debugging (no user-in-the-loop, no spec reconciliation), use `/belmont:debug-auto`.

## Interactive-only

This skill is for interactive REPL sessions in Claude Code, Codex, Cursor, Windsurf, Gemini, Copilot, or Pi. `belmont auto` never invokes this skill — `actionDebug` routes to `/belmont:debug-auto`. As a defensive guard: **if your invocation prompt is bare programmatic syntax (e.g. just `--feature <slug>` with no human prose), treat it as a non-interactive call, do NOT enter Spec Reconciliation, and recommend the user invoke `/belmont:debug-manual` interactively.**

<!-- @include tier-preflight.md -->

<!-- @include feature-detection-multi.md feature_action="ask the user which feature(s) the bug touches" -->

<!-- @include debug-scope-rules.md -->

## Step 0: Load Belmont Context

Before asking the user a single follow-up question, load the spec context. Read these files in order, treating every read as optional — skip silently if the file is absent and record `skipped: not present` in the context-load summary.

**Master files** (always at `.belmont/`):

1. `.belmont/PR_FAQ.md` — strategic vision. If file > 500 lines, ask the user `PR_FAQ.md is <N> lines — load anyway? [y/N]`. Default no on large files.
2. `.belmont/PRD.md` — master feature catalog.
3. `.belmont/TECH_PLAN.md` — master cross-cutting architecture.
4. `.belmont/NOTES.md` — global learnings from prior sessions.

**Per-feature files** — for EACH base in `{bases[]}`:

5. `{base}/PRD.md`
6. `{base}/TECH_PLAN.md`
7. `{base}/PROGRESS.md`
8. `{base}/NOTES.md`
9. Latest `{base}/MILESTONE-M*.done.md` — numeric-sorted by milestone ID, take the highest (most recently shipped). Skip if > 500 lines; tell the user and offer to load if they want it.

After all reads, emit a single **Context-load summary** block:

```
Context loaded
  Master:
    .belmont/PR_FAQ.md      <N lines>   | skipped: not present
    .belmont/PRD.md         <N lines>
    .belmont/TECH_PLAN.md   <N lines>
    .belmont/NOTES.md       <N lines>   | skipped: not present
  Features:
    <slug-1>:
      PRD.md, TECH_PLAN.md, PROGRESS.md, NOTES.md, MILESTONE-M<X>.done.md
    <slug-2>: ...
  Total: <N> files, ~<KB> KB
```

**Graceful degradation for local LLMs**: if the total loaded context exceeds ~50 KB and the session is running on Pi or another local-LLM CLI, surface a warning and ask the user to narrow to a single feature before proceeding:

```
⚠ Loaded context is ~<KB> KB. Local LLMs may truncate or degrade on this size.
Recommended: re-run this skill and select a single feature. Continue anyway? [y/N]
```

## Step 1: Understand the Problem

1. If the user provided a description with the skill invocation, use it as the problem statement.
2. If the description is vague or missing, ask **one** focused clarifying question:
   - What's the expected behaviour?
   - What's the actual behaviour?
   - How do you reproduce it?
3. Reference the context you loaded — does the bug map cleanly to a task in any loaded PRD? Note the task ID(s) for later.
4. Write a single-sentence **problem statement** before proceeding.

## Step 2: Create DEBUG.md

Create `{primary_base}/DEBUG.md` (the primary feature is the first slug in `{bases[]}` — see `feature-detection-multi.md`):

```markdown
# Debug: [Problem Statement]

## Status
- **Mode**: Debug (Manual)
- **Primary feature**: <slug>
- **Additional features**: <slug, slug> | None
- **Iteration**: 1/3

## Problem
[Full description — expected behaviour, actual behaviour, reproduction steps]

## Context
- **Feature(s)**: <names from PRDs>
- **Figma URLs**: [if any, otherwise "None"]
- **Related task IDs**: [from Step 1 reference, otherwise "None"]

## Context Loaded
[Paste the Step 0 context-load summary block here verbatim]

### Learnings from Previous Sessions
[From master NOTES.md + per-feature NOTES.md, or "No previous learnings found."]

### Scope Boundaries
- **In Scope**: Fix the reported bug only
- **Out of Scope**: [from PRD's Out of Scope section(s)]

## Design Specifications
[Written by design-agent if dispatched, otherwise "Not applicable"]

## Iteration History
[Updated by orchestrator after each iteration]

## Investigation & Fix Log
[Written by implementation-agent — current iteration only]

## User Feedback
[Collected by orchestrator from user after each iteration]

## Spec Reconciliation Log
[Written by orchestrator during the Spec Reconciliation phase]
```

**IMPORTANT**: DEBUG.md is the single shared context file between you and the agents. Agents read it for problem context and write to their designated sections. Include enough context for agents to work independently.

## Sub-Agent Dispatch Strategy

Apply the following dispatch configuration:
- **Team name**: `belmont-debug-manual`
- **Parallel agents**: None by default (agents run sequentially per iteration)
- **Sequential agents**: design-agent (optional, iteration 1 only) → implementation-agent
- **Cleanup timing**: After the debug session ends (Step 7)

<!-- @include dispatch-strategy.md -->

## Step 3: Run the Debug Loop

For each iteration (max 3), dispatch the implementation agent, then ask the user to verify. Each agent reads `{primary_base}/DEBUG.md` and writes to its designated section.

Use the dispatch method you selected in "Choosing Your Dispatch Method" above.

---

<!-- @include debug-phase1-design.md -->

---

### Phase 2: Investigation & Fix (every iteration)

**Spawn a sub-agent with this prompt**:

<!-- @include identity-preamble.md agent_role="implementation" agent_file="implementation-agent.md" -->
>
> **DEBUG MODE OVERRIDE**: You are operating in debug mode, not milestone mode.
>
> Read `{primary_base}/DEBUG.md` instead of a MILESTONE file. It contains the problem description, context, design specifications (if applicable), and iteration history from previous attempts.
>
> Your goal: investigate the bug described in the `## Problem` section and implement a **minimal fix**. You are NOT implementing milestone tasks — you are fixing a specific bug.
>
> **Debug-specific rules**:
> - Do NOT commit — the orchestrator handles commits after verification
> - Focus your fix on code; the orchestrator will reconcile specs (PRD, TECH_PLAN, NOTES, PROGRESS) in the Spec Reconciliation phase after the user confirms the fix. Do NOT touch spec files yourself.
> - Do NOT create follow-up tasks — just fix the bug; the orchestrator handles tracking
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
> Write your investigation findings and changes to the `## Investigation & Fix Log` section of `{primary_base}/DEBUG.md`. Include:
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

1. **Read** the `## Investigation & Fix Log` from `{primary_base}/DEBUG.md`
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

5. **Write** the user's feedback and the mapped outcome to the `## User Feedback` section of `{primary_base}/DEBUG.md`:
   ```
   ### Iteration [N]
   **Outcome**: [FIXED | PARTIAL | NO_CHANGE | REGRESSION]
   **User said**: [summary of user's response]
   **Debug log output**: [any log output the user shared, or "None provided"]
   ```

---

## Step 4: Assess Outcome

Use the outcome determined in Phase 3 above.

### On FIXED

1. Proceed to **Spec Reconciliation** (below) — do NOT commit yet.

### On REGRESSION

1. **Revert immediately**: `git checkout -- [changed files]` (read the Investigation & Fix Log for the list of changed files)
2. Update `## Iteration History` in DEBUG.md with what was tried and that it caused a regression
3. Clear the `## Investigation & Fix Log` section
4. If iteration < 3, loop back to Step 3 (Phase 2 + Phase 3 only)
5. If iteration = 3, proceed to **Escalate** (Step 5 from the included partial below)

### On PARTIAL or NO_CHANGE

1. Update `## Iteration History` in DEBUG.md with what was tried, the outcome, and any debug log output the user shared
2. Clear the `## Investigation & Fix Log` section
3. Increment the iteration counter in `## Status`
4. If iteration = 2, proceed to **User Checkpoint** below
5. If iteration < 2, loop back to Step 3 (Phase 2 + Phase 3 only)

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
- If continue → loop back to Step 3 for iteration 3
- If stop → proceed to **Cleanup** (Step 6 from the included partial below)
- If redirect → proceed to **Cleanup**, then suggest `/belmont:implement`

### After iteration 3 (still not fixed)

Proceed to **Escalate** (Step 5 from the included partial below).

## Spec Reconciliation

**Only reach this step when the fix is confirmed FIXED.** Do not commit yet — code + spec edits commit atomically in the Commit and Report step below.

Read `references/debug-manual-spec-reconcile.md` and follow it end-to-end. In summary:

1. Walk the drift catalogue across every loaded spec file (master + per-feature).
2. For each candidate edit, present a unified diff and a `y / N / edit / skip` prompt.
3. Apply approved edits in place. Stage them for the upcoming commit (do NOT commit individually).
4. Append a Five-Whys-style `## Root Cause Patterns` entry to `{base}/NOTES.md` for the feature whose bug this was.
5. Log every candidate (applied, rejected, skipped) to DEBUG.md's `## Spec Reconciliation Log` section.
6. If no drift was found, log `No drift detected — code-only fix.` and continue.

In multi-feature mode, iterate per feature — get per-feature confirmation before walking that feature's drift.

### Hard limits during reconciliation

Restated from `debug-scope-rules.md`:

- No new milestone headings. No renames. No removals. Use polish/follow-up/cleanup naming and `belmont validate` will reject on next auto run.
- No new `[ ]` follow-up tasks for unfixed drift — fix it or skip it; never park work for `/belmont:implement` to find later.
- No `[v]` flips. Use `[x]` only, scoped to the current or last-shipped milestone.
- No edits to features not in `{bases[]}`.

## Commit and Report

Reach this step after Spec Reconciliation finishes (with zero or more spec edits applied).

1. **Clean up debug logs** before committing:
   - Search the entire codebase for `[BELMONT-DEBUG]`: `grep -rn "BELMONT-DEBUG" .`
   - Remove ALL lines containing `[BELMONT-DEBUG]` debug logs
   - Cross-reference with the `### Debug Logs Added` entries in DEBUG.md to ensure none are missed
   - Run the search again to verify zero results: `grep -rn "BELMONT-DEBUG" .`
   - If any remain, remove them and re-verify

2. **Ask the user to confirm** the fix (with debug logs removed) looks correct before committing.

3. **Single atomic commit** — stage code AND spec edits together:
   ```bash
   git add <code files> <spec files>
   git commit -m "$(cat <<'EOF'
   debug: <one-line fix description>[ + spec sync]

   Iterations: <N>
   Code files: <list>
   Spec files: <list>
   Task IDs flipped to [x]: <list, or "none">
   Drift categories addressed: <list, or "none">
   EOF
   )"
   ```
   - Subject ends with ` + spec sync` ONLY if Step 5 applied at least one spec edit.
   - Multi-line body MUST include any task ID flipped `[x]` (so `runEvidenceCheck` finds attribution on a future verify pass).
   - Use the user's preferred commit-message style if they set one in CLAUDE.md (no co-author lines unless explicitly asked).

4. **Report summary**:

```
Debug Fix Complete
==================
Problem:       [original problem statement]
Fix:           [what was changed]
Code files:    [list]
Spec edits:    [N applied, N rejected, N skipped]
Spec files:    [list, or "none"]
Commit:        [short hash] — debug: [message]

Iterations:    [N]
Debug logs:    cleaned up ([N] log points removed)
```

### Optional: relay drift findings to the user

If you skipped any drift candidates during Spec Reconciliation, list them in the report so the user knows what's still open. Do NOT create follow-up tasks for them — the user explicitly opted into a "fix it now or skip it" model.

Proceed to **Cleanup** (Step 6 from the included partial below).

<!-- @include debug-escalate-cleanup.md -->
