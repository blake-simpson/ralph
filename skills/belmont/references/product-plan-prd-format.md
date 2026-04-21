# Product Plan: PRD Format

Use this when writing the feature PRD at the end of the planning session.

Write the PRD to `{base}/PRD.md` (i.e. `.belmont/features/<slug>/PRD.md`) with this structure:

```markdown
# PRD: [Feature Name]

## Overview
[1-2 sentence description]

## Problem Statement
[What problem does this solve?]

## Success Criteria (Definition of Done)
- [ ] Criterion 1
- [ ] Criterion 2
- [ ] Criterion 3

## Acceptance Criteria (BDD)

### Scenario: [Scenario Name]
Given [context]
And [more context]
When [action]
Then [expected result]
And [additional assertions]

## Out of Scope
[What this feature explicitly does NOT include]

## Open Questions
[Questions that need answers before implementation]

## Clarifications
[Answers to open questions, added during the planning phase]

## Technical Context (for implementation agents)
[Add all context needed for follow up agents (Figma URLs, technical decisions from interview, edge cases, conflicts, etc.)]

## Tasks
[List all sub-tasks required to complete the feature]
[Provide all information needed for the implementation agents to understand their isolated task]

### P0-1: [Task Name]
**Severity**: CRITICAL

**Task Description**:
[Detailed description of the sub-task]

**Solution**:
[Detailed description of the solution to the sub-task]

**Notes**:
[Notes needed by sub agents. Figma nodes, key choices, etc.]

**Verification**:
[List of steps to verify the task is complete]
```
