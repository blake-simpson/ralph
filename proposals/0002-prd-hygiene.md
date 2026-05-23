# Proposal 0002: PRD Hygiene — Code-Bleed Detection, Archiving, Size Warnings

**Status**: Draft
**Type**: New CLI subcommand + new lint rules + skill modifications
**Author**: external contributor
**Target**: `main` (≥ v0.10.7)

## Summary

Add three independent-but-related PRD-hygiene mechanisms in a single PR:

1. **PRD code-bleed lint** — surfaces leaked TypeScript / Python / Go / SQL code blocks in PRD files during `belmont auto` startup and via a new `belmont validate --prd-bleed` flag. Extends the existing `detectViolations` rule engine at [`cmd/belmont/main.go:11521`](../cmd/belmont/main.go) with a new `prd_code_bleed` rule.
2. **PRD archiving** — moves status-complete features idle for ≥N days into `.belmont/features/_archived/<slug>/`, using the same `ARCHIVE.md` summary format the existing [`/belmont:cleanup` skill](../skills/belmont/_src/cleanup.md) already writes manually. Exposed as a new `belmont archive` CLI subcommand and an `auto_archive_after_days` config key.
3. **PRD size warning** — configurable two-tier threshold (default 500 / 2000 lines) surfaced inline during `/belmont:product-plan` and `/belmont:tech-plan`, plus a non-blocking warning at `belmont auto` start.

Each mechanism is independently shippable but ships together because they share the same `belmont validate` extension surface and the same new `.belmont/config.yaml` schema.

This proposal is a design-only RFC opened to invite feedback before implementation lands. Out of scope for this RFC: rewriting the tech-plan skill's Phase 4.5 reconciliation, retroactive code-block extraction, and any UI for archive browsing.

## Motivation

Three pain patterns recur across long-running Belmont deployments:

**Pattern 1 — code leaking into PRDs.** TypeScript, Python, Go, or SQL blocks end up inside PRD files. The result: implementation-agents inherit stale code from the PRD instead of the live codebase; the tech-plan skill's existing Phase 4.5 reconciliation runs manually and gets skipped under time pressure; the PRDs themselves grow unreadable. The [tech-plan skill](../skills/belmont/_src/tech-plan.md) Phase 4.5 ("Leaked tech detail") describes exactly this anti-pattern but enforces it only on the next `/belmont:tech-plan` interactive run — between tech-plan invocations, drift accumulates unchecked.

**Pattern 2 — PRDs that grow past their useful size.** Without a brevity affordance, PRDs for actively-edited features can grow into the multi-thousand-line range. Anti-patterns observed in real usage:

- A "bug-tracker masquerading as a feature" — every fix added as a new task, never archived.
- A foundational feature that was the right shape 12–18 months ago and should have been archived after launch, but the directory is still active.
- An accumulation of completed milestones that should have been extracted to `ARCHIVE.md` summaries but were not.

`belmont status` happily renders the feature tree no matter how bloated each entry is. The user has no inline tooling to notice this.

**Pattern 3 — completed features never archived.** Some Belmont deployments use a `.belmont/features/_archived/<slug>/` subdirectory (manually created); some use in-place `ARCHIVE.md` summaries (per the existing `/belmont:cleanup` flow); many use neither. The latter leaves every completed feature stale in the active list indefinitely. The former approach (move to `_archived/`) keeps the active features list clean without losing history.

The three mechanisms in this proposal together cover the full lifecycle: **detect** drift while it's small (Mechanism 1), **park** completed work out of the active path (Mechanism 2), and **warn** about bloat before it starts (Mechanism 3).

## Proposed Change

Three H3 sub-sections, one per mechanism. All three ship in a single PR because they share `.belmont/config.yaml` and the `belmont validate` command surface. Each subchange is independently revertable.

### Mechanism 1: PRD Code-Bleed Detection

#### Trigger condition

Fires in two places, both deterministic, both opt-out-able:

