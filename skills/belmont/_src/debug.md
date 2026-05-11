---
description: Debug router — choose between auto (agent-verified, narrow code fix) and manual (user-verified, deep spec context, in-place spec reconciliation)
alwaysApply: false
---

# Belmont: Debug

This is the debug router. It directs you to the appropriate debug sub-workflow.

## Two Modes

| Mode | Skill | Verification | Best for |
|------|-------|-------------|----------|
| **Auto** | `/belmont:debug-auto` | Verification agent checks each fix | Complex logic bugs, race conditions, issues needing automated testing, narrow code-only fixes |
| **Manual** | `/belmont:debug-manual` | User checks each fix (adds debug logs); deep Belmont context + in-place spec reconciliation after the fix | UI bugs, visual issues, known repro steps, **bugs that exist because the spec drifted from reality**, multi-feature debugging |

The two modes diverge sharply after the fix is confirmed:
- **Auto** commits the code change and ends. The PRD, TECH_PLAN, NOTES stay as they are.
- **Manual** runs a Spec Reconciliation phase that walks the loaded specs, identifies drift, presents diffs for user approval, and commits code + spec edits atomically. This is how Belmont's memory gets corrected so future sessions don't operate on stale truth.

## Route Decision

Check the user's invocation text for mode hints:

**Route to `/belmont:debug-auto`** if the user mentions:
- "auto", "automatic", "full verification", "run tests", "agent verify"
- "just the code", "narrow fix", "don't touch specs"
- Complex or hard-to-reproduce issues where agent-driven iteration beats user-in-the-loop

**Route to `/belmont:debug-manual`** if the user mentions:
- "manual", "I'll check", "I'll verify", "I can test", "quick", "fast"
- "debug logs", "console.log", "logging"
- UI bugs, visual issues, styling problems
- "I know how to reproduce"
- "spec is wrong", "PRD is stale", "TECH_PLAN doesn't match", "fix the docs too", "reconcile specs"
- "across two features", "multi-feature", "spans X and Y"

**If unclear**, ask the user:

> **Which debug mode?**
>
> - **Auto** (`/belmont:debug-auto`) — dispatches a verification agent to check each fix. Narrow code-only fix. More thorough but slower.
> - **Manual** (`/belmont:debug-manual`) — you verify each fix yourself. Loads deep Belmont context (master PR_FAQ + master/feature PRD + TECH_PLAN + NOTES + latest MILESTONE.done). After the fix, walks the loaded specs and offers diffs to reconcile drift in place. Best for bugs that may have a spec-level root cause.

Once the mode is determined, invoke the corresponding skill. Do NOT continue in this file — hand off entirely to the sub-skill.
