# Skill Format & Per-Tool Wiring

**Domains:** cli, skills, agents

**Why this matters.** Belmont installs the same skill content for six different AI CLIs. Before Phase 2 (April 2026) each CLI needed bespoke wiring — `.codex/belmont/` copies, `.cursor/rules/belmont/*.mdc` per-file symlinks, `.windsurf/rules/belmont` directory symlinks, `.gemini/rules/belmont` directory symlinks, `.copilot/belmont/` directory symlinks, plus `.claude/commands/belmont/` copies — and three of those targets were silently dead (the CLI didn't actually scan that path). The agentskills.io standard collapsed all of this into a single canonical location, `.agents/skills/<skill>/SKILL.md`, that every supported CLI auto-discovers natively. Phase 2 converged Belmont onto that layout.

## Invariant

- Skills live in `.agents/skills/belmont/<skill>/SKILL.md` with `name:` and `description:` YAML frontmatter (per agentskills.io spec).
- Each skill folder contains a `references/` subdir with the progressive-disclosure files that skill body actually references — only those, scoped per skill.
- Five of six supported CLIs (Codex, Cursor, Windsurf, Gemini, GitHub Copilot) auto-discover `.agents/skills/` natively. Belmont creates **zero per-tool wiring** for them. Claude Code is the only exception — it needs symlinks at `.claude/agents/belmont` (sub-agents) and `.claude/skills/belmont` (skills) since it scans `.claude/`, not `.agents/`.
- Generated skill folders are NOT committed to git. Only `_src/` and `_partials/` are versioned. Generation runs on demand via `go generate`, on `belmont install --source` (auto-detects staleness), or in `build.sh` before `go build -tags embed`.

## How it's enforced

In `scripts/generate-skills.sh`:
- Walks `skills/belmont/_src/*.md`, expands `<!-- @include ... -->` partials into a temp file, then runs an awk frontmatter rewriter that injects `name: <filename>` after the opening `---` and drops any pre-existing `name:` line. Result lands at `skills/belmont/<skill>/SKILL.md`.
- For each generated SKILL.md, greps the body for `references/<X>.md` patterns and copies only those files from `_src/references/` into `<skill>/references/`. Robust to references that don't follow the `<skill>-` prefix convention (e.g. `models-yaml-format.md`, referenced from `tech-plan.md`'s body).
- Stale skill folders (target dirs with no matching `_src/` source) are pruned. Only directories that already contain a `SKILL.md` are touched, so user-created adjacent dirs are safe.

In `cmd/belmont/main.go`:
- `//go:generate bash scripts/generate-skills.sh` directive (top of file) so `go generate ./...` regenerates.
- `ensureSkillsGenerated(sourceRoot)` runs in source-mode install before reading skills/. If any `_src/` or `_partials/` file mtime is newer than the matching SKILL.md (or no SKILL.md exists), it shells out to `bash scripts/generate-skills.sh`. Embedded mode is unaffected — embed.FS already contains the generated content.
- `syncSkillsFolderDir` (source mode) and `syncEmbeddedSkillsFolderDir` (embedded mode) sync `<skill>/SKILL.md` + per-skill `references/` from source/embed FS to `.agents/skills/belmont/`. Stale skill folders in target are removed.
- `setupTool` for codex/cursor/windsurf/gemini/copilot is a no-op for content (each prints `= .agents/skills/belmont auto-discovered`). Only claude has actual wiring (two symlinks).
- `runLegacyCleanup(projectRoot)` runs once per `runInstall` (before `setupTool`), idempotently removing pre-Phase-2 install artifacts: `.claude/commands/belmont`, `.codex/belmont`, `.cursor/rules/belmont`, `.windsurf/rules/belmont`, `.gemini/rules/belmont`, `.copilot/belmont`, stale `.agents/skills/belmont/*.md` flat files, stale `.agents/skills/belmont/references/` top-level dir, and `belmont:skill-routing` (or older `belmont:codex-skill-routing`) sections in `AGENTS.md` / `GEMINI.md`.

In `.gitignore`: `skills/belmont/*/` excludes every generated skill folder, with `!skills/belmont/_src/` and `!skills/belmont/_partials/` overriding to keep sources tracked.

## Failure mode if you break it

