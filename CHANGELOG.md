# Changelog

## v0.10.0

**Released:** 2026-04-22

### Changes since v0.9.9

- Heavy refactoring + bugfixing for parallel auto mode
- Add `belmont steer` command to instruct agents during auto mode
- Dynamic model assignment based on feature



## v0.9.9

**Released:** 2026-04-21

### Changes since v0.9.8

- reconciliation: unstage reconciliation-report.json from merge commit
- recover: auto-detect AI tool (or accept --tool flag)
- install: write relative symlinks; reconciliation: handle symlinks+parents
- agents: add tactical Web Research guidance to implement/verify/review
- auto: grant WebFetch and WebSearch to claude dispatch
- agents: mark tasks [>] in_progress when starting each task
- skills: token-saver — MILESTONE coordinator + references/ convention



## v0.9.8

**Released:** 2026-04-21

### Changes since v0.9.7

- Allow Belmont to understand the "other side" of the planning equation



## v0.9.7

**Released:** 2026-04-21

### Changes since v0.9.6

- Remove the "tiered" approach from planning question analysis



## v0.9.6

**Released:** 2026-04-20

### Changes since v0.9.5

- Improvements to planning modes



## v0.9.5

**Released:** 2026-04-16

### Changes since v0.9.4

- Improve `belmont status` output



## v0.9.4

**Released:** 2026-04-16

### Changes since v0.9.3

- Enforce design reference comparison in visual verification



## v0.9.3

**Released:** 2026-04-16

### Changes since v0.9.2

- Fix reverify command not finding verified tasks to re-verify
- Prevent implementation agent from verifying its own tasks



## v0.9.2

**Released:** 2026-04-15

### Changes since v0.9.1

- Allow belmont --version/-v as well as default belmont version



## v0.9.1

**Released:** 2026-04-10

### Changes since v0.9.0

- Add belmont to Homebrew
- Add Belmont to Claude Marketplace



## v0.9.0

**Released:** 2026-04-10

### Changes since v0.8.7

- Fully rebuild Belmont tracking system



## v0.8.7

**Released:** 2026-04-09

### Changes since v0.8.6.1

- Make reverify a standalone command instead of auto mode flag



## v0.8.6.1

**Released:** 2026-04-09

### Changes since v0.8.6

- Fix reverify loop stopping after first milestone



## v0.8.6

**Released:** 2026-04-09

### Changes since v0.8.5.2

- Enable --reverify in multi-feature auto mode
- Instruct agents to dynamically allocate ports for non-primary servers



## v0.8.5.2

**Released:** 2026-04-08

### Changes since v0.8.5.1

- Fix recover --clean slug in interrupt preservation message



## v0.8.5.1

**Released:** 2026-04-08

### Changes since v0.8.5

- Fix auto --reverify exiting early when milestones already flipped



## v0.8.5

**Released:** 2026-04-08

### Changes since v0.8.4

- Add `belmont reverify` command and `auto --reverify` mode
- Strengthen rules around Playwright MCP usage



## v0.8.4

**Released:** 2026-04-07

### Changes since v0.8.3

- Improvements to workflow: Cleanup skill, Five Whys method, fixes for "single-feature" auto mode



## v0.8.3

**Released:** 2026-04-02

### Changes since v0.8.2.1

- Fix auto mode resume overwriting worktree progress
- Ensure auto.json is added to gitignore on install



## v0.8.2.1

**Released:** 2026-03-31

### Changes since v0.8.2

- Fix next skill archiving MILESTONE files with task ID instead of milestone ID



## v0.8.2

**Released:** 2026-03-31

### Changes since v0.8.1.1

- Fix auto mode triage/fix-all loop and milestone-scoped verification



## v0.8.1.1

**Released:** 2026-03-31

### Changes since v0.8.1

- Fix Windows build: extract platform-specific syscall usage



## v0.8.1

**Released:** 2026-03-31

### Changes since v0.8.0

- Fix master PROGRESS task counts drifting after tech-plan



## v0.8.0

**Released:** 2026-03-27

### Changes since v0.7.6.1

- Improvements to auto-merging capability
- Move all Belmont tracking inside worktree
- Improved auto detection and install of dependencies, when worktree.json missing
- Add setup concept for Belmont auto features



## v0.7.6.1

**Released:** 2026-03-24

### Changes since v0.7.6

- Fix for claude code hook format



## v0.7.6

**Released:** 2026-03-24

### Changes since v0.7.5

