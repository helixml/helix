# Design: Configurable Model Selection for Claude Subscription Mode

## Architecture Overview

The cloud model selection path in Zed:
1. Cloud API returns `ListModelsResponse` with `default_model` and available model list.
2. `language_models_cloud.rs` stores the cloud-provided default in `CloudModelProvider`.
3. `registry.rs` resolves the active model: user setting → `agent.default_model` → cloud default.
4. `agent_configuration.rs` renders the agent settings panel; it already detects `is_zed_provider` at line ~221 but does not currently show a model picker for subscription mode.

## Approach

Implement both changes together — the default fix is one line, the picker makes it user-configurable:

### 1. Change the Default (Minimum Viable)

In `crates/language_models_cloud/src/language_models_cloud.rs`, when `ListModelsResponse.default_model` is absent or resolves to a Sonnet variant, prefer the highest-versioned model whose ID contains `"opus"` from the returned model list. This mirrors the API key mode's `pick_preferred_model()` logic in `crates/language_models/src/provider/anthropic.rs` (lines 224–245).

### 2. Model Picker in Agent Configuration

In `crates/agent_ui/src/agent_configuration.rs`, inside the block guarded by `is_zed_provider` (around line 221), render a simple three-option picker using the existing `LanguageModelSelector` component scoped to the cloud provider's model list, filtered to one representative per tier (Opus / Sonnet / Haiku). Selection writes to `agent.default_model` via the same path used by API key mode.

**Filter logic:** from the cloud model list, pick the lexicographically greatest model ID that starts with each tier prefix:
- `claude-opus-`
- `claude-sonnet-`
- `claude-haiku-`

This is the same strategy used in API key mode (`pick_preferred_model`) so no new logic is needed.

## Key Files

| File | Change |
|------|--------|
| `crates/language_models_cloud/src/language_models_cloud.rs` | Prefer Opus when server default is absent/Sonnet |
| `crates/agent_ui/src/agent_configuration.rs` | Add 3-option model picker under `is_zed_provider` guard (~line 221) |
| `crates/agent_ui/src/agent_model_selector.rs` | No change — reuse as-is |
| `crates/settings_content/src/agent.rs` | No change — `agent.default_model` already supports this |

## Decisions

- **Hardcoded tiers, dynamic versions**: Don't hardcode specific model IDs (e.g. `claude-opus-4-5`). Instead filter the live model list by prefix so new model releases are picked up automatically.
- **Reuse `LanguageModelSelector`**: The existing selector already handles the icon, label, and persistence path. We just need to pass it a filtered model list.
- **Do not add a new settings key**: `agent.default_model` already persists across restarts; no schema changes needed.
- **Opus as default**: Change the cloud provider's fallback preference to Opus. Users who explicitly saved a Sonnet preference are unaffected (their saved value takes priority via the registry resolution order).
