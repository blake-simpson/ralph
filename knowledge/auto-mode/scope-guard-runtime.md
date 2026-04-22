# Scope Guard (Runtime)

**Why this matters.** Prose rules in skills don't hold on their own. Agents rationalise exceptions, skill content drifts between files, and a single contradictory sentence in one skill lets the whole invariant collapse. The scope guard is the load-bearing **runtime** enforcement — it runs after every agent subprocess exits, compares PROGRESS.md before/after, and reverts anything out of scope. It cannot be bypassed by `git commit --no-verify` because it runs in the Belmont Go process, not inside the agent.

## Invariant

For every agent phase except `actionReplan` (tech-plan), the phase may only:

1. Edit tasks **inside** its target milestone's `## M<N>:` heading.
2. Leave every other milestone's heading and task checkboxes exactly as they were before the shell-out.

Any deviation — a new milestone heading added, a sibling milestone's checkbox flipped, a task state changed outside the target — is reverted and the agent's last commit is amended to carry only the in-scope edits.

## How it's enforced

All in `cmd/belmont/main.go`:

- `executeLoopAction` and `executeTriageAction` snapshot PROGRESS.md before the shell-out via `snapshotProgress` → `progressSnapshot` struct. After `cmd.Wait()` returns (success or failure), they call `runScopeGuard(cfg, action, preSnap)`.
- `runScopeGuard` skips for `actionReplan` (tech-plan legitimately restructures), otherwise re-reads PROGRESS.md, calls `diffScopeViolations(pre, post, action.MilestoneID)` to produce a list of `scopeViolation` structs (`new_milestone` or `out_of_scope_flip` kinds), then `rebuildAfterScopeGuard(pre, post, targetMS)` produces the repaired content, the file is rewritten, `git commit -a --amend --no-edit` folds the revert into the agent's commit, and a `(pending)` entry is appended to STEERING.md via `injectScopeGuardSteering` so the agent sees an explicit correction on its next phase.
- The stream log format matches the steering convention: `[feature][milestone]: [SCOPE-GUARD] reverted N violation(s) — <summary>`.

`diffScopeViolations` treats an empty `targetMS` as "unscoped" (permissive for checkbox flips, still strict on new milestones) so `actionImplementNext` batch sweeps don't false-positive.

## Failure mode if you break it

Without the guard: agents flip checkboxes outside their milestone (the `ea672675` pattern — an M5 polish commit bulk-marked M3's P1-5..P1-8 as `[v]` without implementation). Or agents spawn new milestones mid-run to hold deferred items, which declare `(depends: M<N>)` and run in parallel with siblings whose files they mutate, producing silent merge conflicts.

With a broken guard (e.g., scope comparison wrong): either noise (guard reverts legitimate in-scope edits, agent re-does → loop) or silence (guard misses real violations). The `TestScopeGuard_EA672675ReplayScenario` test in `scope_guard_test.go` is specifically designed to catch the silent-miss regression.

## Don't re-do

- **Git pre-commit hooks.** Bypassable. Agents run with `--permission-mode bypassPermissions` / `--yolo` / `--dangerously-bypass-approvals-and-sandbox` depending on tool; `git commit --no-verify` is one keystroke. Only enforcement **outside** the agent subprocess holds.
- **Filesystem ACLs** making non-scope files read-only. Cross-platform nightmare (Darwin/Linux/Windows differ), and can't represent "only edit your own section of this file" — PROGRESS.md is one file containing every milestone's state.
- **LD_PRELOAD / DYLD_INSERT_LIBRARIES** interposing on file writes. Vastly out of proportion to the problem; platform-specific; tooling nightmare.
- **Per-milestone PROGRESS fragment files** (split PROGRESS.md into `M1.md`, `M2.md`, …). Architecturally cleaner — scope violation becomes structurally impossible for checkbox flips. Rejected for now because it requires every skill that references PROGRESS.md plus every CLI status parser to be rewritten. Revisit only if the runtime guard proves fiddly; both layers are redundant-but-harmless if adopted together.
- **Prose-only enforcement.** We tried this. `verify.md` had the rule, `implement.md` had the contradictory permission. Agents took the permissive path. Runtime enforcement is how we stop relying on agent compliance with text.

## Evidence

The `ea672675` commit in `belmont-test/about-2-dynamic-mode` (studia-web repo) is the canonical replay case — an M5 FWLUP commit bulk-marking M3 tasks `[v]`. The `belmont-test/about-4-fresh` branch shows the same shape of work completing cleanly with the guard active. See [meta/validated-runs.md](../meta/validated-runs.md) for diff commands.

Unit coverage: `cmd/belmont/scope_guard_test.go` → `TestDiffScopeViolations_*`, `TestRebuildAfterScopeGuard_*`, `TestScopeGuard_EA672675ReplayScenario`.

## Known rough edges

- **Stash-before-merge can drop PROGRESS.md edits.** When a worktree branch is being merged back and the master tree has uncommitted state-file changes, `runWaveParallel` stashes them. Post-merge, the stash may not pop cleanly for `.belmont/features/<slug>/PROGRESS.md` specifically, leaving master's PROGRESS with stale checkbox states even though the source code landed correctly. Workaround: `belmont sync` + `belmont reverify` after the run. Proper fix: make the merge path aware of state files and preserve them across stash/pop. Deferred.

## Revisions

- 2026-04-21 — initial: scope guard + diff/rebuild + STEERING correction loop.
- 2026-04-22 — migrated from LEARNINGS.md to knowledge/ tree.
