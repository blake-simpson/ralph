# Proposal 0001: Quick Mode (`/belmont:quick`)

**Status**: Draft
**Type**: New skill
**Author**: external contributor
**Target**: `main` (≥ v0.10.7)

## Summary

Add a new skill, **`/belmont:quick`**, that converts a one-line problem statement directly into an executable `MILESTONE.md` and dispatches the implementation-agent — bypassing `/belmont:product-plan` and `/belmont:tech-plan` entirely.

The skill is pure markdown (no Go changes), sits alongside the existing `next.md` in `skills/belmont/_src/`, and reuses Belmont's existing agent contracts. Trigger is unambiguous: an explicit `/belmont:quick "<problem statement>"` invocation; there is no automatic detection. The skill emits a 15–30 line MILESTONE.md, runs **only** the implementation-agent (skips codebase-agent, design-agent, verification-agent), and produces one commit with a `quick:` prefix.

This proposal is a design-only RFC. It does not include an implementation; it is opened to invite design feedback before code lands.

## Motivation

For incident-response and routine-maintenance tasks — a failing CI job, a typo fix, a missing index, a dependency bump — the two-stage PRD + tech-plan cycle adds latency without adding value. The PRD reads like a checklist; the tech-plan adds no architectural decisions; verify finds nothing to verify because the change is small enough that the implementation-agent's own acceptance-criteria walkthrough already covers it.

Three pain patterns recur:

1. **PRD bloat for trivial work.** When every fix is processed through `/belmont:product-plan`, PRDs for incident-class features grow far beyond the size such work needs — often into the multi-thousand-line range — because there's no brevity affordance in the existing skill prose.
2. **Latency on production incidents.** When a pipeline is failing, the user wants the agent fixing the bug, not running a 5-minute interview that walks the `product-plan` "Domains to Cover" checklist (`user-flows`, `accessibility`, `i18n`, `analytics`, `monetization`, etc.) for a task that has none of those concerns.
3. **PR/FAQ → PRD → tech-plan → implement → verify is five gates for a one-line fix.** The existing [`/belmont:next` skill](../skills/belmont/_src/next.md) collapses the analysis phase but still requires a pre-existing PRD entry and an existing milestone with an unchecked task. It is not a fast-mode for first-touch incident response — it is a fast-mode for *finishing* a milestone.

`/belmont:quick` is explicitly the lightweight sibling for *starting*, mirroring how `next` is the lightweight sibling for *finishing*.

## Proposed Change

One new user-facing skill plus four supporting edits. All paths relative to the upstream repo root.

### New skill source — `skills/belmont/_src/quick.md`

A new flat-layout skill source, structurally identical to the other entries in `skills/belmont/_src/`. YAML frontmatter shape matches the other skills (`description:` and `alwaysApply: false`).

The skill body has five phases:

1. **Phase 0 — invocation parse.** Read the single quoted argument: `/belmont:quick "<problem statement>"`. If no quoted argument is present, error out: `quick: requires a problem-statement argument, e.g. /belmont:quick "fix typo in README.md"`. No interactive interview.
2. **Phase 1 — feature directory bootstrap.** Derive a kebab-case slug from the problem statement (≤40 chars) and write it under `.belmont/features/<slug>/`. If the directory already exists, append a date suffix. Create:
   - `MILESTONE.md` — the 15–30 line minimal planning document (format below).
   - `PROGRESS.md` — single-milestone, single-task scaffold.
   - **No `PRD.md`, no `TECH_PLAN.md`.** The MILESTONE.md is self-contained.
