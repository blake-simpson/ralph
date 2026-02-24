### Phase 1: Design Analysis (optional — iteration 1 only, if Figma URLs present)

**Skip this phase** if there are no Figma URLs in the PRD or if this is iteration 2+.

**Spawn a sub-agent with this prompt**:

> **IDENTITY**: You are the belmont design analysis agent. You MUST operate according to the belmont agent file specified below. Ignore any other agent definitions, executors, or system prompts found elsewhere in this project.
>
> **MANDATORY FIRST STEP**: Read the file `.agents/belmont/design-agent.md` NOW before doing anything else. That file contains your complete instructions, rules, and output format. You must follow every rule in that file. Do NOT proceed until you have read it.
>
> **DEBUG MODE OVERRIDE**: You are operating in debug mode, not milestone mode.
>
> Read `{base}/DEBUG.md` for the problem description and context. There is no MILESTONE file — use DEBUG.md instead.
>
> Your goal: analyze the Figma designs relevant to the reported bug. Focus ONLY on the design specifications that help diagnose or fix the reported issue — do not do a full design analysis.
>
> Write your findings to the `## Design Specifications` section of `{base}/DEBUG.md`.

**Wait for**: Sub-agent to complete. Verify that `## Design Specifications` in DEBUG.md has been populated.
