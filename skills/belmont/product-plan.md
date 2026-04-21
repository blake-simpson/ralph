---
description: Interactive planning session - create PRD and PROGRESS files for a feature
alwaysApply: false
---

# Belmont: Product Plan

You are running an interactive planning session. You should not switch the agent to plan mode. Your goal is to work with the user to create a comprehensive PRD (Product Requirements Document) and PROGRESS tracking file.

This session requires ultrathink-level reasoning — deeply consider product edge cases, user needs, and architectural implications before proposing structure.

## CRITICAL RULES

1. This is ONLY a planning session. Do NOT implement anything.
2. Do NOT create or edit any source code files (no .tsx, .ts, .css, etc.).
3. ONLY write to files in `.belmont/` (PRD.md, PROGRESS.md, and feature directories).
4. Ask questions iteratively until the plan is 100% concrete.
5. Always ask the user for clarification and approval before finalizing.

## FORBIDDEN ACTIONS
- Creating component files
- Editing existing code
- Running package manager or build commands
- Making any code changes

## PRD ↔ TECH_PLAN Boundary

Belmont's planning workflow splits concerns across two documents. Keeping the boundary clean prevents drift — the most common failure mode is PRD and TECH_PLAN disagreeing after tech-plan refinements, which confuses the implementation agent.

### What belongs in the PRD (product surface)

- User goals, target audience, problem statement
- User flows and journeys (what the user does, step by step)
- Acceptance criteria and success criteria (measurable outcomes)
- Content, copy, and tone decisions
- UX behavior and product-level invariants (e.g. "hide the logo slot when no university matches"; "LSEGroup must NOT be matched as LSE")
- Out-of-scope statements
- Priority and scope
- Figma URLs / node IDs (by reference only — never the implementation path they render to)

### What belongs in the TECH_PLAN (implementation surface)

- File paths (`src/...`) and directory structure
- Component architecture (wrapper components, sub-components, composition patterns)
- Direct-usage vs wrapper decisions (e.g. "use `<UniversityLogo>` wrapper" or "use `<Image>` directly")
- Icon / library imports and specific import identifiers (e.g. `Play` vs `PlayCircle` from `lucide-react`)
- Regex *syntax* and implementation patterns (`/\blse\b/i` vs `/lse/i`)
- tRPC / REST endpoint names the implementation commits to
- State management choices, styling approach, data-fetching approach
- Design tokens extracted from Figma, file-level TypeScript interfaces

### Grey-zone rule

When a decision has both a product-visible invariant AND an implementation detail, split them:

- PRD keeps the **behavior**: "LSEGroup must not match as LSE."
- TECH_PLAN keeps the **implementation**: "Use `/(\blse\b|london school of economics)/i` — the `\b` word-boundary is what enforces the invariant."

If the user volunteers an implementation idiom during product-plan ("just use `<Image>` directly"), record it as an **open question for the tech-plan step** — do NOT commit to it in the PRD. Tech-plan may well decide otherwise, and baking it into the PRD creates drift.

## Milestone Sizing Rules

Milestones are the unit of execution for `belmont auto`. Each milestone runs in a single AI session inside an isolated worktree. Small milestones are load-bearing:

- Keep each within a single context window so the implementation agent can hold all relevant files, designs, and task detail at once.
- Keep verification tractable — the verify-agent and code-review-agent re-check the whole milestone; oversized milestones lead to shallow verification.
- Enable parallelism — independent small milestones can run concurrently in separate worktrees.

### Sizing targets

- **Target**: 3–5 tasks per milestone.
- **Soft ceiling**: 6. Going above should be rare and deliberate.
- A milestone is "too big" if any of:
  - More than ~5 tasks
  - Mixes unrelated domains (UI + backend + infra)
  - Requires loading multiple Figma files *and* a separate backend surface
  - Would force the agent to juggle context the verify-agent can't reasonably re-check in one pass

### Growing vs splitting

When new work is discovered during planning or review:

