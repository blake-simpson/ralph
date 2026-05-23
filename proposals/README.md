# Belmont Proposals

This directory holds design-stage proposals (RFCs) for changes to Belmont that warrant discussion before implementation lands.

## When to write a proposal

Open a proposal PR when:

- The change adds a new user-facing surface (new skill, new CLI subcommand, new config file).
- The change modifies an existing invariant (agent contract, milestone-immutability rule, scope guard behaviour).
- The change is large enough that maintainers will want to discuss design before reviewing code.

Small, focused PRs (bug fixes, docs improvements, a single new skill that obviously fits) don't need a proposal — open the PR directly per [`CONTRIBUTING.md`](../CONTRIBUTING.md).

## Naming

`proposals/NNNN-short-slug.md`, where `NNNN` is the next unused four-digit sequence. Examples:

- `0001-quick-mode.md`
- `0002-prd-hygiene.md`

## Structure

Each proposal is self-contained markdown. Suggested H2 sections:

- `## Summary` — one paragraph: what changes, what's in scope, what isn't.
- `## Motivation` — what problem is being solved, with evidence.
- `## Proposed Change` — the technical detail.
- `## User-Facing Behaviour` — worked examples of what the user sees.
- `## Acceptance Criteria` — checkable bullets for when implementation is "done".
- `## Out of Scope` — what this proposal does *not* try to do.
- `## Open Questions` — the points where maintainer input is wanted.
- `## References` — links to relevant code, docs, related proposals.

## Lifecycle

1. **Draft** — opened as a PR to this directory. Maintainers and contributors discuss in the PR thread.
2. **Accepted** — proposal merges to `main` with `Status: Accepted`. Implementation can begin in a separate PR.
3. **Implemented** — once the implementation PR lands, the proposal's `Status:` is updated. The proposal stays in the repo as design history.
4. **Rejected / Withdrawn** — proposal PR closed without merging, or merged with `Status: Rejected` plus a brief rationale.

The implementation PR can link back to the proposal in its description (`Implements proposals/NNNN-short-slug.md`).