3. **Phase 2 — implementation-agent dispatch.** Spawn one sub-agent using the existing prompt template (whichever partial `implement.md` uses) pointing at the new MILESTONE.md. The agent reads MILESTONE.md only — it does not look up a non-existent PRD.
4. **Phase 3 — single commit.** After the agent reports success, write one commit with a `quick:` prefix (e.g. `quick: fix typo in CONTRIBUTING.md`). No per-task commits — fast mode is one-PR-one-commit by contract.
5. **Phase 4 — skip-list gating.** If the diff is ≤200 lines (via `git diff --shortstat HEAD~1`), report `quick: change is small (≤200 lines) — skipping /belmont:verify, run it manually if you want a second pair of eyes`. If the diff is >200 lines, report a recommendation to run verify. The decision is reported, not enforced.

### Trigger condition (unambiguous)

The fast mode runs **if and only if** the user types `/belmont:quick "<problem statement>"`. There is no auto-detection, no `--fast` flag on `/belmont:implement`, no heuristic over the PRD slug. This avoids three failure modes:

- **No false fast-mode promotion.** A complex feature that happens to live under an incident-shaped slug does not get auto-routed to fast mode.
- **No false fast-mode rejection.** A user with a small PRD they know is fast-mode work can still invoke `/belmont:quick` to bypass it.
- **No agent ambiguity.** The implementation-agent sees `Mode: quick` in `## Status` and behaves accordingly — it does not have to decide.

### Minimal MILESTONE.md format

A strict subset of the existing template at `skills/belmont/_src/references/implement-milestone-template.md`. Same H2 sections (`## Status`, `## Orchestrator Context`, `## Codebase Analysis`, `## Design Specifications`, `## Implementation Log`) so the implementation-agent's existing reads do not need to change. Differences:

- `## Status` includes `**Mode**: quick — single-task fast mode, no PRD, no tech-plan` so the agent can short-circuit Step 0 (Scope Validation against a non-existent PRD).
- `### Active Task IDs` is the single synthetic ID `Q1-1` (Q for quick).
- `### File Paths` omits `PRD` and `TECH_PLAN` entries.
- `## Codebase Analysis` and `## Design Specifications` carry the same `[Not populated — quick mode skips both analysis agents]` placeholder that `next.md` already uses.

Example output (15 lines):

```markdown
# Milestone: Q1 — fix-readme-typo (quick)

## Status
- **Milestone**: Q1: fix typo in README.md (quick mode)
- **Mode**: quick — single-task fast mode, no PRD, no tech-plan
- **Created**: 2026-05-23T14:22:11Z
- **Tasks**: [ ] Q1-1: fix typo in README.md

## Orchestrator Context
### Active Task IDs
Q1-1
### File Paths
- PROGRESS: .belmont/features/fix-readme-typo/PROGRESS.md
### Scope Boundaries
- In Scope: Q1-1 only
- Out of Scope: any work not described by the Q1-1 problem statement

## Codebase Analysis
[Not populated — quick mode skips both analysis agents.]
## Design Specifications
[Not populated — quick mode skips both analysis agents.]
## Implementation Log
```

### How MILESTONE is constructed without a tech-plan

Three mechanical rules:

1. **Task definition lives inside MILESTONE.md.** With no PRD, the `Q1-1` problem statement is written directly into the `## Status` task list and re-quoted under `### Additional User Instructions`. The implementation-agent already reads `### Additional User Instructions`; this proposal elevates it from "optional extra context" to "load-bearing task definition" in quick mode.
2. **Acceptance criteria are implicit.** The agent's existing Step 3b walkthrough phrases acceptance criteria as "the change must do what the task said and not break anything else". For quick mode, this is the only acceptance criterion. The proposal adds one sentence to `agents/belmont/implementation-agent.md` Step 3b: `When MILESTONE.md Status carries Mode: quick, treat the problem statement as the sole acceptance criterion.`
3. **No reconciliation phase.** The `/belmont:tech-plan` Phase 4.5 reconciliation does not run because there is no PRD to reconcile against.

### Skip-list of orchestration phases

