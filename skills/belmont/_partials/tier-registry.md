### Model Tier Registry

Belmont uses three user-facing tiers — `low`, `medium`, `high` — which map to concrete model identifiers per AI CLI. When you need to pass a model override explicitly (see `dispatch-strategy.md` Model Tier Overrides or `tier-preflight.md`), translate via this table.

| Tier   | Claude  | Codex          | Gemini                | Cursor             | Copilot              |
|--------|---------|----------------|-----------------------|--------------------|----------------------|
| low    | haiku   | gpt-5.4-mini   | gemini-2.5-flash-lite | sonnet-4           | haiku-4.5            |
| medium | sonnet  | gpt-5.3-codex  | gemini-2.5-flash      | sonnet-4-thinking  | claude-sonnet-4.5    |
| high   | opus    | gpt-5.4        | gemini-2.5-pro        | gpt-5              | gpt-5.4              |

The canonical source is the `modelTiers` map in `cmd/belmont/main.go`. If this table drifts from the Go registry, the Go registry wins — file an issue and update this partial. `scripts/generate-skills.sh --check` is the place to add a drift guard.