- **Default**: create a NEW milestone for the new work rather than inflating an existing one. Use `(depends: M<n>)` when the new milestone genuinely builds on another — only when there is a real file/API/data dependency, not just "related topic."
- **Favor parallelism**: if two clusters of new tasks don't share files or APIs, split them into separate milestones so `belmont auto` can run them in parallel worktrees.
- **Never** merge two small milestones just because they're topically similar — the cost of a too-large context is much higher than the cost of an extra milestone.

## Tech-plan's Back-update Contract

The tech-plan step is responsible for keeping the PRD and PROGRESS in sync with its own decisions. Implementation agents read the PRD — if it disagrees with TECH_PLAN, the implementation will be confused or wrong.

Rules for updating PRD/PROGRESS from the tech-plan session:

1. **Contradictions**: when a tech-plan decision contradicts PRD prose, correct the PRD in the same session. Don't leave two sources of truth.
2. **Refinements**: when a tech-plan decision disambiguates the PRD (e.g. "endpoint A or B" → "endpoint A"), update the PRD to commit to the resolved version. The orchestrator extracts context from the PRD — it must reflect the final decision.
3. **Leaked tech detail**: when the PRD contains technical prose that shouldn't be there (file paths, wrapper choices, icon imports, regex syntax), replace it with a behavior-only description or a short pointer: `See TECH_PLAN.md §<section>.`
4. **New Clarifications**: add product-facing decisions that crystallized during tech-plan (e.g. resolved ambiguities, confirmed invariants) into the PRD's `## Clarifications` section.
5. **PROGRESS dependency annotations**: ensure `(depends: ...)` annotations on milestone headings in PROGRESS.md match the TECH_PLAN's §Implementation Order. Mismatches cause auto-mode to serialize work that could run in parallel, or vice versa.
6. **Non-destructive**: always use Edit to modify specific sections. Never replace entire files. Preserve all existing content, task IDs, and completion status.

## Asking Questions (MANDATORY)

When you need to ask the user a question:

1. **Use your structured question tool** (e.g. `AskUserQuestion`, or equivalent). This is NON-NEGOTIABLE when such a tool is available.
2. **Ask ONE set of related questions at a time** — group related questions into a single tool call, then wait for answers before asking the next set.
3. **NEVER print the question as inline text AND use the tool.** The tool call IS the question — do not duplicate it in your response body.
4. **NEVER ask questions as plain inline text** when a structured question tool exists. No "Question 1: ..." followed by more text. Use the tool.
5. **Fallback**: If no structured question tool is available in your environment, ask questions as plain text — one set at a time, clearly formatted.

## Dynamic Questioning Depth (MANDATORY)

Your question depth must match the *shape* of the work, not a template. A small well-defined change may need one or two questions. A large feature with many domains and open questions needs many rounds — possibly dozens. **There is no round cap.** Keep asking until every relevant aspect has been considered, every ambiguity resolved, and the user has explicitly confirmed nothing is missing.

Depth is driven by two forces, not by a tier:

1. **Breadth** — how many of the *Domains to Cover* (defined below in this skill) are genuinely in scope.
2. **Per-domain uncertainty** — how many unresolved threads each domain opens up.

A domain may take zero rounds if it's clearly out of scope, one round if the brief resolves it, or three or four rounds if each answer opens a new thread. Follow the work, don't ration it.

### Calibrate silently, don't negotiate a tier

Before the first question, silently read the brief and consider:

- How many surfaces / flows / systems are involved?
- Is this greenfield or an extension of existing behaviour?
- Are new user types, external systems, or novel patterns introduced?
- Where are the obvious unknowns and where is the brief already concrete?

Use this to decide which domains are in scope and where to spend interview effort. **Do not announce a "tier" or "size" to the user.** Do not ask the user to pre-approve how many rounds you'll run. Just ask the right questions.

### Walk the domains

