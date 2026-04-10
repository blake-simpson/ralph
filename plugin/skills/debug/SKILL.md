---
name: debug
description: Debug router — choose between auto (agent-verified) and manual (user-verified) debug modes
alwaysApply: false
---

# Belmont: Debug

This is the debug router. It directs you to the appropriate debug sub-workflow.

## Two Modes

| Mode | Skill | Verification | Best for |
|------|-------|-------------|----------|
| **Auto** | `/belmont:debug-auto` | Verification agent checks each fix | Complex logic bugs, race conditions, issues needing automated testing |
| **Manual** | `/belmont:debug-manual` | User checks each fix (adds debug logs) | UI bugs, visual issues, known repro steps, faster iteration |

## Route Decision

Check the user's invocation text for mode hints:

**Route to `/belmont:debug-auto`** if the user mentions:
- "auto", "automatic", "full verification", "run tests", "agent verify"
- Complex or hard-to-reproduce issues
- No clear indication (auto is the default)

**Route to `/belmont:debug-manual`** if the user mentions:
- "manual", "I'll check", "I'll verify", "I can test", "quick", "fast"
- "debug logs", "console.log", "logging"
- UI bugs, visual issues, styling problems
- "I know how to reproduce"

**If unclear**, ask the user:

> **Which debug mode?**
>
> - **Auto** (`/belmont:debug-auto`) — dispatches a verification agent to check each fix. More thorough but slower.
> - **Manual** (`/belmont:debug-manual`) — you verify each fix yourself. Adds `[BELMONT-DEBUG]` logging to help trace the issue. Faster iteration.

Once the mode is determined, invoke the corresponding skill. Do NOT continue in this file — hand off entirely to the sub-skill.