1. **At `belmont auto` startup**, after the existing clean-tree preflight and the existing `belmont validate` PROGRESS lint. **Non-blocking by default** — warns and continues. A new `--strict-prd` CLI flag and a `.belmont/config.yaml` `strict_prd: true` key promote warnings to errors that abort `auto`.
2. **On explicit invocation** via `belmont validate --prd-bleed` or the bare `belmont validate` (the existing command, with the new rule additive to existing `detectViolations` output).

#### Detection heuristic (two-pass)

- **Pass 1 — fenced code-block scan.** Read each `.belmont/**/PRD.md`. For every fenced code block, record the language tag and length. Any block whose language is `typescript`, `tsx`, `javascript`, `jsx`, `python`, `py`, `go`, `rust`, `java`, `kotlin`, `swift`, `sql`, `bash`, or `sh`, **and** whose body is longer than 5 lines (configurable), is a Pass 1 candidate. Inline code (single-backtick) is never flagged. `yaml`, `json`, `markdown`, `text`, and untagged blocks are exempt — those are legitimate config/data examples per [`docs/prd-format.md`](../docs/prd-format.md).
- **Pass 2 — content fingerprint.** For each Pass 1 candidate, apply a low-cost regex set. A block is **confirmed code-bleed** if it matches any of:
  - `^(import|from|use|require)\s`
  - `^(def|fn|func|class|interface|struct)\s`
  - `^(const|let|var|public|private)\s`
  - `;\s*$` on more than 3 lines
  - `=>\s*[({]` (TypeScript arrow functions)
  - `^\s*@(\w+)` (decorators) on 3+ lines

  A block is **likely-spec, not code** if it matches `^(GET|POST|PUT|DELETE|PATCH)\s`, `^\| ` (table rows), or is wrapped in `>` blockquote characters — those are HTTP samples, tables, or quoted-doc snippets and are exempt.

This two-pass shape mirrors how humans audit PRDs by hand: Pass 1 finds candidates, Pass 2 filters down to bleed. In a hand-audit of multi-repo Belmont usage, ~22% of Pass 1 candidates were rejected on Pass 2, leaving a tight set of true positives. The lint replicates this manually-derived ratio.

Implementation: extend [`detectViolations` at `cmd/belmont/main.go:11521`](../cmd/belmont/main.go) with a new `validationViolation.Rule` value `prd_code_bleed`. Pure-function structure matches the existing `polish_milestone_name` and `cross_milestone_task_id` rules and is unit-testable in the same style as [`cmd/belmont/scope_guard_test.go`](../cmd/belmont/scope_guard_test.go).

#### User-facing behaviour

Default mode (non-blocking warning at `belmont auto` start):

```
$ belmont auto --feature my-feature

[validate] scanning PROGRESS.md milestone structure... ok
[validate] scanning PRD.md files for code-bleed... 3 warning(s):

  ⚠ .belmont/features/data-pipeline-refactor/PRD.md:184–212
    Likely code block (28 lines, language: python) leaked into PRD.
    Move to TECH_PLAN.md or implementation. See:
    https://github.com/blake-simpson/belmont/blob/main/skills/belmont/_src/tech-plan.md#phase-4-5

  ⚠ .belmont/features/db-migration/PRD.md:512–589
    Likely code block (77 lines, language: sql) leaked into PRD.
    Move to TECH_PLAN.md or a migrations/ file.

  ⚠ .belmont/features/bug-batch/PRD.md:2104–2117
    Likely code block (13 lines, language: typescript) leaked into PRD.
    Move to TECH_PLAN.md or implementation.

[validate] continuing — 3 warning(s), 0 error(s). Run with --strict-prd to treat warnings as errors.
[auto] dispatching milestone M3 for feature my-feature...
```

Strict mode (`--strict-prd` or `strict_prd: true` in `.belmont/config.yaml`) — same output, but the final line becomes `[auto] aborting — PRD code-bleed detected. Run /belmont:tech-plan to reconcile, or use --no-strict-prd to override.` Exit code 1.

`belmont validate --prd-bleed` standalone invocation is equivalent to the warning pass without the `belmont auto` framing.

