# Debug Spec Reconciliation

**Domains**: skills, state

**Why this matters.** When a Belmont feature ships with a bug, the bug is often a symptom of a spec problem upstream — an acceptance criterion that didn't match shipped behaviour, a TECH_PLAN decision that turned out wrong in practice, a PRD task whose `**Solution**:` describes an approach that was abandoned mid-implementation. Fixing the code without correcting the spec leaves Belmont's memory wrong, so the next `/belmont:implement` or `/belmont:verify` session operates on stale truth and silently re-introduces drift. The point of `/belmont:debug-manual`'s Spec Reconciliation phase is to propagate the discovered truth back into the spec in the same atomic commit as the code fix, so reviewing `git log` tells the complete story of what changed and why.

## Invariant

Interactive `/belmont:debug-manual` is the **only** skill that may edit spec prose in place (PRD task text, TECH_PLAN narrative, NOTES root-cause patterns, PROGRESS `[x]` follow-up flips). Every other skill — `implement`, `verify`, `next`, `debug-auto`, `tech-plan`, triage — continues to operate under `milestone-immutability.md`, which forbids restructuring and is the load-bearing constraint for auto-mode parallel orchestration. The relaxation is bounded:

- **Interactive only.** `debug-manual` is never invoked from `belmont auto` — `actionDebug` routes to `/belmont:debug-auto` (`cmd/belmont/main.go`). The auto-mode `runScopeGuard` cannot fire against `debug-manual` edits.
- **Human-gated.** Each spec edit is presented as a unified diff and gated on explicit `y / N / edit / skip` approval. No silent writes.
- **No restructuring.** `debug-manual` may not add/rename/remove milestone headings, may not use polish/follow-up/cleanup/FWLUP/"deviations from"/"verification fixes" naming, may not add new `[ ]` follow-up tasks for unfixed drift, and may not flip a task to `[v]`. `[x]` flips are scoped to the current or last-shipped milestone of the feature being debugged.
- **Atomic commit.** Code edits and spec edits land in one commit. Spec changes are auditable in `git log` next to the code change they correspond to.

## How it's enforced

1. **Skill prose.** `skills/belmont/_partials/debug-scope-rules.md` is `@include`d in `debug-manual.md` only and lists the allowed/forbidden edits explicitly. Every other skill keeps the `@include milestone-immutability.md` line; `debug-manual` is the sole skill that swapped includes.
2. **`belmont validate`.** Still runs at `belmont auto` startup and rejects polish-pattern milestone names (`polish`, `follow-ups`, `cleanup`, `verification fixes`, `deviations from M<N>`, `from M<N> implementation`, `fwlup(s)`). If `debug-manual` ever adds such a milestone in violation of `debug-scope-rules.md`, the next auto run blocks at startup — fail loud, fail early.
3. **`runEvidenceCheck`.** Commit-message attribution still required for `[v]` flips on a future verify pass. `debug-manual` only writes `[x]` (never `[v]`), but `debug-scope-rules.md` requires task IDs in the commit message body so the evidence check finds attribution if `/belmont:verify` later promotes those tasks.

## Failure mode if you break it

Three failure modes, ranked by severity:

1. **`debug-manual` is allowed to add a polish milestone.** `belmont validate` catches it on the next auto run, blocks startup, and surfaces the violation to the user. The user runs `/belmont:tech-plan` to restructure. Loud failure, cheap to diagnose — this is the desired behaviour of the layered enforcement.
2. **`debug-manual` flips a task to `[v]` instead of `[x]`.** The verify-evidence guard reverts it on the next auto run because the commit didn't go through `/belmont:verify`'s code path. Recoverable — the task drops back to `[x]` and the next verify pass treats it normally.
3. **`debug-manual` edits a feature's specs that wasn't selected.** The user discovers the cross-feature pollution when reviewing the commit or running `git diff`. No runtime guard catches this (it's a prose-level violation); mitigation is the per-feature approval gate in `feature-detection-multi.md`. If we see this in practice, add a runtime check in Go.

## Don't re-do

- **Allowing `debug-auto` to also edit specs in place.** Considered and rejected. `debug-auto` runs verification via a sub-agent with no user-in-the-loop for diff approval. Auto-applying spec edits violates the "user approves each spec edit" invariant; the failure mode would be silent spec corruption that only surfaces at next review.
- **Removing the `@include milestone-immutability.md` line from every skill, not just `debug-manual`.** This was the original temptation and is wrong. The auto-mode disaster (`belmont-test/about-2-dynamic-mode`) preserved in `knowledge/meta/validated-runs.md` shows what happens when auto-mode skills are free to add milestones. The relaxation has to stay scoped to interactive-only invocation.
- **Routing spec edits through `/belmont:tech-plan` instead.** Considered. Rejected because (a) Blake explicitly asked for in-place edits without the extra step, (b) `/belmont:tech-plan` is a planning skill, not a debugging skill — invoking it from a debug session conflates two different mental models, (c) the spec edits a debug session needs are narrow (text rewrites, NOTES additions, `[x]` flips), not the structural reshaping `tech-plan` exists for.
- **Auto-applying every drift candidate without per-edit approval.** Rejected. The user explicitly chose `/belmont:debug-manual` (vs `/belmont:debug-auto`) because they want to verify each step. Per-edit approval matches the user-verified-iteration mental model the skill has had since inception.
- **Filing unfixed drift as new `[ ]` follow-up tasks for `/belmont:implement` to pick up later.** Rejected per Blake's explicit instruction: "fix the actual bugs and fix the drift" in the same session. Parking work for later was the existing pattern and creates exactly the drift-on-drift compounding this work is designed to eliminate.

## Evidence

- `skills/belmont/_src/debug-manual.md` — Step 0 deep context load, Spec Reconciliation phase, Commit-and-Report atomic-commit step.
- `skills/belmont/_partials/debug-scope-rules.md` — the explicit allow/deny list, swapped in for `milestone-immutability.md` in this skill only.
- `skills/belmont/_partials/feature-detection-multi.md` — multi-feature variant; per-feature approval gates spec edits across features.
- `skills/belmont/_src/references/debug-manual-spec-reconcile.md` — drift catalogue, per-edit decision flow, root-cause pattern template.
- `cmd/belmont/main.go` `actionDebug` wiring confirms `debug-manual` is never invoked from auto (only `debug-auto`).

## Known rough edges

- **PR_FAQ.md can be large.** Step 0 has a `> 500 lines` warning + `[y/N]` gate. If users routinely override it on large PR_FAQs, consider section-targeted loading (Customer Letter + Internal FAQ only).
- **Multi-feature pollution risk.** No runtime guard prevents `debug-manual` from editing a feature outside `{bases[]}`. Currently only prose-enforced via `feature-detection-multi.md`'s per-feature approval gate. If observed in practice, add a Go-side check.
- **Multiple debug-manual sessions on the same branch.** No serialization. Two parallel debug-manual sessions editing overlapping specs would race. Belmont's clean-tree-preflight (`requireCleanWorkingTree`) blocks auto mode on dirty trees but doesn't apply to interactive sessions. Mitigation: don't run two interactive debug sessions concurrently on the same branch.

## Revisions

- 2026-05-11 — initial: `debug-manual` enhanced with deep context load + multi-feature + in-place spec reconciliation; `debug-scope-rules.md` partial added; `milestone-immutability.md` include removed from this skill only.