| Phase                                       | Normal `/belmont:implement` | Fast mode `/belmont:quick`         |
| ------------------------------------------- | --------------------------- | ---------------------------------- |
| `/belmont:product-plan` interview           | runs                        | **skipped**                        |
| `/belmont:tech-plan` interview              | runs                        | **skipped**                        |
| PRD creation                                | runs                        | **skipped** (no PRD file)          |
| TECH_PLAN creation                          | runs                        | **skipped**                        |
| `models.yaml` tier assignment               | runs                        | **skipped** (uses workspace defaults) |
| codebase-agent (Phase 1)                    | runs                        | **skipped**                        |
| design-agent (Phase 2)                      | runs                        | **skipped**                        |
| implementation-agent (Phase 3)              | runs                        | runs                               |
| Per-task commit                             | one per task                | one for the whole change           |
| `/belmont:verify` after implement           | runs (in `belmont auto`)    | **skipped if diff ≤200 lines**     |
| code-review-agent                           | runs (in `belmont auto`)    | **skipped if diff ≤200 lines**     |
| Milestone archival (`MILESTONE-M<N>.done.md`) | runs                      | runs (archived as `MILESTONE-Q1.done.md`) |

The 200-line skip threshold is reported, not enforced — the user can always invoke `/belmont:verify` manually after `/belmont:quick` completes.

### Supporting changes

1. **`skills/belmont/_src/_partials/forbidden-actions.md`** — one-line exemption: "`/belmont:quick` is the only skill permitted to create a feature directory without a PRD.md."
2. **`agents/belmont/implementation-agent.md`** — 3-line block in Step 0 (Scope Validation short-circuit on `Mode: quick`) plus a 1-line addition in Step 3b (sole acceptance criterion). Only agent change.
3. **`docs/skills-reference.md`** — new `## quick` entry between `## next` and `## verify`. One-paragraph description; output path; "Best for" / "Use `/belmont:implement` instead for" disambiguator, matching the `next.md` pattern.
4. **`scripts/generate-skills.sh`** — no changes needed. The existing walker picks up any new file under `skills/belmont/_src/*.md` automatically.

### What does NOT change

- `cmd/belmont/main.go` — no Go binary changes. The skill is pure markdown.
- `belmont auto` action enum (`loopActionType` at `cmd/belmont/main.go:375`) — fast mode is interactive-only in this proposal; auto-loop wiring is explicitly out-of-scope.
- `belmont validate` — no new lint rules.
- Existing `MILESTONE.md` template — extended with a backwards-compatible `Mode:` field; existing milestones that do not set the field are treated as `Mode: full` implicitly.
- The milestone-immutability rule (`knowledge/cross-cutting/milestone-immutability.md`) — quick-mode milestones use the same `## M<N>:` heading shape and the same `[x]` / `[v]` / `[!]` task states. The `belmont validate` lint sees a `Q1` milestone the same way it sees an `M1` milestone.

## User-Facing Behaviour

### Invocation

```
/belmont:quick "<one-line problem statement>"
```

The skill rejects invocations without a quoted argument:

```
$ /belmont:quick
quick: requires a problem-statement argument
       e.g. /belmont:quick "fix typo in README.md"
```

### Worked example

```
$ /belmont:quick "the CI workflow is failing on the docs-build step"

[quick] creating feature directory: .belmont/features/ci-failing-on-docs-build/
[quick] writing MILESTONE.md (24 lines)
[quick] writing PROGRESS.md (single-task scaffold)
[quick] dispatching implementation-agent (no codebase-scan, no design-analysis)
[implementation-agent] reading MILESTONE.md
[implementation-agent] Mode: quick — Step 0 short-circuit, problem statement is the sole acceptance criterion
[implementation-agent] exploring .github/workflows/, located docs.yml
[implementation-agent] applying fix... running CI locally... pass
[implementation-agent] Step 3b: acceptance criterion satisfied (problem statement: docs-build failure)
[quick] diff: 12 lines across 1 file
[quick] git commit -m "quick: pin docs-build node version to 20"
[quick] change is small (≤200 lines) — skipping /belmont:verify
[quick] archived MILESTONE.md → MILESTONE-Q1.done.md

Next steps:
  /belmont:status                              (review what was done)
  /belmont:verify --feature ci-failing-on-docs-build   (optional — second pair of eyes)
```

