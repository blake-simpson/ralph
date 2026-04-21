### Core Principle

You are the **orchestrator**. You MUST NOT perform the agent work yourself. Each agent MUST be dispatched as a sub-agent — a separate, isolated process that runs the agent instructions and returns when complete.

**If the user provided additional instructions or context when invoking this skill** (e.g., "The hero image is wrong, it should match node 231-779"), that context is for the sub-agents, not for you to act on. Your only job is to forward it. See "User Context Forwarding" below.

### Dispatch Method

Use the first method below whose required tools are available. Check by tool name — do not guess.

**Agent Teams (preferred)** — requires `TeamCreate`, `Task` (with `team_name`), `SendMessage`, `TeamDelete`.

Create a team with the team name specified above. All `Task` calls pass: `team_name`, `name` (the agent role, e.g. `"codebase-agent"`), `subagent_type: "general-purpose"`, `mode: "bypassPermissions"`. After the skill's work completes, send `shutdown_request` via `SendMessage` to each teammate, then call `TeamDelete`.

**Direct Task (fallback)** — requires `Task` only.

Issue `Task` calls with `subagent_type: "general-purpose"` and `mode: "bypassPermissions"`. No team cleanup needed.

**For parallel agents** (either method): issue all `Task` calls in the same message — they run concurrently and return results directly. Do **NOT** set `run_in_background: true`; foreground tasks are simpler and more reliable.

**For sequential agents**: issue a single `Task` call and wait for it to complete.

**If `Task` is not available**: read each agent file (`.agents/belmont/<agent-name>.md`) and execute its instructions inline — finish one agent completely before starting the next.

### User Context Forwarding (CRITICAL)

When the user provides additional instructions alongside the skill invocation (e.g., `/belmont:verify The hero image is wrong...`):

1. Capture the user's context verbatim
2. Append it to every sub-agent prompt as an "Additional Context from User" section
3. DO NOT act on it yourself — your job is to pass it through

Format (omit entirely if no context was provided):
```
> **Additional Context from User**:
> [paste verbatim]
```

**Why this matters**: the orchestrator seeing actionable instructions and acting on them causes duplicate work and conflicts with sub-agents doing the same thing.

### Dispatch Rules

1. **DO NOT** perform the sub-agents' work yourself
2. **DO NOT** read `.agents/belmont/*-agent.md` files yourself (unless using the inline fallback) — sub-agents read them
3. **DO** prepare all required context (e.g., the MILESTONE file) before spawning any sub-agent
4. **DO** wait for sub-agents to complete before proceeding
5. **DO** handle blockers and errors reported by sub-agents
6. **DO** forward any user-provided context to every sub-agent (see above)
