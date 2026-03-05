# Helix as OpenAI-Compatible Inference Provider

**Date:** 2026-03-05
**Status:** Implemented
**Author:** Claude

## Problem Statement

When using Helix as an inference provider for another Helix instance (e.g., Helix-in-Helix development), the inner Helix needs to discover and use all models configured on the outer Helix. Today this is broken:

1. **`/v1/models` only returns internal models** — Models from configured providers (Anthropic, OpenAI, Nebius, etc.) are missing. The inner Helix sees only `Qwen/Qwen2.5-VL-3B-Instruct` etc.

2. **No Anthropic passthrough** — Anthropic models MUST use the native Anthropic API format (`/v1/messages`) to preserve prompt caching. Going through the OpenAI-compatible `/v1/chat/completions` endpoint silently disables caching and makes everything ~100x more expensive.

3. **Route collision at `/v1/models`** — Both OpenAI and Anthropic APIs use `GET /v1/models` as their model list endpoint, but with different auth headers and response formats. Currently only the OpenAI format is served.

## Architecture

The inner Helix needs **two** provider configurations pointing at the outer Helix:

```
┌─────────────────────────────────────────────────────────────┐
│ Inner Helix                                                  │
│                                                              │
│  OPENAI_BASE_URL=http://outer:8080/v1                       │
│    → GET /v1/models (OpenAI format)                         │
│    → POST /v1/chat/completions                              │
│    → Models: openai/gpt-4o, nebius/llama-3.3-70b, Qwen/... │
│                                                              │
│  ANTHROPIC_BASE_URL=http://outer:8080/v1                    │
│    → GET /v1/models (Anthropic format, via header detection)│
│    → POST /v1/messages (native Anthropic, prompt caching!)  │
│    → Models: claude-haiku-4-5, claude-opus-4-5, ...         │
│                                                              │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│ Outer Helix                                                  │
│                                                              │
│  /v1/models         → dual-mode: detects Anthropic headers  │
│  /v1/chat/completions → OpenAI-compatible (all providers)   │
│  /v1/messages       → Anthropic native passthrough          │
│                                                              │
│  Configured providers: Anthropic, OpenAI, Nebius, Helix     │
└─────────────────────────────────────────────────────────────┘
```

### Why two separate provider configs?

- **Anthropic prompt caching** requires the native Anthropic API format. Requests routed through `/v1/chat/completions` with an `anthropic/` prefix go through an OpenAI→Anthropic translation layer that strips cache control headers. This silently breaks caching and dramatically increases costs.
- **No prefix for Anthropic models** — The inner Helix's Anthropic provider sends `claude-haiku-4-5-20251001` directly via `/v1/messages`, just like talking to `api.anthropic.com`. No `anthropic/` prefix needed.
- **Prefix for OpenAI-compatible models** — `openai/gpt-4o`, `nebius/llama-3.3-70b`, etc. The outer Helix's `/v1/chat/completions` already supports this routing.

## Changes Required

### 1. Dual-mode `/v1/models` endpoint (outer Helix)

The `/v1/models` handler must detect whether the caller wants OpenAI or Anthropic format:

**Detection heuristic:** If the request includes an `anthropic-version` header, return Anthropic-format model list. Otherwise, return OpenAI-format list.

| Header present | Response format | Models returned |
|---------------|----------------|-----------------|
| `anthropic-version` | Anthropic (`{data: [{id, type, display_name, created_at}]}`) | Anthropic provider models only (unprefixed) |
| (none / `Authorization: Bearer`) | OpenAI (`{data: [{id, object, owned_by}]}`) | All OpenAI-compatible provider models (prefixed) + internal helix models (unprefixed) |

**File:** `api/pkg/server/openai_chat_handlers.go` (or wherever `listModels` lives)

```go
func (s *HelixAPIServer) listModels(w http.ResponseWriter, r *http.Request) {
    if r.Header.Get("anthropic-version") != "" {
        s.listModelsAnthropic(w, r)
        return
    }
    s.listModelsOpenAI(w, r)
}
```

### 2. Anthropic model list handler

When called with `anthropic-version` header, **proxy the request to the upstream Anthropic provider** (api.anthropic.com or configured ANTHROPIC_BASE_URL). This preserves exact API compatibility and ensures model lists are always up-to-date.

Implementation: `listModelsAnthropic()` in `openai_model_handlers.go` uses the existing `anthropicProxy` reverse proxy infrastructure.

### 3. OpenAI model list includes provider-prefixed models

When called without Anthropic headers, aggregate models from all providers:

- Internal helix models: unprefixed (`Qwen/Qwen2.5-VL-3B-Instruct`) — **only if at least one runner is connected**
- OpenAI models: prefixed (`openai/gpt-4o`, `openai/gpt-4o-mini`)
- Nebius models: prefixed (`nebius/llama-3.3-70b`)
- TogetherAI models: prefixed (`togetherai/...`)
- **NOT Anthropic** — Anthropic models are served separately via the Anthropic-format endpoint

Implementation: `aggregateAllProviderModels()` in `openai_model_handlers.go` collects from all providers, `prefixModels()` adds provider prefixes, `hasConnectedRunners()` checks scheduler for active runners.

Source: Uses the existing provider model cache. The `refreshAllProviderModels` background job already fetches and caches all provider models every 60s.

### 4. Fix `listAnthropicModels` to use `c.baseURL`

Previously hardcoded to `api.anthropic.com`:

