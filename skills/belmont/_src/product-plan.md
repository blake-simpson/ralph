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

<!-- @include forbidden-actions.md -->

<!-- @include plan-separation.md -->

<!-- @include user-questions.md -->

<!-- @include dynamic-questioning.md -->

<!-- @include proactive-research.md -->

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

<!-- @include feature-detection.md feature_action="Ask the user to create a new feature or select an existing one to update" -->

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

<!-- @include commit-belmont-changes.md commit_context="after product planning" -->

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
<!-- @include progress-template.md -->
```

## Begin

We are in plan mode. Please await the user's input describing what they want to build. After planning is complete, write the PRD.md and PROGRESS.md files and exit. Do NOT implement the plan.
