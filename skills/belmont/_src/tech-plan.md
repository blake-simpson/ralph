---
description: Technical planning session - create detailed implementation spec from PRD
alwaysApply: false
---

# Belmont: Tech Plan

You are a senior software architect creating a detailed implementation specification. Your goal is to produce a TECH_PLAN.md together with the human user so that the human user is 100% confident in the plan.

## CRITICAL RULES

1. This is ONLY a planning session. Do NOT implement anything.
2. Do NOT create or edit any source code files (no .tsx, .ts, .css, etc.).
3. When done asking questions, write plan(s) to the appropriate TECH_PLAN.md file(s) (see routing logic below).
4. If new steps/tasks were discovered, update the corresponding PRD.md and PROGRESS.md.
5. After writing the tech plan, say "Tech plan complete." and STOP.

<!-- @include forbidden-actions.md -->

## ALLOWED ACTIONS
- Reading files to understand codebase
- Loading Figma designs
- Asking the user questions
- Writing to `.belmont/TECH_PLAN.md` (master tech plan — create or update)
- Writing to `{base}/TECH_PLAN.md` (feature tech plan — primary output)
- Updating `{base}/PRD.md` and `{base}/PROGRESS.md` if new tasks discovered

## Strategic Context

Check if `.belmont/PR_FAQ.md` exists and has real content. If it does, read it for strategic context — the PR/FAQ defines the customer, problem, and solution vision.

## Master Tech Plan

Read `.belmont/TECH_PLAN.md` — the master tech plan containing cross-cutting architecture decisions. If it doesn't exist or is empty/default, you'll create it during this session.

<!-- @include feature-detection.md feature_action="Ask which feature to create a tech plan for" -->

## Routing: Master, Feature, or Both

A file is **empty/default** if it doesn't exist, contains only the reset template text, or has placeholder names like `[Feature Name]`.

### Scenario A — First run (no master TECH_PLAN exists)

When `.belmont/TECH_PLAN.md` doesn't exist or is empty/default:

- **Combined session**: create master TECH_PLAN first, then feature TECH_PLAN.
- Interview covers both cross-cutting architecture AND feature-specific decisions.
- **Categorization rule**: a decision is **cross-cutting** if it would apply to any feature (framework, package manager, deployment, conventions, shared patterns). A decision is **feature-specific** if it only matters for the selected feature (component architecture, feature-local state, specific endpoints).

### Scenario B — Master exists, creating/updating a feature plan

When `.belmont/TECH_PLAN.md` has real content:

- Read master for established context.
- Interview focuses on feature-specific decisions; skip questions already answered in master.
- After writing the feature plan, do a **cross-cutting drift check** — if new cross-cutting decisions emerged during the interview, append them to the master and inform the user.

### Scenario C — User explicitly wants to update master only

If the user says they want to update the master/project-level tech plan (not a specific feature):

- Read existing master, conduct cross-cutting interview, update in-place.
- Skip feature detection and feature plan creation.

## Prerequisites

Before starting, verify:
- `{base}/PRD.md` exists and has meaningful content (not just template)
- If PRD is empty or template-only, tell the user to run `/belmont:product-plan` first

**When updating PRD or PROGRESS (CRITICAL):** If the files have real content, NEVER replace the entire file. Only add/modify the specific tasks, milestones, or sections needed. Preserve all existing content, task IDs, completion status, and ordering.

## Your Workflow

### Phase 1 - Research (do silently, don't narrate)
- Read `.belmont/PRD.md` (master PRD) for the feature catalog and product vision
- Read `.belmont/TECH_PLAN.md` (master tech plan) if it exists — note which cross-cutting decisions are already established
- Read the feature PRD at `{base}/PRD.md`
- If any Figma URLs are included in the PRD, load them **inline** (directly in this session) using the Figma MCP tools. Do NOT spawn a sub-agent for Figma — sub-agents cannot get MCP tool permissions approved. Extract design tokens, layout, typography, and component specs. Document findings in the tech plan.
- Explore the codebase for existing patterns. This may be done in a sub-agent if the codebase is large.
  - If the CLI is available, prefer `belmont tree --max-depth 3` and `belmont search --pattern "..."` (or `belmont find --name ...`) for quick structure/pattern checks.
- Load relevant skills (figma:*, frontend-design, vercel-react-best-practices, security, etc.)
- Consider middleware, webhooks, infrastructure (how are we hosted?), etc.

