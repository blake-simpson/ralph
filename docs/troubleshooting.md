# Troubleshooting

## `belmont` command not found

Ensure `~/.local/bin` is in your PATH:

```bash
echo $PATH | tr ':' '\n' | grep local
# If missing:
export PATH="$HOME/.local/bin:$PATH"
```

Or re-run the installer:

```bash
curl -fsSL https://raw.githubusercontent.com/blake-simpson/belmont/main/install.sh | sh
```

## No AI tools detected during install

If your project doesn't have a `.claude/`, `.codex/`, `.cursor/`, etc. directory yet, the installer will ask which tool you're using and create the directory for you.

## Skills not showing up in Claude Code

Verify the agent symlink and copied command folder:

```bash
ls -la .claude/agents/belmont
# Should show: belmont -> ../../.agents/belmont

ls .claude/commands/belmont
# Should list the .md skill files

ls .agents/skills/belmont/
# Should list the .md skill files
```

If the symlink is missing or the skill directories are empty, re-run `belmont install` (or `belmont install --source /path/to/belmont`) and select Claude Code.

## Skills not showing up in Cursor

Cursor uses per-file symlinks with `.mdc` extension. Verify:

```bash
ls -la .cursor/rules/belmont/
# Should show .mdc symlinks pointing to .agents/skills/belmont/*.md
```

If you need to manually refresh, restart Cursor or reload the window.

## PRD is empty / template only

Run the product-plan skill first to create your PRD interactively. The tech-plan and implement skills require a populated PRD.

## Task marked as blocked

Blocked tasks show as `[!]` in `.belmont/PROGRESS.md`. Common causes:
- Figma URL not accessible
- Missing context or dependencies
- Build/test failures that can't be auto-resolved

Fix the underlying issue, change the task's checkbox from `[!]` back to `[ ]` in PROGRESS.md, and re-run implement.

## Want to start fresh

Run the reset skill (`/belmont:reset` in Claude Code) to reset all state files. Alternatively, delete `.belmont/PRD.md`, `.belmont/PROGRESS.md`, `.belmont/TECH_PLAN.md`, `.belmont/MILESTONE.md`, and any `.belmont/MILESTONE-*.done.md` files manually, then re-run `belmont install` (or `belmont install --source /path/to/belmont`) to recreate templates.