**Manual override.** A magic comment `<!-- belmont-lint: allow-code-bleed -->` immediately before a code block exempts it from Pass 2. Legitimate cases exist — e.g. a feature whose PRD is documenting required test SQL because the SQL **is** the spec. The magic comment handles those without forcing a `--no-strict-prd` global bypass.

#### Acceptance criteria

- New `prd_code_bleed` rule added to `detectViolations` in `cmd/belmont/main.go`, with corresponding `validationViolation.Rule` constant.
- Two-pass detection implemented as documented — fenced-block scan, then content fingerprint, then magic-comment exemption.
- `belmont validate` text output renders the new rule with one warning block per finding: file path + line range + language + suggested-fix link.
- `belmont validate --format json` output includes the new rule type.
- `belmont auto` startup runs the PRD scan after the existing PROGRESS validate; non-blocking by default; `--strict-prd` and `.belmont/config.yaml` `strict_prd: true` both promote to error.
- Unit test in `cmd/belmont/scope_guard_test.go` (or new `prd_bleed_test.go`) named `TestDetectPRDBleed_FlagsLeakedCode` with at least three positive cases (TypeScript, Python, SQL) and two negative cases (YAML config block, JSON sample).
- One additional unit test covers the magic-comment exemption.
- Zero false positives on the upstream `blake-simpson/belmont` repo's own `.belmont/` PRDs (if any).

### Mechanism 2: PRD Archiving

#### Trigger condition

Two trigger paths; no automatic archival:

1. **Explicit:** new `belmont archive --feature <slug>` CLI subcommand (or `belmont archive --all` for batch). Both modes prompt `[y/N]` by default; `--yes` skips the prompt for CI/scripted use.
2. **Time-gated suggestion:** at `belmont auto` startup and at `belmont status`, list any feature whose `PROGRESS.md` shows **all milestones complete** (all tasks `[v]`) **and** whose newest tracked file mtime is **≥ N days** in the past, where N defaults to 30 and is configurable via `.belmont/config.yaml` `auto_archive_after_days: 30`. The lister does **not** archive automatically — it prints a suggestion line. Auto-archival is deliberately rejected (see §Open Questions).

The CLI command is the actor; the suggestion is informational. This matches the existing `/belmont:cleanup` invariant: "Non-destructive by default — the preference order is keep > archive > delete."

#### User-facing behaviour

`belmont status` listing with one eligible feature:

```
$ belmont status

Belmont Status
==============

  data-pipeline-refactor     Tasks: 12/12 done | Milestones: 3/3 verified
                             ✓ Archive candidate (no edits in 47 days)
                             Run: belmont archive --feature data-pipeline-refactor
```

Archive invocation:

```
$ belmont archive --feature data-pipeline-refactor

About to archive:
  Feature:       data-pipeline-refactor
  Slug:          data-pipeline-refactor
  Status:        all milestones verified
  Last edit:     2026-04-06 (47 days ago)
  Files:         PRD.md (412 lines), TECH_PLAN.md (208 lines), PROGRESS.md (84 lines),
                 MILESTONE-M1.done.md, MILESTONE-M2.done.md, MILESTONE-M3.done.md, NOTES.md

This will MOVE the entire directory to .belmont/features/_archived/data-pipeline-refactor/
and REPLACE its contents with a single ARCHIVE.md summary (~0.5 KB), following the same
pattern /belmont:cleanup uses today.

Continue? [y/N]: y

✓ Archived to .belmont/features/_archived/data-pipeline-refactor/ARCHIVE.md
✓ Removed 7 source files
✓ Updated master .belmont/PROGRESS.md feature table
✓ Original files preserved in .belmont/features/_archived/data-pipeline-refactor/_originals/
  (delete manually after a successful run, or with `belmont archive --purge data-pipeline-refactor`)
```

Reuses the existing [`/belmont:cleanup` archive flow](../skills/belmont/_src/cleanup.md) (generate `ARCHIVE.md`, remove source files), plus the existing `extractArchiveName` reader at [`cmd/belmont/main.go:2450`](../cmd/belmont/main.go) (so `belmont status --show-archived` continues to work). New surface:

