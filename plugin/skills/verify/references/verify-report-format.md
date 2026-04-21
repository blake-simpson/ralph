# Verify: Overall Status Rules and Report Template

Use this template to produce the final verify summary. Read this file once, at the end of Step 3, to decide the overall status and format the report.

## Determine Overall Verification Status

When deciding the overall status:
- If **only** Polish and/or Suggestion items were found (no Critical, no Warning): report status as **ALL PASSED**. All tasks are marked `[v]` (verified).
- If Critical or Warning items were found: report status as **ISSUES FOUND** or **CRITICAL ISSUES** as appropriate. Tasks with issues remain `[x]`, follow-up `[ ]` tasks are added.

## Report Summary Template

Output a combined summary:

```markdown
# Verification & Code Review Summary

## Overall Status
[ALL PASSED | ISSUES FOUND | CRITICAL ISSUES]

## Verification Results
- Acceptance Criteria: [X/Y passed]
- Visual Verification: [PASS/FAIL/N/A]
- i18n Check: [PASS/FAIL/N/A]
- Functional Tests: [PASS/FAIL]
- Lighthouse Audit: [PASS/WARNING/CRITICAL/N/A]

## Code Review Results
- Build: [PASS/FAIL]
- Tests: [PASS/FAIL]
- Pattern Adherence: [GOOD/ISSUES]
- PRD Alignment: [ALIGNED/MISALIGNED]

## Issues Found
- Critical: [count]
- Warnings: [count]
- Polish: [count] (recorded in NOTES.md, not blocking)
- Suggestions: [count]

## Follow-up Tasks Created
[List of new follow-up tasks added to PROGRESS.md]

## Recommendations
[Any overall recommendations for the project]
```
