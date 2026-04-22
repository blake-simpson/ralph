# Steering

**Domains**: cli, skills, auto-mode

**Why this matters.** Headless AI CLI invocations (Claude Code, Codex, Gemini, Copilot, Cursor in `-p`/batch mode) don't see stdin. Once an auto phase is shell-out'd, the user's only existing interrupt channel is Ctrl+C — which loses work. `belmont steer` is the way to hand new instructions to an in-flight auto run without killing it. The same plumbing doubles as the self-correction channel for the scope guard and verify-evidence guard: when they revert a violation, they inject a `(pending)` entry that the next phase's agent receives as an URGENT prompt block.

## Invariant

- `STEERING.md` contains **only pending entries**. Consumed entries are dropped from disk; when no pending entries remain, the file is deleted.
- Consumption happens **before the shell-out** — exactly once per entry per agent run. If a phase crashes, the entry is still recorded as consumed; the user re-steers explicitly if needed. This prevents the "phase retry re-injects the same instruction" failure mode.
- The file lives **inside the worktree** at `<worktree>/.belmont/features/<slug>/STEERING.md`. In serial single-feature mode it lives at the master feature directory under the same path.
- Preservation across worktree state copy: `copyBelmontStateToWorktree` reads the worktree's `STEERING.md` into memory, performs the wipe-and-recopy from master, then writes STEERING.md back. Master never holds it; without preservation the resume would silently clobber pending user instructions.
- `belmont steer` requires an active `.belmont/auto.json` — steering only applies to in-flight auto mode. Manual skill sessions are steered by typing directly into the terminal.

## How it's enforced

In `cmd/belmont/main.go`:

- **Write path**: `runSteerCmd` — CLI flags `--feature`, `--milestone`, `--message`, `--file`, `-` (stdin), `$EDITOR` fallback. Resolves the feature from `auto.json` (explicit or auto-detected when single); resolves targets (broadcast to every active worktree for the feature, or narrow to `--milestone`); appends a `(pending)` entry per target with RFC3339 UTC timestamp and optional `[M5]` milestone tag.
- **Read/consume path**: `consumePendingSteering(root, feature, milestoneID, phase)` called at the top of `executeLoopAction`. Parses entries, matches pending ones by milestone tag (empty tag matches any milestone), returns the formatted block prefixed with `steeringHeader()` plus count. Rewrites STEERING.md with only remaining pending entries; deletes the file when `len(remainingPending) == 0`.
- **Injection**: the returned block is prepended to the agent prompt before shell-out (both `executeLoopAction` and `executeTriageAction`). Stream prints `[feature][milestone]: [STEERING] injected N instruction(s) — "<preview>"`.
- **Preservation across resume**: `copyBelmontStateToWorktree` in `runMilestoneInWorktree` preserves `STEERING.md` across the feature-dir wipe-and-recopy.
- **Self-correction by other guards**: `injectScopeGuardSteering` (Layer 1) and `injectEvidenceSteering` (Layer 2) both use `appendSteeringEntry` to write pending entries that the next phase consumes. The guard's correction flows through the same channel as user-authored steering.

## Failure mode if you break it

- **Consumed entries persisted in-file**: agents exploring `.belmont/features/<slug>/` re-read the consumed text (it's a file in the directory they routinely scan for context). Wastes input tokens, blurs the signal, may produce duplicate interpretation of already-applied steering. This was the v1 behavior; the current invariant (drop consumed, delete when empty) was the fix.
- **Consumption after shell-out success**: phase retries on failure or within-iteration re-entries would re-inject the same instruction. Runaway risk. Current order (consume → shell-out) guarantees one-shot delivery.
- **Not preserving across state copy**: resume-time wipe silently clobbers user instructions. User sees `belmont steer` report success, then sees zero injection fire in the subsequent run. Debugging this is a nightmare because the write path worked. Covered by `TestCopyBelmontStateToWorktreePreservesSteering`.
- **Preview log too verbose**: printing full steering text to stream when multi-line user instructions are injected produces unreadable walls of color in the terminal. Current format is count + first ~100 chars preview; full text lives in STEERING.md (while pending) or in the stream's injection event (after consumption).

## Don't re-do

- **v1 flip-to-consumed design** (consumed entries kept in-place with `(consumed <ts>)` marker). Wasted input tokens on agents re-reading them. Replaced with drop-when-consumed + delete-when-empty.
- **Sidecar `STEERING.log.md` for audit**. Was considered as a way to keep the audit trail while cleaning the agent-facing file. Rejected because another file in the same directory just doubles the surface area agents can read; the audit already lives in the stderr stream with timestamps.
- **Consume after shell-out success**. Rejected: phase retries would re-inject indefinitely. Consume-before-shell-out gives one-shot semantics. User re-steers if a phase crashed mid-instruction — rare in practice, explicit when it happens.
- **Making `belmont steer` work for manual skill sessions**. Manual skills run outside Belmont's process; there's no consume hook to invoke. User can already type steering directly into the manual terminal. Keeping steering auto-mode-only preserves a clear boundary.

## Evidence

- `belmont-test/about-3-fresh` and `belmont-test/about-4-fresh` in studia-web: both runs show `[STEERING] injected N instruction(s) — …` stream lines, confirming consume fires and delete-when-empty behaves.
- Unit coverage: `cmd/belmont/steer_test.go` → `TestConsumePendingSteering`, `TestConsumePendingSteeringDropsLegacyConsumedOnly`, `TestConsumePendingSteeringMissingFile`, `TestCopyBelmontStateToWorktreePreservesSteering`, `TestStripSteerComments`.

## Known rough edges

- **STEERING.md left on disk when auto completes with only-consumed entries.** `consumePendingSteering` deletes the file when `len(remainingPending) == 0`, but that only fires during an auto phase. If the last auto phase consumed the final entry, the deletion happens. If auto ends with consumed entries remaining because of a different code path, the file sticks around. Cosmetic; fix is a cleanup sweep at end-of-auto.
- **Multi-feature runs**: `runSteerCmd` currently handles single-feature parallel mode cleanly. Multi-feature mode (`auto.json` with feature-slug keys in `Worktrees`) has not been exercised end-to-end with steer. Likely needs one code path tweak when we use multi-feature for real.

## Revisions

- 2026-04-21 — initial: `belmont steer` command, `STEERING.md` lifecycle, scope guard / verify guard self-correction integration.
- 2026-04-21 — v2 lifecycle: drop consumed, delete when empty (replaced flip-in-place v1 to save agent input tokens).
- 2026-04-21 — preserve STEERING.md across `copyBelmontStateToWorktree` resume.
- 2026-04-22 — migrated from LEARNINGS.md to knowledge/ tree.
