# Debug Manual: Spec Reconciliation (Step 5)

This is the detailed procedure for the post-fix spec reconciliation step in `/belmont:debug-manual`. The skill body orchestrates; this file holds the catalogue, templates, and decision flow.

You only reach this step after the user confirmed the fix is **FIXED** and before the final commit. The goal is to propagate what you learned during debugging back into Belmont's specs so the next session operates on accurate truth.

## Drift catalogue

Walk every loaded spec file (master + per-feature) and look for any of the following drift categories. Skip categories with no matches.

| # | Category | Where it lives | What you may write |
|---|---|---|---|
| 1 | Acceptance criteria contradicted by reality | `{base}/PRD.md` task's `**Verification**:` or BDD criteria | Edit text in place to match shipped behaviour |
| 2 | `**Solution**:` outdated | `{base}/PRD.md` task | Edit text in place |
| 3 | Task description vague / misleading | `{base}/PRD.md` task description | Tighten the description |
| 4 | PRD scope misunderstood | `{base}/PRD.md` Overview / Problem Statement / Out of Scope | Edit in place; preserve heading structure |
| 5 | TECH_PLAN decision wrong or stale | `{base}/TECH_PLAN.md` or `.belmont/TECH_PLAN.md` | Edit narrative in place; preserve heading structure |
| 6 | Master architecture decision contradicted | `.belmont/TECH_PLAN.md` | Edit in place; flag to user before proposing |
| 7 | PR_FAQ-level product misunderstanding | `.belmont/PR_FAQ.md` | Surface to user FIRST; edit only with explicit per-edit approval |
| 8 | Follow-up task completed by this fix | `{base}/PROGRESS.md` | Flip `[ ]` → `[x]` (NEVER `[v]`); current-or-last-shipped milestone only |
| 9 | Root cause pattern worth remembering | `{base}/NOTES.md` `## Root Cause Patterns` | Append Five-Whys entry |

## Per-edit decision flow

For each candidate edit you identify:

### 1. Build the diff

Read the current file content. Write the proposed content. Render a unified-diff-style block:

```diff
--- {file path} (current)
+++ {file path} (proposed)
@@ <heading or section> @@
- <old line(s)>
+ <new line(s)>
```

Keep the surrounding context minimal — show ±2 lines around the change. For multi-section edits, show one hunk per section.

### 2. Explain the why

Before the diff, write **one short sentence** explaining what the fix revealed that motivates this edit. Example:

> The fix moved API rate-limiting from the worker pool to the gateway. PRD's `**Solution**:` still names the worker pool.

### 3. Ask the user

Present the explanation, the diff, and a four-option prompt:

```
Apply this edit? [y / N / edit / skip]
  y    — apply as proposed
  N    — reject; do not apply
  edit — open in $EDITOR (or paste an alternative), then re-confirm
  skip — defer; log to DEBUG.md and continue with the next edit
```

### 4. Act on the response

- **y** → write the edit. Add the file to the staged set for the upcoming commit.
- **N** → do not write. Log to DEBUG.md `## Spec Reconciliation Log` as `REJECTED` with the user's reason if they gave one.
- **edit** → present an editor handoff: dump the proposed content to a temporary path, ask the user to make changes and return, then re-render the diff against the current file. Loop on `y / N / edit / skip` until resolved.
- **skip** → log to DEBUG.md `## Spec Reconciliation Log` as `SKIPPED — surface in final report`. Continue.

### 5. Log to DEBUG.md

For every candidate edit (applied, rejected, or skipped), append to `## Spec Reconciliation Log`:

```markdown
### Edit <N>: <one-line summary>
- **Category**: <drift category number + name>
- **File**: `{file path}`
- **Decision**: APPLIED | REJECTED | SKIPPED
- **Reason** (if N/skip): <user's reason or "no reason given">
- **Before** (snippet):
  ```
  <old text, 1–3 lines>
  ```
- **After** (snippet, only if APPLIED or edited):
  ```
  <new text, 1–3 lines>
  ```
```

## Root Cause Pattern template (category 9)

Mirror the format used by `/belmont:verify` so a single NOTES.md format works everywhere — same template, same fields. Walk the Five-Whys ladder (Why 1 → Why 5, stop early if you reach the root cause sooner) before drafting the entry; the agent only writes the distilled output, not the intermediate "whys".

Append to `{base}/NOTES.md` under `## Root Cause Patterns` (create the section if it doesn't exist):

```markdown
### [YYYY-MM-DD] Pattern: <short descriptive name>
**Issue**: <one-line description of the bug that was fixed>
**Root Cause**: <the deepest "why" — the fundamental pattern that allowed the bug>
**Prevention**: <actionable rule for future implementation>
**Source**: debug-manual session — <feature slug>, fix commit will be `<hash placeholder>`
```

Use today's date in `YYYY-MM-DD` from the conversation environment. The fix commit hash isn't known until Step 4 commits everything atomically — leave it as the literal string `<commit-hash>` and rely on the post-commit summary to surface the real hash to the user. Do NOT rewrite NOTES.md after the commit to insert the hash; that's extra churn for no real value.

## Multi-feature mode

If the skill loaded multiple features (`{bases[]}` has > 1 entry), iterate the drift catalogue per feature in selection order. For each feature, present the per-feature drift summary up-front:

```
Feature: <slug>
  Drift candidates found: <N>
    1. Category <X>: <one-line description>
    2. ...
  Continue with this feature? [y / skip feature / abort]
```

Then walk each candidate edit through the per-edit flow above. After all features are processed, emit the cross-feature summary:

```
Spec reconciliation complete.
  Features touched: <list of slugs>
  Edits applied:    <N>
  Edits rejected:   <N>
  Edits skipped:    <N>
  Files modified:   <list of file paths>
```

## Hard rules (also stated in `debug-scope-rules.md`)

- Never add a milestone heading. Never rename one. Never remove one.
- Never use polish/follow-up/cleanup/FWLUP/"deviations from"/"verification fixes" naming.
- Never add new `[ ]` tasks for unfixed drift — fix it or skip it; do not park it.
- Never flip a task to `[v]` — that is `/belmont:verify`'s job.
- Never edit a feature's specs that was not selected at Step 0.
- PROGRESS.md `[x]` flips: scoped to the current or last-shipped milestone of the feature being debugged. Sibling milestones are off-limits.
- Commit message body MUST include task IDs whose state was flipped (e.g. `P1-M3-2`) so `runEvidenceCheck` finds attribution on a future verify pass.

## When to skip Step 5 entirely

If you walked all loaded specs and found zero drift candidates (rare but possible — sometimes a bug is just a bug, not a spec problem), say so to the user once and proceed to Step 4 commit with code-only changes. Log to DEBUG.md `## Spec Reconciliation Log`: `No drift detected across N loaded specs — code-only fix.`
