# Product Plan: PRD.md Format

Use this template when writing `{base}/PRD.md` (i.e. `.belmont/features/<slug>/PRD.md`).

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
[Detailed description of the sub-task — what problem this solves and what the user should experience when it's done]

**Solution**:
[Describe WHAT the task produces from the user's perspective — screens, behaviors, invariants, acceptance conditions, content/copy. Do NOT describe HOW (file paths, components, wrappers, imports, regex syntax, endpoint names) — implementation is the tech-plan's responsibility. If you need to reference a Figma node or external source, do so by id / URL, not by implementation path.]

**Notes**:
[Notes needed by sub agents. Figma nodes, key product decisions, open questions flagged for the tech-plan step. Avoid technical idioms.]

**Verification**:
[List of steps to verify the task is complete — user-observable outcomes and acceptance criteria. Leave build/lint/typecheck to the standard verify pipeline.]
```
