# Skills Reference

## `working-backwards`

Amazon-style Working Backwards document creation. Produces a PR/FAQ with press release, FAQs, and appendix.

- Guides you through customer definition, problem statement, and solution
- Writes a one-page press release with leader quote and customer testimonial
- Creates external (customer) and internal (stakeholder) FAQs
- Includes appendix with product backlog, KPIs, and competitive analysis
- Enforces writing quality: no weasel words, data over adjectives, under 30 words per sentence
- Scales interview depth to the work — calibrates silently from the brief, walks a fixed domain checklist, digs on ambiguity, and skips what's already settled (no round cap, no visible tier)
- Delegates market, competitor, pricing, and regulatory research to `Explore` / `general-purpose` sub-agents and cites sources in the appendix
- Does NOT create PRDs or implementation plans — that comes next

**Output**: `.belmont/PR_FAQ.md`

## `product-plan`

Interactive planning session. Creates the PRD and PROGRESS files. Supports multi-feature products with a master PRD (feature catalog) and per-feature PRDs.

- Calibrates silently from the brief (no visible tier) and walks a fixed **Domains to Cover** checklist (user flows, edge cases, accessibility, privacy, notifications, monetization, etc.) — running as many rounds per domain as the work requires
- Digs on ambiguity, skips what the brief or prior answers already settle, and only exits when every relevant domain is resolved and the user explicitly confirms nothing more to add
- Creates structured PRD with prioritized tasks (P0-P3)
- Organizes tasks into milestones in PROGRESS.md
- Includes Figma URLs, acceptance criteria, verification steps
- Delegates deep product research (competitive patterns, compliance frameworks, WCAG criteria) to `Explore` / `general-purpose` sub-agents and cites sources in a `### Research Notes` subsection of the PRD
- Does NOT implement anything -- plan mode only

**Output**: `.belmont/PRD.md`, `.belmont/PROGRESS.md` (master feature summary), `.belmont/features/<slug>/PRD.md`, `.belmont/features/<slug>/PROGRESS.md`

## `tech-plan`

Technical planning session. Creates a detailed implementation specification.

- Requires an existing PRD (run plan first)
- Acts as a senior architect reviewing and refining the plan
- Calibrates silently from the PRD and existing master tech plan (no visible tier) and walks a fixed **Domains to Cover** checklist (rendering, data model, auth, observability, testing, CI/CD, migration, etc.) — skipping domains already settled by the master tech plan or prior answers
- Runs as many rounds per domain as the work requires; digs on ambiguity and only exits when every relevant domain is resolved
- Loads Figma designs and extracts exact design tokens
- Produces concrete file structures, component skeletons, API types
- Maps PRD tasks to specific code sections
- Delegates framework / library / version / migration / security research to `Explore` / `general-purpose` sub-agents, flags stale sources (>12 months), and cites URLs in the `## References` section
- Interactive Q&A until the exit criteria are met (every relevant domain covered, user explicitly confirms no more open questions)

**Output**: `.belmont/TECH_PLAN.md`

## `implement`

Implements the next pending milestone from the PRD.

- Reads PROGRESS.md to find the first incomplete milestone
- Creates a **MILESTONE file** (`.belmont/MILESTONE.md`) with orchestrator context
- Runs 3 agents, each reading from and writing to the MILESTONE file:
  1. **Codebase Scan** (codebase-agent) -- Reads MILESTONE + codebase, writes `## Codebase Analysis` *(parallel with 2)*
  2. **Design Analysis** (design-agent) -- Reads MILESTONE + Figma, writes `## Design Specifications` *(parallel with 1)*
  3. **Implementation** (implementation-agent) -- Reads MILESTONE only, writes code + `## Implementation Log` *(after 1+2)*
- After each task: marks it as `[x]` done in PROGRESS.md
- After all milestone tasks: marks the milestone complete
- **Archives the MILESTONE file** (`MILESTONE-M2.done.md`) to keep context clean for next run
- Creates follow-up tasks (plain `[ ]` entries) for out-of-scope issues discovered during implementation
- Handles blockers gracefully -- marks blocked tasks as `[!]` and skips to the next

## `next`

Implements just the next single pending task — a lightweight alternative to the full implement pipeline.

- Reads PROGRESS.md to find the first unchecked task in the first pending milestone
- Creates a **minimal MILESTONE file** with just the single task's context (skips analysis agents)
- Dispatches the single task to the `implementation-agent` as a sub-agent
- After the task is done: marks it as `[x]` done in PROGRESS.md
- If it was the last task in the milestone, marks the milestone complete
- **Archives the MILESTONE file** after completion
- Creates follow-up tasks (plain `[ ]` entries) for any out-of-scope issues

**Best for**: Follow-up tasks from verification, small fixes, well-scoped isolated work.
**Use `/belmont:implement` instead for**: Large tasks, first tasks in a milestone, tasks needing Figma analysis.

## `verify`

Runs verification and code review on all completed tasks.

- Runs two agents **in parallel**:
  - **Verification Agent** -- Checks acceptance criteria, Figma pixel comparison (Playwright headless), i18n text keys, edge cases, accessibility
  - **Code Review Agent** -- Runs build and test commands (auto-detects package manager: npm, pnpm, yarn, or bun), reviews code against project patterns, checks PRD alignment
