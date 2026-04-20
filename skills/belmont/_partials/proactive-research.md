## Proactive Research (MANDATORY on trigger)

You MUST proactively use web research when the plan depends on current, external knowledge. Relying solely on training data produces stale or generic plans. The user is better served by a plan informed by up-to-date sources.

### Step 1 — Watch for triggers

Kick off research (silently, alongside questioning) whenever any signal in the **Research Triggers** checklist (defined in a section of this skill below) appears in the brief or surfaces during the interview.

If a trigger fires, you do **not** need to ask the user "should I research this?" — just launch the research. You will surface findings back to the user for a decision (Step 4).

### Step 2 — Delegate deep research to a sub-agent

Deep research MUST be delegated to an `Explore` or `general-purpose` sub-agent. This keeps the planner's context window clean for the interview and allows heavier multi-source investigation.

Give the sub-agent a tight brief:

- **Scope**: exactly what question to answer (e.g. "compare Stripe Billing vs. Paddle vs. Lemon Squeezy for EU B2B SaaS with tax compliance").
- **Recency filter**: prefer sources from 2025 or later. Explicitly ask the sub-agent to flag anything older.
- **Source preference**: official docs, release notes, RFCs, and vendor changelogs over blog posts. Primary sources over secondary.
- **Output shape**: a short summary + 2–4 candidate options, each with pros, cons, current version, maintenance signal, and a primary source URL.
- **Length cap**: ask for a ≤300-word report so findings slot cleanly into the plan.

Inline `WebFetch` is acceptable **only** for single URLs the user provided or that you need to fetch verbatim (e.g. a specific docs page). Do not loop inline fetches — delegate instead.

### Step 3 — Flag stale sources

If any source the sub-agent cites is older than ~12 months, mark it `(potentially stale — last updated YYYY-MM)` in the plan. Prefer more recent sources when available.

### Step 4 — Loop findings back through the user

After a research pass lands, summarize it back to the user via your structured question tool with concrete options. Research feeds **more** questions, not fewer — the user picks the direction:

> "Research found three viable options for [X]: A (pros/cons), B (pros/cons), C (pros/cons). My default recommendation is B because [reason]. Which way do you want to go?"

Do not finalize the plan until the user has chosen. If the user picks "Other", incorporate their choice and continue.

### Step 5 — Embed findings in the plan (not just chat)

Research outputs must land in the plan file itself so downstream agents can see them:

- **PRD**: add a `### Research Notes` subsection inside `## Technical Context` (or `## Clarifications`) with a bulleted list of findings, each with its source URL and a one-sentence summary.
- **TECH_PLAN**: populate the `Alternatives Considered` column of the `## Decision Log` from research, and add a `## References` section at the bottom listing all cited URLs with a one-sentence summary each.
- **PR_FAQ**: put cited data in the `## Appendix` → `### Supporting Data` section. Every claim in the press release backed by research should cite its source in the appendix.

Never leave research findings only in the chat — the plan must stand alone.

### What NOT to research

- **Internal codebase patterns** — read the code instead.
- **Settled decisions** — don't re-open questions the user has already answered.
- **Trivia** that doesn't affect the plan.
- **Well-known facts** inside your training cutoff where recency doesn't matter (e.g. "what is JSON").

When in doubt, ask the user whether research would be useful before kicking off a sub-agent.
