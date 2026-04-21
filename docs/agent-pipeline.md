# Agent Pipeline Details

All implementation agents communicate through the **MILESTONE file** (`.belmont/MILESTONE.md`). The MILESTONE file is a **coordination document**: it lists the active task IDs and points sub-agents at the canonical PRD and TECH_PLAN. Each sub-agent reads the MILESTONE file, fetches the full task definitions from the PRD directly, and writes its output to a designated section of MILESTONE. The three sub-agent-written sections (`## Codebase Analysis`, `## Design Specifications`, `## Implementation Log`) remain the source of truth for the downstream agents in the pipeline.

**Important**: Do not edit `{base}/PRD.md` while `/belmont:implement` or `/belmont:verify` is running — sub-agents now read the PRD live rather than receiving a pinned copy embedded in MILESTONE, so mid-run edits can cause agents to see inconsistent versions.

## Phase 1: Codebase Scan (codebase-agent) — *parallel with Phase 2*

**File**: `.agents/belmont/codebase-agent.md` | **Model**: Sonnet

Reads the MILESTONE file, then scans the project:
- Framework, language, styling, testing stack
- Project structure and conventions
- Related code, utilities, and type definitions
- CLAUDE.md rules (if present)
- Import patterns, error handling patterns, test patterns

**Writes to**: `## Codebase Analysis` section of MILESTONE.md

## Phase 2: Design Analysis (design-agent) — *parallel with Phase 1*

**File**: `.agents/belmont/design-agent.md` | **Model**: Sonnet

Reads the MILESTONE file, then analyzes Figma designs when provided:
- Loads designs via Figma Plugin or MCP
- Extracts exact colors, typography, spacing, effects
- Maps to existing design system components
- Identifies new components to create
- Produces implementation-ready component code

**Writes to**: `## Design Specifications` section of MILESTONE.md

**Blocking**: If Figma URLs are provided but fail to load, the task is blocked.

## Phase 3: Implementation (implementation-agent) — *after Phases 1+2*

**File**: `.agents/belmont/implementation-agent.md` | **Model**: Opus

Reads the complete MILESTONE file (all research phases' output):
- Types/interfaces first, then utilities, then components
- Follows project patterns from `## Codebase Analysis`
- Uses design specifications from `## Design Specifications` for UI code
- Writes unit tests and Playwright E2E tests (for web UI tasks)
- Runs verification: `tsc`, `lint:fix`, `test`, `build`
- Commits to git with structured commit message
- Reports out-of-scope issues as follow-up tasks

**Writes to**: `## Implementation Log` section of MILESTONE.md

## Verification (verification-agent)

**File**: `.agents/belmont/verification-agent.md` | **Model**: Sonnet

Verifies implementations against requirements:
- Reads PRD, TECH_PLAN, and archived MILESTONE files for context
- Acceptance criteria pass/fail
- Visual comparison with Figma (Playwright headless)
- i18n key verification
- Functional testing (happy path, edge cases, accessibility)

## Code Review (code-review-agent)

**File**: `.agents/belmont/code-review-agent.md` | **Model**: Sonnet

Reviews code for quality and alignment:
- Reads PRD, TECH_PLAN, and archived MILESTONE files for context
- Runs build, test, and E2E test commands (auto-detects the project's package manager)
- Checks pattern adherence and CLAUDE.md compliance
- Verifies PRD/tech plan alignment
- Security and performance review
- Categorizes issues: critical, warnings, suggestions