See the **Domains to Cover** section of this skill for the domain checklist. For each *relevant* domain, run one or more `AskUserQuestion` rounds until the domain is actually resolved — not just touched once. Tightly related sub-questions can be grouped into a single call (per the `user-questions.md` rules), but a single call rarely resolves a domain with real depth.

A domain may be skipped only if it is *genuinely irrelevant* to the work. When skipping, record it in `## Clarifications` as `- [domain]: skipped — not applicable because [reason]`. Do not skip a domain merely because it feels tedious.

### Go deep where it matters

- **Dig on ambiguity** — if an answer reveals a new subsystem, a tension with an earlier answer, an edge case, or a half-resolved constraint, follow it with another round. Keep pulling the thread until it terminates.
- **Escalate when scope grows** — if an answer surfaces substantial new scope (a new user type, a new integration, a new flow), acknowledge it silently and continue interviewing until the new scope is fully covered. Do not cap yourself because "we've already asked a lot".

### Skip what's already settled

- **Don't re-ask what the brief, the PRD, the master plan, or a prior answer already resolves.** Note the resolution in `## Clarifications` ("Resolved from PRD §Overview: ...") and move on.
- **Don't ask painfully obvious questions.** If a competent agent can infer the answer from context (e.g. "should this responsive web app work on mobile?"), state the inference as an assumption in `## Clarifications` and move on. If the assumption is non-trivial, surface it to the user for confirmation in a batch at the end rather than one-at-a-time.
- **Don't ask questions whose answer doesn't affect the plan.** Trivia is waste.

### Exit criteria

Finalize the plan only when **all** of these are true:

1. Every relevant domain in the **Domains to Cover** list has been resolved — not merely touched — or explicitly marked skipped in `## Clarifications` with a reason.
2. No open threads remain — every answer that spawned a follow-up question has had its follow-up answered.
3. The user has explicitly confirmed, via your structured question tool, that they have nothing more to add. Do not assume silence means done.
4. Every user answer is captured in `## Clarifications` verbatim enough that an implementation agent can trace every decision back to the interview.
5. Any research findings have been surfaced to the user and incorporated (see Proactive Research).

If any of these fail, keep asking. Round count is an output of the work, not a limit on it.

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

For a product planning session, the relevant domains (per the Dynamic Questioning framework above) are:

- **User & audience** — who specifically, in what context, with what prior expectations?
- **Problem & motivation** — why now? What triggered this? What pain does it remove?
- **Primary flow** — step-by-step happy path, from entry point to success.
- **Alternate flows & variations** — first-time user, returning user, power user, admin / elevated roles.
- **Edge cases** — empty states, very long / malformed content, errors, permission-denied, network failure, concurrency, rate limits.
- **Success criteria** — measurable business / user outcomes; how do we know it worked?
- **Industry / competitive conventions** — what do users already expect based on comparable products?
- **Content & copy** — tone, length, personalization, tone-of-voice constraints.
- **Accessibility** — keyboard navigation, screen reader semantics, contrast, reduced motion, target WCAG level.
- **Internationalization / localization** — languages, RTL, locale-specific formatting, dynamic text expansion.
- **Analytics & telemetry** — which events to track, why, and which dashboards they feed.
- **Trust, privacy, legal** — PII handling, consent, data retention, audit trails, regulatory framing (GDPR, COPPA, HIPAA, …).
- **Onboarding / discovery** — how users find the feature, how they learn to use it, empty-first-use state.
- **Notifications & cross-surface** — email, push, in-app, cross-device touchpoints.
- **Offline / degraded states** — behaviour without connectivity or with partial data.
- **Monetization** — pricing, entitlements, paywalls, billing events (only if commercial).

## Research Triggers

Kick off a research sub-agent (per the Proactive Research framework above) when any of these appear in the brief or during the interview:

