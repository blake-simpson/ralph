# Parallel Wave Orchestration

**Why this matters.** The auto loop's hard work is orchestrating multiple milestones in parallel via git worktrees: creating them, copying state into them, running the loop inside each, merging their branches back in milestone-ID order, surfacing conflicts, and cleaning up. Every corner we cut here (shortcuts, symlinks, silent merges) has come back as a silent-data-loss bug. Uniform behavior per wave is worth the small startup cost.

## Invariant

- Every wave — including single-milestone waves — runs through the worktree path. No master-tree shortcut.
- Each worktree has isolated `.belmont/` state (a copy, not a symlink). The agent commits state changes to `belmont/auto/<feature>/<milestone>` as part of its work.
- Worktree-local files that master never holds (`STEERING.md`, and potentially others added later) are preserved across the resume-time wipe-and-recopy.
- Merges happen in milestone-ID order, sequentially, with pre-merge overlap reporting.
- Live state is observable from outside the run via `belmont status --feature <slug>`, which per-milestone overlays each worktree's view of its own milestone on top of master's baseline.

## How it's enforced

In `cmd/belmont/main.go`:

- `runAutoParallel` unconditionally dispatches to `runWaveParallel` for every wave (no single-milestone master-tree shortcut; that was removed 2026-04-22). The shortcut helper `singleMilestoneHasExistingWorktree` was deleted.
- `runWaveParallel` → `runMilestoneInWorktree`:
  - `git worktree add -b belmont/auto/<feature>/<ms>`
  - `copyBelmontStateToWorktree` overlays master's feature state on top of the worktree's HEAD checkout. Preserves `STEERING.md` (and future peers — see `ensureMigrations` if more appear) across the wipe-and-recopy.
  - Setup hooks run with the worktree's `$PORT` / `$BELMONT_PORT` / `$BELMONT_BASE_URL` etc. (see [cross-cutting/port-isolation.md](../cross-cutting/port-isolation.md)).
  - `runLoop(mCfg)` executes inside the worktree; `cfg.Root = wtPath` so everything the loop reads/writes is worktree-scoped.
- Merge loop in `runWaveParallel`:
  - Sort successes by `parseMilestoneNum`.
  - Before each merge, `reportMergeOverlap(cfg.Root, branch, msID, mergedFiles)` prints a visibility warning listing files the branch touches that earlier-merged siblings also touched. **Does not block** — scope guards + verify evidence + milestone-immutability should catch the cases where overlap implies scope leak; this is diagnostic so a human can still review before pushing.
  - Record this branch's touched files in `mergedFiles` for the next iteration's overlap check.
- Live status in `buildStatus`: when `loadAutoWorktreeStateByMilestone` returns a non-empty map, `overlayLiveMilestones` replaces each active milestone's tasks with the worktree's current view. Each overlaid milestone carries a `LiveFrom` pointer so the renderer tags it `(live from worktree)`.
- `auto.json` schema carries `mode` (`"single-feature-parallel"` or `"multi-feature"`) and `feature` slug so readers can tell per-milestone-worktree runs from feature-per-worktree runs.

## Failure mode if you break it

- **Re-introducing the single-milestone shortcut**: M1 runs in master tree; scope-guard amends rewrite the user's working branch history directly; `belmont steer` targets the wrong root; rollback is a `git reset` rather than `worktree remove`. Asymmetry between waves makes every other mechanism harder to reason about.
- **Not preserving STEERING.md across state copy**: the resume-time wipe-and-recopy silently deletes any pending user instructions that landed before auto resumed. User's steer reports success; zero injection fires. (This was the 2026-04-21 STEERING.md loss bug.)
- **Missing merge overlap report**: two branches write the same file, git picks one arbitrarily, the other's work disappears. Only detectable later when the feature "looks wrong." (This was the hero-section.tsx overwrite in the about-2 run.)
- **Missing live status overlay**: user has no way to observe parallel work in progress; has to wait until merge to see whether M2 is stuck or making progress. Blind flying on 30–60 minute wave durations.

## Don't re-do

- **Master-tree shortcut for single-milestone waves.** Was in place as an optimization to save ~5–10s of worktree setup. Cost: asymmetric behavior per wave, scope-guard amends on the wrong branch, confused `belmont steer` targeting. Rejected in the same session it was diagnosed; do not bring it back even under a flag. If worktree setup ever becomes a real bottleneck, make setup faster.
- **Symlinked `.belmont/` state across worktrees.** Was the pre-2025 default. Resulted in state races: worktree A's agent could read PROGRESS.md mid-flip while worktree B was writing. The copy-based isolation solved it; the merge-time reconciliation via `mergeWorktreeBranch` + reconciliation-agent handles the inevitable conflicts semantically.
- **Auto-serialize-on-directory-overlap** (detect that two milestones touch the same files at plan time and force serial execution). Heuristic is unreliable before either milestone runs. Over-serializes conservatively, kills the point of parallel mode. Scope guard + merge overlap report between them give the same coverage at run time without the heuristic.
- **Caching or daemonizing `belmont status`** so it doesn't re-read every worktree's PROGRESS.md per call. Current cost is ~N file reads per invocation; N is usually <10; infrastructure cost of a daemon is far too high for a CLI that runs on demand.

## Evidence

`belmont-test/about-4-fresh` in studia-web: clean parallel wave (M2/M3/M4) with merge-overlap report firing on `about/page.tsx`, reconciliation-agent resolving the import-union at high confidence, live status overlay showing `(live from worktree)` tags during the run. See [meta/validated-runs.md](../meta/validated-runs.md).

Unit coverage: `cmd/belmont/scope_guard_test.go` → `TestOverlayLiveMilestones_*`, `TestCopyBelmontStateToWorktreePreservesSteering`.

## Known rough edges

- **`stash-before-merge` can drop state-file edits.** When master has uncommitted changes under `.belmont/features/<slug>/` at merge time, `runWaveParallel` stashes them to clear the tree, merges the worktree branch, but PROGRESS.md edits captured in the stash may not pop cleanly. Result: master's PROGRESS.md shows `[ ]` for tasks that actually landed in code. Workaround: `belmont sync` + `belmont reverify`. Proper fix: make the merge path aware of `.belmont/features/<slug>/` as state-sensitive.
- **`STEERING.md` left on disk at completion.** The file is only deleted by `consumePendingSteering`, which fires during auto phases. When auto ends with only consumed entries, nothing triggers the final delete. Cosmetic; fix is a cleanup sweep at end-of-auto.

## Revisions

- 2026-04-21 — initial (worktree lifecycle, state copy).
- 2026-04-22 — removed single-milestone master-tree shortcut; unified every wave through `runWaveParallel`.
- 2026-04-22 — added live-status overlay via `overlayLiveMilestones`, `worktreeTracker.feature`/`mode` fields, `loadAutoWorktreeStateByMilestone`.
- 2026-04-22 — added `reportMergeOverlap` pre-merge visibility.
- 2026-04-22 — migrated from LEARNINGS.md to knowledge/ tree.