- A new `_archived/` subdirectory under `.belmont/features/`.
- An `_originals/` preservation subdir inside each archive holding the unmodified source files — a safety net for one major release cycle; after that, `_originals/` is dropped by default unless `keep_archive_originals: true` is set.
- A `--purge` flag to delete `_originals/` after the user is satisfied.
- The master `.belmont/PROGRESS.md` `## Features` table updated to mark the row as archived (same column the existing `status` JSON `ArchivedFeatures` field at [`main.go:1886`](../cmd/belmont/main.go) already reads).

#### Acceptance criteria

- New `belmont archive` subcommand: `--feature <slug>`, `--all`, `--feature <slug> --yes`, `--purge <slug>`. Help text via `belmont archive --help`.
- Archive writes a slim `ARCHIVE.md` (≤30 lines: feature name, slug, completed-on date, milestone count, task count, link to git log for full history) using the format the existing `/belmont:cleanup` skill produces.
- Archive MOVES the directory to `.belmont/features/_archived/<slug>/` rather than archiving in place (see §Open Questions on the in-place alternative).
- `_originals/` subdir created on archive, removed on `--purge`, retained by default for one major release cycle.
- `belmont status` lists archive candidates (no auto-archive — suggestion is informational).
- `auto_archive_after_days` config key defaults to 30; `0` disables the suggestion.
- Master `.belmont/PROGRESS.md` `## Features` table updated on archive (slug + status column = `archived`).
- Existing manual `_archived/` directories detected and respected — if `.belmont/features/_archived/<slug>/` already exists with an `ARCHIVE.md`, `belmont archive` reports the feature as already archived and exits 0.
- Unit tests for: archive of a verified feature, refusal to archive a feature with `[!]` blocked tasks, refusal to archive a feature with `[ ]` pending tasks, idempotency of repeat invocations.

### Mechanism 3: PRD Size Warning

#### Trigger condition

Three trigger paths, all advisory:

1. **During `/belmont:product-plan`** — before writing or appending to a `PRD.md`, check the current line count. If ≥ threshold (default 500), display a warning inline and ask whether to continue or split.
2. **During `/belmont:tech-plan`** — same check, displayed at the start of the session (so the tech-plan interview can suggest splitting before any new content lands).
3. **At `belmont auto` startup** — alongside the PRD code-bleed lint from Mechanism 1, emit a warning per oversize PRD. Non-blocking.

All three paths are advisory. There is no `--strict-prd-size` mode; size is a softer signal than code-bleed because some features genuinely need a longer document.

#### Configurable threshold (two-tier)

```yaml
# .belmont/config.yaml
prd_size_warning_lines: 500    # default; 0 disables
prd_size_critical_lines: 2000  # default; second tier — different message
```

500 lines = "you might want to consider splitting." 2,000 = "this PRD is now actively harming the agent loop." The two-tier shape matches the empirical distribution observed in multi-repo Belmont deployments (most large PRDs are in the 500–1,500 band; a handful are in the multi-thousand-line band where the user-visible signal needs to be louder).

#### User-facing behaviour

`/belmont:product-plan` inline warning (interactive session, before any writes):

```
[product-plan] checking existing PRD size for feature: bug-batch

⚠ PRD.md is 4,830 lines — above the size critical threshold (2,000 lines).

This usually signals one of:
  - A feature that should be split into multiple smaller features
  - A bug-tracker masquerading as a feature (each bug fix is its own task without archival)
  - Accumulated task definitions from completed milestones that should have been archived

Recommendations:
  - Run /belmont:cleanup to extract completed milestones to ARCHIVE.md summaries
  - Run belmont archive --feature bug-batch if all milestones are complete
  - Use /belmont:product-plan --feature bug-batch-followup to split off new work

Continue with the current PRD? [Y/n]: _
```

`belmont auto` startup warning:

```
[validate] scanning PRD.md sizes...
  ⚠ bug-batch/PRD.md: 4,830 lines (>2,000 critical threshold)
  ⚠ uat-suite/PRD.md: 3,617 lines (>2,000 critical threshold)
  ⚠ platform-foundation/PRD.md: 3,132 lines (>2,000 critical threshold)
  ⚠ data-pipeline-refactor/PRD.md: 712 lines (>500 warning threshold)

[validate] continuing — 4 warning(s), 0 error(s).
```