- **Competitive benchmarking** — "how do other products do X?"
- **Industry-standard UX patterns** — e.g. expected flows for checkout, sign-up, password reset, notifications opt-in.
- **Accessibility standards** — specific WCAG success criteria for the component being built.
- **Compliance / regulatory context** — GDPR, COPPA, HIPAA, PCI-DSS, SOC2, local tax rules, age-gating law.
- **Content / copy conventions** — required disclosures (financial, medical, legal), platform-specific guidelines (Apple, Google).
- **Pricing / monetization benchmarks** — common tier structures, trial lengths, typical conversion copy.
- **Notification & transactional email norms** — CAN-SPAM, double-opt-in, unsubscribe conventions.
- **Prior-art examples** — when the user invokes "like Notion does it" / "like Linear does it", confirm the actual pattern instead of guessing.

## ALLOWED ACTIONS
- Reading files to understand the codebase
- If any Figma URLs are included, load them **inline** (directly in this session) using the Figma MCP tools. Do NOT spawn a sub-agent for Figma — sub-agents cannot get MCP tool permissions approved. Extract design context (layout, colors, typography, component structure, copy) and incorporate findings into the PRD.
- Asking the user questions
- Writing to `.belmont/PRD.md`, `.belmont/PROGRESS.md`, `.belmont/features/`, and master `.belmont/PROGRESS.md`
- Creating feature directories under `.belmont/features/`
- Using WebFetch for inline lookups of single user-provided URLs
- Spawning `Explore` or `general-purpose` sub-agents for deep web research (see Proactive Research)

## Strategic Context

Before planning, check if `.belmont/PR_FAQ.md` exists and has real content (not just template text). If it does, read it and use it as strategic context for planning — the PR/FAQ defines the customer, problem, and solution vision that should inform the PRD.

## Master PRD

Read `.belmont/PRD.md` — the master feature catalog. If it's empty/default, you'll create it during this session.

## Feature Selection

Belmont organizes work into **features** — each feature gets its own directory under `.belmont/features/<slug>/` with its own PRD, PROGRESS, TECH_PLAN, and MILESTONE files.

### Select the Active Feature

1. List all feature directories under `.belmont/features/`
2. If features exist: read each feature's `PRD.md` for its name and status, then Ask the user to create a new feature or select an existing one to update
3. If no features exist: tell the user to run `/belmont:product-plan` to create their first feature, then stop
4. Set the **base path** to `.belmont/features/<selected-slug>/`

### Base Path Convention

Once the base path is resolved, use `{base}` as shorthand:
- `{base}/PRD.md` — the feature PRD
- `{base}/PROGRESS.md` — the feature progress tracker
- `{base}/TECH_PLAN.md` — the feature tech plan
- `{base}/MILESTONE.md` — the active milestone file
- `{base}/MILESTONE-*.done.md` — archived milestones
- `{base}/NOTES.md` — learnings and discoveries from previous sessions

**Master files** (always at `.belmont/` root):
- `.belmont/PR_FAQ.md` — strategic PR/FAQ document
- `.belmont/PRD.md` — master PRD (feature catalog)
- `.belmont/PROGRESS.md` — master progress tracking (feature summary table)
- `.belmont/TECH_PLAN.md` — master tech plan (cross-cutting architecture)

## Creating the Master PRD (first time)

If `.belmont/PRD.md` is empty/default and no features exist yet, create the **master PRD** as a living global document:

```markdown
# Product: [Product Name]

## Vision
[1-2 sentence product vision, drawn from PR_FAQ if available]

## Constraints
[Global constraints that apply across all features — performance budgets, browser support, accessibility requirements, etc.]

## Cross-Cutting Decisions
[Product decisions that span multiple features. Actively curate this section — edit/remove stale info, don't just append. Examples: navigation patterns, shared UX conventions, data model decisions.]
```

This is a **living document**. Skills and agents actively curate it — editing existing sections, removing stale info, and updating decisions as the product evolves. It is NOT a feature catalog (features are tracked in PROGRESS.md).

Also create `.belmont/PROGRESS.md` (the master progress file) if it doesn't exist or still contains template/placeholder text:

