# Implement: Important Rules

Keep these rules in mind throughout the implement workflow.

1. **Create the MILESTONE file first** - Write it with full orchestrator context (PRD + TECH_PLAN) before spawning any agent
2. **MILESTONE is the single source of truth** - Sub-agents read ONLY the MILESTONE file. Everything they need must be in it.
3. **Minimal agent prompts** - Agents read from the MILESTONE file, not from your prompt
4. **All tasks, all phases** - Pass every task in the milestone through every phase. Exactly 3 sub-agents per milestone.
5. **Parallel research, then implement** - Codebase + Design run simultaneously, then Implementation runs after both complete
6. **Dispatch to sub-agents** - Spawn a sub-agent for each phase. Do NOT do the phase work inline.
7. **Read the Implementation Log** - After Phase 3 completes, read the `## Implementation Log` from the MILESTONE file to know what was done
8. **Update PROGRESS.md** - Keep PROGRESS.md current with task state changes. Add follow-up `[ ]` tasks for any out-of-scope issues reported by the implementation agent.
9. **Don't skip phases** - Even if no Figma design, still run the design phase (it handles the no-design case)
10. **Clean up the MILESTONE file** - Archive it after the milestone is complete
11. **Quality over speed** - Ensure build, tests, and self-checks pass before marking tasks done
12. **Stay in scope** - Never implement anything not traceable to a PRD task in the current milestone
