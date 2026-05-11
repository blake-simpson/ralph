# Monorepo Workspaces

**Domains**: cli, skills, agents

**Why this matters.** Belmont was originally built assuming the project root *is* the package root. In a monorepo (Turborepo, Nx, pnpm/npm/yarn/bun workspaces, Cargo, Go workspaces, uv), that assumption breaks in three places at once: env files end up at the worktree root but get consumed inside `packages/<name>/` (Prisma's TS config loader doesn't walk up); install/build/test commands run at the wrong scope (`pnpm run build` at root vs. `pnpm --filter <id> run build` per workspace); and the dev-server bundler invocation expects a `package.json` at cwd. The Prisma failure on 2026-05-07 was the canonical surfacing — `npm install` recursed into `packages/<workspace>/` for `prisma generate` postinstall, which then errored on missing `DATABASE_URL` because the env file Belmont copied was at the worktree root (`~/.belmont/worktrees/<project>/<slug>/.env`), not inside the workspace where Prisma actually runs.

## Invariant

When a monorepo is detected (or declared via `worktree.json`):

- `BELMONT_MONOREPO=1` is exported. Single-package projects do **not** see this var — every monorepo-aware code path is gated on it.
- `.env*` files are seeded into the worktree root (existing behavior) **and** into qualifying workspace dirs. A workspace qualifies if (a) its `package.json` has a `postinstall` script or env-consuming deps (`prisma`, `@prisma/client`, `dotenv`, `dotenv-cli`, `drizzle-kit`, `tsx`, `vite-node`); (b) its Cargo workspace has `build.rs`; (c) its `pyproject.toml` declares `[project.scripts]` / `[tool.poetry.scripts]`; or (d) the user listed explicit `env_files` in `worktree.json`.
- `BELMONT_PRIMARY_WORKSPACE` (id) and `BELMONT_PRIMARY_WORKSPACE_PATH` (relative path) point at the workspace whose dev server gets `$BELMONT_PORT`. Singular by design — one primary per worktree. Other workspaces use the dynamic `FREE_PORT` pattern.
- `BELMONT_WORKSPACES` is a JSON array of every workspace; agents enumerate it for multi-service verification.
- The dominant monorepo type is exposed as `BELMONT_MONOREPO_TYPE` (one of `turborepo`, `nx`, `pnpm`, `npm`, `yarn`, `bun`, `cargo`, `go`, `uv`, `lerna`, `rush`).
- Explicit `workspaces` / `primary_workspace` in `.belmont/worktree.json` always override auto-detection. No prompting.
- Single-package projects are unaffected. None of these env vars are exported, and skills' `if BELMONT_MONOREPO=1` guards short-circuit.

## How it's enforced

Three mechanisms in combination, in this order of precedence:

1. **CLI auto-detection (mechanical).** `detectWorkspaces(root)` in `cmd/belmont/main.go` probes the root for signal files in dominance order: `turbo.json` > `nx.json` > `pnpm-workspace.yaml` > `package.json#workspaces` > `lerna.json` > `rush.json` > `Cargo.toml#[workspace]` > `go.work` > `pyproject.toml#[tool.uv.workspace]`. Each parser is tolerant — malformed signal files return `(nil, false)` rather than aborting. Workspaces are deduplicated by path so a Turborepo with both `turbo.json` and `pnpm-workspace.yaml` reports once.

2. **Worktree.json override (explicit).** `worktreeHooks.Workspaces` (a `map[string]workspaceOverride`) replaces auto-detection when present. `resolveWorkspaces(root, hooks)` returns the merged result and picks the primary via `pickPrimary`: explicit `primary_workspace` field > first workspace with a `dev` script > first detected.

3. **Env propagation + skill awareness (downstream).** `monorepoEnvVars(workspaces, primary, mType)` builds the BELMONT_MONOREPO* env slice; `buildWorktreeEnv` appends it to every subprocess. `seedWorkspaceEnv` is called from `copyEnvFiles` for each detected workspace. Skills (`worktree-awareness.md` partial) and agents (`codebase-agent.md`, `implementation-agent.md`, `code-review-agent.md`, `verification-agent.md`, `reconciliation-agent.md`) gate workspace-aware command guidance on `BELMONT_MONOREPO=1`.

State copy: `copyBelmontStateToWorktree` carries `worktree.json` from master into the worktree's `.belmont/`. The in-worktree `loadWorktreeHooks(root)` is then called with `root = wtPath`, so the same `workspaces` config is visible inside the worktree — keeps the override coherent across master and worktree views.

## Failure mode if you break it

