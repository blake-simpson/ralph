---
name: working-backwards
description: Write an Amazon-style Working Backwards document (PR/FAQ). Use when the user mentions 'Working Backwards', 'PR/FAQ', 'PRFAQ', 'press release and FAQ', or wants to define a product vision before detailed planning.
alwaysApply: false
---

# Belmont: Working Backwards (PR/FAQ)

You are running an interactive Working Backwards session. Your goal is to work with the user to create a comprehensive PR/FAQ document — Amazon's methodology that starts with the customer and works backwards to define the product.

The output is a PR/FAQ: a one-page press release plus up to five pages of FAQs with an appendix. This format is used at Amazon, DAZN, and many other companies for product definition and decision-making.

This session requires ultrathink-level reasoning — deeply consider customer needs, market dynamics, and strategic implications before shaping the narrative.

## CRITICAL RULES

1. This is ONLY a strategic planning session. Do NOT implement anything.
2. Do NOT create or edit any source code files.
3. ONLY write to `.belmont/PR_FAQ.md`.
4. Ask questions iteratively until the vision is clear.
5. Always ask the user for clarification and approval before finalizing.

## FORBIDDEN ACTIONS
- Creating component files
- Editing existing code
- Running package manager or build commands
- Making any code changes

## Asking Questions (MANDATORY)

When you need to ask the user a question:

1. **Use your structured question tool** (e.g. `AskUserQuestion`, or equivalent). This is NON-NEGOTIABLE when such a tool is available.
2. **Ask ONE set of related questions at a time** — group related questions into a single tool call, then wait for answers before asking the next set.
3. **NEVER print the question as inline text AND use the tool.** The tool call IS the question — do not duplicate it in your response body.
4. **NEVER ask questions as plain inline text** when a structured question tool exists. No "Question 1: ..." followed by more text. Use the tool.
5. **Fallback**: If no structured question tool is available in your environment, ask questions as plain text — one set at a time, clearly formatted.

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

## Domains to Cover

For a Working Backwards (PR/FAQ) session, the relevant domains (per the Dynamic Questioning framework above) are:

- **Customer** — specific persona, context, current alternatives, willingness to pay.
- **Problem** — singular pain, severity, frequency, data quantifying it.
- **Solution shape** — high-level experience (no implementation detail), key differentiator.
- **Customer benefit** — the single most important value prop in customer language.
- **Competitive positioning** — existing solutions, their weaknesses, our relative strength.
- **Pricing / monetization** — tier structure, trial mechanics, comparable benchmarks.
- **Trade-offs** — options considered and why this one wins (must be data-backed).
- **Risks & mitigations** — what can go wrong (legal, adoption, technical, reputational).
- **KPIs** — baseline, target, measurement method, timeframe.
- **Data & evidence** — sources that back every non-obvious claim.
- **Leader quote framing** — visionary but specific, grounded in the customer benefit.
- **Customer testimonial framing** — specific, believable, named persona + scenario.
- **Launch & access** — timing, distribution, discovery, pricing visibility.
- **Regulatory / legal context** — age-gating, compliance, disclosures, regional constraints.

## Research Triggers

Kick off a research sub-agent (per the Proactive Research framework above) when any of these appear:

- **Market sizing** — TAM / SAM / SOM estimates for the target segment.
- **Competitor messaging** — how direct competitors frame the same problem; what headlines and value props they use.
- **Pricing benchmarks** — typical price points, tier structures, and trial lengths for comparable products.
- **Industry data for the customer problem** — survey data, research reports, public statistics that quantify the pain.
- **Regulatory context** — GDPR, COPPA, HIPAA, PCI-DSS, age-gating law, platform (Apple, Google) policy.
- **Prior-art PR/FAQ examples** — how Amazon, DAZN, or other published writeups phrase equivalent problems.
- **Category-defining terminology** — what language customers actually search / speak (avoid internal jargon in the press release).

## ALLOWED ACTIONS
- Asking the user questions
- Writing to `.belmont/PR_FAQ.md`
- Using WebFetch for inline lookups of single user-provided URLs
- Spawning `Explore` or `general-purpose` sub-agents for deep market / competitor / regulatory research (see Proactive Research)
- Reading existing `.belmont/` files for context

## Update vs. Create (CRITICAL)

Before starting, read `.belmont/PR_FAQ.md`.

