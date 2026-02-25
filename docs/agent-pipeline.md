# Agent Pipeline Details

All implementation agents communicate through the **MILESTONE file** (`.belmont/MILESTONE.md`). Each agent reads its context from the file and writes its output to a designated section. This eliminates the need for the orchestrator to pass large outputs between agents.

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
- Writes unit tests
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
- Runs build and test commands (auto-detects the project's package manager)
- Checks pattern adherence and CLAUDE.md compliance
- Verifies PRD/tech plan alignment
- Security and performance review
- Categorizes issues: critical, warnings, suggestions