- Both agents read the PRD, TECH_PLAN, and archived MILESTONE files for full context
- Categorizes issues: Critical / Warnings / Suggestions
- Creates follow-up tasks (plain `[ ]` entries) in PROGRESS.md for anything that needs fixing
- Produces a combined summary report

## `debug`

Router that directs to the appropriate debug sub-workflow. Detects mode from user's invocation text or asks the user to choose.

## `debug-auto`

Auto debug loop — dispatches a verification agent to check each fix attempt.

- Uses the **agent-dispatch model** — each agent (implementation, verification, optionally design) runs in its own context window via `DEBUG.md` as shared context
- Tight investigate-fix-verify loop with max 3 iterations
- Dispatches design-agent on iteration 1 if Figma URLs are present in the PRD
- Reverts immediately on regression (`git checkout -- [files]`)
- User checkpoint after iteration 2 before continuing
- Single atomic commit with `debug:` prefix after user confirms the fix
- Ephemeral `DEBUG.md` — created at start, deleted when session ends
- Optional PRD integration: can mark follow-up tasks complete if relevant

**Best for**: Complex logic bugs, race conditions, issues needing automated test verification.

## `debug-manual`

Manual debug loop — the user verifies each fix instead of dispatching a verification agent. Faster iteration.

- Same agent-dispatch model as auto, but **no verification agent** — user checks each fix
- Implementation agent adds strategic `[BELMONT-DEBUG]` logging (5-15 log points per iteration)
- After each fix, presents summary and asks user to verify with debug log output
- All `[BELMONT-DEBUG]` log lines are automatically cleaned up before committing
- Same max 3 iterations, regression handling, and user checkpoint as auto mode

**Best for**: UI bugs, visual issues, known reproduction steps, anything the user can quickly verify.
**Use `debug-auto` instead for**: Complex logic bugs, race conditions, issues requiring automated testing.
**Use `/belmont:next` or `/belmont:implement` instead for**: New features, large multi-file changes.

## `review-plans`

Reviews alignment between planning documents and the codebase. Detects drift, conflicts, and gaps across the entire document hierarchy.

- Compares PR/FAQ vision against master PRD feature catalog
- Checks each feature's PRD and tech plan against master documents
- Verifies task/milestone consistency between PRD and PROGRESS files
- Scans codebase for unplanned implementations or stale task statuses
- Presents each finding interactively with resolution options
- Can update PRDs, tech plans, PROGRESS files, and NOTES based on decisions
- Does NOT modify source code — planning audit only

**When to use**: After implementation sessions, before major milestones, or periodically to keep plans aligned with reality.

## `cleanup`

Reduce input token bloat by archiving completed features, removing stale milestone files, trimming notes, and auditing convention files.

- Scans all `.belmont/` state and identifies completed features, archived milestones, stale notes
- Presents each item individually — user chooses to archive, keep, delete, or skip per item
- Archives completed features into slim `ARCHIVE.md` summaries (~0.5 KB vs ~5-15 KB original)
- Audits CLAUDE.md, AGENTS.md, `.cursorrules`, `.windsurfrules` for stale file paths and outdated conventions
- Checks tool directories (`.claude/`, `.codex/`, `.cursor/`, etc.) for stale copies or broken symlinks
- Does NOT modify source code or tool directories — only `.belmont/` state and convention files

**When to use**: After completing a batch of features, when context windows feel bloated, or periodically during long-running projects.

## `reset`

Reset belmont state. In feature mode, choose to reset a specific feature, all features, or everything including masters and PR/FAQ.

- Shows a summary of current state (feature name, task/milestone counts, completion status)
- Asks for explicit confirmation before resetting
- Resets PRD.md and PROGRESS.md to blank templates
- Deletes TECH_PLAN.md if it exists
- Does NOT touch agents, skills, or any source code

**Resets**: `.belmont/PR_FAQ.md`, `.belmont/PRD.md`, `.belmont/PROGRESS.md`, `.belmont/TECH_PLAN.md`, `.belmont/MILESTONE.md`, `.belmont/MILESTONE-*.done.md`, `.belmont/features/`

## `status`

Read-only progress report. Does not modify any files.

Example output (project-level):

```
Belmont Status
==============

Product: My App

PR/FAQ: Written
Master Tech Plan: Ready

Chat Application (chat-app)
  Tasks: 3/7 done  |  Milestones: 1/3 done
    M1: Foundation (verified)
    M2: Core Features (in_progress)
    M3: Polish (todo)
  Next: P1-2 — Add real-time message updates
  Blocked:
    - [!] P1-3: Figma design not accessible

Use --feature <slug> for detailed task-level status.
```

Example output (feature-level with `--feature chat-app`):

```
Belmont Status
==============

Feature: Chat Application

Tech Plan: Ready

Tasks: 3 done, 1 in progress, 1 blocked, 2 todo (of 7 total)

  [v] P0-1: Set up project structure
  [v] P0-2: Implement authentication flow
  [x] P1-1: Create chat message component
  [>] P1-2: Add real-time message updates
  [!] P1-3: Implement file attachments
  [ ] P2-1: Add emoji picker
  [ ] P2-2: Dark mode support

Milestones:
  M1: Foundation (verified)
  M2: Core Features (in_progress)
  M3: Polish (todo)

Blocked Tasks:
  - P1-3: Figma design not accessible

Recent Activity:
---
Last completed: P1-1 - Create chat message component
```
