# Worktree Rebase on Resume

**Why this matters.** Belmont's master `Dependencies` column (parsed by `parseMasterDeps`) only captures dependencies the user has declared up front. Implicit cross-feature task deps — e.g. feature `core-drill`'s home screen calling `/browse` which lives in feature `reference-browse` — are invisible to the scheduler at plan time. Two siblings in the same wave can therefore legitimately interact: one ships first, the other marks `[!]` blocked, then the first merges. Without an automatic rebase on resume, the blocked worktree stays pinned to its pre-merge fork point forever; the `[!]` blocker stays valid in the worktree's view even after the depended-upon feature is on main. The user's next `belmont auto` run for that feature re-pauses on the same `[!]` because the code it needs still appears to be missing.

## Invariant

- Every fresh `belmont auto` invocation that hits `handleStaleWorktree`'s `[r]`-resume branch attempts `git rebase <mainHEAD>` in the worktree before any agent phase runs.
- The rebase is gated on a clean worktree: if `git status --porcelain` is non-empty (ignoring `.belmont/` which is `assume-unchanged`), the rebase is **skipped** with a yellow `⚠ Skipped rebase of <id> — worktree has uncommitted changes` line. Resume proceeds with the worktree on its previous base.
- On rebase conflict, the rebase is **aborted** (`git rebase --abort`); the worktree HEAD is left exactly where it was, and a yellow `⚠ Rebase of <id> aborted: <git output>` line is emitted. Conflict resolution is never attempted automatically.
- `.belmont/features/<slug>/STEERING.md` (and the rest of `.belmont/` which is `assume-unchanged` in worktrees) is untouched by the rebase. STEERING preservation is verified by `TestRebaseWorktreeOnMain_PreservesSteeringMd`.
- Rebase fires only on **fresh invocation**. The `[r]`-resume path *inside* an already-running auto loop does not re-rebase mid-loop; that would risk shifting branches under an active scope-guard amend cycle.

## How it's enforced

In `cmd/belmont/main.go`:

- `rebaseWorktreeOnMain(mainRoot, wtPath string) (newCommits int, err error)` — pure helper near `removeWorktree`. Returns `errWorktreeDirty` (with `newCommits == 0`) on dirty-tree skip; returns a wrapped `"rebase conflict: …"` error after `git rebase --abort` on conflict; returns `(N, nil)` after a successful rebase where `N` is the number of new main commits brought in (computed via `git rev-list --count <merge-base>..<mainHEAD>`). The target SHA is reached directly (no fetch) because the worktree shares `.git/objects` with `mainRoot`.
- `announceWorktreeRebase(id, n, err)` — shared logging helper so both call sites print identical wording (`↻ Rebased <id> worktree onto main (<N> new commits)` on success, `⚠ Skipped rebase of <id>…` on dirty, `⚠ Rebase of <id> aborted…` on conflict, silent on a no-op).
- `handleStaleWorktree` calls `rebaseWorktreeOnMain` + `announceWorktreeRebase` immediately after the `[r]`-resume choice is confirmed (both the "worktree still exists" and "reattach to branch" sub-branches), **before** any `.belmont/` refresh logic. Conflicts there don't block resume; they just leave the worktree as-is and let the agent re-evaluate.
- The `auto-mode/multi-feature-scheduling.md` invariant on `MaxParallel <= 1` ("merges interleave with execution") means the rebase target advances mid-wave on serial runs, so a paused later feature in the same wave can naturally pick up an earlier sibling's merge on the next attempt.

Test coverage in `cmd/belmont/rebase_test.go`:
- `TestRebaseWorktreeOnMain_NoOpWhenAtMain`
- `TestRebaseWorktreeOnMain_BringsInNewMainCommits`
- `TestRebaseWorktreeOnMain_SkipsOnDirtyWorktree`
- `TestRebaseWorktreeOnMain_AbortsOnConflict`
- `TestRebaseWorktreeOnMain_PreservesSteeringMd`
- `TestRebaseWorktreeOnMain_ReportsZeroWhenWorktreeAhead`

