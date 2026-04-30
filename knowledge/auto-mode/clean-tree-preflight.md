# Clean-Tree Preflight & Update Auto-Commit

**Why this matters.** Worktree merges write back to the same branch the user started auto on. Any uncommitted, unstaged, or untracked file in that branch that the merged content would overwrite triggers `error: Your local changes to the following files would be overwritten by merge` and aborts the merge — leaving a preserved worktree, a half-finished run, and a manual recovery for the user. The most common precursor was `belmont update` rewriting the installer's own files (`.agents/belmont/`, `.agents/skills/belmont/`) without committing them; the user then ran `belmont auto` against a tree that looked clean to them but wasn't to git. This entry exists because that exact scenario chewed up a 30-minute Phase 2 / scrum-master M5 run in the wild (April 2026).

## Invariant

- `belmont auto` refuses to start when `git status --porcelain --untracked-files=normal` reports anything.
- `belmont update` auto-commits Belmont-managed files (only those) after a successful self-update + skill reinstall.
- The two together close the loop: `update` no longer leaves uncommitted state behind, and `auto` no longer trusts that the user committed before running.

## How it's enforced

In `cmd/belmont/main.go`:

- `requireCleanWorkingTree(root)` (near `validateRepoState`) runs `git status --porcelain --untracked-files=normal`. Non-empty output → error. Returns nil silently when the path isn't a git repo (rare but possible — `auto` will fail downstream anyway with a clearer message).
  - Path classification via `pathIsBelmontManaged` against `belmontManagedPaths` (`.agents/belmont`, `.agents/skills/belmont`, `.claude/agents/belmont`, `.claude/commands/belmont`, `.codex/belmont`, `.cursor/rules/belmont`, `.windsurf/rules/belmont`, `.gemini/rules/belmont`, `.copilot/belmont`, `AGENTS.md`). When any porcelain entry matches, the error message names the `belmont update` situation explicitly; otherwise it's a generic dirty-tree warning.
  - Note: `strings.TrimSpace` on the porcelain output corrupts the first line — porcelain entries start with a status code that may include a leading space (e.g. ` M path`). Use `strings.TrimRight(out, "\n")` instead.
- `runAutoCmd` calls `requireCleanWorkingTree(absRoot)` once after tool resolution, before dispatching to `runLoop` / `runAutoMultiFeature` / `runAutoParallel`. One check covers every dispatch path. Bypassed by `--allow-dirty` (explicit opt-out) and `--dry-run` (no merges happen).
- `commitBelmontUpdate(root, version)` (in `runUpdate`) runs after the auto-install shell-out succeeds:
  1. Skip silently if `git rev-parse --is-inside-work-tree` doesn't return `"true"`.
  2. Filter `belmontManagedPaths` to paths that exist on disk via `os.Lstat` (claude-only installs don't have `.codex/belmont/` etc.; `git add` errors on missing pathspecs and aborts the whole call if any are absent).
  3. `git add -- <existing-paths>` stages.
  4. `git diff --cached --quiet -- <existing-paths>` detects no-op (avoids empty commits on repeat runs).
  5. **`git commit -m "Update Belmont to vX.Y.Z" -- <existing-paths>`** with a pathspec. The pathspec is critical: without it, `git commit` would also sweep in any unrelated changes the user had previously staged.
  6. Hooks run normally (no `--no-verify`). On hook failure: yellow warning, files left staged, manual `git commit -m "..."` printed for retry.
- `belmont update --no-commit` skips `commitBelmontUpdate` entirely and prints the equivalent manual command.

Test coverage in `cmd/belmont/commit_update_test.go` exercises the happy path, no-op detection, non-git-dir skip, preservation of unrelated user work (both staged and unstaged), and both message variants of `requireCleanWorkingTree`.

## Failure mode if you break it

- **Drop the preflight**: re-introduces the original April 2026 bug. `update` writes new agent files, user runs `auto` immediately, worktree forks off the dirty tree, M-late merges back, merge aborts mid-feature. User loses ~30 min of agent work and has to recover by hand via `belmont recover --merge`.
- **Drop the path filter in `commitBelmontUpdate`**: `git add` errors on the first missing path (e.g. `.codex/belmont` in a claude-only install) and aborts before staging any of the others. Silent regression — update appears to succeed but commits nothing.
- **Drop the pathspec on `git commit`**: any unrelated change the user had staged (e.g. they were partway through composing a commit when they ran `belmont update`) gets swept into the "Update Belmont to vX.Y.Z" commit. Loud regression but easy to miss in a test that only checks "did Belmont files land in HEAD."
- **`strings.TrimSpace` on porcelain output**: the first line's leading status-code space gets eaten, `line[3:]` skips the leading `.` of the path, `pathIsBelmontManaged` misses the match, the Belmont-aware hint never fires. Cosmetic but defeats the whole point of the helpful error. (This was the bug found during initial testing — fixed by switching to `TrimRight(out, "\n")`.)
- **Run `git commit` with `--no-verify`**: bypasses repo's pre-commit hooks. Some users intentionally rely on hooks to catch lint/format issues; silently bypassing breaks their guarantees. Stay aligned with what a manual `git commit` would do.

## Don't re-do

- **Soft-warn instead of hard-block on dirty tree.** A warning the user can ignore loses the protection — the failure mode is silent (worktree merges fine until M-late) and the user has no reason to think about it during the noisy auto run. The block must be enforced at startup, before any worktree exists, with `--allow-dirty` as the documented escape hatch.
- **Auto-stash on the user's behalf.** Tempting because it's silent, but a `git stash` you didn't run yourself is invisible context that can get popped at the wrong time, conflict on pop, or be forgotten and `git stash list` later. The user explicitly committing or stashing keeps the mental model honest.
- **Auto-commit unrelated user changes too.** "Helpfully" sweeping the working tree into a Belmont commit is the opposite of what `--allow-dirty` users want, and would silently mix Belmont's update with the user's in-progress feature work. The pathspec on `git commit` is non-negotiable.
- **Move the auto-commit to `belmont install`.** `install` runs in many contexts (initial setup, partial reinstalls, scripted bootstraps, CI). A surprise commit from any of those is intrusive. Keep the commit in `update` only — that's the path with a clear "I just rewrote your tooling" semantics.
- **Always commit `--no-verify`.** Pre-commit hooks aren't optional in the repos that have them. Let them run; if they fail, leave the staged files in place and tell the user how to retry. (Confirmed user choice.)
- **Compute the `belmontManagedPaths` allow-list dynamically by scanning the `setupTool` switch.** Tempting because it'd auto-track new tools, but the allow-list is the explicit contract of "what `belmont update` is allowed to commit on the user's behalf." Reviewers should be able to grep one slice and know the answer. Keep it static; update it when adding a tool.

## Evidence

- Reproduced and traced from a real Phase 2 / scrum-master M5 failure (April 2026) where `git merge` aborted on 14 paths under `.agents/belmont/` and `.agents/skills/belmont/` after a clean-looking auto run.
- Unit coverage: `cmd/belmont/commit_update_test.go` → `TestCommitBelmontUpdate_*`, `TestRequireCleanWorkingTree_*`.

## Revisions

- 2026-04-30 — initial (clean-tree preflight in `runAutoCmd` + `update` auto-commit + `--allow-dirty` / `--no-commit` opt-outs).
