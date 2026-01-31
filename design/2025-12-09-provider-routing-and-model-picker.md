# Provider Routing and Model Picker Improvements

**Date:** 2025-12-09
**Status:** Investigation / Planning
**Author:** Luke

## Background

Investigation into how the OpenAI-compatible endpoint handles provider routing when given model names with provider prefixes (e.g., `nebius/Qwen/Qwen3-Coder`).

## Current Provider Routing Logic

### Flow in `openai_chat_handlers.go`

```go
// 1. Parse provider prefix from model name
providerFromModel, modelWithoutPrefix := model.ParseProviderFromModel(chatCompletionRequest.Model)
// "nebius/Qwen/model" → provider="nebius", model="Qwen/model"

// 2. Check if prefix is a known provider
if providerFromModel != "" {
    if s.isKnownProvider(r.Context(), providerFromModel, ownerID) {
        validatedProvider = providerFromModel
        chatCompletionRequest.Model = modelWithoutPrefix
    }
    // If not known, treat whole string as model name
}
```

### `isKnownProvider` checks (in order):

1. **Global providers** (hardcoded): `openai`, `togetherai`, `anthropic`, `helix`, `vllm`
2. **System-owned providers**: Stored in DB with `owner=system` (from env vars)
3. **User-defined providers**: Custom endpoints owned by the requesting user

---

## Provider Endpoint Types (Architecture)

Provider endpoints have two key fields:
- `endpoint_type`: `global`, `user` (org/team are TODO, not implemented)
- `owner`: user ID for personal providers, `system` for env-configured providers

### Visibility Rules

| Endpoint Type | Owner | Visible To |
|--------------|-------|------------|
| `global` | anyone | **All users** (admins only can create) |
| `user` | user_id | **Only that user** |
| `global` | `system` | **All users** (from env vars like `OPENAI_API_KEY`) |

### Query Logic (`store_provider_endpoints.go`)

```go
// Returns: (user's personal providers) OR (all global providers)
query = query.Where("owner = ? AND endpoint_type = ?", q.Owner, types.ProviderEndpointTypeUser)
if q.WithGlobal {
    query = query.Or("endpoint_type = ?", types.ProviderEndpointTypeGlobal)
}
```

### Key Constraints

- **Only admins can create global endpoints** (`provider_handlers.go:419`)
- Non-admin users can only create `user` type endpoints (personal, visible only to them)
- Org-scoped providers (`endpoint_type = 'org'`) are **not implemented** - marked as TODO

---

## Issue 1: Provider Visibility - Clarified

### Observed Bug

```
failed to get client: failed to get client: no client found for provider: Qwen_Mrek, available providers: [helix]
```

### Root Cause Analysis

The user likely created a provider thinking it was "global", but:
1. **Only admins can create global endpoints** - non-admins get `endpoint_type = 'user'`
2. A `user` type provider is only visible to its owner
3. Other users won't see it in the provider list

### Resolution

If the provider needs to be globally available:
1. An admin must create it, OR
2. An admin must update the existing provider's `endpoint_type` to `global`

### Error Message Fix (Completed)

The error message was misleading - it only listed static global clients (`helix`), not database providers. Fixed in commit `23564fb13` to include all providers that were actually checked.

### Critical Bug: `isKnownProvider` Didn't Check Admin-Created Global Providers

**Root Cause Found:** The `isKnownProvider` function only checked:
1. Static global providers (openai, togetherai, anthropic, helix, vllm)
2. System-owned providers (`Owner = 'system'`)
3. User's OWN providers (`Owner = current_user_id`)

It did NOT check for admin-created global providers where `Owner = other_user_id` and `endpoint_type = 'global'`.

