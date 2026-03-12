# Design: Fix Context Length and API Key in Settings-Sync-Daemon

## Root Cause Analysis

### Problem 1: Wrong context length (128K instead of 200K)

**The real issue is simpler than it first appears.** Zed already has built-in definitions for all current Claude models with correct context lengths, output limits, cache configs, etc. The daemon's `injectAvailableModels()` is *overriding* those correct built-ins with a broken `Custom` model entry.

**How Zed's model selection works (`zed/crates/language_models/src/provider/anthropic.rs` → `provided_models()`):**

1. Populate a `BTreeMap` with all built-in models, keyed by `model.id()` (e.g. `"claude-opus-4-6-latest"`)
2. Then iterate `available_models` from settings.json and **insert into the same map**, keyed by `model.name`
3. If a settings entry has the same key as a built-in, it **replaces** the built-in with a `Custom` model

The daemon injects `name: "claude-opus-4-6"` which is a *different* key from the built-in `"claude-opus-4-6-latest"`, so it doesn't replace — it adds a **second, broken model** alongside the correct built-in. This Custom model has wrong context (128K fallback), no cache configuration, no beta headers, etc.

**The chain of failure in detail:**

1. `buildCodeAgentConfigFromAssistant()` tries `GetModelInfo()` with model name `claude-opus-4-6`
2. model_info.json (OpenRouter) uses dot-format slugs (`anthropic/claude-opus-4.6`) — lookup fails due to dash vs dot mismatch
3. `MaxTokens` stays 0 → daemon falls back to hardcoded `128000`
4. Daemon writes `available_models: [{name: "claude-opus-4-6", max_tokens: 128000}]` to settings.json
5. Zed creates a `Custom` model from this, missing all the metadata the built-in has

**But none of this matters for built-in models.** Zed already knows `claude-opus-4-6` — it has correct values hardcoded. The daemon shouldn't be injecting it at all.

**Authoritative context lengths (from Zed built-ins in `zed/crates/anthropic/src/anthropic.rs`):**

| Model | ID | Context | Max Output |
|-------|-----|---------|------------|
| ClaudeOpus4_6 | `claude-opus-4-6-latest` | **200,000** | 128,000 |
| ClaudeOpus4_6_1mContext | `claude-opus-4-6-1m-context-latest` | **1,000,000** | 128,000 |
| ClaudeOpus4_5 | `claude-opus-4-5-latest` | 200,000 | 64,000 |
| ClaudeSonnet4_6 | `claude-sonnet-4-6-latest` | 200,000 | 64,000 |
| ClaudeSonnet4 | `claude-sonnet-4-latest` | 200,000 | 64,000 |

The 1M variants are explicit opt-in (separate model entries). Do NOT trust model_info.json (OpenRouter) for context lengths — it conflates the 1M variant into the base model.

### Problem 1b: `normalizeModelIDForZed` missing 4.6 models

`normalizeModelIDForZed()` in `api/pkg/external-agent/zed_config.go` converts model names to `-latest` format for Zed's `default_model` config. It has cases for `claude-opus-4-5`, `claude-sonnet-4-5`, `claude-haiku-4-5`, but **no cases for any 4.6 models**. So `claude-opus-4-6` passes through unnormalized. Zed's `Model::from_id()` still resolves it (via `id.starts_with("claude-opus-4-6")`), but we should be consistent and add the 4.6 entries.

### Problem 2: api_key in settings.json is dead config

Zed's `AnthropicSettings` struct only has two fields:
```rust
pub struct AnthropicSettings {
    pub api_url: String,
    pub available_models: Vec<AvailableModel>,
}
```

There is **no `api_key` field**. Zed reads API keys from the `ANTHROPIC_API_KEY` env var (via `ApiKeyState` in `api_key.rs`) or the system keychain. The env var is already set by `DesktopAgentAPIEnvVars()`.

The daemon's `injectLanguageModelAPIKey()` writes a token to a file where nothing reads it.

### Problem 3: Contradictory intent between API handler and daemon

The zed-config API handler (`zed_config_handlers.go` line 166) deliberately omits `api_key`:
```go
// api_key is NOT included here — Zed reads ANTHROPIC_API_KEY / OPENAI_API_KEY from
// container env vars (set by DesktopAgentAPIEnvVars). Only api_url is needed in settings.
```

But then the daemon's `injectLanguageModelAPIKey()` re-injects it, contradicting that deliberate omission.

## Solution Design

### Fix 1: Stop injecting built-in models into `available_models`

**Location:** `api/cmd/settings-sync-daemon/main.go` → `injectAvailableModels()`

The simplest correct fix: **skip injection for models that Zed already has built-in definitions for.** Zed knows all current Claude, GPT, and Gemini models. The `available_models` mechanism is only needed for truly custom models (e.g. `helix/qwen3:8b` routed through OpenAI-compatible API).

**Approach:** Before injecting into `available_models`, check if the model name matches a known Zed built-in by checking the provider's API type. For `anthropic` provider models, Zed has built-in definitions for all Claude models — `injectAvailableModels()` should skip injection when `APIType == "anthropic"`. For `openai` provider models going through the OpenAI-compatible proxy with a `provider/model` prefix, Zed won't have a built-in and injection is still needed.

This is much better than trying to get the right context length from model_info.json (which has wrong values anyway). The built-in model has the correct context length, cache config, beta headers, thinking mode support — everything. A `Custom` model from `available_models` has none of that.

