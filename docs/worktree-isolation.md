# Worktree Isolation

When `belmont auto` runs multiple features or milestones in parallel, each one executes in its own [git worktree](https://git-scm.com/docs/git-worktree). This page explains how isolation works and how to configure it for your project.

## How It Works

Each parallel worktree gets:

- **`.env*` files copied** from the project root automatically (since they're gitignored and not present in fresh worktrees)
- **A unique port** assigned automatically via `PORT` and `BELMONT_PORT` environment variables
- **Its own file tree** — gitignored directories like `node_modules/`, `.next/`, `dist/` are local to each worktree
- **Shared `.belmont/` state** via symlink for coordination
- **Process group isolation** — processes started by the AI agent are tracked and cleaned up when the worktree is removed

## Environment Variables

These are automatically set for every worktree:

| Variable | Description |
|----------|-------------|
| `PORT` | A unique free port assigned to this worktree. Most frameworks (Next.js, Vite, Express, Rails, Django) respect this automatically. |
| `BELMONT_PORT` | Same value as `PORT`. Use this in skills/agents for explicit port references. |
| `BELMONT_WORKTREE` | Set to `1` when running in a worktree. Use to detect worktree context. |

## Worktree Hooks

Create `.belmont/worktree.json` in your project to configure lifecycle hooks:

```json
{
  "setup": ["npm install --prefer-offline"],
  "teardown": [],
  "env": {
    "NEXT_TELEMETRY_DISABLED": "1"
  }
}
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `setup` | `string[]` | Commands to run after worktree creation, before the AI agent starts. Runs in the worktree directory with `PORT`/`BELMONT_PORT` available. |
| `teardown` | `string[]` | Commands to run before worktree removal. Also runs on interrupt (Ctrl+C). |
| `env` | `object` | Extra environment variables injected into both hooks and the AI agent process. |

### Examples

**Node.js (npm)**
```json
{
  "setup": ["npm install --prefer-offline"]
}
```

**Node.js (pnpm)**
```json
{
  "setup": ["pnpm install --prefer-offline"]
}
```

**Python**
```json
{
  "setup": ["python -m venv .venv", ".venv/bin/pip install -r requirements.txt"]
}
```

**Ruby (Bundler)**
```json
{
  "setup": ["bundle install"]
}
```

## Common Concerns

### Port Conflicts

Handled automatically. Each worktree gets a unique `PORT` from the OS. Most web frameworks respect the `PORT` environment variable by default. The AI agents are also instructed to use `$PORT` when starting dev servers.

### Dependency Isolation

Git worktrees get their own file tree. Since `node_modules/` is typically gitignored, each worktree starts without dependencies. Use the `setup` hook to install them.

If one feature adds a new package, that change is isolated to its worktree until merged. Other worktrees are unaffected.

For disk space efficiency with Node.js projects, consider using [pnpm](https://pnpm.io/) (shared content-addressable store) or the `--prefer-offline` flag.

### Build Interference

Build output directories (`.next/`, `dist/`, `.turbo/`, etc.) are gitignored and therefore local to each worktree. Builds in one worktree cannot corrupt another.

Note: With Next.js specifically, running `next build` while `next dev` is running in the **same** worktree can cause turbopack issues. Each worktree should either run dev OR build, not both simultaneously.

### Database Conflicts

Multiple features writing to the same local database can cause issues. Strategies:

- Use per-worktree database names via the `env` field:
  ```json
  {
    "env": {
      "DATABASE_URL": "postgresql://localhost/myapp_test"
    }
  }
  ```
- Use SQLite with a gitignored path (each worktree gets its own copy)
- Use a shared database with feature-specific prefixes

### Shared Caches

Most build caches (`.next/cache`, `.turbo/`, `__pycache__/`) are gitignored and thus worktree-local. No special configuration needed.

### API Rate Limits

Multiple AI agents may hit the same external APIs in parallel. This is not something belmont can mitigate — consider using mock servers or rate-limiting configuration in your test environment.
