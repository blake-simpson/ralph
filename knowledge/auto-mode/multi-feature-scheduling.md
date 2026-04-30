# Multi-Feature Scheduling — Dep Gating & Wave Order

**Why this matters.** `belmont auto --features=A,B,C` (and `--all`) groups features into waves via Kahn's topological sort over the master `Dependencies` column. Two scheduling rules are load-bearing for not-wasting-an-agent-run: (1) when a feature pauses on `[!]` blockers, its dependents must skip — they cannot make progress without the dep's code merged — and (2) sibling tie-breaks within a wave must respect the user's CLI `--features=` order, not alphabetical. Both were broken in the wild on a 3-feature studia-web run (April 2026): `foundation` paused on one `[!]`, `parent` and `student` still launched in wave 2 from base HEAD, both cascade-paused on missing primitives. The execution-plan banner also reordered `student, parent` → `parent, student` alphabetically, a separate surprise.

## Invariant

- **Pause cascades.** If a feature returns `errFeaturePaused` (or is skipped because a dep paused), its slug enters `pausedSlugs`. Subsequent waves run `filterWaveByBlocked` against `failedSlugs` ∪ `pausedSlugs`; dependents are skipped before any worktree is created, with a `Reason` of `"failed"` or `"paused"` mirroring the blocking dep's state.
- **CLI order wins for siblings.** `runAutoMultiFeature` builds its `features` slice by iterating the user-supplied `slugs` in order; `computeFeatureWaves` gathers each wave's `ready` set by scanning that input slice (no map iteration, no alphabetical post-sort). `--all` has no caller-supplied order and is sorted alphabetically once at `resolveFeatureSlugs` for determinism.
- **Pre-flight readiness warning.** Before scheduling, `scanReadiness` emits one yellow line per requested feature whose declared dep is not yet `isFeatureTerminal`. Warning-only — the operator can Ctrl-C before launch.
- **Halt summary on pause-cascade.** When `totalMerged == 0 && len(pausedSlugs) > 0`, the final report swaps the generic "N feature(s) failed" block for a structured `⏸ paused / ⊘ skipped / Fix and rerun.` block.
- **Resume is plan-free.** `[r]`-resume in `handleStaleWorktree` does not re-evaluate the dep graph — and doesn't need to. Each fresh `belmont auto` invocation re-runs `computeFeatureWaves` from disk; if the dep is still `[!]`-blocked, it pauses again and the cascade rule fires again.

## How it's enforced

In `cmd/belmont/main.go`:

- `filterWaveByBlocked(wave, failed, paused)` — pure helper near `computeFeatureWaves`. Scans each feature's deps in declaration order; first failed dep wins (failed > paused for messaging since "failed" is the harder signal and the user should see it first). Returns `(runnable, []skipResult)` where `skipResult{Slug, DepSlug, Reason}` lets the caller emit per-reason coloured logs and route the slug into the correct set.
- `runAutoMultiFeature`:
  - Maintains `failedSlugs map[string]bool` and `pausedSlugs map[string]bool` side-by-side.
  - On `errFeaturePaused` from a wave goroutine: log `⏸ <slug> paused`, set `pausedSlugs[slug] = true`, append to `allFailures` (so the existing non-zero exit path triggers). Do **not** set `failedSlugs`.
  - Per-wave: `runnable, skipped := filterWaveByBlocked(w.Features, failedSlugs, pausedSlugs)`. For each `skipResult`, emit yellow (`paused`) or red (`failed`) line, route into the matching set so the cascade chain continues into the next wave, and append to `allFailures`.
  - End-of-run: if `totalMerged == 0 && len(pausedSlugs) > 0`, replace the generic failure list with the structured halt summary distinguishing originating-pauses from skipped-due-to-paused (heuristic: `Err.Error()` starts with `"dependency "` for the latter).
- `computeFeatureWaves` ordering:
  - In-degree map built once over input.
  - Wave-loop `ready` set is gathered by scanning the **input slice**, not the map, so caller order is preserved with zero extra sort. The previous `sort.Slice(ready, by-slug)` block was deleted.
- `resolveFeatureSlugs(--all)` calls `sort.Strings(slugs)` before returning so the alphabetical contract is set at the call site, not inside the scheduler.
- `scanReadiness(features) []readinessWarning` — pure. Reuses `isFeatureTerminal` and surfaces `TasksBlocked` count when the dep is `In Progress` with `[!]` tasks.

