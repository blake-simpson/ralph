# Tech Plan: Master TECH_PLAN.md Format

Use this when writing `.belmont/TECH_PLAN.md` in Scenario A (first run) or Scenario C (master-only update).

The master TECH_PLAN is a **living document** for cross-cutting architecture decisions. Skills and agents actively curate it — editing existing sections, removing stale info, and updating decisions as the architecture evolves. Write to `.belmont/TECH_PLAN.md` with this structure:

```markdown
# Technical Plan: [Product Name]

## Overview
[2-3 sentences on the product-level technical vision]

## Stack & Tooling
| Category        | Choice                   | Rationale |
|-----------------|--------------------------|-----------|
| Framework       | e.g. Next.js 15          | [why]     |
| Package Manager | e.g. pnpm                | [why]     |
| Styling         | e.g. Tailwind CSS 4      | [why]     |
| Deployment      | e.g. Vercel              | [why]     |
| Testing         | e.g. Vitest + Playwright | [why]     |

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
- **E2E**: [tool, scope, critical paths — if applicable]

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

## References
[External sources that informed decisions above. One bullet per source: `- [Short title](URL) — one-sentence summary.` Flag stale sources `(potentially stale — last updated YYYY-MM)` if older than ~12 months.]
```
