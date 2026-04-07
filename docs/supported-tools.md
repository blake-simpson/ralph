# Supported Tools

Agents and skills are always installed to `.agents/` -- the single source of truth shared across all tools.

Each AI tool is wired to `.agents/skills/belmont/` in the way it expects. Some tools use symlinks, while others get a copied/synced directory:

| Tool               | Symlink                                                 | Target                                                                                    | How to Use                                                            |
|--------------------|---------------------------------------------------------|-------------------------------------------------------------------------------------------|-----------------------------------------------------------------------|
| **Claude Code**    | `.claude/agents/belmont`<br/>`.claude/commands/belmont` | `agents -> .agents/belmont` (symlink)<br/>`commands` copied from `.agents/skills/belmont` | Slash commands: `/belmont:product-plan`, `/belmont:implement`, etc.   |
| **Codex**          | `.codex/belmont`                                        | Copied from `.agents/skills/belmont`                                                      | `AGENTS.md` includes Belmont routing for `belmont:<skill>` prompts    |
| **Cursor**         | `.cursor/rules/belmont/*.mdc`                           | `→ .agents/skills/belmont/*.md`                                                           | Toggle rules in Settings > Rules, or reference in Composer/Agent mode |
| **Windsurf**       | `.windsurf/rules/belmont`                               | Symlink to `.agents/skills/belmont`                                                       | Reference rules in Cascade                                            |
| **Gemini**         | `.gemini/rules/belmont`                                 | Symlink to `.agents/skills/belmont`                                                       | Reference rules in Gemini                                             |
| **GitHub Copilot** | `.copilot/belmont`                                      | Symlink to `.agents/skills/belmont`                                                       | Reference files in Copilot Chat                                       |
| **Any other tool** | *(none)*                                                | `.agents/skills/belmont/`                                                                 | Point your tool at the skill files directly                           |

Cursor uses per-file symlinks. Windsurf/Gemini/Copilot use a directory symlink. Claude Code and Codex use copied skill files.

## Claude Code Usage

Skills become native slash commands:

```
/belmont:working-backwards  Define product vision (PR/FAQ)
/belmont:product-plan   Interactive PRD creation
/belmont:tech-plan      Technical implementation plan
/belmont:implement      Implement next milestone (full pipeline)
/belmont:next           Implement next single task (lightweight)
/belmont:verify         Run verification and code review
/belmont:debug          Debug router (choose auto or manual)
/belmont:debug-auto    Auto debug loop (agent-verified)
/belmont:debug-manual  Manual debug loop (user-verified, faster)
/belmont:status         View progress
/belmont:review-plans   Review document alignment and detect drift
/belmont:cleanup        Archive completed features, reduce token bloat
/belmont:reset          Reset state and start fresh
```

## Codex Usage

Skills are copied into `.codex/belmont/`, and Belmont adds/updates a small section in `AGENTS.md` so Codex can resolve local Belmont skills. To use them:

1. Open Codex in your project directory
2. Prompt with a skill reference like `belmont:implement` or "Use the belmont:implement skill"
3. Codex should resolve `.agents/skills/belmont/implement.md` (fallback `.codex/belmont/implement.md`)
4. You can still point Codex at the skill file directly when starting a session

## Cursor Usage

Skills are installed as rules (`.mdc` files). To use them:

1. Open **Settings > Cursor Settings > Rules**
2. You'll see the belmont rules listed (product-plan, tech-plan, implement, next, verify, status, cleanup, reset)
3. Enable the one you want to activate
4. Start a Composer or Agent session -- the rule will be loaded as context
5. Or reference them directly: *"Follow the belmont implement workflow"*

In the **Cursor Agent CLI**, you can reference the skill files directly:

```bash
cursor agent --rules .cursor/rules/belmont/implement.mdc
```

## Generic / Other Tools

If your tool isn't auto-detected, the agent and skill files are still plain markdown. Point your tool at:

- **Skills**: Read from `.agents/skills/belmont/` (or wherever you've placed them)
- **Agents**: `.agents/belmont/codebase-agent.md`, `implementation-agent.md`, etc.
- **State**: `.belmont/PR_FAQ.md`, `.belmont/PRD.md`, `.belmont/PROGRESS.md` (master feature summary), `.belmont/TECH_PLAN.md`, `.belmont/MILESTONE.md`, `.belmont/features/`

You can paste the skill content directly into a chat or configure your tool to load it as system context.
