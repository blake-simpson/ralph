## Step 5: Escalate

If 3 iterations were exhausted without a fix:

```
Debug limit reached (3 iterations). This issue may need the full pipeline.
Recommendation: /belmont:implement or /belmont:next with a follow-up task.

Summary of attempts:
- Iteration 1: [what was tried, result]
- Iteration 2: [what was tried, result]
- Iteration 3: [what was tried, result]
```

Proceed to Step 6 (Cleanup).

## Step 6: Cleanup

**DEBUG.md is ephemeral** — delete it regardless of outcome.

1. **Delete DEBUG.md**: `rm {base}/DEBUG.md`
   - On FIXED: delete after commit is confirmed
   - On escalation: delete after reporting
   - On user stop: delete after reporting
   - On unrecoverable REGRESSION: delete after revert

2. **Tear down team (Approach A only)**:
   If you created a team:
   - Send `shutdown_request` via `SendMessage` to each teammate still active
   - Wait for shutdown confirmations
   - Call `TeamDelete` to remove team resources

   Skip this if you used Approach B or C.

### Commit Planning File Changes

After completing all updates to `.belmont/` planning files, commit them:

1. **Check if `.belmont/` is git-ignored** — run:
   ```bash
   git check-ignore -q .belmont/ 2>/dev/null
   ```
   If exit code is 0, `.belmont/` is ignored — skip this section entirely.

2. **Check for changes** — run:
   ```bash
   git status --porcelain .belmont/
   ```
   If there is no output, nothing to commit — skip the rest.

3. **Stage and commit** — stage only `.belmont/` files and commit:
   ```bash
   git add .belmont/ && git commit -m "belmont: update planning files after debug fix"
   ```

## Step 7: Final Actions

Once done, prompt the user to "/clear" and then "/belmont:status", "/belmont:verify", or "/belmont:next".
   - If you are Codex, instead prompt: "/new" and then "belmont:status", "belmont:verify", or "belmont:next"

## Scope Guardrails

These are hard rules. Do not break them:

1. **Fix only the reported issue** — no refactoring, no feature additions, no "improvements"
2. **DEBUG.md is the shared context file** — agents read from and write to it
3. **Dispatch to agents** — do NOT investigate, fix, or verify yourself (unless Approach C fallback)
4. **No PRD task creation** — if you discover new issues, mention them in the report but don't create tasks
5. **Max 3 iterations** — if you can't fix it in 3 tries, escalate
6. **Revert on regression** — if a fix makes things worse, undo it immediately
7. **Single commit** — one atomic commit for the fix, only after user confirms
8. **Minimal changes** — touch the fewest files possible to fix the issue
9. **Cleanup always** — delete DEBUG.md when the session ends, regardless of outcome