Test coverage in `cmd/belmont/scope_guard_test.go`:
- `TestComputeFeatureWaves_PreservesInputOrder`, `TestComputeFeatureWaves_DependencyBeatsInputOrder`
- `TestResolveFeatureSlugs_AllFlagAlphabetical`
- `TestFilterWaveByBlocked_PausedDepCascades`, `TestFilterWaveByBlocked_FailedAndPausedDistinct`, `TestFilterWaveByBlocked_TransitiveSkip`, `TestFilterWaveByBlocked_FailedWinsOverPaused`
- `TestScanReadiness_FlagsNonTerminalDeps`, `TestScanReadiness_TerminalDepsSilent`

## Failure mode if you break it

- **Drop pause-gating** (revert to "don't add to failedSlugs (downstream deps may still be satisfiable)"): the studia-web cascade returns. Foundation pauses, parent+student each launch a worktree off base HEAD with no foundation primitives, agent realises all M1 tasks are unimplementable, marks them `[!]`, pauses. Three orphaned worktrees, ~3 wasted agent-minutes, user has to clean up and figure out the dep order manually. Worse: if any sibling actually merges, the user gets a partial merge that looks "successful" until they read the diff.
- **Drop CLI-order preservation** in `computeFeatureWaves`: alphabetical sibling order ignores user intent. Cosmetic-looking but it shapes the run — with `--max-parallel=1`, the alphabetical winner runs first and may leave the user-preferred starting feature for last.
- **Drop the pre-flight readiness warning**: silent foot-gun. Cascade still fires, but the operator only learns about the dep state by waiting for wave 2's first `⊘ skipped` line, by which point a worktree has been created and torn down. The warning lets them Ctrl-C in seconds.
- **Make pre-flight abort instead of warn**: kills legitimate workflows where the user wants to start dep-and-dependent in the same invocation (which is exactly what `--features=A,B,C` is for — A is wave 1, B/C are wave 2). The warning is correct; the abort would be over-eager.
- **Halt the whole queue on any pause** (instead of skip-dependents-of-paused only): kills the value of independent feature branches in the dep graph. If the user passes `--features=A,B,X` where `B depends:A` and `X` is independent, halting on A's pause would stop X needlessly. Skip-dependents preserves the parallel-mode value while still gating dependents correctly.

## Don't re-do

- **Halt-the-whole-queue-on-any-pause as default.** Considered and rejected. The right rule is skip-dependents-of-paused; halt only when the queue empties (every remaining feature was skipped). For users who want strict halt semantics, the right knob is a dedicated `--halt-on-pause` flag — not flipping the default.
- **Threading a `preferredOrder` parameter into `computeFeatureWaves`.** Considered and rejected. Cleaner and shorter to make the function's ordering contract "input order wins" and have callers pre-arrange. One parameter saved, one obvious mental model gained.
- **Re-evaluating the dep graph on `[r]`-resume.** Not needed. The next fresh invocation re-reads on-disk state, re-runs `computeFeatureWaves`, and the cascade rule does the right thing. Adding plan-aware resume would duplicate logic for zero gain.
- **Auto-expanding `--features` to include transitively-required deps not in the list.** Surprise behaviour. If `student` depends on `foundation` and the user passed only `--features=student`, today the scheduler silently treats the missing dep as satisfied (in-degree only counts deps present in `bySlug`). That's a separate near-bug — the right fix is a clear error or warning, not implicit set expansion. Documented here so it doesn't get conflated with the cascade fix.
- **Counting `[!]` tasks at run end by parsing the worktree's `PROGRESS.md`.** Considered for the halt summary's "1 blocked task(s)" line. Rejected — the pre-flight warning already shows the count from the start-of-run snapshot, and the post-run state is already in the worktree for the user to inspect via `belmont status`. Don't duplicate.

## Evidence

- Original studia-web bug report (Sophos, April 2026): `--features=tutor-subscriptions-foundation,tutor-subscriptions-student,tutor-subscriptions-parent --max-parallel=1`. Foundation paused on `P0-M1-FIX-2`; parent+student both launched off base HEAD and re-paused with all M1 tasks `[!]`'d. Execution plan also showed `parent, student` instead of CLI's `student, parent`.
- Unit coverage: `cmd/belmont/scope_guard_test.go` → `TestComputeFeatureWaves_*`, `TestFilterWaveByBlocked_*`, `TestScanReadiness_*`, `TestResolveFeatureSlugs_AllFlagAlphabetical`.

## Revisions

- 2026-04-30 — initial (cascade-skip on pause via `pausedSlugs`, CLI-order via input-slice iteration, `scanReadiness` pre-flight warning, structured halt summary).
