# Verify: Five Whys Root Cause Analysis

Use this when Critical or Warning issues were found and you need to capture root causes and prevention rules for the implementation agent to learn from.

**Only run this step if Critical or Warning issues were found.** Skip entirely if only Polish/Suggestion items exist.

For each Critical or Warning issue, perform a root cause analysis using Amazon's Five Whys framework:

1. **Ask "Why?" up to five times**, tracing from the symptom to the root cause:
   - Why 1: Immediate cause (what went wrong)
   - Why 2: Contributing factor (why the immediate cause happened)
   - Why 3: Process gap (what process failure allowed it)
   - Why 4: Systemic reason (why the process gap exists)
   - Why 5: Root pattern (the fundamental behavior to change)
   - Stop early if the root cause is reached before the fifth why.

2. **Distill a prevention rule** — one concise, actionable statement the implementation agent can follow. Example: "Always use semantic design tokens instead of hex colors because the design system requires theme support."

3. **Group similar issues** — if multiple issues share the same root cause, combine into one entry.

4. **Write to NOTES.md** — Append to `{base}/NOTES.md` under a `## Root Cause Patterns` section (create section if absent). Format each entry as:

```markdown
### [YYYY-MM-DD] Pattern: <short descriptive name>
**Issue**: <one-line description of what was found>
**Root Cause**: <the deepest "why" — the fundamental pattern to change>
**Prevention**: <actionable rule for the implementation agent>
**Source**: <milestone ID / task ID where the issue was found>
```

Keep entries scannable — the implementation agent reads these before every task. Each entry should be understood in under 10 seconds.