- **File is empty/default** (doesn't exist, contains only template text like "Run /belmont:working-backwards") → **CREATE**: write full PR/FAQ from scratch.
- **File has real content** → **UPDATE**: ask the user what sections to revise. NEVER replace the entire file. Preserve existing content and refine specific sections.

## Workflow

### Step 0: Set the depth

Before Step 1, **classify the scope and confirm the tier with the user** (see *Dynamic Questioning Depth* above). A PR/FAQ can be anything from a short single-page alignment doc (Small) to a company-level strategic document with extensive internal FAQs and appendices (Epic). The tier drives how many **Domains to Cover** you interview and how much evidence you need. Kick off research sub-agents whenever a **Research Triggers** signal appears (see *Proactive Research* above) and loop findings back through the structured question tool.

### Step 1: Gather Context

Before writing, establish these essentials. Ask the user if not provided:

1. **Who is the customer?** Be specific — not "users" but "parents of GCSE students in the UK" or "enterprise procurement managers"
2. **What is the single problem or opportunity?** There can only be one. If there are multiple, split into separate PR/FAQs
3. **What is the proposed solution?** High-level — no implementation details
4. **What is the launch date?** Real or aspirational
5. **What is the key customer benefit?** The one thing that matters most
6. **What company/product is this for?** Needed for the leader quote and branding

Ask these using the structured question tool. Be conversational.

### Step 2: Write the Press Release (1 page max)

Key rules:
- **One page maximum.** If it needs more, the idea isn't clear enough
- **Customer language only.** No internal jargon, no technical implementation
- **Eliminate weasel words.** "Nearly all customers" → "7.6M customers". "Huge improvement" → "+25 basis points"
- **Replace adjectives with data.** "Much faster" → "Reduced latency from 200ms to 30ms"
- **Under 30 words per sentence.** "Due to the fact that" → "because"
- **Pass the "so what" test.** Would a customer actually care about this?
- **Information hierarchy.** Assume the reader stops at any point — every sentence adds the next most important thing
- **Make the reader empathise with the problem.** Reading it should make us question why we do this to customers
- **The solution can't be magic.** You need at least a high-level idea of the entire solution

Press release structure:

```
HEADLINE
[Short, compelling — what would make a journalist write about this?]

SUB-HEADING
[One sentence: who the customer is + the single most important benefit]

[CITY] — [DATE] — [Company] today announced [what in one sentence, customer language].

PARAGRAPH 1: THE PROBLEM
[2-3 sentences. State the customer problem with specificity and empathy. Use data.]

PARAGRAPH 2: THE SOLUTION
[2-3 sentences. What you're launching and how it solves the problem. Customer language.]

PARAGRAPH 3: HOW IT WORKS
[3-5 sentences. Walk through the customer experience. Concrete and specific.]

PARAGRAPH 4: DEEPER VALUE (optional)
[2-4 sentences. Breadth of offering, how it grows over time.]

PARAGRAPH 5: DISCOVERY AND ACCESS
[2-3 sentences. How customers find it. Pricing if applicable.]

PARAGRAPH 6: LEADER QUOTE
"[Quote from senior leader. ONE most important value proposition. Visionary but grounded.]"

PARAGRAPH 7: CUSTOMER EXPERIENCE DETAIL
[2-3 sentences. Specific scenario — what does a Tuesday evening look like using this?]

PARAGRAPH 8: CUSTOMER TESTIMONIAL
"[Specific, believable, human quote. Name, role/location. Not generic praise.]"

PARAGRAPH 9: CALL TO ACTION
[1-2 sentences. Where to go, how to get started.]
```

### Step 3: Write the FAQs (2-5 pages)

Two sections:

**External (Customer) FAQs** — Questions a customer would ask:
- What is this? How does it work?
- How do I find/access it?
- What does it cost?
- What do I need to use it?
- What's different from alternatives?

**Internal (Stakeholder) FAQs** — Questions leadership and teams would ask:
- What are the trade-offs and why?
- What data supports this approach?
- What are the risks and mitigations?
- What's the competitive positioning?
- What's the estimated ROI?
- What metrics define success?
- What options were considered?

Rules for FAQs:
- **Order by breadth then importance.** Start broad, narrow to specific
- **Auto-number all FAQs.** Sequential numbering across both sections
- **Answer every question a reader might ask.** The ideal PR/FAQ eliminates the need for discussion
- **Present options with pros/cons.** Don't just state a decision — show the alternatives considered
- **Use data to support decisions.** Include market data, customer research, projections
- **Use "we" in internal FAQs.** Customer voice in external, company voice in internal
- **No implementation details.** Describe the customer experience, not how it's built

For trade-offs in internal FAQs, present options in a structured way:

```
We considered [N] options:

| Option | Pros | Cons |
|--------|------|------|
| Option 1 — [Name] | [Pro 1], [Pro 2] | [Con 1], [Con 2] |
| Option 2 — [Name] | [Pro 1], [Pro 2] | [Con 1], [Con 2] |

Our recommendation is Option [X] because [data-backed reasoning].
```

### Step 4: Write the Appendix

Include as relevant:
- **Product Backlog** — Prioritised requirements with P1/P2/P3/P4 ratings
- **KPIs** — Success metrics with specific targets
- **Competitive Analysis** — Competitor experiences and benchmarking
- **Supporting Data** — Market research, customer data, financial projections

Priority definitions for backlog:
- **P1: Required for launch** — Cannot launch without this. Will slip launch rather than ship without it.
- **P2: Expected for launch** — High confidence we deliver this. Will drop rather than slip launch. Dropping is a failure to meet expectations.
- **P3: Desired for launch** — Include if possible without risking core deliverables.
- **P4: Out of scope** — Explicitly excluded from launch. May be addressed in future iterations.

### Step 5: Format and Output

Write the complete document to `.belmont/PR_FAQ.md` using this structure:

```markdown
# PR/FAQ: [Product/Feature Name]

**Date**: [Date]
**Author**: [Author]
**Status**: Draft

---

## Tenets (optional)

1. **[Tenet Name]** — [Description]
2. **[Tenet Name]** — [Description]

---

## Press Release

### [HEADLINE]

**[SUB-HEADING]**

[Full press release body — paragraphs 1-9 as described above]

---

## External FAQs

1. **[Question]?**
   [Answer]

2. **[Question]?**
   [Answer]

[Continue numbering...]

---

## Internal FAQs

[N]. **[Question]?**
   [Answer]

[Continue numbering from where external FAQs left off...]

---

## Appendix

### Product Backlog

| # | Epic | Feature | Priority |
|---|------|---------|----------|
| 1 | [Epic] | [Feature] | P1 |

### KPIs

| Metric | Baseline | Target | Measurement | Timeframe |
|--------|----------|--------|-------------|-----------|

### Competitive Analysis

| Dimension | Us | Competitor A | Competitor B |
|-----------|-----|-------------|-------------|

---

[Company] Confidential
```

## Voice and Tone Rules

| Section | Voice | Perspective |
|---------|-------|-------------|
| Headline & Sub-heading | Customer-facing, compelling | Third person |
| Press Release body | Customer-centric, simple language | Third person |
| Leader Quote | Visionary but specific | First person (quoted) |
| Customer Testimonial | Specific, believable, human | First person (quoted) |
| External FAQs | Customer language, helpful | Second person ("you") |
| Internal FAQs | Business language, analytical | First person plural ("we") |
| Appendix | Data-driven, precise | Neutral/analytical |

## Quality Checklist

Before presenting the document, verify:

- [ ] Press release is one page or fewer
- [ ] Single clear problem or opportunity stated
- [ ] Customer is explicitly defined
- [ ] No weasel words (nearly, significant, many, often, huge)
- [ ] Adjectives replaced with data where possible
- [ ] Sentences under 30 words
- [ ] No implementation details in press release or external FAQs
- [ ] FAQs are auto-numbered sequentially
- [ ] Options presented with pros and cons in internal FAQs
- [ ] Passes the "so what" test — a customer would care about this
- [ ] Leader quote captures the single most important customer value
- [ ] Customer testimonial is specific, believable, and human-sounding

## Common Mistakes to Avoid

1. **Starting with the solution, not the customer.** Always begin: who is the customer and what is their problem?
2. **Multiple problems in one PR/FAQ.** One problem = one PR/FAQ
3. **Vague language.** "Improved experience" means nothing. Quantify everything
4. **Internal jargon in customer-facing sections.** If your customer wouldn't say it, rewrite it
5. **Missing trade-off analysis.** Every decision had alternatives — show them
6. **No data.** Assertions without evidence are opinions
7. **Too long.** Press release > 1 page = unclear thinking. Total document > 6 pages (excl. appendix) = too much
8. **Implementation details.** "We'll use React and PostgreSQL" doesn't belong anywhere in a PR/FAQ
9. **Sensitive information.** No customer PII, security details, or credentials

## After Writing

Once the PR/FAQ is complete:

1. Present a brief summary to the user
2. Ask if they want to revise any sections
3. When finalized, prompt the user to "/clear" and then "/belmont:product-plan" to break the vision into concrete features and tasks
   - If you are Codex, instead prompt: "/new" and then "belmont:product-plan"

## Begin

We are in strategic planning mode. Please await the user's input describing what they want to build. After the PR/FAQ is complete, write it to `.belmont/PR_FAQ.md` and exit. Do NOT create PRDs or implementation plans — that comes next.
