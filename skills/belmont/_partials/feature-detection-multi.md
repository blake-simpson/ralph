## Feature Selection (multi-feature capable)

Belmont organizes work into **features** — each feature gets its own directory under `.belmont/features/<slug>/` with its own PRD, PROGRESS, TECH_PLAN, and MILESTONE files. This skill supports debugging across one OR multiple features in the same session.

### Select the Active Feature(s)

1. List all feature directories under `.belmont/features/`.
2. If no features exist: tell the user to run `/belmont:product-plan` to create their first feature, then stop.
3. If the user's invocation prose names specific features by slug or by recognisable name, pre-select those features and confirm with the user (`Use features X and Y? [y / change selection]`).
4. Otherwise, read each feature's `PRD.md` for its name and status, then {{feature_action}}.
5. Present a numbered list and ask the user to multi-select:

   ```
   Which feature(s) is this bug related to?

     1. auth-session-fix  — Persistent session refresh logic
     2. dashboard-charts  — Revenue trend visualisations
     3. settings-profile  — User profile edit page

   Reply with:
     - A single number (e.g. "2") for one feature
     - Comma-separated numbers (e.g. "1,3") for cross-feature debugging
     - "all" to include every feature
   ```

6. Validate the response. Reject invalid selections and re-prompt rather than guessing.
7. Resolve each selected feature to its base path: `.belmont/features/<selected-slug>/`.

### Base Path Convention

This skill works with a **list of base paths** `{bases[]}` rather than a single `{base}`. When iterating over loaded context, spec reconciliation, or per-feature reporting, walk the list and operate on each base path independently:

- `{base}/PRD.md` — that feature's PRD
- `{base}/PROGRESS.md` — that feature's progress tracker
- `{base}/TECH_PLAN.md` — that feature's tech plan (optional)
- `{base}/NOTES.md` — that feature's learnings (optional)
- `{base}/MILESTONE-*.done.md` — that feature's archived milestones

When a step only makes sense for a single feature (e.g. creating the shared DEBUG.md), nominate the **primary feature** — the one the user-described symptom most clearly belongs to, or the first selection if it's a tie. Put DEBUG.md under the primary feature's base path; reference cross-feature context inside it explicitly.

**Master files** (always at `.belmont/` root, shared across features):
- `.belmont/PR_FAQ.md` — strategic PR/FAQ document
- `.belmont/PRD.md` — master PRD (feature catalog)
- `.belmont/PROGRESS.md` — master progress tracking (feature summary table)
- `.belmont/TECH_PLAN.md` — master tech plan (cross-cutting architecture)
- `.belmont/NOTES.md` — global learnings

### Degenerate cases

- **Single feature selected**: behave exactly like the single-feature `feature-detection.md` partial — `{bases[]}` has one entry; iteration steps still work; UX-wise, drop the "per-feature" framing in user-facing messages.
- **All features selected** in a project with many features: warn the user that context will be heavy ("you've selected N features — loaded context may exceed local-model limits") and offer to narrow before proceeding.
- **No features available**: route to `/belmont:product-plan`, same as the single-feature partial.
