### Reconcile State Files

Before committing, audit `{base}/PRD.md` and `{base}/PROGRESS.md` for drift and fix any discrepancies:

1. **Task ↔ checkbox sync** — For each task in PROGRESS.md milestone sections:
   - Find the matching `### P...:` header in PRD.md by task ID
   - If the PRD header has ✅ but the PROGRESS checkbox is `[ ]` → change to `[x]`
   - If the PROGRESS checkbox is `[x]` but the PRD header lacks ✅ → add ✅ to the header

2. **Milestone status sync** — For each milestone heading in PROGRESS.md:
   - If ALL its tasks are `[x]` and heading is not `✅` → change to `### ✅ M...:`
   - If ANY task is `[ ]` and heading IS `✅` → change to `### ⬜ M...:`

3. **Blocker cleanup** — In the `## Blockers` section of PROGRESS.md:
   - Remove entries whose referenced task ID is now marked ✅ in PRD.md
   - If section becomes empty, set to `None`

4. **Overall status line** — Update `## Status:` in PROGRESS.md:
   - All milestones ✅ → `## Status: ✅ Complete`
   - Mix of ✅ and ⬜/🔄 → `## Status: 🟡 In Progress`
   - All ⬜ → `## Status: 🔴 Not Started`

Only fix actual discrepancies — if files already agree, make no changes.