```markdown
# Progress: [Product Name]

## Features

| Feature | Slug | Priority | Dependencies | Status | Milestones | Tasks |
|---------|------|----------|-------------|--------|------------|-------|
| [Feature Name] | [feature-slug] | P1 | None | Not Started | 0/N | 0/N |

## Recent Activity
| Date | Feature | Activity |
|------|---------|----------|
```

**Dependencies format**: Use feature slugs, comma-separated (e.g. `setup, auth`). Use `None` for features with no dependencies. Features with dependencies execute after their dependencies complete when using `belmont auto --all`.

Then immediately proceed to create the first feature (below).

## Creating or Updating a Feature

When the user selects or creates a feature:

1. **Generate slug**: lowercase, hyphens, no special chars (e.g. "User Authentication" → `user-authentication`)
2. **Create directory**: `.belmont/features/<slug>/`
3. **Write feature PRD**: `.belmont/features/<slug>/PRD.md` (using the PRD format below)
4. **Write feature PROGRESS**: `.belmont/features/<slug>/PROGRESS.md` (using the PROGRESS format below)
5. **Update master PRD**: If any cross-cutting product decisions were made during planning, add them to `.belmont/PRD.md`'s `## Cross-Cutting Decisions` section. Edit existing entries if they changed.
6. **Update master PROGRESS**: Add or update the feature's row in `.belmont/PROGRESS.md`'s `## Features` table with the feature name, slug, priority, dependencies, initial status, milestone/task counts. Set Dependencies to slugs of features this one requires (data, APIs, infrastructure) — use `None` if independent. Add a row to `## Recent Activity` noting the feature was created or updated.

When **updating** an existing feature (its PRD.md has real content): only add/modify the specific tasks, milestones, or sections needed. NEVER replace the entire file. Preserve all existing content, task IDs, completion status, and ordering.

If the existing PRD/PROGRESS already contains standalone verification/QA/testing tasks (a legacy anti-pattern — see "Important Considerations" below), do NOT mirror that pattern for the new work you are planning. Surface the stale tasks to the user, explain that verification is automatic, and offer to migrate their criteria into the adjacent feature task's `**Verification**:` field. Do not remove them silently — always ask.

## Process

1. Load relevant skills for the domain (figma:*, frontend-design, vercel-react-best-practices, security, etc.)
2. Ask the user what they want to build.
3. **Calibrate silently** (see *Dynamic Questioning Depth* above) — read the brief, decide which domains are in scope, note the obvious unknowns. Do not announce a tier to the user; just start asking.
4. Walk the **Domains to Cover** checklist. For each relevant domain, run as many rounds as it takes to actually resolve it. Dig on ambiguity; skip what the brief, PRD, or prior answers already settle. No round cap.
5. **Trigger research proactively** (see *Proactive Research* above) whenever a signal from the **Research Triggers** checklist appears. Delegate deep research to a sub-agent; loop findings back to the user with options.
6. If Figma design URLs are included, load them inline using Figma MCP tools. Extract design context and add exact Figma URLs to the PRD for future agents to use.
7. Consider edge cases, dependencies, blockers. Be proactive — surface questions the user may not have thought to ask.
8. Verify the **exit criteria** from the Dynamic Questioning framework: every relevant domain resolved (or explicitly marked skipped), every follow-up thread closed, user has explicitly confirmed nothing more to add, all answers captured in `## Clarifications`.
9. Break the feature down into implementable milestones and tasks. Keep milestones small and focused. Group related tasks that can be completed in a single session.
10. Write the finalized PRD.md and PROGRESS.md (in UPDATE mode, only add/modify — never replace). Include a `### Research Notes` subsection in `## Technical Context` if research was performed.
11. Exit — do NOT start implementation.

## Question Scope (CRITICAL)

This is a **product** planning session, NOT a technical planning session. Technical decisions are made in the follow-up tech-plan step (`/belmont:tech-plan`).

