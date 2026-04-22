# Port Isolation

**Domains**: cli, skills, agents

**Why this matters.** When three sibling worktrees run in parallel, each wants a dev server, each wants Playwright, each wants Lighthouse. If any of them default to `localhost:3000`, they collide — silently. Symptom: some agents successfully use `$BELMONT_PORT`, others default to :3000 and hit a sibling's server, builds fail at OG image prerender, verifies misattribute the failures, implement phases re-thrash to "fix" the broken build, and the whole wave cascades into incoherence. This actually happened — see `belmont-test/about-3-fresh` in studia-web.

## Invariant

In a worktree (`$BELMONT_WORKTREE=1`):

- The project's **primary dev server** binds to `$BELMONT_PORT`.
- **Any other server** (Storybook, Prisma Studio, mock API, docs server) uses a dynamically-allocated free port, not a default from `package.json`.
- **Every URL** an agent uses is `$BELMONT_BASE_URL/...`. Never `http://localhost:3000/...` even if a committed config file or prior MILESTONE says so.
- Hardcoded ports in config files (`playwright.config.ts`, `cypress.config.*`) are **stale**. They are overridden at runtime by Belmont's env vars (`PLAYWRIGHT_BASE_URL`, `CYPRESS_baseUrl`) — not edited.
- If the worktree's port is taken, the agent **stops and reports a blocker**. Never kill unknown processes to free a port; that process may belong to the user, a sibling worktree, or a system service.

## How it's enforced

Two mechanisms in combination:

1. **Environment variables (mechanical; doesn't require agent cooperation).** `buildWorktreeEnv(port, extraEnv)` in `cmd/belmont/main.go` exports:
   - `PORT=<assigned>`
   - `BELMONT_PORT=<assigned>`
   - `BELMONT_BASE_URL=http://localhost:<assigned>`
   - `BELMONT_WORKTREE=1`
   - `PLAYWRIGHT_BASE_URL=http://localhost:<assigned>` — Playwright reads this and overrides `use.baseURL` / `webServer.url` at runtime
   - `CYPRESS_baseUrl=http://localhost:<assigned>` — Cypress reads this and overrides `baseUrl` at runtime
   - `VITE_PORT=<assigned>` — Vite honors this
   - Plus any `env` from `.belmont/worktree.json`
   
   These env vars alone eliminate the `localhost:3000` Playwright trap regardless of whether the agent reads the partial — Playwright picks up its env var even if the config says otherwise.

2. **Prose rules (reinforcement).** `skills/belmont/_partials/worktree-awareness.md` contains:
   - A `Port variables set for you` reference table (every env var listed above)
   - A `Port decision tree` — primary dev server uses bundler CLI directly with `$BELMONT_PORT` (a table per stack: Next.js, Vite, Astro, Nuxt, Remix, SvelteKit); secondary servers use a dynamically-allocated free port (`FREE_PORT=$(python3 -c "import socket…")`).
   - `Hard rules` — never curl/probe `localhost:3000`; hardcoded ports in committed configs are stale (override via env, do NOT edit configs); `npm run dev`/`pnpm dev`/`yarn dev` wrappers may not forward `$PORT` reliably (use the bundler CLI directly); kill only what you own (a port conflict is a blocker, not a license to `kill`).
   - The partial is `@include`d in every skill that might start a server. The same rules are copied into `agents/belmont/implementation-agent.md` and `agents/belmont/verification-agent.md`.

## Failure mode if you break it

- **Without env var defaults** (only prose rules): agents comply inconsistently. Some use `$BELMONT_PORT`; others run `npm run dev` in a subshell that doesn't inherit `PORT` and fall back to `:3000`. Playwright with a hardcoded `baseURL: 'http://localhost:3000'` in config navigates to port 3000 regardless — agent has no way to override short of editing the config (which pollutes the merge). This is the state the about-3 run ran in; two of three worktrees' browser tabs ended up on `localhost:3000`.
- **Without prose rules** (only env vars): covers the Playwright/Cypress/Vite common case but misses bespoke scripts. If a project has a `check-prod.sh` that greps `curl http://localhost:3000` hardcoded, agents following it hit the collision.
- **Editing checked-in configs to swap :3000 → $BELMONT_PORT**: every worktree fixes the same file, merges collide, state pollutes master. Non-starter.

## Don't re-do

- **Editing committed configs** to swap `:3000` → `$BELMONT_PORT`. Merge pollution; every worktree would "fix" the same file. Env-var override is non-invasive and never enters git.
- **Auto-starting the dev server from the setup hook** so the agent doesn't choose. Rejected: dev server lifetime doesn't match setup hook lifetime (would either leak processes across the wave or need new plumbing for teardown). Let the agent start servers; just make it mechanically impossible for them to pick the wrong port.
- **Killing conflicting processes automatically.** If port 3000 is occupied, it's someone else's — user's local dev session, a sibling worktree, a system service. Auto-killing would be catastrophic. Rule: STOP and report. Always.
- **Prose-only enforcement.** Tried this. Partial + agent files had the rules; compliance was inconsistent; Playwright still defaulted to :3000 because the config said so. If a rule can be expressed mechanically via an env var, prefer that.
- **Adding a `belmont port-preflight` subcommand** for agents to sanity-check before starting servers. Considered; deferred. The env-var + prose combination is carrying the load. Revisit if port conflicts return.

## Evidence

- `belmont-test/about-3-fresh` in studia-web: port cascade (user screenshot with two tabs on `localhost:3000`). This is the `without env vars` failure mode captured.
- `belmont-test/about-4-fresh` in studia-web: clean three-worktree wave; no port conflicts; Playwright screenshots of worktree-specific URLs. See [meta/validated-runs.md](../meta/validated-runs.md).

## Known rough edges

- **Bundler scripts that hardcode `-p 3000`** (e.g., a project whose `package.json` has `"dev": "next dev -p 3000"`) defeat the `PORT` env var because `-p` is an explicit override. The rule "invoke the bundler CLI directly with `$BELMONT_PORT`" is the answer; any agent following the wrapper here would collide. Caught only by the prose rules. Hardening would require parsing `package.json` scripts at setup time and rewriting them in the worktree, which is invasive and not worth it for an edge case.
- **Non-framework servers** (custom Node scripts, Python servers, etc.) that don't read `PORT` by convention need explicit per-tool command knowledge. The partial lists common cases; agents may encounter an unknown one. Current fallback: the agent reads the tool's docs or asks. Acceptable.

## Revisions

- 2026-04-22 — initial: env var fallback (`PLAYWRIGHT_BASE_URL` etc.), hardened partial with decision tree + bundler table + hard rules, agent files updated.
- 2026-04-22 — migrated from LEARNINGS.md to knowledge/ tree.