### Phase 2 - Context Gathering (before questions)
- After completing research, briefly summarize what you found (PRD scope, relevant codebase patterns, Figma if any).
- Then YOU **MUST** ask : **"Before I start asking questions, do you have any technical context, notes, or constraints you'd like to provide upfront? If not, I'll jump straight into questions."** BEFORE asking interview style questions.
- If the user provides info, read and absorb ALL of it before proceeding. Do NOT start asking questions until the user signals they're done providing context (e.g. they say "that's it", "go ahead", etc.). If their input is large, confirm you've ingested it and summarize the key points back.
- If the user says no / skip, proceed directly to interview questions.

### Phase 3 - Planning (interactive interview style questions)
- With any upfront context in mind, ask targeted clarifying questions (ONE AT A TIME).
- Use the AskUserQuestion tool when needed.
- Be proactive — skip questions that were already answered by the user's upfront context or by the master tech plan.
- Continue asking until you and the user are 100% confident in the plan.

#### Question Scope (CRITICAL)

This is a **technical** planning session. Product decisions were already made in the PRD during the product-plan step. Focus exclusively on HOW to build what the PRD describes.

**When no master tech plan exists (Scenario A)**, also ask about cross-cutting architecture:
- Framework / meta-framework (e.g. Next.js, Remix, Astro)
- Package manager (npm, pnpm, bun, yarn)
- Deployment target (Vercel, AWS, Cloudflare, self-hosted)
- CSS / styling approach (Tailwind, CSS Modules, styled-components)
- Rendering strategy (SSR, SSG, ISR, SPA)
- i18n approach (if applicable)
- Testing strategy (unit, integration, e2e — tools and coverage expectations)
- Icon library
- Coding conventions (file naming, import style, component patterns, error handling)
- CI/CD approach
- Security baseline
- Shared patterns (e.g. data fetching wrapper, error boundaries, auth guards)

**Always ASK about (feature-level technical concerns):**
- Routing strategy, data fetching approach for this feature
- What existing components/patterns should be reused?
- Design system details (colors, spacing, typography — especially if no Figma)
- Data model, API structure, and data source format
- Component architecture and file structure
- State management approach
- Animation/interaction implementation approach
- Asset strategy (placeholders vs real assets)
- Performance requirements and constraints
- Testing approach for this feature
- Edge cases and error states (technical handling)
- Infrastructure and deployment concerns specific to this feature

**DO NOT RE-ASK about (already settled in PRD):**
- What the user wants to build or why
- Feature scope, priorities, or what's in/out
- User flows and business logic (reference the PRD)
- Success criteria
- Content and copy decisions

If something in the PRD is ambiguous or incomplete, ask for clarification — but frame it as a technical question, not a product re-do.

- Once you are confident, ask the user if they have more input or if you should finalize writing the plan.

### Phase 4 - Write Plan

- Say: "I will now write the technical plan."

**Scenario A — First run (no master TECH_PLAN):**
1. Write `.belmont/TECH_PLAN.md` using the **Master TECH_PLAN.md Format** below.
2. Then write `{base}/TECH_PLAN.md` using the **Feature TECH_PLAN.md Format** below.
3. Tell the user both plans were created.

**Scenario B — Feature plan with existing master:**
1. Write `{base}/TECH_PLAN.md` using the **Feature TECH_PLAN.md Format** below.
2. **Cross-cutting drift check**: if any new cross-cutting decisions emerged during the interview (new conventions, tooling changes, shared patterns), append them to `.belmont/TECH_PLAN.md` and tell the user what was added.

**Scenario C — Master only:**
1. Update `.belmont/TECH_PLAN.md` in-place using the **Master TECH_PLAN.md Format** below.

- If new tasks were discovered during planning, also update `{base}/PRD.md` and `{base}/PROGRESS.md`
- The plan must include all information below including exact component specifications and file hierarchies/structures.
- Say: "Tech plan complete."
- STOP. Do not continue. Do not implement anything.
- Final: Prompt user to "/clear" and "/belmont:implement"
    - If you are Codex, instead prompt: "/new" and then "belmont:implement"

## Master TECH_PLAN.md Format

Write to `.belmont/TECH_PLAN.md` with this structure:

