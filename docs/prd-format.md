# PRD & Progress Format

## PRD.md

Tasks use structured markdown with priority levels:

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

### P0-1: Set up authentication ✅
**Severity**: CRITICAL
**Task Description**: Implement OAuth2 login flow
**Solution**: Use next-auth with Google provider
**Verification**: npm run test, manual login test

### P1-1: Create dashboard layout
**Severity**: HIGH
**Task Description**: Build the main dashboard page
**Figma**: https://figma.com/file/xxx/node-id=123
**Solution**: Responsive grid layout with sidebar
**Verification**: npm run build, visual comparison with Figma
```

## Task States

| Marker       | State     | Meaning                                                |
|--------------|-----------|--------------------------------------------------------|
| *(none)*     | Pending   | Not yet started                                        |
| `✅`          | Complete  | Task finished and verified                             |
| `🚫 BLOCKED` | Blocked   | Cannot proceed (missing info, Figma unavailable, etc.) |
| `🔵 FWLUP`   | Follow-up | Discovered during implementation or verification       |

## Priority Levels

| Priority | Severity | Meaning                               |
|----------|----------|---------------------------------------|
| P0       | CRITICAL | Must be done first, blocks other work |
| P1       | HIGH     | Core functionality                    |
| P2       | MEDIUM   | Important but not blocking            |
| P3       | LOW      | Nice to have                          |

## PROGRESS.md

Tracks milestones, session history, and blockers:

```markdown
# Progress: Feature Name

## Status: 🟡 In Progress

## PRD Reference
.belmont/PRD.md

## Milestones

### ✅ M1: Foundation
- [x] P0-1: Set up authentication
- [x] P0-2: Database schema

### ⬜ M2: Core Features
- [ ] P1-1: Dashboard layout
- [ ] P1-2: User settings

## Session History
| Session | Date/Time           | Context Used    | Milestones Completed |
|---------|---------------------|-----------------|----------------------|
| 1       | 2026-02-05 10:00:00 | PRD + TECH_PLAN | M1                   |

## Decisions Log
[Numbered list of key decisions with rationale]

## Blockers
[None currently]
```
