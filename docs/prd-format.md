# PRD & Progress Format

## PRD.md

The PRD is a **living specification** -- purely requirements, no status markers. It is actively curated as the source of truth for *what* to build. Task headers have no status emoji.

```markdown
# PRD: Feature Name

## Overview
Brief description of the feature.

## Problem Statement
What problem does this solve?

## Success Criteria (Definition of Done)
- [ ] Criterion 1
- [ ] Criterion 2

## Acceptance Criteria (BDD)

### Scenario: User logs in
Given a registered user
When they enter valid credentials
Then they see the dashboard

## Technical Approach
High-level implementation strategy.

## Tasks

### P0-1: Set up authentication
**Severity**: CRITICAL
**Task Description**: Users can sign in to the product and return to a protected dashboard.
**Solution**: A sign-in screen accepts a Google account; on success the user lands on the dashboard with their name and avatar. Signed-out users hitting a protected page are redirected to sign-in.
**Verification**: New user completes Google sign-in and reaches the dashboard; signed-out visit to `/dashboard` redirects to sign-in.

### P1-1: Create dashboard layout
**Severity**: HIGH
**Task Description**: Build the main dashboard page
**Figma**: https://figma.com/file/xxx/node-id=123
**Solution**: Responsive layout matching the Figma node at mobile, tablet, and desktop breakpoints. Sidebar is collapsible on mobile.
**Verification**: Visual parity with Figma at all three breakpoints; sidebar collapses below md.
```

**Key points:**
- No status markers (emoji) on task headers -- status lives in PROGRESS.md only
- Follow-up tasks discovered during implementation are added as plain tasks (no special tag)
- The `**Verification**:` field lists *criteria* for the task, not a separate task. Do not create standalone "Verification", "QA", or "Unit Tests" tasks — verification runs automatically via `/belmont:verify` after each milestone.
- PRD tasks describe **WHAT** the user sees or experiences, not HOW it's implemented. Technical decisions (libraries, file paths, wrapper components, endpoint names, regex syntax) belong in `TECH_PLAN.md`. The tech-plan step reconciles PRD and TECH_PLAN at the end of its session — see `skills/belmont/_partials/plan-separation.md` for the boundary rules.

## Priority Levels

| Priority | Severity | Meaning                               |
|----------|----------|---------------------------------------|
| P0       | CRITICAL | Must be done first, blocks other work |
| P1       | HIGH     | Core functionality                    |
| P2       | MEDIUM   | Important but not blocking            |
| P3       | LOW      | Nice to have                          |

## PROGRESS.md

**Single source of truth for all state.** Tracks task status, milestones, session history, and decisions. Milestone status is computed from tasks (no emoji on milestone headers). There is no separate `## Blockers` section or `## Status:` line -- blocked tasks use the `[!]` checkbox and overall status is computed.

### Task States

| Checkbox | State       | Meaning                                                |
|----------|-------------|--------------------------------------------------------|
| `[ ]`    | Todo        | Not yet started                                        |
| `[>]`    | In Progress | Currently being worked on                              |
| `[x]`    | Done        | Task finished, not yet verified                        |
| `[v]`    | Verified    | Task finished and verified                             |
| `[!]`    | Blocked     | Cannot proceed (missing info, Figma unavailable, etc.) |

### Example

```markdown
# Progress: Feature Name

## PRD Reference
.belmont/PRD.md

## Milestones

### M1: Foundation
- [v] P0-1: Set up authentication
- [v] P0-2: Database schema

### M2: Core Features
- [>] P1-1: Dashboard layout
- [ ] P1-2: User settings

## Session History
| Session | Date/Time           | Context Used    | Milestones Completed |
|---------|---------------------|-----------------|----------------------|
| 1       | 2026-02-05 10:00:00 | PRD + TECH_PLAN | M1                   |

## Decisions Log
[Numbered list of key decisions with rationale]
```

### Master PROGRESS.md

The master PROGRESS.md (at `.belmont/PROGRESS.md` in multi-feature projects) contains a features table with these columns:

| Feature | Priority | Dependencies | Status | Milestones | Tasks |
|---------|----------|--------------|--------|------------|-------|

Feature-level status is computed from task states. There is no separate status line.

### Master PRD.md and TECH_PLAN.md

- **Master PRD.md** -- Living global document covering vision, constraints, and cross-cutting decisions. No features table (that lives in master PROGRESS.md).
- **Master TECH_PLAN.md** -- Living global document for cross-cutting architecture decisions.