- Huge changes to auto workflow



## v0.7.5

**Released:** 2026-03-20

### Changes since v0.7.4

- Add --dry-run flag to belmont auto



## v0.7.4

**Released:** 2026-03-20

### Changes since v0.7.3

- Fix for user question tooling in planning modes



## v0.7.3

**Released:** 2026-03-19

### Changes since v0.7.2

- First version of E2E test flow integration
- Add authors section to README



## v0.7.2

**Released:** 2026-03-17

### Changes since v0.7.1

- Add Apache 2.0 license to Belmont



## v0.7.1

**Released:** 2026-03-17

### Changes since v0.7.0

- Add Ultrathink to Belmont planning modes



## v0.7.0

**Released:** 2026-03-16

### Changes since v0.6.0

- Use wave based parallelisation, with consideration of feature dependencies
- Rename `/belmont:review` to `/belmont:review-plans`
- Move to new "auto" logic
- V1 of parallel development. Working on multiple milestones within a git worktree.



## v0.6.0

**Released:** 2026-03-10

### Changes since v0.5.1

- Add smart rules engine to loop decision system
- Align README style
- Reorganize README into focused overview with docs/ reference pages



## v0.5.1

**Released:** 2026-02-24

### Changes since v0.5.0

- Split debug skill into auto and manual sub-workflows



## v0.5.0

**Released:** 2026-02-23

### Changes since v0.4.4

- Redesign debug skill to use agent-dispatched pipeline



## v0.4.4

**Released:** 2026-02-20

### Changes since v0.4.3

- Auto-cleanup verification screenshots and auto-commit .belmont/ files
- Allow Belmont to commit it's changed plans automatically after implementation



## v0.4.3

**Released:** 2026-02-19

### Changes since v0.4.2

- Let verifier make follow up tasks, it's not good at fixing things directly.



## v0.4.2

**Released:** 2026-02-19

### Changes since v0.4.1

- Release vv0.4.2
- Strength rule to cleanup Playwright MCP screenshots



## vv0.4.2

**Released:** 2026-02-19

### Changes since v0.4.1

- Strength rule to cleanup Playwright MCP screenshots



## v0.4.1

**Released:** 2026-02-19

### Changes since v0.4.0

- Remove tracked build artifacts and update .gitignore
- Add conditional Lighthouse audit phase to verification agent



## v0.4.0

**Released:** 2026-02-18

### Highlights

- **Working Backwards (PR/FAQ)**: New `/belmont:working-backwards` skill — define your product vision using Amazon's Working Backwards methodology before breaking it into features and tasks. Produces `.belmont/PR_FAQ.md` with a press release, FAQs, and product backlog.
- **Sub-Feature Architecture**: Belmont now organizes work into per-feature directories under `.belmont/features/<slug>/`. Each feature gets its own PRD, TECH_PLAN, PROGRESS, and MILESTONE files. A master PRD at `.belmont/PRD.md` acts as the feature catalog.
- **Document Review & Drift Detection**: New `/belmont:review` skill — interactively reviews alignment between your PR/FAQ, master PRD, feature PRDs, tech plans, PROGRESS files, and actual codebase. Surfaces drift, conflicts, and gaps with resolution options for each finding.
- **Live Notes**: New `/belmont:note` skill — save learnings, workarounds, environment quirks, and debugging insights to `NOTES.md` so they persist across sessions and context compactions. The implementation agent also captures non-obvious discoveries automatically after each task.
- **Recommend committing `.belmont/` to git**: The installer no longer adds `.belmont/` to `.gitignore`. Planning documents (PR/FAQ, PRD, TECH_PLAN) are meant to be shared with your team. If you previously had `.belmont/` in your `.gitignore`, consider removing it.

### New Skills

- **`/belmont:working-backwards`** — Amazon-style PR/FAQ creation. Guides you through customer definition, problem statement, and solution. Enforces writing quality: no weasel words, data over adjectives, under 30 words per sentence.
- **`/belmont:review`** — Alignment review across all planning documents. Compares PR/FAQ vision against master PRD, checks feature PRDs against master, verifies task/milestone consistency, scans codebase for unplanned implementations. Presents findings interactively with resolution options.
- **`/belmont:note`** — Capture learnings and discoveries to feature-level or global `NOTES.md`. Supports categories: environment, workaround, discovery, credential, pattern, debugging, performance.

### New `.belmont/` Directory Structure

The `.belmont/` directory has been restructured to support multi-feature products:

