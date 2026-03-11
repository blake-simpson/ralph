# Progress: [Feature Name]

## Status: 🔴 Not Started

## PRD Reference
.belmont/PRD.md

## Milestones

### ⬜ M1: [Milestone Name]
- [ ] Task 1
- [ ] Task 2

### ⬜ M2: [Milestone Name] (depends: M1)
- [ ] Task 1

### ⬜ M3: [Milestone Name] (depends: M1)
- [ ] Task 1

### ⬜ M4: [Milestone Name] (depends: M2, M3)
- [ ] Task 1

> **Dependency syntax**: Add `(depends: M1)` or `(depends: M1, M3)` after the milestone name to declare dependencies. When dependencies are present, `belmont auto` will run independent milestones in parallel via git worktrees. If no milestones have `(depends: ...)`, they run sequentially (default behavior).

## Session History
| Session | Date/Time           | Context Used | Milestones Completed |
|---------|---------------------|-----------------|----------------------|

## Decisions Log
[Numbered list of key decisions with rationale]

## Blockers
[Any blocking issues]
