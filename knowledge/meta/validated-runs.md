# Validated Runs

**Why this matters.** The parallel auto mode has behaved correctly on a real feature end-to-end. If it breaks in the future, the first question is "what changed since it last worked?" — and the answer is best served by preserved branches showing known-good behavior side-by-side with known-bad. These branches are **reference material**, not work artifacts; don't garbage-collect them without a deliberate replacement.

## Preserved branches (studia-web, feature: `about`)

Three branches in `/Users/blake/code/clients/Sophos/studia-web`, all rooted at tech-plan commit `c465dd77`:

| Branch | State of the guards | What it proves |
|---|---|---|
| `belmont-test/about-2-dynamic-mode` | Pre-guards baseline | The failure modes that motivated the whole design. M5 polish milestone spawned, ran in parallel with M2, both wrote `hero-section.tsx`, silent merge picked one. Commit `ea672675` bulk-marked M3's P1-5..P1-8 as `[v]` without implementation. |
| `belmont-test/about-3-fresh` | L0 / L1 / L2 / L3 guards, no port isolation | Scope guard and verify-evidence worked: no polish milestone, no rubber-stamped `[v]`. But port cascade fired — agents hit `localhost:3000` collisions, verify flaked, M2 re-thrashed. Motivates the Level 1 env vars. |
| `belmont-test/about-4-fresh` | Full set: scope guard + verify evidence + port isolation + milestone-immutability + merge overlap report | Complete end-to-end clean run. Wave 1 M1, Wave 2 M2/M3/M4 parallel, merge-overlap surfaced the `about/page.tsx` shared file, reconciliation-agent resolved via `import-union` at high confidence keeping all 8 section imports from both sides. No silent losses. No M5 spawned. Source files correctly landed. |

## Three-way diff commands

From the studia-web repo. These are the commands a future agent would run to triage a regression.

```bash
# Source-file shape: did the final merged about page actually implement everything?
git diff belmont-test/about-2-dynamic-mode..belmont-test/about-4-fresh -- src/components/about/
# Expected: about-4 has full component implementations; about-2 has stubs for P1-5..P1-8 files.

# PROGRESS.md structural evolution
git diff belmont-test/about-2-dynamic-mode..belmont-test/about-4-fresh -- .belmont/features/about/PROGRESS.md
# Expected: about-4 has only M1-M4; about-2 has M5.

# Commit history — who landed what tasks
git log --oneline belmont-test/about-4-fresh --not main | head -40
git log --oneline belmont-test/about-2-dynamic-mode --not main | head -40
# Expected: about-4 has one commit per task named by its task ID (verify-evidence requires this);
# about-2 has bulk-mark commits like "ea672675 P3-FWLUP-3: … mark done" that touched tasks it had no right to.

# Merge anatomy
git log --merges --first-parent belmont-test/about-4-fresh
# Expected: clean sequence of sibling merges in milestone-ID order, no failed reconciliation markers.
```

## If a regression appears

Before assuming the design is wrong, reproduce against one of these branches:

1. **Make sure the failure shows up on `about-4-fresh` too.** If it only shows up on a new branch, the regression is in the *new* branch, not the design.
2. **Binary-search the knowledge tree**: the failure's class usually maps to one or two entries. A checkbox-flip regression → `auto-mode/scope-guard-runtime.md`. A port conflict → `cross-cutting/port-isolation.md`. A polish milestone appearing → `cross-cutting/milestone-immutability.md`. Read the relevant entry's `Don't re-do` section — someone may have already walked past this.
3. **Don't weaken a guard as a workaround**. All five layers interact; weakening one usually breaks assumptions another layer depended on.

## Preservation policy

- Keep the three `belmont-test/about-*-fresh` branches indefinitely in the studia-web repo.
- If the repo is archived or the remote is decommissioned, export the three branches' commit logs and final diffs into a separate snapshot file and keep the pointer here.
- Do not casually rebase, squash, or force-push these branches. They are documentation frozen in git.

## Revisions

- 2026-04-22 — initial: three preserved branches, three-way diff commands, regression-triage flow.
