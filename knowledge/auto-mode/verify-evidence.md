# Verify Evidence Check

**Why this matters.** The verify skill can rubber-stamp `[v]` on tasks whose code is still scaffold. A well-structured scaffold passes build, lint, typecheck, and visual smoke tests; "renders without crashing" is indistinguishable from "actually implemented." Without an independent evidence check, a feature that looks `[v]` across the board can be 30% unimplemented — this is how `P1-5`..`P1-8` ended up marked verified in `about-2-dynamic-mode` while their component files were byte-for-byte the P0-1 scaffolds.

## Invariant

Every `[v]` flip in PROGRESS.md must be preceded by at least one commit on the current branch (since the merge base with main/master) whose message mentions the task ID. A flip without supporting commit evidence is reverted post-phase.

## How it's enforced

In `cmd/belmont/main.go`, called after `runScopeGuard`:

- `runEvidenceCheck(cfg, action, preSnap)` is invoked after every `actionVerify` phase. Other phases are skipped — only verify marks tasks `[v]`.
- `findEvidenceMissingFlips(root, pre, post, targetMS)` walks post-snapshot's milestones, collects every task whose state flipped to `v` this phase, and calls `taskHasCommit(root, taskID, sinceRef)` per candidate.
- `taskHasCommit` runs `git log --format=%B%x1e <mergeBase>..HEAD` and greps each commit message with a regex that ensures word-boundary around the task ID (`(^|[^A-Za-z0-9-])P1-1([^A-Za-z0-9-]|$)`) so `P1-1` doesn't false-positive on `P1-12`.
- `findMergeBaseRef` tries `main`, `master`, `origin/main`, `origin/master` in order. If none resolves, the search is unscoped (entire `HEAD` log) and `taskHasCommit` **fails open** (returns true) to avoid blocking real work on a shallow clone or detached HEAD.
- On missing evidence: `revertEvidenceMissing` rewrites the task's line in PROGRESS.md from `[v]` back to the pre-phase state (usually `[x]`), `git commit -a --amend --no-edit` folds the revert into the verify agent's commit, `[VERIFY-GUARD] reverted N [v] flip(s) lacking commit evidence — …` prints to the stream, and `injectEvidenceSteering` appends a `(pending)` STEERING entry naming the specific tasks so the next phase's agent sees the correction.

## Failure mode if you break it

- **No evidence check**: verify agent marks task `[v]`, no implementing commit exists, follow-up phases assume the task is done, downstream milestones build on a stub. Final feature ships incomplete but reports 100% verified. (This is the `about-2-dynamic-mode` failure mode — see [meta/validated-runs.md](../meta/validated-runs.md).)
- **Check too strict** (e.g., missing the word-boundary regex): P1-1 gets credited by commits for P1-12 or P1-100. False-positive evidence; silent pass-through of bad `[v]`s.
- **Check too lax** (e.g., `fails open` fires on every run because merge-base lookup fails): the guard is there but does nothing. Would manifest as verify-guard stream lines never appearing during real runs.
- **Evidence sourced from master log instead of branch log**: picks up task IDs from prior features or unrelated work. Merge-base scoping is what makes the check specific to this branch's work.

## Don't re-do

- **Heuristic depth checks** (file size delta, line count delta, AST depth beyond scaffold). Forgery-prone: an agent can satisfy them by padding stub files. Commit-log evidence requires a commit with the specific task ID, which is itself the thing we want.
- **Evidence manifest emitted by the verify skill** (JSON with `{taskID: {file, line_count, contains: [...]}}` which the CLI validates). Would be stronger — the skill knows exactly what to check — but much more infrastructure: new JSON schema, skill-side emission logic, CLI-side parsing, forward-compat concerns. Deferred. Commit-evidence works today with zero protocol changes because the task-ID-in-commit-message convention already exists in practice.
- **Running all tests from the CLI as the evidence**. Already covered by the verify skill itself; the CLI-level check is about attribution, not test results.
- **Blocking on failed evidence rather than reverting and correcting.** Would halt progress on genuinely lightweight tasks that the agent verified via reasoning rather than a commit. The `revert + STEERING correction` path lets the agent self-correct (either by producing the commit or by raising the task as a blocker). Keeps the loop moving.

## Evidence

`belmont-test/about-4-fresh` in studia-web: the full 4-milestone run completed with `[v]` flips only for tasks whose commits named them. No repeat of the P1-5..P1-8 rubber-stamp pattern. See [meta/validated-runs.md](../meta/validated-runs.md).

Unit coverage: `cmd/belmont/scope_guard_test.go` → `TestFindEvidenceMissingFlips_NoGitRepo`, `TestRevertEvidenceMissing_FlipsTaskLineBack`. The no-repo test verifies fail-open behavior.

## Known rough edges

- **Multi-task commits** (e.g., `"P1-5 + P1-6: both done"`). The regex matches each ID independently, so both tasks get credit. If someone writes `"Implemented P1-5 through P1-8"` the regex matches `P1-5` and `P1-8` literally — `P1-6` and `P1-7` would NOT match. Convention: list each task ID separately in the commit message. Not a bug in the check, a documentation point for commit-message style.
- **Commit-less tasks** (configuration-only, documentation-only tasks that happen to leave no file change). Should be rare in practice; verify skill must either make a docs commit naming the task or leave the task `[x]` and report to the user. If this becomes common, revisit and consider the evidence manifest approach.

## Revisions

- 2026-04-21 — initial: commit-log evidence check, word-boundary regex, fail-open on git errors.
- 2026-04-22 — migrated from LEARNINGS.md to knowledge/ tree.
