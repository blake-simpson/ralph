Domains: cli, skills, agents, auto-mode

# Dual invocation paths: auto + interactive

## Why this matters

Belmont skills are the same SKILL.md files on disk regardless of how they're invoked, but the *invocation paths* are completely different:

- **Auto mode** — the Go CLI assembles the prompt itself and shells out to the AI tool's headless print mode (`claude -p`, `codex exec`, `pi -p`, …). The tool's own skill discovery is bypassed entirely; Belmont injects the skill body, agent files, project state, and steering text directly into the subprocess prompt. Stdout is parsed for JSON decisions and structured events.
- **Interactive mode** — the user types a skill name (e.g. `/belmont:implement` in Claude Code; `belmont:implement` matched by description in Codex/Cursor/Pi/etc.) into the AI tool's *live REPL*, and the tool's own skill discovery loads `SKILL.md` from `.agents/skills/belmont/<skill>/`. The tool — not Belmont — assembles the prompt; Belmont's contribution is purely the on-disk content (skills, agent files, optional `AGENTS.md` routing).

These paths share an on-disk surface but exercise different code in the AI tool itself. A skill that works in one routinely breaks in the other unless co-designed.

## Invariant

Every change to tool integration, skill content, sub-agent dispatch, model-tier handling, or the on-disk surface (`.agents/skills/`, `.agents/belmont/`, `.belmont/`, `AGENTS.md`) MUST be evaluated against both paths and verified end-to-end in both before being marked done.

## How it's enforced

- `AGENTS.md` (and the `CLAUDE.md` symlink to it) carries a top-level "Both invocation paths or it's not done" section that loads on every session — the rule appears before any specific tool integration guidance.
- Plan files that touch this surface are expected to contain explicit "Auto mode" and "Interactive mode" sub-sections.
- Verification sections in such plans should run a smoke test in each mode (e.g. `belmont auto --tool X` once, then a live REPL session invoking the same skill by name once).
- (Future) `belmont validate` could lint plan files for the section pair — not implemented yet, called out as a future hardening rather than a guarantee.

## Failure mode if you break it

- **Symptom A — interactive-side regression.** `belmont auto --tool X` works green. A user typing `/belmont:implement` (or natural language matching a skill description) into X's REPL gets "skill not found", or the skill body runs but can't locate sub-agent files. Auto mode injected those files directly into the prompt; interactive mode relies on the skill body's own filesystem references being resolvable.
- **Symptom B — partial tier handling.** Tier preflight fires correctly during auto mode (where Belmont passes `--model` flags itself) but the interactive-mode `tier-preflight.md` partial doesn't list the new tool's model-switch command, so the user is told to switch models but not how to do it for *this* tool.
- **Symptom C — installer surface regression.** A new tool integration ships, `belmont auto --tool X` is green, but the tool's interactive REPL doesn't auto-discover `.agents/skills/` because the installer didn't write whatever routing surface that specific tool needs (e.g. Claude Code's per-skill symlinks at `.claude/commands/belmont/<skill>.md`).

## Don't re-do

- **"Just test auto mode, interactive will follow."** It doesn't. The Pi integration plan (the entry's prompting cause) initially only covered the `belmont auto --tool pi` shell-out path; the user rejected the plan and asked for explicit interactive coverage. The two paths share files but exercise completely different code paths inside the AI tool itself.
- **"Symlink everything per-tool like Claude Code."** Overkill. Six of seven supported tools auto-discover `.agents/skills/` directly via the agentskills.io standard; only Claude Code needs symlinks (because it discovers commands at `.claude/commands/<name>.md` rather than skills at `.agents/skills/`). The agentskills.io standard *is* the contract for the other six.
- **"Use only the tool's interactive REPL for everything (skip headless)."** Rejected because auto mode is what enables Belmont's parallel waves, evidence checks, scope guards, and steering pipeline — all of which require Belmont controlling the prompt. Interactive mode can't drive multi-worktree orchestration.
- **"Use only headless / auto mode (skip interactive)."** Also rejected. Many users want to run `/belmont:status` or `/belmont:next` without spinning up the full auto loop. Interactive mode is the lighter-weight, exploratory entry point and removing it would make Belmont feel like a black box.

## Evidence

- `skills/belmont/_partials/dispatch-strategy.md` already routes Claude (parallel sub-agents via Task tool) vs everyone else (sequential inline) — this is the dispatch half of the dual-path concern.
- `skills/belmont/_partials/tier-preflight.md` only fires for non-Claude CLIs in interactive mode (auto mode handles tier via `--model` flag passed to the subprocess) — this is the tier half.
- `cmd/belmont/main.go` `setupTool` shows both shapes side-by-side: Claude Code gets per-skill symlinks (because of its commands-discovery model), while Codex / Cursor / Windsurf / Gemini / GitHub Copilot / Pi all get the same no-op (auto-discovery via agentskills.io). Adding a new tool means deciding which side it lands on.
- Pi integration plan (this entry's prompting cause): first draft covered only `belmont auto` shell-out; user rejected and asked for explicit interactive coverage. Plan now has Part 1.5 (interactive-mode integration) sitting alongside Part 2 (auto-mode integration).

## Revisions

- 2026-05-10 — created during Pi integration planning; rule was already implicit in the codebase (dispatch-strategy + tier-preflight partials reflect both paths) but had no front-of-house statement to gate planning sessions.
