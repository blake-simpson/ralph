# Monorepo Support

Belmont auto-detects monorepos and adjusts worktree setup so AI agents can run commands in the correct package, find env files in the place where postinstall scripts actually run, and discover sibling workspaces. This page covers what's detected, what gets seeded into worktrees, and how to override the defaults.

## Supported monorepo systems

| System | Detection signal | How members are resolved |
|---|---|---|
| Turborepo | `turbo.json` | piggybacks on `package.json` `workspaces` (or `pnpm-workspace.yaml`) |
| Nx | `nx.json` | piggybacks on `package.json` `workspaces` (or `pnpm-workspace.yaml`) |
| pnpm workspaces | `pnpm-workspace.yaml` | parses simple `packages:` glob list |
| npm / yarn / bun workspaces | `package.json` `workspaces` array | expands globs (literal, `pkg/*`, `pkg/**`); negation `!` is honored |
| Lerna | `lerna.json` | parses `packages` field |
| Rush | `rush.json` | parses `projects[].projectFolder` |
| Cargo workspaces | `Cargo.toml` `[workspace]` section | parses `members` glob list |
| Go workspaces | `go.work` | parses `use (...)` directives |
| uv workspaces | `pyproject.toml` `[tool.uv.workspace]` | parses `members` |

Detection is tolerant: malformed signal files are treated as "no signal here" rather than aborting. Multiple signals can coexist (a Turborepo with `pnpm-workspace.yaml` is detected as `turborepo`).

When no signal matches, Belmont falls back to single-package behavior — every existing project keeps working without changes.

## What Belmont does for monorepos

When a monorepo is detected, every parallel worktree gets these additional behaviors on top of the [worktree isolation](worktree-isolation.md) basics:

### Workspace-aware env file seeding

Belmont copies `.env*` from the project root into the worktree root (existing behavior) **and** into qualifying workspace directories. A workspace qualifies for env seeding if any of the following is true:

- It declares an explicit `env_files` list in `worktree.json` (always seeded).
- Its `package.json` has a `postinstall` script (or `postinstall:*` variant).
- Its dependencies include `prisma`, `@prisma/client`, `dotenv`, `dotenv-cli`, `drizzle-kit`, `tsx`, or `vite-node`.
- Its `Cargo.toml` workspace has a `build.rs` build script.
- Its `pyproject.toml` declares `[project.scripts]` or `[tool.poetry.scripts]`.

Pure-code workspaces (types-only packages, util libraries with no env consumption) are **not** seeded — this avoids polluting `git status` and accidentally creating files where users may have committed `.env.example`.

If the seeded path would not be matched by `.gitignore`, Belmont prints a warning so you can fix your ignore rules before committing.

### Workspace-aware env vars

In addition to the standard worktree env vars (`PORT`, `BELMONT_PORT`, `BELMONT_BASE_URL`, etc.), Belmont exports:

| Variable | Value |
|---|---|
| `BELMONT_MONOREPO` | Always `1` when in a monorepo. Use as a guard. |
| `BELMONT_MONOREPO_TYPE` | One of `turborepo`, `nx`, `pnpm`, `npm`, `yarn`, `bun`, `cargo`, `go`, `uv`, `lerna`, `rush`. |
| `BELMONT_PRIMARY_WORKSPACE` | ID of the primary workspace (the one that hosts the dev server with `$BELMONT_PORT`). |
| `BELMONT_PRIMARY_WORKSPACE_PATH` | Path of the primary workspace, relative to the worktree root. |
| `BELMONT_WORKSPACES` | JSON array of `[{"id":"web","path":"packages/web"}, ...]`. |

Single-package projects don't get these vars, and skills' `if BELMONT_MONOREPO=1` checks short-circuit. Adding monorepo support has zero impact on existing single-package installs.

### Singular `BELMONT_PORT` is preserved

The dev server port is still singular. The primary workspace's dev server gets `$BELMONT_PORT`. Other workspaces (mock APIs, docs sites, Storybook) follow the existing rule: AI agents allocate a free port at runtime via the `FREE_PORT=$(python3 -c …)` snippet documented in [worktree-isolation.md](worktree-isolation.md). This avoids inventing a port-naming scheme and works the same way for single- and multi-package projects.

## Overriding auto-detection

Drop a `.belmont/worktree.json` with the new fields to override or augment what Belmont auto-detects:

```json
{
  "setup": ["pnpm install --prefer-offline"],
  "teardown": [],
  "env": {
    "NEXT_TELEMETRY_DISABLED": "1"
  },

  "primary_workspace": "web",
  "workspaces": {
    "web": {
      "path": "packages/studia-web",
      "env_files": [".env", "packages/studia-web/.env.local"]
    },
    "api": {
      "path": "apps/api"
    }
  }
}
```

