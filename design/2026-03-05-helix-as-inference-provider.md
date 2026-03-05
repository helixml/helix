# Helix as OpenAI-Compatible Inference Provider

**Date:** 2026-03-05
**Status:** Draft
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

When called with Anthropic headers, query the Anthropic provider's cached model list and return in Anthropic API format:

```json
{
  "data": [
    {"id": "claude-haiku-4-5-20251001", "type": "model", "display_name": "Claude Haiku 4.5", "created_at": "2025-10-01T00:00:00Z"},
    {"id": "claude-opus-4-5-20251101", "type": "model", "display_name": "Claude Opus 4.5", "created_at": "2025-11-01T00:00:00Z"}
  ],
  "has_more": false,
  "first_id": "claude-haiku-4-5-20251001",
  "last_id": "claude-opus-4-5-20251101"
}
```

Source: Use the existing provider model cache from `refreshAllProviderModels`. Filter to Anthropic provider only.

### 3. OpenAI model list includes provider-prefixed models

When called without Anthropic headers, aggregate models from all providers:

- Internal helix models: unprefixed (`Qwen/Qwen2.5-VL-3B-Instruct`)
- OpenAI models: prefixed (`openai/gpt-4o`, `openai/gpt-4o-mini`)
- Nebius models: prefixed (`nebius/llama-3.3-70b`)
- TogetherAI models: prefixed (`togetherai/...`)
- **NOT Anthropic** — Anthropic models are served separately via the Anthropic-format endpoint

Source: Use the existing provider model cache. The `refreshAllProviderModels` background job already fetches and caches all provider models every 60s.

### 4. Fix `isAnthropicProvider` and `listAnthropicModels`

Currently hardcoded to `api.anthropic.com`:

```go
// Current (broken for proxy use)
func isAnthropicProvider(baseURL string) bool {
    return strings.Contains(baseURL, "https://api.anthropic.com")
}

func (c *RetryableClient) listAnthropicModels(ctx context.Context) ([]types.OpenAIModel, error) {
    url := "https://api.anthropic.com/v1/models"  // hardcoded!
    ...
}
```

Fix: Use `c.baseURL` and detect by provider type, not URL pattern. This allows the inner Helix to point its Anthropic provider at `http://outer-api:8080/v1` and have model listing work.

### Files to modify

| File | Change |
|------|--------|
| `api/pkg/server/server.go` | No route changes needed — `/v1/models` handler does detection |
| `api/pkg/server/openai_chat_handlers.go` | Split `listModels` into dual-mode with header detection |
| `api/pkg/openai/helix_openai_server.go` | `ListModels()` — aggregate provider-prefixed models |
| `api/pkg/openai/openai_client_anthropic.go` | Fix `isAnthropicProvider()` and `listAnthropicModels()` to use `c.baseURL` |
| `api/pkg/server/provider_handlers.go` | Expose cached provider models for the model list handlers |

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

## Testing

1. **Dual-mode `/v1/models`:**
   - `curl /v1/models -H "Authorization: Bearer ..."` → returns OpenAI-format list with prefixed models
   - `curl /v1/models -H "x-api-key: ..." -H "anthropic-version: 2023-06-01"` → returns Anthropic-format list
2. **Inner Helix discovery:**
   - Inner Helix onboarding model picker shows Claude models AND OpenAI/Nebius models
3. **Anthropic caching:**
   - Create project with Claude model → verify requests go through `/v1/messages` (not `/v1/chat/completions`)
   - Check outer Helix logs for Anthropic cache hit headers
4. **No regression:**
   - Existing direct Anthropic/OpenAI configurations still work
   - Unprefixed internal models still work