### ASK about (product concerns):
- What the user wants to build and why (vision, goals, problem statement)
- Target users / audience
- User flows and journeys (what does the user do step by step?)
- Feature requirements and business logic
- Content and copy decisions
- Priority and scope (what's in vs. out)
- Success criteria from a user/business perspective
- Edge cases in user behavior
- Design intent (if no Figma: what should it look and feel like?)

### DO NOT ASK about (defer to tech-plan):
- Framework or library choices (Next.js vs Remix, React vs Vue, etc.)
- Package manager preferences (npm, pnpm, bun, etc.)
- Routing strategy (App Router vs Pages Router, etc.)
- i18n library or localization setup
- Data source format (static file vs API endpoint vs CMS)
- Animation library or implementation approach
- Asset strategy (placeholders vs real assets)
- Component architecture or file structure
- State management approach
- Styling approach (Tailwind vs CSS modules, etc.)
- Specific pricing values or placeholder content (these come from designs/implementation)
- Whether to add a separate verification / QA / testing task or milestone — verification runs automatically after each milestone via `/belmont:verify`; per-task criteria live in the task's `**Verification**:` field.

If the user volunteers technical preferences unprompted, record them as **open questions for the tech-plan step** — do NOT commit them as decisions in the PRD. The tech-plan step may well decide otherwise, and baking a technical idiom into the PRD creates drift (the PRD ends up saying "use `<Image>` directly" while the tech-plan correctly introduces a wrapper component). The only exceptions are genuinely cross-cutting product constraints the tech-plan must honor (e.g. "must ship inside the existing Next.js 15 app", "must reuse the existing design-system primitives") — never file paths, wrapper-vs-direct component choices, library imports, regex syntax, or endpoint names.

See the plan-separation partial above for the full PRD ↔ TECH_PLAN boundary rules.

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
   git add .belmont/ && git commit -m "belmont: update planning files after product planning"
   ```

**Note**: PROGRESS.md is the single source of truth for task state. PRD.md is a pure spec document with no status markers — do not add emoji or state indicators to PRD task headers.

Final: Prompt user to "/clear" and then "/belmont:tech-plan"
   - If you are Codex, instead prompt: "/new" and then "belmont:tech-plan"
   - If this was the first feature in a new product, also mention they can create more features later by running `/belmont:product-plan` again

## Important Considerations

Guidance on the verification anti-pattern, per-task verification fields, milestone grouping, and dependency annotations is in `references/product-plan-considerations.md`. Read it before breaking the feature down into milestones and tasks.

## PRD Format

**Read `references/product-plan-prd-format.md` for the full PRD template**, then write it to `{base}/PRD.md` (i.e. `.belmont/features/<slug>/PRD.md`). Fill every section; leave empty placeholders only for "Out of Scope" and "Open Questions" when genuinely empty.

## PROGRESS Format

Write the PROGRESS to `{base}/PROGRESS.md` (same base path as the PRD) with this structure:

```markdown
# Progress: [Feature Name]

## PRD Reference
.belmont/PRD.md

## Milestones

### M1: [Milestone Name]
- [ ] P0-1: Task description
- [ ] P0-2: Task description

### M2: [Milestone Name] (depends: M1)
- [ ] P1-1: Task description

### M3: [Milestone Name] (depends: M1)
- [ ] P1-1: Task description

### M4: [Milestone Name] (depends: M2, M3)
- [ ] P1-1: Task description

> **Dependency syntax**: Add `(depends: M1)` or `(depends: M1, M3)` after the milestone name to declare dependencies. When dependencies are present, `belmont auto` will run independent milestones in parallel via git worktrees. If no milestones have `(depends: ...)`, they run sequentially (default behavior).

> **Task states**: `[ ]` todo, `[>]` in_progress, `[x]` done, `[v]` verified, `[!]` blocked. Milestone status is computed from its tasks — do not add status emoji to milestone headers.

## Session History

| Date | Action | Details |
|------|--------|---------|

## Decisions Log

(none yet)
```

## Begin

We are in plan mode. Please await the user's input describing what they want to build. After planning is complete, write the PRD.md and PROGRESS.md files and exit. Do NOT implement the plan.
