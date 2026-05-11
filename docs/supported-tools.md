# Supported Tools

Belmont skills install as agentskills.io-format folders at `.agents/skills/belmont/<skill>/SKILL.md`. Six of seven supported AI CLIs auto-discover this path natively — the install does **zero per-tool wiring** for them. Claude Code is the exception: it discovers slash commands at `.claude/commands/<name>.md` (with subfolders becoming namespace prefixes), so Belmont creates per-skill symlinks at `.claude/commands/belmont/<skill>.md` pointing at the canonical SKILL.md.

| Tool               | Wiring                                                               | How to use                                              |
|--------------------|----------------------------------------------------------------------|---------------------------------------------------------|
| **Claude Code**    | `.claude/agents/belmont` symlink + `.claude/commands/belmont/<skill>.md` per-skill symlinks → `.agents/skills/belmont/<skill>/SKILL.md` | `/belmont:product-plan`, `/belmont:implement`, etc.     |
| **Codex**          | none — `.agents/skills/` auto-discovered (Codex 0.126+)              | Prompt `belmont:<skill>` — surfaces via `/skills`       |
| **Cursor**         | none — `.agents/skills/` auto-discovered (Cursor Skills system)      | Prompt `belmont:<skill>` — auto-loaded by description   |
| **Windsurf**       | none — `.agents/skills/` auto-discovered (Cascade v1.13.6+)          | Prompt `belmont:<skill>` — auto-loaded by description   |
| **Gemini**         | none — `.agents/skills/` is the documented alias for `.gemini/skills/` | Prompt `belmont:<skill>` — surfaces via `/skills`       |
| **GitHub Copilot** | none — `.agents/skills/` auto-discovered                              | Prompt `belmont:<skill>` — surfaces via Copilot CLI     |
| **Pi** ([pi.dev](https://pi.dev)) | none — `.agents/skills/` auto-discovered (agentskills.io)             | Prompt `belmont:<skill>` — Pi loads SKILL.md by description |
| **Any other tool** | none                                                                  | Point your tool at `.agents/skills/belmont/<skill>/SKILL.md` |

Each `<skill>/SKILL.md` carries `name:` + `description:` YAML frontmatter (required by agentskills.io) plus a `references/` subdir with the progressive-disclosure files that skill body references.

Belmont detects which tools to install for via three signals:
- conventional project dirs (`.claude/`, `.codex/`, `.cursor/`, `.pi/`, …) already present;
- tool binaries on PATH (`claude`, `codex`, `cursor-agent`, `gemini`, `copilot`, `pi`);
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
| Pi            | `pi`             | `pi -p [--provider <p> --model <m>] "<prompt>"` — provider/model resolved from `~/.belmont/local-llms.json`; YOLO is Pi's default so no auto-approve flag is needed |

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

### Codex / Cursor / Windsurf / Gemini / GitHub Copilot / Pi

All six auto-discover `.agents/skills/belmont/<skill>/SKILL.md`. Open the tool in your project directory and prompt with a skill reference like `belmont:implement` — the CLI's Skills system surfaces and activates the skill via its `description:` frontmatter.

For Codex specifically, the `/skills` slash command lists discovered skills. For Gemini, the same. For Cursor, you can also browse them via the Skills panel in the IDE.

### Pi (local-LLM workflow)

Pi ([pi.dev](https://pi.dev)) is uniquely well-suited to driving Belmont with locally-hosted models — its YOLO-by-default tool execution and OpenAI-compatible provider configuration mean it can run Belmont's auto loop offline against LM Studio, Ollama, vLLM, llama.cpp's server, etc.

**Pi-side configuration** (`~/.pi/agent/models.json`) — declare each runtime as a provider:

```json
{
  "providers": {
    "lm-studio": {
      "baseUrl": "http://localhost:1234/v1",
      "api": "openai-completions",
      "apiKey": "lm-studio",
      "compat": { "supportsDeveloperRole": false },
      "models": [
        { "id": "qwen/qwen3.6-35b-a3b" }
      ]
    }
  }
}
```

For Ollama swap `baseUrl` to `http://localhost:11434/v1`. The `apiKey` is required by Pi but ignored by local servers. `supportsDeveloperRole: false` is required for any backend that doesn't expose OpenAI's `developer` role (most local servers don't).

**Belmont-side tier mapping** (`~/.belmont/local-llms.json`) — map Belmont's `low`/`medium`/`high` tiers to Pi's providers + models:

```json
{
  "pi": {
    "tiers": {
      "low":    { "provider": "lm-studio", "model": "qwen/qwen3.6-35b-a3b" },
      "medium": { "provider": "lm-studio", "model": "qwen/qwen3.6-35b-a3b" },
      "high":   { "provider": "lm-studio", "model": "qwen/qwen3.6-35b-a3b" }
    }
  }
}
```

Mix and match — point `high` at a stronger model (e.g. DeepSeek-Coder via Ollama) and keep `low`/`medium` on a fast Qwen for code edits. Per-project overrides go in `<project>/.belmont/local-llms.json`. Per-shot env-var overrides: `BELMONT_PI_PROVIDER_<TIER>` / `BELMONT_PI_MODEL_<TIER>` (or single-value `BELMONT_PI_PROVIDER` / `BELMONT_PI_MODEL` applied to every tier).

If neither file nor env var is present, Belmont passes no `--model` flag and Pi falls back to whatever default `~/.pi/agent/models.json` defines — Belmont stays out of Pi's way.

**Tool-calling caveat for local Qwen:** Qwen2.5-Coder on LM Studio has [broken tool calling](https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues/825) — the model emits a non-hermes `<tools>` tag format that LM Studio's OpenAI-compat layer doesn't parse, and `tool_calls` arrives empty. Belmont's auto loop is 100% tool-call-driven, so file edits and bash silently fail. **Use Qwen3-Coder (or newer)** which uses the standard hermes format. Different runtimes (Ollama, vLLM) parse Qwen2.5-Coder correctly; the issue is specifically the LM Studio + Qwen2.5 combination.

### Generic / Other Tools

If your tool isn't auto-detected, the skill files are still plain markdown. Point your tool at:

- **Skills**: `.agents/skills/belmont/<skill>/SKILL.md` plus the `<skill>/references/` subdir
- **Agents**: `.agents/belmont/codebase-agent.md`, `implementation-agent.md`, etc.
- **State**: `.belmont/PR_FAQ.md`, `.belmont/PRD.md`, `.belmont/PROGRESS.md`, `.belmont/TECH_PLAN.md`, `.belmont/features/`

You can paste the skill content directly into a chat or configure your tool to load it as system context.

## Migration from older Belmont versions

If you've upgraded from a Belmont version that wrote into `.codex/belmont/`, `.cursor/rules/belmont/`, `.windsurf/rules/belmont/`, `.gemini/rules/belmont/`, `.copilot/belmont/`, `.claude/skills/belmont` (the 0.10.x nested-namespace symlink that Claude Code 2.1.x silently ignored), `.claude/plugins/belmont` (a brief 0.10.4-dev attempt that also wasn't auto-discovered), or maintained a `belmont:skill-routing` section in `AGENTS.md` / `GEMINI.md`, the next `belmont install` (or `belmont update`) automatically removes those legacy paths. The cleanup is idempotent — safe to re-run.