**Why "enqueuing" error occurred:**
1. Request with model `Qwen/Qwen3-Coder-30B...`
2. `isKnownProvider("Qwen", achraf_user_id)` returned FALSE (Issam's global provider not found)
3. Provider defaulted to `helix` (local scheduler)
4. Helix scheduler tried to look up `Qwen/...` in local model store
5. Model not found → "error enqueuing request: error getting model: not found"

**Fix:** Updated `isKnownProvider` to also check `ListProviderEndpoints` with `WithGlobal: true`, which includes admin-created global providers regardless of owner.

---

## Issue 2: Advanced Model Picker - Duplicate Model Names

### Problem

In the advanced model picker UI, if multiple providers offer a model with the same name (e.g., `gpt-4o` from both OpenAI and Azure), they appear as a single option. Users can't distinguish or select which provider's version they want.

### Current Behavior

```
Model Picker shows:
- gpt-4o          ← Which provider? Unclear!
- claude-sonnet-4-5-latest
- llama-3.1-70b
```

### Desired Behavior

**Show provider and model as separate UI components:**

```
Model Picker shows:
┌─────────────┬────────────────────────────────────┐
│ Provider    │ Model                              │
├─────────────┼────────────────────────────────────┤
│ openai      │ gpt-4o                             │
│ openai      │ gpt-4o-mini                        │
│ azure       │ gpt-4o                             │
│ anthropic   │ claude-sonnet-4-5-latest           │
│ together    │ llama-3.1-70b                      │
│ nebius      │ Qwen/Qwen3-Coder-30B-A3B-Instruct  │
└─────────────┴────────────────────────────────────┘
```

This makes it visually unambiguous which field is the provider and which is the model. The underlying storage continues to use separate `provider` and `model` fields.

### Implementation Considerations

1. **Model list API** - Should return provider info with each model
2. **Frontend display** - Show provider prefix or group by provider
3. **Selection value** - Store both provider and model in the selection
4. **Backwards compatibility** - Existing saved selections without provider prefix

### Implemented Fix

**Commit:** `feature/clone-task-across-projects` branch

Modified `AdvancedModelPicker.tsx` to show provider as a prominent chip/badge next to the model name:

**Before:**
```
[Provider Icon] gpt-4o
               openai (small secondary text)
```

**After:**
```
[Provider Icon] [openai] gpt-4o
                description (if any)
                pricing info (if available)
```

Changes:
1. Added a `<Chip>` component displaying `model.provider.name` in the primary text row
2. Removed duplicate provider name from secondary text (was shown twice)
3. Moved pricing info to secondary text (previously had provider name + pricing)
4. Added left margin to secondary text to align with model name

This makes it immediately clear which provider each model comes from, even when the same model name appears from multiple providers.

### Files Modified

- `frontend/src/components/create/AdvancedModelPicker.tsx` - Added provider chip to model list items

---

## Issue 3: Model Names That Look Like Provider Prefixes

### Customer Error

```
500 error enqueuing request: error getting model: not found
```

This error occurs when:
1. Model is stored/sent as `Qwen/Qwen3-Coder-30B-A3B-Instruct` (full HF model ID)
2. `ParseProviderFromModel` extracts `Qwen` as provider, `Qwen3-Coder-30B-A3B-Instruct` as model
3. `isKnownProvider("Qwen")` returns **TRUE** because there's a provider configured called `qwen`
4. Request routes to `qwen` provider with stripped model name `Qwen3-Coder-30B-A3B-Instruct`
5. **Model not found** - because the actual model name is `Qwen/Qwen3-Coder-30B-A3B-Instruct`, not `Qwen3-Coder-30B-A3B-Instruct`

The bug: A provider named `qwen` exists in the system, so the HF model ID `Qwen/Qwen3-Coder` gets incorrectly parsed as routing to that provider.

### Problem

Some model names naturally contain slashes (Hugging Face model IDs):
- `Qwen/Qwen3-Coder-30B-A3B-Instruct`
- `meta-llama/Meta-Llama-3.1-70B-Instruct`
- `mistralai/Mistral-7B-Instruct-v0.3`

The current `ParseProviderFromModel` extracts the first segment as a potential provider prefix:
- `Qwen/Qwen3-Coder` → provider=`Qwen`, model=`Qwen3-Coder`

If someone creates a provider called "Qwen" (to route Qwen models to a specific endpoint), this creates ambiguity:

```
User sends: model="Qwen/Qwen3-Coder-30B"

Current behavior:
1. ParseProviderFromModel → provider="Qwen", model="Qwen3-Coder-30B"
2. isKnownProvider("Qwen") → TRUE (user created a Qwen provider)
3. Routes to "Qwen" provider with model="Qwen3-Coder-30B"

But what if user meant:
- Route to default provider with model="Qwen/Qwen3-Coder-30B" (the full HF model ID)
```

### The Ambiguity

| Input | User Intent A | User Intent B |
|-------|---------------|---------------|
| `Qwen/Qwen3-Coder` | Provider: Qwen, Model: Qwen3-Coder | Provider: (default), Model: Qwen/Qwen3-Coder |
| `openai/gpt-4o` | Provider: openai, Model: gpt-4o | N/A (gpt-4o isn't a HF model) |

### Possible Solutions

**Option A: Require explicit provider prefix syntax**
```
provider::model    ← Provider routing
provider/model     ← Treated as model name only
```
Example: `nebius::Qwen/Qwen3-Coder` routes to nebius with model `Qwen/Qwen3-Coder`

**Option B: Check if the full string is a known model first**
```go
// Before parsing prefix, check if full model name exists in any provider
if modelExistsInAnyAccessibleProvider(fullModelName) {
    // Don't parse prefix, use full name as model
    return
}
// Otherwise, try parsing prefix
```

**Option C: Only parse prefix for known providers**
Current behavior, but document that HF-style model IDs may conflict with custom provider names.

**Option D: Require explicit model selector in UI**
When user selects a model, always store both provider and model explicitly. Only fallback to prefix parsing for raw API calls.

### Implemented Fix: Option B (Check cached model list first)

**Commit:** `feature/clone-task-across-projects` branch

Added `findProviderWithModel()` function to `openai_chat_handlers.go` that:
1. Before parsing provider prefix, checks if the full model name exists in any accessible provider's model list
2. **First checks global providers** (helix, openai, togetherai, anthropic, vllm) from env vars via cached model lists
3. **Then checks database-stored providers** via cached model lists AND static `Models` field
4. If found, uses that provider AND keeps the full model name (no prefix stripping)
5. If not found in any provider, falls back to existing prefix parsing logic

**Implementation:**
```go
// Before parsing prefix:
if strings.Contains(chatCompletionRequest.Model, "/") {
    foundProvider := s.findProviderWithModel(ctx, chatCompletionRequest.Model, ownerID)
    if foundProvider != "" {
        validatedProvider = foundProvider
        // Keep full model name - don't strip prefix
    }
}

// findProviderWithModel checks (in order):
// 1. Global providers from env vars (helix, openai, etc.) with cache key "provider:system"
// 2. DB providers' cached model lists with cache key "provider:owner"
// 3. DB providers' static Models field
```

**How the cache works:**
- Cache key: `"{provider_name}:{owner}"`
- Cache value: JSON-encoded `[]types.OpenAIModel`
- TTL: Configured via `s.Cfg.WebServer.ModelsCacheTTL`
- Populated when: UI calls `/api/v1/provider-endpoints?with_models=true`

**Architecture Note:** There are three types of providers:

1. **Static env-var providers** (helix, openai, togetherai, anthropic, vllm)
   - Created from env vars like `OPENAI_API_KEY`, `TOGETHER_API_KEY`, etc.
   - NOT stored in database, managed by ProviderManager in-memory
   - Cache key: `"{provider}:system"`

2. **Dynamic env-var providers** (`DYNAMIC_PROVIDERS` env var)
   - Format: `provider1:api_key1:base_url1,provider2:api_key2:base_url2`
   - Stored in database with `Owner: "system"`, `EndpointType: "global"`
   - Cache key: `"{provider}:system"`

3. **User/Admin-created providers** (via UI or API)
   - Stored in database with `Owner: user_id` or admin-created global endpoints
   - Cache key: `"{provider}:{owner}"`

**Model lists per provider:**
- `Models` (pq.StringArray) - Stored in database, configured by admin at provider creation
- Cached model list - In-memory only (Ristretto), populated when fetching from provider's `/v1/models`

The fix checks all three provider types via their cached model lists, plus the static `Models` field for DB providers.

### Files Modified

- `api/pkg/server/openai_chat_handlers.go` - Added `findProviderWithModel()` and modified prefix parsing logic

---

## Related Fix: Helix→Zed Provider Mapping

**Commit:** `10806e24c` on `fix/multi-provider-model-routing`

Fixed the issue where Helix provider names (like "Nebius") were passed directly to Zed's settings, but Zed only recognizes a fixed set of providers.

### Solution

Added `mapHelixToZedProvider()` function:
- `anthropic` → Zed's `anthropic` provider, model normalized to `-latest`
- Everything else → Zed's `openai` provider, model prefixed with `provider/`

### Example Transformations

| Helix Provider | Model | → Zed Provider | → Zed Model |
|---------------|-------|----------------|-------------|
| `anthropic` | `claude-sonnet-4-5` | `anthropic` | `claude-sonnet-4-5-latest` |
| `openai` | `gpt-4o` | `openai` | `openai/gpt-4o` |
| `Nebius` | `Qwen/Qwen3-Coder` | `openai` | `Nebius/Qwen/Qwen3-Coder` |

---

## Issue 4: Provider Edit Dialog Cannot Change Name

### Problem

The provider edit dialog in the frontend does not allow changing the provider name. This is confusing when:
1. A provider was created with a name that conflicts with HuggingFace model ID prefixes (e.g., `Qwen`)
2. User wants to rename to avoid conflicts (e.g., `qwen-endpoint`)
3. The UI doesn't show the name field as editable

### Resolution

Updated the provider edit dialog to allow name changes. This required both backend and frontend changes.

### Implemented Fix

**Backend changes:**
1. Added `Name` field to `UpdateProviderEndpoint` struct in `api/pkg/types/provider.go`
2. Added name update logic with duplicate validation in `api/pkg/server/provider_handlers.go`
   - Only updates name if provided and different from existing
   - Checks for duplicate names among user's accessible providers

**Frontend changes:**
1. Added `name` to formData state in `EditProviderEndpointDialog.tsx`
2. Added editable Name TextField with validation
3. Included name in the submit body

### Files Modified

- `api/pkg/types/provider.go` - Added `Name` field to `UpdateProviderEndpoint`
- `api/pkg/server/provider_handlers.go` - Added name update logic with duplicate check
- `frontend/src/components/dashboard/EditProviderEndpointDialog.tsx` - Added name field to UI

---

## Issue 5: Background Model Cache Refresh

### Problem

The `findProviderWithModel()` function relies on cached model lists to correctly handle HuggingFace-style model IDs. However, the cache was only populated when:
1. A user opened the model picker in the UI (calls `/api/v1/provider-endpoints?with_models=true`)
2. The cache hadn't expired (TTL: 1 minute by default)

This created a gap for:
- **API-only clients** (like Qwen Coder hitting the Helix OpenAI endpoint directly) - never triggered cache population
- **Server restarts** - cache would be empty until a UI user opened the model picker
- **Providers coming back online** - if a provider was down at startup, its models wouldn't be cached

### Implemented Fix

Added `StartModelCacheRefresh()` function that runs as a background goroutine:

```go
// Runs immediately on startup, then periodically based on ModelsCacheTTL
func (s *HelixAPIServer) StartModelCacheRefresh(ctx context.Context)

// Called by StartModelCacheRefresh to refresh all provider model lists
func (s *HelixAPIServer) refreshAllProviderModels(ctx context.Context)
```

**Behavior:**
1. Runs immediately on server startup
2. Runs periodically (interval = max(ModelsCacheTTL, 30s))
3. Fetches model lists from all providers (global env-var providers + database providers)
4. Caches results with TTL
5. Handles errors gracefully (logs and continues if a provider is down)
6. Logs success/error counts for monitoring

**Log output example:**
```
starting background model cache refresh                    refresh_interval=1m0s
completed model cache refresh                              success_count=3 error_count=1 duration=2.5s
```

### Files Modified

- `api/pkg/server/provider_handlers.go` - Added `StartModelCacheRefresh()` and `refreshAllProviderModels()`
- `api/pkg/server/server.go` - Call `StartModelCacheRefresh()` on server startup

---

## Summary

All issues have been addressed:

| Issue | Description | Status |
|-------|-------------|--------|
| Issue 1 | `isKnownProvider` didn't check admin-created global providers | ✅ Fixed |
| Issue 2 | Model picker shows duplicate model names without provider distinction | ✅ Fixed (provider chip added) |
| Issue 3 | HuggingFace model IDs incorrectly parsed as provider prefixes | ✅ Fixed (`findProviderWithModel()`) |
| Issue 4 | Provider edit dialog cannot change name | ✅ Fixed |
| Issue 5 | Model cache only populated on UI access | ✅ Fixed (background refresh) |

## Next Steps

1. [ ] Deploy and test changes in staging
2. [ ] Monitor logs for `completed model cache refresh` to verify background task is running