| Field | Type | Default | Meaning |
|---|---|---|---|
| `primary_workspace` | string | first workspace with a `dev` script (or first detected) | Workspace ID that gets `$BELMONT_PORT` for the primary dev server. |
| `workspaces` | map | auto-detected | Workspace ID → `{path, env_files}`. When present, replaces auto-detection completely. |
| `workspaces.<id>.path` | string | required | Workspace directory, relative to project root. |
| `workspaces.<id>.env_files` | string[] | empty | Extra env files (relative to project root) to seed into the workspace. The workspace dir is seeded even if it would otherwise be skipped by the auto heuristic. |

All four new fields are optional. Existing `worktree.json` files with no monorepo fields parse identically and behave the same way as before.

## How AI agents use this

When `BELMONT_MONOREPO=1`, Belmont's skills and agents adjust their behavior:

- **Codebase agent** enumerates workspaces in `## Codebase Analysis` so downstream agents know the layout.
- **Implementation agent** uses workspace filters for build/test/lint/install commands. It runs `pnpm --filter web run build` instead of `pnpm run build`, `npm -w api install foo` instead of `npm install foo`, etc.
- **Code-review agent** runs build/test scoped to the workspace(s) the milestone touched.
- **Verification agent** `cd`s into `$BELMONT_PRIMARY_WORKSPACE_PATH` (or uses the workspace tool's filter) before invoking the bundler, then starts additional servers via the `FREE_PORT` pattern when verification needs them.
- **Reconciliation agent** keeps lockfile post-resolve commands at the repo root (correct for every JS/Cargo/Go monorepo system).

Skills that should not behave differently — port allocation, scope guards, evidence checks, steering — are unchanged.

## Troubleshooting

### Prisma `DATABASE_URL` errors during install

Symptom: `npm install` fails inside a worktree with `PrismaConfigEnvError: Cannot resolve environment variable: DATABASE_URL`. The `.env` is at the monorepo root but `prisma generate` runs inside `packages/<your-package>/` and Prisma's TS config loader doesn't auto-load parent `.env` files.

**Fix**: Belmont's auto env-seeding handles this for any workspace whose `package.json` has a `postinstall` script or includes `prisma`/`@prisma/client` in its deps. If your workspace doesn't trigger the heuristic but still needs the env (e.g. a custom build script that reads `.env`), add explicit `env_files` to your `worktree.json`:

```json
{
  "workspaces": {
    "web": {
      "path": "packages/web",
      "env_files": [".env", "packages/web/.env.local"]
    }
  }
}
```

### Agent runs `pnpm dev` from the wrong directory

Symptom: the dev server starts but binds to a random port the agent isn't using, or it errors that no `dev` script is defined at the root.

**Fix**: confirm `BELMONT_MONOREPO=1` is set (it should be after auto-detection). The implementation/verification agent's worktree-awareness rules then tell it to `cd "$BELMONT_PRIMARY_WORKSPACE_PATH"` or use `pnpm --filter "$BELMONT_PRIMARY_WORKSPACE" exec next dev -p $BELMONT_PORT`. If detection didn't fire (e.g. you have a non-standard layout), add an explicit `workspaces` map and `primary_workspace` to `worktree.json`.

### `.gitignore` warning after env seed

Symptom: Belmont prints `⚠ Seeded packages/web/.env but it is not gitignored …`.

**Fix**: update your `.gitignore` to cover nested env files. The most common pattern:

```
.env
.env.*
!.env.example

# Or, for monorepo-wide coverage:
**/.env
**/.env.*
!**/.env.example
```

Belmont won't ever commit a seeded `.env` (worktrees commit only the feature branch), but a stray nested `.env` can show up in `git status` and confuse interactive workflows.

### CI/CD doesn't see the same env

Symptom: builds work locally in the Belmont worktree but fail in CI.

**Fix**: `worktree.json` setup hooks and env seeding are local to Belmont's worktree machinery. CI doesn't run Belmont. Mirror critical setup (env wiring, `dotenv -e`, install commands) in your CI configuration as well.

## Non-goals

These are deliberately not in scope right now and may surface in future iterations:

- **Per-feature workspace targeting** as enforced schema. Use `[WEB]` / `[API]` task prefixes by convention, or note target workspaces in the optional `## Target Workspace(s)` section of `PRD.md`.
- **Pre-allocated per-workspace ports** beyond `$BELMONT_PORT`. The dynamic `FREE_PORT` pattern handles multi-service verification.
- **Bazel / Buck / Pants detection.** Heavyweight build systems with custom rules; defer until requested.
- **`{workspace}` template substitution in hooks.** Hook authors can use `$BELMONT_PRIMARY_WORKSPACE` directly.

See [knowledge/cross-cutting/monorepo-workspaces.md](../knowledge/cross-cutting/monorepo-workspaces.md) for the architectural invariants and rejected alternatives.