**Fallback for truly custom models:** Keep `injectAvailableModels()` for non-built-in models, but update the fallback default from `128000` to `200000`.

### Fix 2: Complete `normalizeModelIDForZed` for all Anthropic model IDs

**Location:** `api/pkg/external-agent/zed_config.go` → `normalizeModelIDForZed()`

The actual model IDs from Anthropic's API (fetched via `curl -H "anthropic-version: 2023-06-01" -H "x-api-key: ..." http://api:8080/v1/models`) are:

| Anthropic model ID | `normalizeModelIDForZed` result | Correct? |
|---|---|---|
| `claude-sonnet-4-6` | unchanged (no match) | ❌ |
| `claude-opus-4-6` | unchanged (no match) | ❌ |
| `claude-opus-4-5-20251101` | `claude-opus-4-5-latest` | ✅ |
| `claude-haiku-4-5-20251001` | `claude-haiku-4-5-latest` | ✅ |
| `claude-sonnet-4-5-20250929` | `claude-sonnet-4-5-latest` | ✅ |
| `claude-opus-4-1-20250805` | unchanged (no match) | ❌ |
| `claude-opus-4-20250514` | unchanged (no match) | ❌ |
| `claude-sonnet-4-20250514` | unchanged (no match) | ❌ |
| `claude-3-haiku-20240307` | `claude-3-haiku-latest` | ✅ |

Only the 4.5-era and 3.x models are handled. Add the missing cases (order matters — more specific prefixes must come first to avoid false matches):
- `claude-opus-4-6*` → `claude-opus-4-6-latest`
- `claude-sonnet-4-6*` → `claude-sonnet-4-6-latest`
- `claude-opus-4-1*` → `claude-opus-4-1-latest`
- `claude-opus-4*` (after 4-1, 4-5, 4-6 checks) → `claude-opus-4-latest`
- `claude-sonnet-4*` (after 4-5, 4-6 checks) → `claude-sonnet-4-latest`

This ensures the `default_model` in agent settings uses the canonical `-latest` ID that exactly matches Zed's built-in model keys for every model the user can select from Anthropic's API.

### Fix 3: Remove `injectLanguageModelAPIKey`

**Location:** `api/cmd/settings-sync-daemon/main.go`

1. Delete the `injectLanguageModelAPIKey()` method entirely.
2. Remove calls in `syncFromHelix()` (~line 737) and `checkHelixUpdates()` (~line 1113).
3. Remove corresponding test.

No replacement needed — `ANTHROPIC_API_KEY` env var already handles auth.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| How to fix context length | Don't inject built-in models into `available_models` | Zed's built-ins are authoritative — injecting creates a worse `Custom` model that's missing cache config, beta headers, etc. |
| How to identify built-in models | Check `APIType == "anthropic"` (and similar for other native providers) | Simple, correct — all Anthropic models have built-in Zed definitions |
| `normalizeModelIDForZed` | Add 4.6 model entries | Consistency with existing 4.5 entries; ensures `default_model` exactly matches built-in IDs |
| `api_key` in settings.json | Remove entirely | Zed ignores it; unnecessary token exposure |
| Fallback for custom models | 200,000 (from 128,000) | Safer default for non-built-in models; rarely hit |

## Files Changed

| File | Change |
|------|--------|
| `api/cmd/settings-sync-daemon/main.go` | Skip `injectAvailableModels()` for models with native Zed provider support (anthropic); delete `injectLanguageModelAPIKey()`; update fallback from 128K to 200K |
| `api/cmd/settings-sync-daemon/main_test.go` | Update tests: remove api_key assertions; add test that anthropic models are NOT injected into available_models; keep test that custom models ARE injected |
| `api/pkg/external-agent/zed_config.go` | Add `claude-opus-4-6` and `claude-sonnet-4-6` cases to `normalizeModelIDForZed()` |
| `api/pkg/external-agent/zed_config_test.go` | Add test cases for 4.6 model normalization |

## What We're NOT Changing

- **`GetModelInfo()` / model_info.go** — The dash-vs-dot lookup failure in model_info.json is a real bug, but fixing it doesn't help here because OpenRouter's context lengths are wrong anyway (1M for base claude-opus-4.6 when Anthropic says 200K). The right fix is to not inject built-in models at all, making the lookup irrelevant for this case. The model_info lookup bug can be fixed separately if needed for other consumers (e.g. billing/pricing).
- **`CodeAgentConfig.MaxTokens`** — Still populated from model_info when available (for non-built-in models), but no longer drives settings.json for built-in models.

## Codebase Patterns Discovered

- **Zed's `provided_models()` merges built-ins with `available_models` from settings.** Settings entries override built-ins by key. Built-in IDs use `-latest` suffix (e.g. `claude-opus-4-6-latest`); injected names don't — so they create duplicate entries rather than overriding. Either way, injecting built-in models is wrong.
- **`normalizeModelIDForZed()`** maintains a manual mapping of model prefixes to `-latest` IDs. New model versions need to be added here. The 4.6 entries were missed.
- **Zed settings.json schema is strict.** `AnthropicSettings` has only `api_url` + `available_models`. No `api_key` field exists. Auth is env var or keychain.
- **`DesktopAgentAPIEnvVars()`** is the canonical auth injection point — sets `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, etc. as container env vars.
- **model_info.json (OpenRouter) is unreliable for context lengths.** It conflates extended-context variants into base models. Zed's built-in definitions are authoritative for models it knows about.