- **Without auto-detection** (only explicit override): every monorepo project must hand-author `worktree.json` to be usable. The Prisma failure recurs on first run; users hit the bug before they know they need the config. The whole point of "first-class support" is auto-detection on first run.
- **Without explicit override**: auto-detection has limits. Custom layouts (workspaces nested under non-standard parents, env consumption via a custom build script that doesn't list `dotenv` as a dep) won't trigger the heuristic. Power users need an escape hatch.
- **Pre-allocating per-workspace ports** beyond `BELMONT_PORT`: invents a naming scheme (`BELMONT_WEB_PORT`, `BELMONT_API_PORT`, `BELMONT_<NAME>_PORT`?) and forces every existing skill to grow a port-selection decision tree. The dynamic `FREE_PORT` rule already handles multi-service verification.
- **Broad env seeding (every detected workspace)**: pollutes pure-code workspaces (types-only packages, util libraries) where `.env` may shadow committed `.env.example`. Narrow heuristic + explicit `env_files` opt-in is the right blast radius.
- **Leaking the gate (skills tell single-package agents to use `--filter`)**: commands silently fail or run in wrong context. Always test single-package projects continue to work after any skill update.

## Don't re-do

- **Separate `.belmont/workspaces.yaml` file.** Considered. Cut: the schema is deeper than the flat `models.yaml` parser can handle, and `worktree.json` already exists as JSON with a clean place to extend. Splitting config across two files for no real win.
- **`{workspace}` template substitution in `worktree.json` hooks.** Considered. Cut: shell-quoting `{workspace}` correctly across pnpm/turbo/nx/bun is a maintenance burden. Hooks already run in a shell and can use `$BELMONT_PRIMARY_WORKSPACE` directly.
- **Pre-allocating `BELMONT_<NAME>_PORT` for each workspace.** See "failure mode" above. The `FREE_PORT=$(python3 -c …)` pattern is already mandated by `worktree-awareness.md` and works for any number of secondary servers.
- **A `belmont workspaces` subcommand** (detect/list/etc.). Considered. Cut: visibility lives in `belmont status`'s top-of-output `Monorepo:` line and the `monorepo` object in JSON output. A standalone subcommand adds surface area without solving a real workflow.
- **Prompting during `belmont install` to detect & write `workspaces.yaml`.** Considered. Cut: couples a conceptually independent feature (monorepo support) to onboarding flow and risks regressing the smooth single-package path. Auto-detection at worktree-creation time with a one-line `Detected <type> monorepo` log is enough.
- **Per-feature `workspace:` frontmatter in PRD.md.** Deferred. Many features legitimately span multiple workspaces (a typed contract change touching `packages/types`, `apps/web`, `apps/api`). Use task prefixes (`[WEB]`, `[API]`) by convention instead — that's free and reversible. Revisit only if convention proves insufficient.
- **Bazel / Buck / Pants detection.** Out of scope. Heavyweight build systems with custom rules; defer until a real user requests it. Detection signals are cheap; adding them is one diff away.
- **Auto-allocating per-workspace setup hook working dirs.** The user can `cd` inside a hook command if needed. Adding workspace-scoped hook dispatch would invite users to write per-workspace `setup` lists, which complicates the schema.

## Evidence

- A `belmont auto` run on 2026-05-07 against a Turborepo monorepo: `PrismaConfigEnvError: Cannot resolve environment variable: DATABASE_URL` during `npm install` postinstall. `.env` was at the worktree root; Prisma's TS config loader (post-`prisma.config.ts` migration) requires explicit dotenv loading; the workspace was nested at `packages/<workspace>/`. This is the canonical motivator.
- Cross-language detection survey: pnpm (`pnpm-workspace.yaml`), turbo (`turbo.json` + `package.json#workspaces`), nx (`nx.json` + `package.json#workspaces`), Cargo (`[workspace]` `members`), Go (`go.work` `use`), uv (`pyproject.toml#[tool.uv.workspace]`). Each has a stable signal file Belmont can probe in <1ms.

## Known rough edges

- **Env-signal heuristic false negatives.** A workspace that consumes env via a non-standard pattern (custom build script greps `.env` directly without listing `dotenv` as a dep) won't trigger auto-seeding. Documented escape hatch: explicit `env_files` in `worktree.json`. Hardening would require AST-level inspection of build scripts, which isn't worth it for an edge case.
- **Postinstall traps beyond Prisma.** Husky `prepare`, `patch-package`, `node-gyp` builds, Tauri's `tauri build` setup all run from workspace dirs and may need env. The current heuristic catches Prisma + dotenv-listed deps; others may need explicit `env_files`. Acceptable; documented in `docs/troubleshooting.md`.
- **`.gitignore` coverage.** Belmont seeds `.env` into `packages/<name>/.env`. If the user's `.gitignore` only covers root-level `.env` (not `**/.env`), the seeded file shows up in `git status`. Belmont prints a warning post-seed via `git check-ignore`; doesn't block. Users fix by updating `.gitignore`.
- **CI/local divergence.** `worktree.json` setup hooks run only in Belmont; CI doesn't run Belmont. Mirror critical setup in CI manually. Documented; can't be auto-fixed.

## Revisions

- 2026-05-07 — initial: detect Turborepo/Nx/pnpm/npm/yarn/bun/Lerna/Rush/Cargo/Go/uv; seed env into qualifying workspace dirs; export BELMONT_MONOREPO* env vars; skill + agent additive updates; `worktree.json` schema extension with `primary_workspace` + `workspaces` overrides.
