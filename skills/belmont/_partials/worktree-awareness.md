## Worktree Environment

If the environment variable `BELMONT_WORKTREE` is set to `1`, you are running in an isolated git worktree for parallel execution. Several sibling worktrees may be running the same project concurrently on different ports. Ignoring the port rules below **will cause silent merge conflicts, verification flakes, and processes killing each other** — treat this section as load-bearing.

### Port variables set for you

Belmont populates these before your process starts. Use them directly; do not guess at port numbers, and do not copy ports out of `package.json` or config files.

| Variable | Purpose |
|---|---|
| `BELMONT_PORT` | Unique primary port for this worktree. Use for the project's dev server. |
| `PORT` | Mirror of `BELMONT_PORT`. Most bundlers (Next.js, many Node servers) honor this. |
| `BELMONT_BASE_URL` | `http://localhost:$BELMONT_PORT`. Use anywhere a URL is expected. |
| `PLAYWRIGHT_BASE_URL` | Overrides `use.baseURL` / `webServer.url` in `playwright.config.*` at runtime. Playwright reads this automatically. |
| `CYPRESS_baseUrl` | Overrides `baseUrl` in `cypress.config.*` at runtime. Cypress reads this automatically. |
| `VITE_PORT` | Mirror of `BELMONT_PORT` for Vite-based projects. |
| `BELMONT_WORKTREE` | Set to `1`. Presence signals that worktree rules apply. |

### Port decision tree

**Question 1 — is this the project's primary dev server?**

Yes: invoke the bundler CLI directly with the worktree's port. **Do NOT use `npm run dev` / `pnpm dev` / `yarn dev`** — those wrappers may not forward `$PORT` reliably (different projects wire them differently, and some scripts add `-p 3000` or similar literally). Go around the wrapper.

| Project stack | Command to run |
|---|---|
| Next.js | `next dev -p $BELMONT_PORT` (add `--turbo` if the project uses Turbopack) |
| Vite | `vite --port $BELMONT_PORT` |
| Astro | `astro dev --port $BELMONT_PORT` |
| Nuxt | `nuxt dev --port $BELMONT_PORT` |
| Remix | `remix dev` with `PORT=$BELMONT_PORT` (Remix honors `PORT`) |
| SvelteKit | `vite dev --port $BELMONT_PORT` |
| Rails / Django / Flask | pass the port via the framework's `-p`/`--port` flag |

No, it's a secondary server (Storybook, Prisma Studio, docs, mock API, etc.): **dynamically allocate a free port and pass it explicitly.** Do NOT use the port from `package.json` scripts — those defaults (6006 for Storybook, 5555 for Prisma Studio, etc.) collide across parallel worktrees.

```bash
FREE_PORT=$(python3 -c "import socket; s=socket.socket(); s.bind(('127.0.0.1',0)); print(s.getsockname()[1]); s.close()")

# Then pass FREE_PORT to the tool, bypassing the npm wrapper:
npx storybook dev -p $FREE_PORT --no-open
npx prisma studio --port $FREE_PORT
npx @stoplight/prism mock api.yaml --port $FREE_PORT
```

### Hard rules

1. **Never curl, probe, or assume `localhost:3000`** (or any other well-known default) is "yours". A port that's already bound from outside your worktree belongs to someone else — another worktree, the user's own dev session, the previous run. Always use `$BELMONT_PORT` / `$BELMONT_BASE_URL`.
2. **Hardcoded ports in committed config files are stale.** If `playwright.config.ts` sets `baseURL: 'http://localhost:3000'`, the env vars above override it at runtime — **do NOT edit the config**. Run tests as normal; Playwright/Cypress/etc. will pick up the env var. Editing a checked-in config to change the port would pollute the merge.
3. **Hardcoded ports in planning docs are stale.** If a `TECH_PLAN.md`, `PRD.md`, `NOTES.md`, or archived `MILESTONE-*.done.md` mentions `localhost:3000` or any specific port, treat it as documentation from a prior non-parallel run. Your ground truth is `$BELMONT_BASE_URL`.
4. **Never run `npm run dev` / `pnpm dev` / `yarn dev` / `npm run storybook` / `npm run test:e2e` without first confirming** the wrapped command forwards `$PORT` and `$PLAYWRIGHT_BASE_URL`. When in doubt, bypass the wrapper and invoke the underlying CLI (`next dev`, `vite`, `playwright test`) directly.
5. **Kill only what you own.** If your dev server fails to start because the port is taken, STOP and report it as a blocker — do not free the port by killing unknown processes. Another worktree, the user, or a system service may own it.
6. **If a port is in use, find another one — do not retry the same port.** The `FREE_PORT=$(python3 -c ...)` snippet above is idempotent and safe.

### Beyond ports

- **Dependencies**: worktree setup hooks have already run (e.g., `npm install`). Do NOT re-install unless you're explicitly adding a new package as part of the task.
- **Build isolation**: `.next/`, `dist/`, `node_modules/`, and other gitignored directories are local to this worktree. Other worktrees are unaffected by your builds.
- **Scope**: only modify files within this worktree. Changes will be merged back via git — the scope guard will revert edits outside your target milestone.
