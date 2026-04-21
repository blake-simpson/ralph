# MILESTONE.md Template

Use this template when creating `{base}/MILESTONE.md` in Step 2 of the implement skill.

The MILESTONE file is a **coordination document**: it names the active tasks and points sub-agents at the canonical PRD and TECH_PLAN. Do NOT copy PRD/TECH_PLAN content into it — the pointers are enough, and duplicating content wastes context across every sub-agent invocation.

Fill in the `## Orchestrator Context` section using information from PROGRESS.md and the user's invocation context:

```markdown
# Milestone: [ID] — [Name]

## Status
- **Milestone**: [e.g., M2: Core Features]
- **Git Baseline**: [Run `git rev-parse HEAD` and record the SHA here — this is used by verification agents to distinguish new code from pre-existing code]
- **Created**: [timestamp]
- **Tasks**:
  - [ ] [Task ID]: [Task Name]
  - [ ] [Task ID]: [Task Name]
  ...

## Orchestrator Context

### Current Milestone
[Milestone ID and name, with the full list of incomplete tasks in this milestone]

### Active Task IDs
[Comma-separated list of the incomplete task IDs in this milestone, e.g. `P0-1, P0-2, P1-3`. Sub-agents look up each task's full definition (description, solution, acceptance criteria, Figma URLs, notes) in {base}/PRD.md.]

### File Paths
- **PRD**: {base}/PRD.md — authoritative task definitions, acceptance criteria, Figma URLs
- **TECH_PLAN**: {base}/TECH_PLAN.md — technical specs (if present)
- **Master TECH_PLAN**: .belmont/TECH_PLAN.md — cross-cutting architecture (if present)
- **PROGRESS**: {base}/PROGRESS.md
- **Feature Notes**: {base}/NOTES.md
- **Global Notes**: .belmont/NOTES.md

### Scope Boundaries
- **In Scope**: Only the task IDs listed above in this milestone
- **Out of Scope**: See the "Out of Scope" section of {base}/PRD.md — nothing outside the listed task IDs
- **Milestone Boundary**: Do NOT implement tasks from other milestones

### Learnings from Previous Sessions
[If `.belmont/NOTES.md` exists, copy its contents here under "#### Global Notes".]
[If `{base}/NOTES.md` exists, copy its contents here under "#### Feature Notes".]
[If neither exists, write "No previous learnings found."]

### Additional User Instructions
[If the user provided extra context or instructions when invoking this skill, copy it here verbatim. Otherwise write "None."]

## Codebase Analysis
[Written by codebase-agent — stack, patterns, conventions, related code, utilities]

## Design Specifications
[Written by design-agent — tokens, component specs, layout code, accessibility]

## Implementation Log
[Written by implementation-agent — per-task status, files changed, commits, issues]
```

**IMPORTANT**: The `## Orchestrator Context` section is the **coordination hub** — it names the active tasks and points sub-agents at the PRD and TECH_PLAN. Sub-agents read the PRD and TECH_PLAN directly from the paths in `### File Paths` for full task definitions, acceptance criteria, Figma URLs, and technical specs. Do NOT copy PRD/TECH_PLAN content into this section.

**The three sub-agent-written sections (`## Codebase Analysis`, `## Design Specifications`, `## Implementation Log`) remain the source of truth for downstream agents** — those ARE written into MILESTONE and ARE read by Phase 3 (implementation-agent). Only the PRD/TECH_PLAN content is externalised; the sub-agent hand-off data stays inside MILESTONE.

The three section headings should be present but empty — each agent will fill in its section.
