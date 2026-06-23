# Requirements: Subscription Mode Should Default to Latest Opus, Not Opus 4.6

## Background

Spec task 002134 added configurable model selection for Claude subscription mode, defaulting to Opus. However, the model ID `claude-opus-4-6` was hardcoded in ~7 places across backend and frontend. Anthropic has since released Opus 4.7 and 4.8, but the subscription default still points at 4.6. The `RECOMMENDED_CODING_MODELS` list in the frontend already knows about `claude-opus-4-7`, but the subscription dropdown and backend default were never updated.

Claude Code's `resolveModelPreference()` function (in the claude-agent-acp package) performs substring matching on the model value from managed-settings.json. Claude Code's `--model` flag already accepts tier-level shorthand names like `"opus"`, `"sonnet"`, `"haiku"` and resolves them to the latest available version of that tier. Since the settings-sync-daemon writes `CodeAgentConfig.Model` directly into managed-settings.json, passing a shorthand like `"opus"` should auto-resolve to the current latest Opus without hardcoding a specific version.

## User Stories

**US-1: Default to the latest Opus**
As a Helix user with a Claude subscription, I want new agents to default to the latest Opus so I get the best available model without manually changing settings — including when Anthropic ships future versions like Opus 4.9.

**US-2: No code changes when new models ship**
As a Helix developer, I want the subscription model identifiers to auto-resolve to the latest version so that no code changes are needed when Anthropic releases a new Opus, Sonnet, or Haiku.

## Acceptance Criteria

**AC-1 — Default resolves to latest Opus**
- New subscription agents default to `"opus"` (the tier-level shorthand), which Claude Code's `resolveModelPreference()` resolves to the current latest Opus (currently `claude-opus-4-8`).
- The subscription model dropdown offers tier-level choices: Opus, Sonnet, Haiku — without pinning to specific minor versions.
- Existing agents with `claude-opus-4-6` saved continue to work (no migration needed — `resolveModelPreference()` still resolves versioned IDs).

**AC-2 — Verification in subscription container**
- Manually test that writing `"model": "opus"` to managed-settings.json results in Claude Code using the latest Opus (currently 4.8) in a real subscription container.
- Verify that `"sonnet"` and `"haiku"` similarly resolve to their latest versions.

**AC-3 — `normalizeModelIDForZed` handles 4.7 and 4.8**
- `claude-opus-4-7` normalizes to `claude-opus-4-7-latest` (not `claude-opus-4-latest`).
- `claude-opus-4-8` normalizes to `claude-opus-4-8-latest` (not `claude-opus-4-latest`).
- (This is for the API-key path, not subscription, but is a pre-existing bug worth fixing.)

## Out of Scope

- Dynamically fetching the model list from Anthropic's API (we maintain our own curated list).
- Migrating existing agents' stored `claude-opus-4-6` values — they work as-is via `resolveModelPreference()`.
- Adding specific Opus versions (4.7, 4.8) as separate dropdown options (the shorthand auto-resolves).
