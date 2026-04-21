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
