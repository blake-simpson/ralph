# Troubleshooting

## `belmont` command not found

Ensure `~/.local/bin` is in your PATH:

```bash
echo $PATH | tr ':' '\n' | grep local
# If missing:
export PATH="$HOME/.local/bin:$PATH"
```

Or re-install:

```bash
# Via Homebrew
brew reinstall belmont

# Or via curl
curl -fsSL https://raw.githubusercontent.com/blake-simpson/belmont/main/install.sh | sh
```

## No AI tools detected during install

If your project doesn't have a `.claude/`, `.codex/`, `.cursor/`, etc. directory yet, the installer will ask which tool you're using and create the directory for you.

## Skills not showing up in Claude Code

Verify the agent symlink + per-skill slash-command symlinks:

```bash
ls -la .claude/agents/belmont
# Should show: belmont -> ../../.agents/belmont

ls -la .claude/commands/belmont/
# Should show one .md symlink per skill, each → ../../../.agents/skills/belmont/<skill>/SKILL.md

ls .agents/skills/belmont/
# Should list one folder per skill, each with SKILL.md inside
```

If you're upgrading from Belmont 0.10.x and `claude -p`'s init message shows zero `belmont:*` entries, you're likely still on the dead `.claude/skills/belmont` (or briefly `.claude/plugins/belmont`) layout — Claude Code 2.1.x never discovered either of those. Run `belmont install` to migrate; the legacy cleanup pass removes the dead paths and writes the per-skill `.claude/commands/belmont/<skill>.md` symlinks. If symlinks are missing after re-install, re-run with `belmont install --source /path/to/belmont` and select Claude Code.

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

## `belmont auto` refuses to start: "working tree is not clean"

`auto` requires a clean working tree because worktree merges back into the starting branch will fail if uncommitted changes overlap the merged paths. The error lists the offending paths.

Most common cause: a recent `belmont update` rewrote files under `.agents/belmont/` or `.agents/skills/belmont/` and the user never committed them. Recent versions auto-commit these files; older versions did not. To resolve:

```bash
git stash -u                       # stash everything (incl. untracked)
git commit -am "Update Belmont"    # or commit your changes
belmont auto --feature ...         # then retry

# Last resort:
belmont auto --feature ... --allow-dirty
```

`--dry-run` also bypasses the check (no merges happen).

## `belmont update` auto-commit failed (pre-commit hook)

`belmont update` runs `git commit` with hooks enabled. If a hook fails, the Belmont-managed files are left staged. Fix whatever the hook complained about (e.g. lint, formatting) and re-run the printed `git commit -m "Update Belmont to vX.Y.Z"` manually. To skip auto-commit on the next update, run `belmont update --no-commit` and commit the files yourself.

## Monorepo: `npm install` fails with `PrismaConfigEnvError: Cannot resolve environment variable: DATABASE_URL`

You're hitting this if `belmont auto` runs in a monorepo (e.g. `packages/<app>/`) and Prisma's TS config loader can't find `.env`. Belmont copies `.env*` from the project root into the worktree root and into qualifying workspace dirs (those with `postinstall` scripts or Prisma deps). If your workspace doesn't trigger the heuristic, declare it explicitly in `.belmont/worktree.json`:

```json
{
  "workspaces": {
    "web": {
      "path": "packages/studia-web",
      "env_files": [".env", "packages/studia-web/.env.local"]
    }
  }
}
```

See [Monorepo Support](monorepo-support.md) for the full schema.

## Monorepo: agent runs commands at the repo root instead of the workspace

If the implementation/verification agent is running `pnpm run build` instead of `pnpm --filter <id> run build`, confirm `belmont status` shows a `Monorepo:` line at the top. If not, Belmont didn't detect your monorepo (e.g. unusual layout, no signal file). Add an explicit `workspaces` map and `primary_workspace` to `.belmont/worktree.json` — the override always wins over auto-detection.

## Monorepo: warning that `.env` is not gitignored

Belmont prints `⚠ Seeded packages/web/.env but it is not gitignored` after seeding env files into a workspace dir whose path isn't covered by `.gitignore`. Belmont won't commit the seeded file (worktrees only commit their feature branch), but the warning is a heads-up for interactive workflows. Common fix:

```
**/.env
**/.env.*
!**/.env.example
```

## Monorepo: setup hooks differ from CI

`worktree.json` setup hooks run only in Belmont worktrees. CI doesn't run Belmont. If your CI is failing where local Belmont runs succeed, mirror critical setup (env wiring, install commands, `dotenv -e` shims) in your CI configuration as well.