- **Removing the agentskills.io frontmatter `name:` field**: skills become unindexed by Codex's `/skills`, Cursor's Skills panel, Gemini's `activate_skill` tool, and Windsurf's Cascade activation. Symptom: agent doesn't recognize `belmont:<skill>` references. Fix: re-run `generate-skills.sh` so the awk injector re-adds the field.
- **Reverting to flat `<name>.md` layout**: only Claude Code's legacy `.claude/commands/` would still see them. Codex/Cursor/Gemini/Windsurf/Copilot scan for `<dir>/SKILL.md`, not `<file>.md`. Symptom: silent zero-discovery for five of six CLIs.
- **Committing generated output**: drift between generated and committed becomes a recurring source of "did you regenerate?" footguns. Phase 1 had this exact problem with the flat layout — every partial change required two commits (source + generated). Don't reintroduce it.
- **Per-skill references lost or wrong**: skill body says `references/foo.md` but the file isn't in `<skill>/references/`. Symptom: agent reports "file not found" mid-skill. The grep + copy in `generate-skills.sh` solves this; `generate-plugin.sh`'s old `*-prefix glob` did NOT (it missed `models-yaml-format.md` for tech-plan). Don't go back to prefix globs.
- **Skipping `runLegacyCleanup` after upgrade**: users who came from Phase 1 keep stale `.codex/belmont/` and `belmont:skill-routing` sections in AGENTS.md/GEMINI.md indefinitely. Symptom: confused users seeing "ghost" Belmont content in their AGENTS.md they didn't write. Cleanup is idempotent and runs every install — keep it.
- **Detect-tools regression**: if `detectTools` only checks dir presence (the pre-Phase-1 default), upgrades from Phase 1 silently drop Codex/Copilot/Gemini wiring (those tools never created marker dirs). The fix is the three-signal detection: dir presence OR PATH binary OR existing routing marker in AGENTS.md/GEMINI.md.

## Don't re-do

- **Restructure source `_src/` to a folder layout** so it matches the output 1:1. Tempting because it's "cleaner". Cost: every skill becomes a directory, harder to scan in `ls _src/`, more boilerplate when adding/renaming, and the actual content is still a single markdown file per skill. Flat sources + folder generation is the better tradeoff.
- **Per-CLI install wiring** that copies content into `.codex/`, `.gemini/rules/`, `.copilot/`, etc. The agentskills.io standard means every supported CLI reads `.agents/skills/`. Adding per-tool dirs is dead writes that confuse users (they see Belmont files in places the tool doesn't actually look).
- **Strip the `<skill>-` prefix from reference filenames** (so `implement-milestone-template.md` becomes `milestone-template.md` inside `implement/references/`). Tempting because the prefix is redundant. Cost: skill body path-rewriting required, brittle. Keep the prefixed naming — paths in skill bodies resolve correctly without rewrites.
- **Use a top-level shared `references/` dir** (`skills/belmont/references/<topic>.md`) instead of per-skill folders. Was Phase 1's layout. Cost: skill body relative paths resolve to the wrong location once the skill itself moves into a subfolder; plus references are loaded for every skill regardless of need. Per-skill folders give precise progressive disclosure.
- **Keep AGENTS.md / GEMINI.md routing sections** "as a fallback for new CLIs that don't auto-discover". Costs ~10 lines in user-owned files for ambiguous benefit. Every supported CLI as of April 2026 auto-discovers `.agents/skills/`. If a future CLI doesn't, add wiring then; don't pollute every project preemptively.
- **Skip `--bare` / hermetic-mode flags** when adding new tools. The bare-mode pattern (Claude's `--bare`, Codex's `--ignore-user-config`, Gemini's `--skip-trust`) is the right way to make scripted runs deterministic. Belmont doesn't use these today but should add them when porting the auto loop to a CI environment.

## Evidence

- Verified against live docs (April 2026) for all six CLIs:
  - Cursor: cursor.com/docs/context/skills lists `.agents/skills/` among workspace skill dirs.
  - Gemini: geminicli.com/docs/cli/skills/ confirms `.agents/skills/` is the documented `.gemini/skills/` alias and "takes precedence" over `.gemini/skills/`.
  - Codex: codex-rs/core-skills/loader.rs constants `SKILLS_FILENAME = "SKILL.md"` and `AGENTS_DIR_NAME = ".agents"`.
  - Windsurf: docs.windsurf.com/windsurf/cascade/skills says `.agents/skills/` is auto-discovered alongside `.windsurf/skills/`.
  - Copilot: docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-skills lists `.agents/skills` among the project-scope scan paths.
  - Claude Code: code.claude.com/docs/en/skills documents `.claude/skills/<namespace>/SKILL.md` as the canonical location.
- Unit coverage: `cmd/belmont/commit_update_test.go` includes `TestRunLegacyCleanup_RemovesLegacyDirsAndAgentsSection`, `TestRunLegacyCleanup_Idempotent`, `TestCommitBelmontUpdate_StagesDeletionOfLegacyPath`.

## Revisions

- 2026-04-30 — initial entry (Phase 2 SKILL.md migration: source layout flat, generation produces folder layout, generated output gitignored, all 5 non-Claude CLIs become zero-config, legacy cleanup pass added).
