# Supported Tools

Belmont skills install as agentskills.io-format folders at `.agents/skills/belmont/<skill>/SKILL.md`. Five of six supported AI CLIs auto-discover this path natively — the install does **zero per-tool wiring** for them. Only Claude Code needs an explicit symlink (because it expects skills under `.claude/skills/`, not `.agents/skills/`).

| Tool               | Wiring                                                               | How to use                                              |
|--------------------|----------------------------------------------------------------------|---------------------------------------------------------|
| **Claude Code**    | `.claude/agents/belmont` and `.claude/skills/belmont` symlinks       | `/belmont:product-plan`, `/belmont:implement`, etc.     |
| **Codex**          | none — `.agents/skills/` auto-discovered (Codex 0.126+)              | Prompt `belmont:<skill>` — surfaces via `/skills`       |
| **Cursor**         | none — `.agents/skills/` auto-discovered (Cursor Skills system)      | Prompt `belmont:<skill>` — auto-loaded by description   |
| **Windsurf**       | none — `.agents/skills/` auto-discovered (Cascade v1.13.6+)          | Prompt `belmont:<skill>` — auto-loaded by description   |
| **Gemini**         | none — `.agents/skills/` is the documented alias for `.gemini/skills/` | Prompt `belmont:<skill>` — surfaces via `/skills`       |
| **GitHub Copilot** | none — `.agents/skills/` auto-discovered                              | Prompt `belmont:<skill>` — surfaces via Copilot CLI     |
| **Any other tool** | none                                                                  | Point your tool at `.agents/skills/belmont/<skill>/SKILL.md` |

Each `<skill>/SKILL.md` carries `name:` + `description:` YAML frontmatter (required by agentskills.io) plus a `references/` subdir with the progressive-disclosure files that skill body references.

Belmont detects which tools to install for via three signals:
- conventional project dirs (`.claude/`, `.codex/`, `.cursor/`, …) already present;
- tool binaries on PATH (`claude`, `codex`, `cursor-agent`, `gemini`, `copilot`);
- a Belmont skill-routing section in `AGENTS.md` / `GEMINI.md` (signals a previous install).

## Headless invocation

Belmont's `auto` loop shells out to each tool's CLI in headless mode. The flag combinations are kept current with each tool's docs:

| Tool          | Binary           | Invocation                                                                                                              |
|---------------|------------------|-------------------------------------------------------------------------------------------------------------------------|
| Claude Code   | `claude`         | `claude -p "<prompt>" --permission-mode bypassPermissions --allowedTools "Bash,Read,Write,Edit,..." --output-format stream-json --verbose` |
| Codex         | `codex`          | `codex exec "<prompt>" --dangerously-bypass-approvals-and-sandbox --json -C <root>`                                     |
| Cursor        | `cursor-agent`   | `cursor-agent -p --force --output-format json "<prompt>"` (prompt is the trailing positional)                           |
| Gemini        | `gemini`         | `gemini -p "<prompt>" --approval-mode yolo --output-format json` (`--yolo` is deprecated)                               |
| GitHub Copilot| `copilot`        | `copilot -p "<prompt>" --yolo`                                                                                          |

Cursor's CLI is installed as both `cursor-agent` (legacy) and `agent` (current canonical name) — Belmont targets `cursor-agent` for stability, since the unambiguous name is less likely to collide with other tools that might expose a generic `agent` binary.

## Per-tool usage

### Claude Code

Skills become native slash commands:

```
/belmont:working-backwards  Define product vision (PR/FAQ)
/belmont:product-plan       Interactive PRD creation
/belmont:tech-plan          Technical implementation plan
/belmont:implement          Implement next milestone (full pipeline)
/belmont:next               Implement next single task (lightweight)
/belmont:verify             Run verification and code review
/belmont:debug              Debug router (choose auto or manual)
/belmont:debug-auto         Auto debug loop (agent-verified)
/belmont:debug-manual       Manual debug loop (user-verified, faster)
/belmont:status             View progress
/belmont:review-plans       Review document alignment and detect drift
/belmont:cleanup            Archive completed features, reduce token bloat
/belmont:reset              Reset state and start fresh
```

### Codex / Cursor / Windsurf / Gemini / GitHub Copilot

All five auto-discover `.agents/skills/belmont/<skill>/SKILL.md`. Open the tool in your project directory and prompt with a skill reference like `belmont:implement` — the CLI's Skills system surfaces and activates the skill via its `description:` frontmatter.

For Codex specifically, the `/skills` slash command lists discovered skills. For Gemini, the same. For Cursor, you can also browse them via the Skills panel in the IDE.

### Generic / Other Tools

If your tool isn't auto-detected, the skill files are still plain markdown. Point your tool at:

- **Skills**: `.agents/skills/belmont/<skill>/SKILL.md` plus the `<skill>/references/` subdir
- **Agents**: `.agents/belmont/codebase-agent.md`, `implementation-agent.md`, etc.
- **State**: `.belmont/PR_FAQ.md`, `.belmont/PRD.md`, `.belmont/PROGRESS.md`, `.belmont/TECH_PLAN.md`, `.belmont/features/`

You can paste the skill content directly into a chat or configure your tool to load it as system context.

## Migration from older Belmont versions

If you've upgraded from a Belmont version that wrote into `.codex/belmont/`, `.cursor/rules/belmont/`, `.windsurf/rules/belmont/`, `.gemini/rules/belmont/`, `.copilot/belmont/`, `.claude/commands/belmont/`, or maintained a `belmont:skill-routing` section in `AGENTS.md` / `GEMINI.md`, the next `belmont install` (or `belmont update`) automatically removes those legacy paths. The cleanup is idempotent — safe to re-run.
