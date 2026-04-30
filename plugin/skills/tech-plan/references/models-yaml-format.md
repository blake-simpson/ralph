# Per-Feature Model Tiers: `models.yaml` Format

Belmont assigns each sub-agent (codebase, design, implementation, verification, code-review, reconciliation) to a tier — `low`, `medium`, or `high` — which is mapped to a concrete model ID for whichever AI CLI runs the work (Claude Code, Codex, Gemini, Cursor, Copilot).

Tiers are stored per feature in `.belmont/features/<slug>/models.yaml`. The tech-plan skill writes this file after assessing the feature's effort profile; the Belmont Go CLI reads it when spawning each phase, and the orchestrator skills (implement, verify, code-review) read it when dispatching sub-agents on Claude Code.

## Schema

```yaml
# Generated during /belmont:tech-plan. Safe to hand-edit.
profile: frontend-heavy
planning: high
tiers:
  codebase: medium
  design: high
  implementation: high
  verification: high
  code-review: medium
  reconciliation: high
```

### Fields

- **`profile`** *(string, free-form)* — short label describing the feature's character. Used for human context; not parsed by Belmont beyond display. Common values: `frontend-heavy`, `backend-heavy`, `fullstack`, `infra`, `docs`, `research`, `refactor`. You can invent a better label if none fits.

- **`planning`** *(string, usually `high`)* — records the tier used for product-plan and tech-plan. This is always `high` in practice because planning produces the spec every downstream agent executes against. Editing this value has no runtime effect (the Go CLI hardcodes `planningTier = "high"`); it exists as a permanent record of the decision.

- **`tiers`** *(map of agent → tier)* — the core config. Each agent gets one of `low`, `medium`, or `high`. Unknown agents are ignored; missing agents fall back to their frontmatter default in `agents/belmont/<name>.md`. Recognized agents:
  - `codebase` — exploration / pattern scanning
  - `design` — Figma extraction, token mapping, visual spec
  - `implementation` — code generation, acceptance validation
  - `verification` — test runs, visual diff, acceptance checking
  - `code-review` — diff review, lint / pattern validation
  - `reconciliation` — merge-conflict semantic resolution (applied at merge-time, not per-milestone)

## Tier → Model mapping

The Belmont Go CLI maps each tier to a CLI-specific model ID. See the `modelTiers` map in `cmd/belmont/main.go` for the canonical table. Current mapping (check the Go source for latest):

| Tool    | low                        | medium                | high                |
|---------|----------------------------|-----------------------|---------------------|
| claude  | haiku                      | sonnet                | opus                |
| codex   | gpt-5.4-mini               | gpt-5.3-codex         | gpt-5.4             |
| gemini  | gemini-2.5-flash-lite      | gemini-2.5-flash      | gemini-2.5-pro      |
| cursor  | sonnet-4                   | sonnet-4-thinking     | gpt-5               |
| copilot | haiku-4.5                  | claude-sonnet-4.5     | gpt-5.4             |

Tiers are stable; model IDs get bumped in the Go registry when tools ship new versions.

## Starting-point examples (non-definitive)

These are **illustrative heuristics only** — the planning model is expected to reason about the specific feature at hand, not pattern-match to a profile label.

- **frontend-heavy** (rich interactive UI, lots of visual/Figma work): design=high, implementation=high, verification=high, codebase=medium, code-review=medium, reconciliation=high.
- **backend-heavy** (APIs, data-layer, migrations): design=low (no UI), implementation=high, verification=medium (unit tests), codebase=medium, code-review=medium, reconciliation=high.
- **infra** (config, CI, deployment, pipelines): everything medium, reconciliation=high.
- **docs** (content-only changes, README refreshes, ADRs): everything low, reconciliation=medium.
- **refactor** (no behavior change, lots of code movement): implementation=high (reasoning about preservation), verification=medium, reconciliation=high (merge conflicts likely).
- **research** (exploration, prototyping): codebase=high (pattern inference), implementation=low (throwaway code), verification=low.

Again, these are loose anchors. A "frontend-heavy" feature that's just restyling one button probably warrants all-`low` except reconciliation. A "docs" feature that rewrites the entire ADR catalog probably warrants `medium` implementation. Reason about the specific work.

## Fallback behavior

- If `models.yaml` is absent, each agent uses the `model:` value from `agents/belmont/<name>.md` frontmatter (Sonnet for most, Opus for reconciliation).
- If `models.yaml` exists but omits an agent, that agent falls back to its frontmatter default.
- If a tier value is invalid (`extreme`, typos, etc.), the runtime omits `--model` and the tool uses its own default model.
- The user can accept Belmont defaults explicitly during tech-plan — in that case the skill does NOT create `models.yaml`, and the runtime falls through to frontmatter defaults.

## Editing by hand

The file is plain YAML with a deliberately flat schema. Editing by hand is supported and intentional — if the tech-plan's recommendation turns out to be wrong mid-implementation, just edit the file and re-run `belmont auto` (or restart your manual session). The Go parser ignores unknown keys and empty values, so comments and extra fields won't break it.