```
.belmont/
  PR_FAQ.md                    <- NEW: Strategic vision (Working Backwards)
  PRD.md                       <- Now a master feature catalog
  TECH_PLAN.md                 <- Master cross-cutting architecture
  PROGRESS.md                  <- Master progress (feature summary table)
  NOTES.md                     <- Global learnings (optional)
  features/                    <- NEW: Per-feature directories
    <feature-slug>/
      PRD.md
      TECH_PLAN.md
      PROGRESS.md
      MILESTONE.md
      NOTES.md
```

**Upgrading from v0.3.x**: Run `belmont update && belmont install` in your project. Then ask your AI agent to look at the updated Belmont skills and adjust your `.belmont/` directory to match the new structure — it will help migrate your existing PRD and PROGRESS into a feature directory.

### CLI Changes

- `belmont status` now supports `--feature <slug>` flag for feature-specific status
- `belmont status` (without `--feature`) shows a project-level overview with all features, their progress, and next tasks
- `belmont status` now reports PR/FAQ readiness
- `belmont install` creates `.belmont/PR_FAQ.md` template and `.belmont/features/` directory
- `belmont install` no longer adds `.belmont/` to `.gitignore` — planning docs should be committed

### Agent Changes

- Renamed `core-review-agent.md` to `code-review-agent.md` for clarity
- All agents now read file paths from the orchestrator's prompt instead of hardcoding `.belmont/` paths — enables the sub-feature directory structure
- Implementation agent now captures learnings to `NOTES.md` after each task (Step 5b)
- Verification agent now more strongly nudged to use Playwright for visual verification
- All skill prompts updated with feature selection logic and base path convention

### .gitignore Change

Previous versions of Belmont added `.belmont/` to your `.gitignore` during install. **This is no longer the case.** We now recommend checking `.belmont/` into source control so your team shares planning context (PR/FAQ, PRDs, tech plans, progress).

If you previously had `.belmont/` gitignored, consider removing that line:

```bash
# Remove .belmont/ from .gitignore if present
sed -i '' '/.belmont/d' .gitignore
```

## v0.3.5

**Released:** 2026-02-13

### Changes since v0.3.4

- Fix Figma access in planning skills by using inline MCP calls



## v0.3.4

**Released:** 2026-02-13

### Changes since v0.3.3

- Separate product vs technical question scope in planning skills



## v0.3.3

**Released:** 2026-02-12

### Changes since v0.3.2

- Bugfixing prompts



## v0.3.2

**Released:** 2026-02-11

### Changes since v0.3.1

- Refactor to allow skills generation. Adds strategies to remove token input.



## v0.3.1

**Released:** 2026-02-11

### Changes since v0.3.0

- Remove Claude settings
- Fix GitHub Copilot detection to use .copilot/ directory instead of .github/



## v0.3.0

**Released:** 2026-02-11

### Changes since v0.2.0

- Improve tech-plan to consider infrastructure + SQL optimisation



## v0.2.0

**Released:** 2026-02-11

### Highlights

- **Single-command install**: `curl -fsSL https://raw.githubusercontent.com/blake-simpson/belmont/main/install.sh | sh`
- **Self-updating binary**: `belmont update` downloads the latest release from GitHub
- **Embedded skills/agents**: Release binaries include all skills and agents — no source directory needed
- **Version info**: `belmont version` now shows version, commit SHA, and build date
- **Release automation**: GitHub Actions builds cross-platform binaries on tag push

### Changes

- Added `//go:embed` support — release binaries embed all skills and agents
- Added `belmont update` command with `--check` and `--force` flags
- Added `scripts/build.sh` for building release binaries with embedded content
- Added `scripts/release.sh` for preparing releases (changelog + tag)
- Added `.github/workflows/release.yml` for CI-driven cross-platform builds
- Added `install.sh` (root) — public curl-pipe-sh installer
- Added version injection via ldflags (`Version`, `CommitSHA`, `BuildDate`)
- Modified `belmont install` to use embedded files when no `--source` is specified
- Modified `belmont version` to show version, commit, and build date

## v0.1.0

**Released:** 2025-01-01

### Initial Release

- Go CLI with `install`, `status`, `tree`, `find`, `search`, `version` commands
- Agent-agnostic installer supporting Claude Code, Codex, Cursor, Windsurf, Gemini, and GitHub Copilot
- Markdown-based skills and agents for structured AI coding sessions
- PRD and PROGRESS tracking with milestone support