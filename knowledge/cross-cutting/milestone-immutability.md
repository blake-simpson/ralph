# Milestone Immutability

**Domains**: skills, state, auto-mode

**Why this matters.** Agents left to their own devices will invent a "Polish / follow-ups from M<N>" milestone to hold deferred items discovered during implement or verify. That milestone declares `(depends: M<N>)`, which makes it a sibling of every later `M<N+i>` that also depends on M<N>. But its actual work mutates files those siblings depend on. Running them in parallel produces silent merge conflicts that only surface after the user reviews the final feature and sees something is wrong. The root cause of the about-2-dynamic-mode cascade was exactly this — an M5 polish milestone editing `hero-section.tsx` in parallel with M2.

## Invariant

Only `/belmont:tech-plan` may add, remove, rename, or re-parent a `## M<N>:` heading in PROGRESS.md. Every other skill (implement, verify, next, debug-auto, debug-manual, triage) may only edit tasks **inside** existing milestone headings.

Routing for discovered work:

- **Follow-up from M<N>'s own implement/verify cycle** → new `[ ]` task inside M<N>.
- **Follow-up blocked by work that will land in a later M<N+k>** → new `[!]` task inside M<N>, one-line reason naming M<N+k>. Reopens as `[ ]` when the blocker lifts.
- **Cosmetic / nice-to-have item the user may never want** → append to `NOTES.md` under `## Polish`. Not a milestone task.
- **Never a new milestone.** Not "M<last+1>: Polish", not "M<N>-FIX", not "MX: Deviations from M<N>", not "MY: Verification Fixes". Even if existing PROGRESS.md already contains such a milestone from a prior run, do not add to it and do not create siblings.

## How it's enforced

Three layers, each sufficient on its own but deployed together for defense in depth:

1. **Skill prose** — canonical text lives in `skills/belmont/_partials/milestone-immutability.md` and is `@include`d into `implement.md`, `verify.md`, `next.md`, `debug-auto.md`, `debug-manual.md`, `tech-plan.md`, and referenced by `prompts/belmont/post-verify-triage.md`. The partial is the single source of truth; skill bodies point to it rather than paraphrasing.
2. **Runtime scope guard** — `runScopeGuard` in `cmd/belmont/main.go` reverts new milestone headings added during any non-`actionReplan` phase. See [auto-mode/scope-guard-runtime.md](../auto-mode/scope-guard-runtime.md).
3. **CLI lint** — `belmont validate` detects residual violations in PROGRESS.md (polish-pattern milestone names; cross-milestone task IDs like `P3-FWLUP-M2-1` sitting under a non-M2 milestone). Runs at `belmont auto` startup; interactive runs get `[y/N]` prompt, non-interactive abort.

## Failure mode if you break it

Without enforcement: a polish milestone declared `(depends: M1)` becomes a sibling of M2, M3, M4 which also depend on M1. All four run in parallel. The polish milestone mutates `hero-section.tsx` (which it considers M1's) while M2 is actively writing that file (because M2 owns the hero task). Merge picks one side arbitrarily. No error, no warning — only the final feature looking wrong.

With broken enforcement (one layer regressed but others intact): the regressed layer allows the violation; the next layer catches it. Stream shows `[SCOPE-GUARD] reverted 1 violation(s) — new milestone M5 "Polish / follow-ups…"`. Agent sees STEERING correction, adapts or escalates. Loud failure, cheap to diagnose. This is why the three layers compose.

## Don't re-do

- **`implement.md:135-137` with the "create a new milestone with the next sequential number" permission.** This was the exact loophole that produced the M5 milestone in about-2. Removed; replaced with "always extend the current milestone." If you're tempted to add an "escape hatch" for cross-milestone work, the correct escape hatch is `[!]` with a reason, not a new milestone.
- **Allowing `triage`'s `defer_and_proceed` to create polish milestones.** The post-verify-triage prompt explicitly forbids this now. Deferral means NOTES.md or same-milestone `[!]`, never a new milestone.
- **Per-milestone PROGRESS fragment files** (`M1.md`, `M2.md`, …). Architecturally cleaner (scope violation becomes structurally impossible for checkbox flips). Rejected as a bigger refactor than skill-prose + runtime guard combined. Revisit only if the runtime guard proves fiddly; the two approaches are redundant-but-harmless if both adopted.
- **Pre-plan-time regex that blocks milestones with "Polish" / "Follow-ups" in the name.** Would false-positive on legitimate cross-cutting milestones like "M6: Accessibility audit across public routes." `belmont validate` uses targeted patterns (`polish`, `follow-ups`, `cleanup`, `verification fixes`, `deviations from M<N>`, `from M<N> implementation`, `fwlup(s)`) that match the anti-pattern without false-flagging real work.

## Evidence

`belmont-test/about-2-dynamic-mode` (the M5 spawn + cascade) vs `belmont-test/about-4-fresh` (clean M1–M4 run, zero milestones created mid-flight) in the studia-web repo. See [meta/validated-runs.md](../meta/validated-runs.md).

Unit coverage: `cmd/belmont/scope_guard_test.go` → `TestDetectViolations_PolishMilestoneNames`, `TestDetectViolations_CrossMilestoneTaskID`, `TestDiffScopeViolations_DetectsNewMilestone`.

## Known rough edges

- **Pre-existing bad milestones in a legacy PROGRESS.md.** If a feature was planned before the rule landed and contains an M5-polish milestone, skills will see it and may add to it. Agents are told to flag such milestones in their summary rather than migrate automatically. `belmont validate` detects these; the user runs `/belmont:tech-plan` to restructure.
- **`validate` regex may catch legitimate cross-cutting milestones** whose description happens to contain "cleanup" or "polish" in some innocuous way. Acceptable false-positive rate — the violation is easily waved past with the interactive `[y/N]` prompt. If false-positives become common, tighten the regex.

## Revisions

- 2026-04-21 — initial: canonical partial, `implement.md` loophole closed, tech-plan / verify / triage tightened, `belmont validate` lint added.
- 2026-04-22 — migrated from LEARNINGS.md to knowledge/ tree.
