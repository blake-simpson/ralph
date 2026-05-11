<!-- Debug-manual scope rules. Replaces the milestone-immutability @include for interactive debug-manual ONLY. Every other skill that touches PROGRESS.md keeps milestone-immutability.md unchanged. -->

## Spec-edit scope (interactive debug-manual)

Interactive `/belmont:debug-manual` is the **only** Belmont skill that may edit spec prose in place. The relaxation is deliberate and bounded:

- This skill never runs from `belmont auto` (only `/belmont:debug-auto` does — see `cmd/belmont/main.go`'s `actionDebug` wiring). The auto-mode `runScopeGuard` cannot fire against debug-manual edits.
- A human is in the loop for every edit. Each spec change is presented as a unified diff and gated on explicit `y / N / edit / skip` approval before any write.
- Edits are atomic with the code fix they correspond to — same commit, so the spec-change rationale lives in `git log` alongside the code change that motivated it.

### What you MAY edit

| File | What you may change |
|---|---|
| `{base}/PRD.md` | Acceptance criteria text, `**Solution**:` / `**Verification**:` field text, task descriptions, Overview / Problem Statement prose, Success Criteria, Out of Scope |
| `{base}/TECH_PLAN.md` | Decision narrative, library choices, file-path references, API shapes — anything where reality has moved past the written record |
| `.belmont/TECH_PLAN.md` | Cross-cutting decisions that the fix proved wrong or incomplete |
| `.belmont/PR_FAQ.md` | Only with per-edit explicit approval. PR_FAQ is strategic — flag it for the user before proposing diffs |
| `.belmont/PRD.md` | Master feature-catalog entries (status text, dependency notes) |
| `{base}/NOTES.md` | Append a `## Root Cause Patterns` entry (Five-Whys-style, mirroring `references/verify-five-whys.md`'s template) |
| `{base}/PROGRESS.md` | Flip `[ ]` → `[x]` on follow-up tasks that this fix completed, **scoped to the current or last-shipped milestone of the feature being debugged**. Never `[v]` (that's `/belmont:verify`'s job). Never touch sibling-milestone tasks. |

### What you MUST NOT do

- **No new milestones using polish/follow-up/cleanup/FWLUP/"deviations from"/"verification fixes" naming patterns.** `belmont validate` will reject these on the next auto run and block startup. This anti-pattern stays banned (see `knowledge/cross-cutting/milestone-immutability.md` for the cascade it caused).
- **No new `[ ]` follow-up tasks for unfixed drift.** If drift is real, fix it in place this session. If it's out of scope, log it in DEBUG.md's `## Spec Reconciliation Log` and surface in the final report — don't park it for `/belmont:implement` to find later.
- **No silent edits to other features' specs.** In multi-feature mode you got explicit approval to load multiple features; reconciliation still happens per-feature with separate approval gates.
- **No restructuring**: no renaming milestone headings, no adding milestone headings of any kind, no removing milestones, no reordering tasks across milestones. Structural changes route through `/belmont:tech-plan`.
- **No flipping a task to `[v]`.** Only `/belmont:verify` may mark verified. If the fix completed a follow-up, `[x]` is correct; verify will promote it later.

### Commit attribution rule

When you flip a task `[ ]` → `[x]` in PROGRESS.md, the commit message body MUST mention the task ID (e.g. `P1-M3-2`). The auto-loop's `runEvidenceCheck` walks branch commits looking for task-ID attribution before allowing a later `[v]` flip; missing IDs cause silent reverts on the next verify pass.

### If you find a pre-existing bad milestone

If PROGRESS.md already contains a milestone whose name matches the forbidden patterns above (e.g. a legacy "M5: Polish" from a pre-rule run):

- Do NOT add tasks to it.
- Do NOT depend on it from any edit you make.
- Surface the issue in your end-of-session report and suggest `belmont validate` + `/belmont:tech-plan` to restructure.

This matches the behaviour of every other skill — debug-manual's edit relaxation does not extend to legitimising bad milestone structures left behind by prior runs.
