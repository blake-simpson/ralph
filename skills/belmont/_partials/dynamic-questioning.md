## Dynamic Questioning Depth (MANDATORY)

Your question depth MUST scale with the complexity of the work. A one-line tweak needs 1–2 questions. A multi-surface feature with new user types, data models, and integrations needs 10+ rounds across every relevant domain. **Never stop purely because you've hit "a few" rounds.** Stop when the scope is fully covered.

### Step 1 — Classify the scope (before the first question round)

Silently assess the task across these five axes:

- **Surface count** — how many pages / screens / flows / endpoints are involved?
- **New vs. extension** — greenfield concept or extension of existing behaviour?
- **New user types / roles** — does it introduce new personas, permission tiers, or audiences?
- **External systems** — new integrations, APIs, data sources, providers, side effects?
- **Novelty** — does this break new ground for the product (new domain, UX pattern, business model, tech stack)?

Map to a **tier**:

| Tier     | Signal                                                    | Target rounds | Behaviour                                                         |
|----------|-----------------------------------------------------------|---------------|-------------------------------------------------------------------|
| Trivial  | 1 axis, pure extension (e.g. copy tweak, colour change)   | 1             | One confirming round, finalize                                    |
| Small    | 2–3 axes, mostly extension                                | 2–3           | Cover core concerns of the feature only                           |
| Medium   | 3–5 axes, one novel element                               | 4–6           | Add edge cases + UX/state coverage                                |
| Large    | 5 axes OR multiple novel elements                         | 7–10          | Exhaustive domain coverage, likely triggers research              |
| Epic     | Cross-cutting, touches every surface, new product line    | 10+           | Escalate: consider decomposing into sub-features                  |

### Step 2 — Confirm the tier with the user (before any domain questions)

Use your structured question tool to announce the classification and give the user a chance to correct it. Example:

> "I'm treating this as a **Large** feature — it introduces a new user type, touches 4 surfaces, and depends on a new payments provider. I'll ask ~8 rounds across user flows, edge cases, content, accessibility, analytics, privacy, notifications, and monetization. Does that match your expectation, or should I scope it tighter / wider?"

Offer the options: `Confirm tier`, `Downgrade (I want it smaller)`, `Upgrade (it's bigger than that)`. Respect the user's correction — their framing wins.

### Step 3 — Cover every relevant domain

Walk the **Domains to Cover** checklist for this skill (defined in a section of this skill below — each skill defines its own product/tech/PR-FAQ-specific list). For each **relevant** domain, run at least one `AskUserQuestion` round. Group tightly-related sub-questions into a single call (per the `user-questions.md` rules).

**Skipping domains**: a domain may be skipped only if it is *genuinely irrelevant* to the task. When skipping, record it in the plan's `## Clarifications` section as `- [domain]: skipped — not applicable because [reason]`. Do not skip a domain merely because it feels tedious.

### Step 4 — Re-tier dynamically

Upgrade the tier mid-interview if:

- A user answer surfaces a subsystem you didn't know about.
- Research uncovers a convention, regulation, or constraint you hadn't accounted for.
- The user describes more scope than the initial brief implied.

Downgrade only if the user explicitly scopes down. Announce any re-tier to the user the same way you announced the initial tier.

### Step 5 — Exit criteria

Finalize the plan ONLY when **all** of these are true:

1. Every relevant domain has had at least one question round (or is explicitly marked skipped in `## Clarifications`).
2. The user has explicitly confirmed they have no more open questions — ask this with the structured question tool, don't assume.
3. All answers are captured in the plan's `## Clarifications` section verbatim enough that an implementation agent can trace every decision back to a user answer.
4. Any research findings have been surfaced to the user and incorporated (see Proactive Research).

If any of these fail, keep asking. The round count is an indicator, not a limit.