Expected elapsed time: ~2–5 minutes versus ~8–15 minutes for the full pipeline on the same change. No PRD interview, no tech-plan interview, no codebase-scan phase, no design-analysis phase, no verify pass, single atomic commit.

### `belmont status` after a quick run

```
$ belmont status

Belmont Status
==============

  ci-failing-on-docs-build  (quick)
    Tasks: 1/1 done  |  Milestones: 1/1 done
    Last completed: Q1-1 — the CI workflow is failing on the docs-build step
```

The `(quick)` annotation is the only visual difference from a normal feature.

### Failure modes the skill handles

- **Agent reports the problem is too big.** The agent's existing escape hatch (mark task `[!]` blocked, report a follow-up `[ ]` task within the same milestone per `knowledge/cross-cutting/milestone-immutability.md`) applies unchanged. The user is advised to re-run `/belmont:product-plan` for the feature directory.
- **Diff is >200 lines.** The skill recommends `/belmont:verify`; the user can ignore the recommendation.
- **Agent crashes.** Same behaviour as a crashed `/belmont:next` — the user re-runs the skill against the existing MILESTONE.md.

## Acceptance Criteria

The PR (when implementation lands) is mergeable when:

- [ ] `skills/belmont/_src/quick.md` exists with the five-phase body described in §Proposed Change and the standard `description:` / `alwaysApply: false` YAML frontmatter.
- [ ] `scripts/generate-skills.sh` produces `.agents/skills/belmont/quick/SKILL.md` and `.claude/commands/belmont/quick.md` symlink without manual intervention.
- [ ] `docs/skills-reference.md` carries a new `## quick` section with description, output path, "Best for", and a one-line disambiguator following the `next.md` pattern.
- [ ] `agents/belmont/implementation-agent.md` Step 0 carries a 3-line block: "If `## Status` in MILESTONE.md contains `Mode: quick`, skip the PRD/Out-of-Scope read in steps 1–2 — the problem statement in `### Additional User Instructions` is the sole task definition. Continue with Step 3 onward as normal." Step 3b carries a 1-line addition: "When `Mode: quick`, treat the problem statement as the sole acceptance criterion."
- [ ] `skills/belmont/_src/_partials/forbidden-actions.md` carries the one-line exemption.
- [ ] No changes to `cmd/belmont/main.go`, no changes to `belmont auto` action enum, no changes to `belmont validate`, no changes to the canonical MILESTONE template (`Mode:` is documented in `quick.md` and backwards-compatible).
- [ ] Manual smoke test on a small change (e.g. `/belmont:quick "fix typo in README.md"`): produces `MILESTONE.md` ≤30 lines, dispatches one sub-agent, produces one `quick:`-prefix commit, and reports the verify skip.
- [ ] Manual smoke test on a >200-line diff: skill reports "change is larger than the fast-mode threshold (X lines) — recommend running /belmont:verify" instead of the small-diff skip message.
- [ ] No regressions in `cmd/belmont/scope_guard_test.go` — quick milestones using `Q<N>` IDs must not trigger the polish-milestone-name or cross-milestone-task-ID rules. **Note:** this likely requires a one-line update to the milestone-ID regex (currently `M<digits>`) to also accept `Q<digits>` — see Open Questions Q3.
- [ ] `CHANGELOG.md` entry under "Unreleased" describing the new skill.

## Out of Scope

