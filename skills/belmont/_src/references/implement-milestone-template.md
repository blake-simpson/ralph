# MILESTONE.md Template

Use this template when creating `{base}/MILESTONE.md` in Step 2 of the implement skill.

Fill in the `## Orchestrator Context` section using information from the PRD, PROGRESS, and TECH_PLAN:

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

### Relevant PRD Context
[Extract from PRD.md: the Overview, Problem Statement, Technical Approach, and Out of Scope sections. Also extract the FULL task definitions for every incomplete task in this milestone — copy them verbatim from the PRD including all fields (description, solution, notes, verification, Figma URLs, etc.)]

### Relevant Technical Context
[Extract from TECH_PLAN.md: file structures, component specifications, TypeScript interfaces, implementation guidelines, and architecture decisions relevant to this milestone's tasks. Include code patterns and API specs. If no TECH_PLAN exists, write "No TECH_PLAN.md found."]

### File Paths
- **PRD**: {base}/PRD.md
- **PROGRESS**: {base}/PROGRESS.md
- **Feature Notes**: {base}/NOTES.md
- **Global Notes**: .belmont/NOTES.md

### Scope Boundaries
- **In Scope**: Only tasks listed above in this milestone
- **Out of Scope**: [Copy the PRD's "Out of Scope" section verbatim]
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

**IMPORTANT**: The `## Orchestrator Context` section is the **single source of truth** for all sub-agents. It must contain ALL information they need — task definitions verbatim from the PRD, relevant TECH_PLAN specs, scope boundaries, and learnings from previous sessions. Sub-agents read ONLY the MILESTONE file, so anything not in it will be invisible to them. Copy task definitions verbatim — don't summarize.

The three section headings (`## Codebase Analysis`, `## Design Specifications`, `## Implementation Log`) should be present but empty — each agent will fill in its section.
