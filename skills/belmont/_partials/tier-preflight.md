### Model Tier Preflight (non-Claude CLIs)

Non-Claude CLIs (Codex, Gemini, Cursor, Copilot) run the entire skill in a single top-level session at whichever model the session was started with — there's no sub-agent dispatch to override mid-session. Before doing any heavy work, compare the **required tier** for the current skill to the **session's current model** and surface a warning if they diverge. Do NOT block execution; let the user decide.

**Workflow at start-of-skill (non-Claude only)**:

1. **Read** `.belmont/features/<slug>/models.yaml`. If absent, skip this preflight (defaults apply).
2. **Determine the required tier for this skill**:
   - `implement` → `tiers.implementation`
   - `verify` → `tiers.verification`
   - `code-review` (if applicable) → `tiers.code-review`
   - others → skip preflight unless the skill specifies its own tier.
3. **Map the required tier to a model ID for the current CLI** using `tier-registry.md`.
4. **Compare to the session's current model**:
   - Codex: run `/model` or check session settings.
   - Gemini: check `/model`.
   - Cursor: check `/model`.
   - Copilot: check `/model`.
5. **If they diverge**, print this warning block before doing any further work:

   ```
   ⚠ Model tier mismatch
   models.yaml says this phase should run at <tier> (<expected-model-id>).
   Your session is currently on <current-model-id>.
   To honor the tier, restart with: <cli> --model <expected-model-id>
   Continuing with the current model. Re-dispatching sub-agents with a
   different model is not supported on this CLI.
   ```

6. **Proceed with the skill**. The warning is informational; it never blocks execution.

**Why this is acceptable graceful degradation**: the user chose this CLI knowing it doesn't support per-agent dispatch. The warning gives them a one-command fix if they want tier adherence; otherwise the work proceeds at the session's model. Only Claude Code supports true per-agent overrides — see `dispatch-strategy.md` Model Tier Overrides for that path.