## Failure mode if you break it

- **Drop the rebase entirely.** Paused worktree stays on its stale fork point. Cross-feature implicit deps that landed on main between runs stay invisible to the worktree's agent. `[!]` blockers re-fire on resume; cascade-skipped dependents re-skip. This is exactly the geoguesser-meta 2026-05-12 failure that motivated the entry: `reference-browse` merged after `core-drill` paused, `core-drill` resumed against pre-merge main, FIX-3 (`[!]` on `/browse`) stayed blocked.
- **Auto-flip `[!]` → `[ ]` post-rebase.** The rebase delivers code, but only the agent can know whether the blocker is actually resolved (the file may exist with a different signature, the route may exist but 404, etc.). Mechanical flipping would lie to downstream phases. Leave it to the implementation agent's next blocker re-evaluation pass.
- **Drop the clean-tree gate and stash automatically.** Risks losing in-flight `debug-manual` edits or hand-applied fixes the user did between runs. Skip-and-warn is the safe default; if a user really wants to rebase a dirty worktree, they can commit or stash manually first.
- **Try to resolve conflicts automatically.** Belmont has no semantic understanding of what changes are correct here; auto-resolution would silently fork branches and produce subtle bugs. Abort-and-warn keeps the failure visible and recoverable.
- **Rebase on `[r]`-resume mid-loop (not just fresh invocation).** Would shift the branch under an active scope-guard amend cycle in the same loop, breaking the post-phase revert invariants. Keep it bounded to fresh-invocation resume.

## Don't re-do

- **Fetch from `origin` before rebasing.** Considered. Unnecessary — worktree and main repo share `.git/objects`, so the main HEAD SHA is already reachable. Fetching `origin` also touches user-configured remotes (network calls, auth) and isn't relevant: we want main's local tip, not its remote.
- **Refresh `.belmont/features/<slug>/PROGRESS.md` from main after a successful rebase.** Considered. The worktree's PROGRESS.md is the *live* state (with `[!]` markers, in-progress flips, etc.); main's copy is the *last-merged* snapshot, which is stale for any paused feature. Overlaying main's would clobber the worktree's truth. Trust the worktree's PROGRESS.md and let the agent re-evaluate blockers.
- **Eagerly propagate each merge to other paused sibling worktrees mid-wave at `MaxParallel > 1`.** Considered. Adds conflict-handling complexity in the hot path and would require stdin per paused sibling. The next fresh invocation catches the same cases via this entry's rebase-on-resume; no need to do it mid-wave.
- **Use `git pull --rebase` instead of `git rebase <sha>`.** `git pull` requires an upstream tracking branch, which Belmont feature branches don't have. The direct SHA form keeps the helper self-contained and doesn't touch remote config.
- **Hook the rebase into the auto-cleanup fall-through (non-interactive `belmont auto` discovering a stale branch).** That path deliberately *deletes* the stale branch and starts fresh — there's nothing to rebase. Out of scope. (Whether non-interactive should default to resume rather than restart is a separate question, tracked elsewhere.)

## Evidence

- Geoguesser-meta cascade, 2026-05-12. `belmont auto --all --max-parallel=1` placed `content-pipeline`, `reference-browse`, `core-drill` in the same wave (no master-level deps between them). `core-drill`'s M2 home screen needed `/browse`; the agent correctly marked `P1-M2-FIX-3` `[!]` blocked. Wave's post-wave merge loop merged `reference-browse` **after** `core-drill` paused, so `/browse` was now on main but `core-drill`'s worktree was still on the pre-merge fork point. Three dependent features cascade-skipped. The full failure shape is preserved in the user's report under `i-m-working-in-code-personal-apps-geogue-sunny-hummingbird.md`.
- Unit coverage: `cmd/belmont/rebase_test.go`.

## Revisions

- 2026-05-12 — initial (`rebaseWorktreeOnMain` helper, fresh-invocation gate, clean-tree skip, conflict abort, STEERING preservation verified). Motivated by geoguesser-meta cross-feature implicit task dep cascade.