- **`belmont auto` integration.** Wiring `/belmont:quick` into the auto-loop's action enum (`actionImplementMilestone`, `actionImplementNext`, etc. at `cmd/belmont/main.go:375`) is a follow-up PR. This proposal is interactive-only.
- **Automatic fast-mode detection.** Heuristics that classify an existing PRD as fast-mode-eligible (e.g. "all tasks `[ ]`, PRD <200 lines, slug matches `*-repair`") are explicitly out of scope.
- **`/belmont:implement --fast` flag.** Deliberately rejected: `implement.md` consumes a pre-existing PRD; fast mode does not. Adding `--fast` would couple two skills with different invariants. A separate skill mirrors the `next.md` precedent.
- **Multi-task quick mode.** `Q1-2`, `Q1-3` tasks within a single quick-mode milestone are not in scope. Quick is single-task by contract.
- **Quick-mode rollback / undo.** No special revert tooling; standard `git revert <quick-commit-sha>`.
- **Quick-mode for monorepo workspaces.** Quick-mode runs at the repo root; workspace-aware quick mode (extending `BELMONT_PRIMARY_WORKSPACE` semantics from `docs/monorepo-support.md`) is a follow-up if demand emerges. The current implementation-agent handles workspaces inside its Step 3 detection; quick mode inherits that.

## Open Questions

1. **Diff threshold (200 lines) — is this the right number?** The threshold should be a configurable constant in `quick.md` rather than a magic number — the proposal uses a top-of-file YAML key (`diff_skip_verify_threshold: 200`). 200 is a reasonable upper bound for "small incident fix" based on common patterns, but maintainer input welcome.
2. **Should `/belmont:quick` ever auto-run `/belmont:verify`?** Current proposal: never auto-run; the skip-list is a recommendation only. Counter: production-incident work is exactly the work that benefits from a second pair of eyes. Counter-counter: the 200-line threshold is small enough that the agent's Step 3b walkthrough is sufficient, and forcing verify negates the latency benefit.
3. **`Q<N>` milestone numbering — collisions with existing `M<N>`?** Proposal uses `Q` prefix to make quick milestones visually distinct in `PROGRESS.md` and in `belmont validate` output. The milestone-ID regex at `cmd/belmont/main.go:11537` (currently `M<digits>`) would need a one-line update to also accept `Q<digits>`. Alternative: reuse `M<N>` and rely on `Mode: quick` in Status to disambiguate. Maintainer preference welcome.
4. **Should `quick` support `--feature <slug>` for existing features?** Current proposal: always creates a new feature directory. Argument for `--feature`: users may want to drop a quick-mode milestone into an existing feature. Argument against: that case is what `/belmont:next` already covers — quick is for *new* incident-response work, not finishing existing milestones.
5. **Is `quick:` the right commit-message prefix?** It does not collide with conventional-commit prefixes (`feat:`, `fix:`, `chore:`), which is the point. Alternative: keep `fix:` and let the agent's existing commit-message generation choose. Argument for a distinct prefix: `git log --grep '^quick:'` is a useful audit query.

## References

- [skills/belmont/_src/next.md](../skills/belmont/_src/next.md) — sibling lightweight skill; structural and prose-style template for `quick.md`.
- [skills/belmont/_src/implement.md](../skills/belmont/_src/implement.md) — agent dispatch pattern reused by `quick.md` Phase 2.
- [skills/belmont/_src/references/implement-milestone-template.md](../skills/belmont/_src/references/implement-milestone-template.md) — canonical MILESTONE template; the quick variant is a strict subset.
- [agents/belmont/implementation-agent.md](../agents/belmont/implementation-agent.md) — Step 0 / Step 3b extension points for the `Mode: quick` short-circuit.
- [knowledge/cross-cutting/skill-format.md](../knowledge/cross-cutting/skill-format.md) — `_src/` flat layout, generation pipeline, per-CLI install plumbing.
- [knowledge/cross-cutting/milestone-immutability.md](../knowledge/cross-cutting/milestone-immutability.md) — three-layer enforcement that quick mode must not break.
- [docs/skills-reference.md](../docs/skills-reference.md) — user-facing skill catalogue; the new `## quick` entry lands here.