#### Acceptance criteria

- New config keys `prd_size_warning_lines` (default 500) and `prd_size_critical_lines` (default 2000) parsed from `.belmont/config.yaml`. Both default-applied when the file or keys are absent.
- `belmont auto` startup emits one warning per oversize PRD, with the correct tier annotated.
- `/belmont:product-plan` skill body modified to display the inline warning before any PRD writes and to ask `Continue? [Y/n]`. The user's `n` aborts with a one-line suggestion to run `belmont archive` or `/belmont:cleanup`.
- `/belmont:tech-plan` skill body modified to display a read-only warning at session start (no prompt — tech-plan does not write the PRD body).
- `belmont validate --prd-size` standalone flag for CI/scripted use; reports oversize PRDs and exits 0 (warning-only — never errors).
- Setting `prd_size_warning_lines: 0` fully disables the warning (and the critical-tier check).
- Unit test covering: 0-line config disables both checks, defaults applied when config absent, both tiers fire correctly on a 3,000-line sample PRD, only warning tier fires on a 700-line sample.

### Cross-mechanism configuration

All three mechanisms read from a single new `.belmont/config.yaml` file. Existing Belmont features (per [`docs/cli-commands.md`](../docs/cli-commands.md) and the JSON config files at `~/.belmont/local-llms.json` and `.belmont/local-llms.json`) do not use a top-level YAML config — this proposal introduces one.

```yaml
# .belmont/config.yaml (all keys optional, defaults shown)

strict_prd: false                 # Mechanism 1 — promote PRD code-bleed warnings to errors
prd_code_bleed_min_lines: 5       # Mechanism 1 — minimum fenced-block length to consider

auto_archive_after_days: 30       # Mechanism 2 — days idle before archive suggestion
keep_archive_originals: true      # Mechanism 2 — preserve _originals/ subdir
                                  # Note: defaults to true for one major release after this PR ships,
                                  # then defaults to false

prd_size_warning_lines: 500       # Mechanism 3 — warning tier threshold
prd_size_critical_lines: 2000     # Mechanism 3 — critical tier threshold
```

The file is optional; absent file means all defaults. A separate `belmont config` subcommand is out of scope for this proposal (the user edits the file directly), matching the convention for `.belmont/local-llms.json`.

## User-Facing Behaviour

### Day-1 user experience after upgrade

```
$ belmont auto --feature platform-foundation

[validate] scanning PROGRESS.md milestone structure... ok
[validate] scanning PRD.md files for code-bleed... 4 warning(s)  ← Mechanism 1
[validate] scanning PRD.md sizes... 3 warning(s), 1 critical    ← Mechanism 3
[validate] suggesting archive candidates... 2 eligible          ← Mechanism 2 suggestion

⚠ The following PRDs contain leaked code blocks:
  - data-pipeline-refactor/PRD.md:184–212 (28 lines, python)
  - db-migration/PRD.md:512–589 (77 lines, sql)
  - bug-batch/PRD.md:2104–2117 (13 lines, typescript)
  - platform-foundation/PRD.md:891–944 (53 lines, python)

⚠ The following PRDs exceed the size threshold:
  - bug-batch/PRD.md: 4,830 lines (critical >2,000)
  - uat-suite/PRD.md: 3,617 lines (critical >2,000)
  - platform-foundation/PRD.md: 3,132 lines (critical >2,000)
  - data-pipeline-refactor/PRD.md: 712 lines (warning >500)

ℹ The following features are archive candidates:
  - data-pipeline-refactor (idle 47 days, all verified)
    Run: belmont archive --feature data-pipeline-refactor
  - infra-cost-audit (idle 89 days, all verified)
    Run: belmont archive --feature infra-cost-audit

[auto] continuing — 8 warning(s), 1 critical, 0 errors. Use --strict-prd to abort on warnings.
[auto] dispatching milestone M3 for feature platform-foundation...
```

