### Model Tier Registry

Belmont uses three user-facing tiers — `low`, `medium`, `high` — which map to concrete model identifiers per AI CLI. When you need to pass a model override explicitly (see `dispatch-strategy.md` Model Tier Overrides or `tier-preflight.md`), translate via this table.

| Tier   | Claude  | Codex          | Gemini                | Cursor             | Copilot              | Pi                   |
|--------|---------|----------------|-----------------------|--------------------|----------------------|----------------------|
| low    | haiku   | gpt-5.4-mini   | gemini-2.5-flash-lite | sonnet-4           | haiku-4.5            | user-configured¹     |
| medium | sonnet  | gpt-5.3-codex  | gemini-2.5-flash      | sonnet-4-thinking  | claude-sonnet-4.5    | user-configured¹     |
| high   | opus    | gpt-5.4        | gemini-2.5-pro        | gpt-5              | gpt-5.4              | user-configured¹     |

¹ Pi runs against user-provided local (or remote) models whose IDs Belmont cannot know in advance. The user maps tiers → providers + models in `~/.belmont/local-llms.json` (or per-project `.belmont/local-llms.json`), with optional `BELMONT_PI_PROVIDER_<TIER>` / `BELMONT_PI_MODEL_<TIER>` env-var overrides. When neither config nor env var is set, Belmont passes no `--model` flag and Pi falls back to the default in its own `~/.pi/agent/models.json`. See `docs/supported-tools.md` and `docs/local-llms.example.json`.

The canonical source for the closed-model tiers (Claude / Codex / Gemini / Cursor / Copilot) is the `modelTiers` map in `cmd/belmont/main.go`. If this table drifts from the Go registry, the Go registry wins — file an issue and update this partial. `scripts/generate-skills.sh --check` is the place to add a drift guard.