```go
// Before (broken for proxy use)
func (c *RetryableClient) listAnthropicModels(ctx context.Context) ([]types.OpenAIModel, error) {
    url := "https://api.anthropic.com/v1/models"  // hardcoded!
    ...
}
```

Fixed to use `c.baseURL`:

```go
// After (works with any Anthropic-compatible endpoint)
func (c *RetryableClient) listAnthropicModels(ctx context.Context) ([]types.OpenAIModel, error) {
    baseURL := c.baseURL
    if baseURL == "" {
        baseURL = "https://api.anthropic.com/v1"
    }
    // ... normalize URL and append /models
}
```

Also removed hardcoded 200K context length — model info provider fills this in via `getProviderModels()`.

### Files modified

| File | Change |
|------|--------|
| `api/pkg/server/openai_model_handlers.go` | New dual-mode `listModels()` with `listModelsAnthropic()` and `listModelsOpenAI()`, `aggregateAllProviderModels()`, `hasConnectedRunners()`, `prefixModels()` |
| `api/pkg/openai/openai_client_anthropic.go` | Fixed `listAnthropicModels()` to use `c.baseURL`, removed hardcoded context length |

## Helix-in-Helix Configuration

After the fix, the inner Helix `.env` needs:

```bash
# OpenAI-compatible provider (rolls up OpenAI, Nebius, Helix internal models)
OPENAI_API_KEY=<outer-helix-api-key>
OPENAI_BASE_URL=http://outer-api:8080/v1

# Anthropic provider (native API passthrough, preserves prompt caching)
ANTHROPIC_API_KEY=<outer-helix-api-key>
ANTHROPIC_BASE_URL=http://outer-api:8080/v1

# Network: make outer-api resolvable from inner containers
OUTER_API_IP=<ip-of-outer-helix-api-container>
```

Both use the same outer Helix URL and API key. The outer Helix differentiates based on:
- `/v1/chat/completions` → OpenAI format → routes by model prefix
- `/v1/messages` → Anthropic format → routes to Anthropic provider
- `/v1/models` + `anthropic-version` header → Anthropic model list
- `/v1/models` without → OpenAI model list (with prefixed provider models)

## What Already Works

- `/v1/messages` on outer Helix → proxies to Anthropic (native format, caching preserved)
- `/v1/chat/completions` with `openai/gpt-4o` → routes to OpenAI provider
- `/v1/chat/completions` with `Qwen/Qwen2.5-VL-3B-Instruct` → routes to internal scheduler
- Provider model cache (`refreshAllProviderModels`) → fetches all provider models every 60s

## Testing Strategy

We are developing inside a Helix-in-Helix session: an inner Helix dev stack running inside a desktop container managed by the outer (production) Helix. This gives us a natural test setup.

### What we can test before deploying to outer Helix

These are inner-Helix-side changes only:

1. **`isAnthropicProvider` / `listAnthropicModels` fix** — Verify the Anthropic client uses `c.baseURL` instead of hardcoded `api.anthropic.com`. Confirm Go compiles, unit test if feasible.
2. **OpenAI model list aggregation** — Implement in `helix_openai_server.go`, verify it compiles and includes prefixed models from the provider cache.
3. **Dual-mode `/v1/models` handler** — Implement header detection, verify it compiles.

### What requires deploying to outer Helix first

The outer Helix needs the dual-mode `/v1/models` and provider-prefixed model aggregation deployed before the inner Helix can discover models through it.

**Deployment flow:**
1. Implement all changes on this branch
2. User merges/deploys to outer Helix (can be done while retaining this session)
3. Restart inner Helix API (`docker compose -f docker-compose.dev.yaml up -d api`)

### End-to-end verification (after outer Helix deploy)

All run from inside this dev session:

```bash
# 1. Dual-mode /v1/models on outer Helix
# OpenAI format — should include prefixed models from all OpenAI-compatible providers
curl -s "http://outer-api:8080/v1/models" \
  -H "Authorization: Bearer $OPENAI_API_KEY" | jq '.data[].id'
# Expected: "Qwen/Qwen2.5-VL-3B-Instruct", "openai/gpt-4o", "nebius/llama-3.3-70b", ...
# NOT expected: any "anthropic/claude-*" (those go on the Anthropic endpoint)

# Anthropic format — should return Anthropic models unprefixed
curl -s "http://outer-api:8080/v1/models" \
  -H "x-api-key: $ANTHROPIC_API_KEY" \
  -H "anthropic-version: 2023-06-01" | jq '.data[].id'
# Expected: "claude-haiku-4-5-20251001", "claude-opus-4-5-20251101", ...

# 2. Inner Helix provider discovery
# Should show both providers with models
curl -s "http://localhost:8080/api/v1/provider-endpoints?with_models=true" \
  -H "Cookie: access_token=..." | jq '.[].name, .[].status'
# Expected: "openai" "ok", "anthropic" "ok"

# 3. Anthropic native format preserved
# Send a request through inner Helix that routes to outer Helix's /v1/messages
# Check outer Helix API logs for:
#   - Request going to /v1/messages (NOT /v1/chat/completions)
#   - Anthropic cache headers in response

# 4. UI verification
# - Open http://localhost:8080/onboarding
# - Model picker should show Claude AND OpenAI/Nebius models
# - Create project with claude-haiku-4-5-20251001
# - Start a task, verify agent completes planning phase
```

### Regression checks

- Direct Anthropic/OpenAI configurations (not via outer Helix) still work
- Unprefixed internal helix models still work via `/v1/chat/completions`
- Frontend provider endpoints page unchanged
- Existing Zed agent sessions unaffected