### Day-N steady state

Once the user has reconciled the warnings (move leaked code to TECH_PLAN; archive the eligible features; split or archive the oversize PRDs):

```
$ belmont auto --feature platform-foundation

[validate] scanning PROGRESS.md milestone structure... ok
[validate] scanning PRD.md files for code-bleed... ok
[validate] scanning PRD.md sizes... ok
[validate] suggesting archive candidates... none

[auto] dispatching milestone M3 for feature platform-foundation...
```

Zero noise once hygiene is maintained — that's the design target.

## Acceptance Criteria

Grouped per mechanism. (Repeats the per-mechanism criteria above for ease of review.)

### Cross-cutting

- `.belmont/config.yaml` parser added (new file in `cmd/belmont/main.go` or a sibling); tolerates missing file, missing keys, comments, and unknown keys (logged at debug level).
- [`docs/cli-commands.md`](../docs/cli-commands.md) updated with new `belmont archive` and `belmont validate --prd-bleed` / `--prd-size` invocations.
- [`docs/prd-format.md`](../docs/prd-format.md) updated with a §Size Guidance and §Code-Bleed Anti-Patterns sub-section.
- `CHANGELOG.md` entry under "Unreleased" describing all three mechanisms.
- No regressions in existing `cmd/belmont/scope_guard_test.go` milestone-immutability tests.

## Out of Scope

- **Rewriting the `/belmont:tech-plan` Phase 4.5 reconciliation.** The existing skill already documents the manual reconciliation; this proposal adds detection but does not automate the move of code out of PRDs into TECH_PLAN files. That is a follow-up if lint usage demonstrates demand.
- **Retroactive code-block extraction.** The lint warns; the user (or a future `belmont reconcile` skill) fixes. No automated fix-up because moving code requires architectural judgement about where it belongs.
- **Auto-archive without user confirmation.** Deliberately rejected — the `/belmont:cleanup` invariant ("non-destructive by default") applies. An "auto-archive on `belmont auto` start with `[y/N]` prompt" mode could be a follow-up.
- **Per-feature config overrides.** All three mechanisms read from a single `.belmont/config.yaml`. Per-feature overrides (`.belmont/features/<slug>/config.yaml`) are not in scope.
- **Granular per-PRD-section size limits.** The size lint counts total lines, not `## Tasks` length or `## Notes` length. A nuanced "your `## Tasks` section is too long" rule is a follow-up.
- **PR/FAQ size warning.** `PR_FAQ.md` sizes are unaudited and out of scope; this proposal is PRD-specific.
- **Master `.belmont/PRD.md` size warning.** The master PRD is the feature catalogue, not a feature spec — different size dynamics. Out of scope.
- **Browser-based archive viewer.** No UI for browsing `.belmont/features/_archived/`. `belmont status --show-archived` is the existing CLI surface and remains the only viewer.
- **Belmont fast-mode integration.** If proposal [0001](./0001-quick-mode.md) lands, its `/belmont:quick` skill writes feature directories without PRD files. The size and code-bleed lints should both skip features with no PRD — a ~4-line check.

## Open Questions