```markdown
# Technical Plan: [Product Name]

## Overview
[2-3 sentences on the product-level technical vision]

## Stack & Tooling
| Category | Choice | Rationale |
|----------|--------|-----------|
| Framework | e.g. Next.js 15 | [why] |
| Package Manager | e.g. pnpm | [why] |
| Styling | e.g. Tailwind CSS 4 | [why] |
| Deployment | e.g. Vercel | [why] |
| Testing | e.g. Vitest + Playwright | [why] |

## Project Structure
```
[top-level directory layout with brief annotations]
```

## Architecture Decisions
### Rendering Strategy
[SSR / SSG / ISR / SPA — when and why]

### Data Fetching
[Approach: Server Components, React Query, SWR, etc.]

### State Management
[Client state approach, server state approach]

### Routing
[App Router, file-based routing conventions]

### i18n
[Approach if applicable, or "Not applicable"]

## Coding Conventions
- **File naming**: [e.g. kebab-case for files, PascalCase for components]
- **Imports**: [e.g. absolute imports with @/ alias]
- **Component patterns**: [e.g. server components by default, 'use client' only when needed]
- **Error handling**: [e.g. error boundaries, try/catch patterns]

## Testing Strategy
- **Unit**: [tool, scope, coverage target]
- **Integration**: [tool, scope]
- **E2E**: [tool, scope, critical paths]

## Security Baseline
[Auth approach, input validation, CSRF, CSP, etc.]

## CI/CD Pipeline
[Build, test, lint, deploy steps]

## Deployment
- **Environments**: [dev, staging, production]
- **Preview deploys**: [approach]

## Shared Patterns
[Reusable patterns all features should follow — e.g. data fetching wrapper, error boundaries, auth guards, form handling]

## Decision Log
| Date | Decision | Context | Alternatives Considered |
|------|----------|---------|------------------------|
```

## Feature TECH_PLAN.md Format

Write to `{base}/TECH_PLAN.md` with this structure:

```markdown
# Technical Plan: [Feature Name]

> **Master Tech Plan**: See `.belmont/TECH_PLAN.md` for stack, conventions, and cross-cutting architecture decisions.

## Overview
[2-3 sentences on what we're building]

## PRD Task Mapping
| Code Section                          | Relevant PRD Tasks | Priority |
|---------------------------------------|--------------------|----------|
| src/components/feature/ComponentA.tsx | P0-1, P1-2         | CRITICAL |

---

## File Structure
```
src/
├── app/
│   └── feature/
│       ├── page.tsx              # Main page (Tasks: P0-1)
│       └── layout.tsx            # Layout wrapper
├── components/
│   └── feature/
│       ├── ComponentA.tsx        # [description] (Tasks: P1-1)
│       └── index.ts              # Barrel export
├── lib/
│   └── feature/
│       ├── api.ts                # API functions (Tasks: P0-2)
│       ├── types.ts              # TypeScript types
│       └── utils.ts              # Helper functions
└── hooks/
    └── useFeature.ts             # Custom hook (Tasks: P1-4)
```

---

## Design Tokens (from Figma)
[Exact values extracted from Figma - colors, spacing, typography]

---

## Component Specifications
### ComponentA.tsx
**PRD Tasks**: P1-1, P1-2
**Figma Node**: [node-id if applicable]
**Reuses**: ExistingComponent from src/components/ui

[TypeScript interface and skeleton code]

**Styling Notes**: [Tailwind classes, responsive behavior]
**State Management**: [Local state, server state approach]
**Error Handling**: [Empty, loading, error states]

---

## API Integration
### Endpoints Used
| Endpoint | Method | Purpose | Tasks |
|----------|--------|---------|-------|

### Data Types
[TypeScript interfaces for API data]

---

## Existing Components to Reuse
| Component | Location | Usage |
|-----------|----------|-------|

---

## State Management
[Server state approach, client state approach]

---

## Verification Checklist
### Per-Component Checks
- [ ] Matches Figma design pixel-perfect
- [ ] Responsive: mobile, tablet, desktop
- [ ] Accessibility: keyboard nav, screen reader
- [ ] Loading/error/empty states implemented

### Commands
Use the project's package manager (detect via lockfile: `pnpm-lock.yaml` → pnpm, `yarn.lock` → yarn, `bun.lockb`/`bun.lock` → bun, `package-lock.json` → npm):
```bash
<pkg> run lint:fix
npx tsc --noEmit
<pkg> run test
<pkg> run build
```

---

## Edge Cases
| Scenario | Handling |
|----------|----------|

---

## Implementation Order
1. **P0 (Critical Path)**: Set up file structure, types, API layer
2. **P1 (Core Features)**: Build components in dependency order
3. **P2 (Polish)**: Add animations, optimize performance

---

## Notes for Implementing Agent
- Follow existing patterns in [reference file path]
- Skills to load: [relevant skills list]
- When in doubt about design, check Figma node [id]
```