1. **In-place `ARCHIVE.md` vs moved-to-`_archived/`.** The existing `/belmont:cleanup` supports the in-place pattern. In-place archival leaves stale directory entries cluttering the active features listing — the moved-to-`_archived/` pattern keeps it clean. This proposal chooses the moved-to-`_archived/` pattern for `belmont archive`. Should the in-place pattern be deprecated, kept as `--mode in-place` flag, or untouched? **Recommendation:** keep `/belmont:cleanup`'s in-place pattern untouched (it's the manual flow), but `belmont archive` always moves. Document the divergence in `docs/cli-commands.md`.
2. **Should auto-archive ever happen non-interactively?** The proposal is suggestion-only. A future "auto-archive on `belmont auto` start with `[y/N]` prompt, fully-automatic with `--yes`" is plausible. **Recommendation:** defer; gather signal from suggestion-only usage first.
3. **Code-bleed rule's language allowlist.** Flags TypeScript, JavaScript, Python, Go, Rust, Java, Kotlin, Swift, SQL, Bash. Should it also flag `yaml` and `json` above some size? **Recommendation:** keep YAML/JSON exempt by default (legitimate PRD content per `docs/prd-format.md`); add a `prd_strict_code_languages: [yaml, json]` opt-in config key for users who want stricter.
4. **PRD size threshold defaults — is 500 too low?** 500 lines covers most PRDs that benefit from a structural review; 2,000 is the "this is now actively harmful" tier. The defaults may need tuning for upstream's typical PRD shape. **Recommendation:** ship 500/2000 as defaults but document the tradeoff prominently in `docs/prd-format.md`.
5. **`.belmont/config.yaml` schema — does this set a precedent for config bloat?** This proposal introduces the first project-level YAML config file. Future skills may want to add keys. **Recommendation:** scope this PR's keys to the three mechanisms; document the file as the standard location for project-level Belmont config; refuse to load unknown top-level sections (debug-log them) to keep the surface tight.
6. **Should the code-bleed lint run during `/belmont:product-plan` as well as `belmont auto`?** Currently only at `belmont auto` start and on explicit `belmont validate --prd-bleed`. Adding it to interactive `/belmont:product-plan` sessions would catch leaks at the moment they're introduced. **Recommendation:** defer to a follow-up — interactive integration requires the skill to surface tooling output back to the user, which is a different design problem than CLI command output.
7. **Magic-comment syntax — `<!-- belmont-lint: allow-code-bleed -->` vs `<!-- belmont:lint allow=code-bleed -->`.** The first is human-readable; the second is parseable as a structured directive. This proposal uses the first because the only directive today is "allow this single rule on the next block". If multiple rule-types accumulate, the second syntax is more extensible. Maintainer preference welcome.
8. **Interaction with `belmont auto --allow-dirty`.** The clean-tree preflight currently bypasses the working-tree check. Should `--allow-dirty` also bypass the PRD lint? **Recommendation:** no — `--allow-dirty` covers git state, not PRD content. The PRD lint has its own `--no-strict-prd` (the inverse of `--strict-prd`).

## References

- [`cmd/belmont/main.go` `detectViolations` (line 11521)](../cmd/belmont/main.go) — extension point for the new `prd_code_bleed` rule.
- [`cmd/belmont/main.go` `runValidateCmd` (line 11436)](../cmd/belmont/main.go) — host for new `--prd-bleed` / `--prd-size` flags.
- [`cmd/belmont/main.go` `extractArchiveName` (line 2450)](../cmd/belmont/main.go) — existing reader for `ARCHIVE.md` summaries; reused by Mechanism 2.
- [`skills/belmont/_src/cleanup.md`](../skills/belmont/_src/cleanup.md) — manual archive flow; the precedent Mechanism 2 automates.
- [`skills/belmont/_src/tech-plan.md`](../skills/belmont/_src/tech-plan.md) — Phase 4.5 PRD reconciliation; the manual flow Mechanism 1 detects automatically.
- [`skills/belmont/_src/product-plan.md`](../skills/belmont/_src/product-plan.md) — interactive host for Mechanism 3's inline size warning.
- [`docs/prd-format.md`](../docs/prd-format.md) — PRD format spec; updated with §Size Guidance and §Code-Bleed Anti-Patterns.
- [`docs/cli-commands.md`](../docs/cli-commands.md) — CLI command reference; updated with `belmont archive`, `--prd-bleed`, `--prd-size`.
- [`knowledge/cross-cutting/milestone-immutability.md`](../knowledge/cross-cutting/milestone-immutability.md) — three-layer enforcement pattern; Mechanism 1 reuses the same `detectViolations` rule engine.
- [`cmd/belmont/scope_guard_test.go`](../cmd/belmont/scope_guard_test.go) — unit test style for new `prd_code_bleed` rule tests.
- [Proposal 0001: Quick Mode](./0001-quick-mode.md) — companion proposal; the PRD-skip short-circuit in §Out of Scope assumes its `Mode: quick` marker.
