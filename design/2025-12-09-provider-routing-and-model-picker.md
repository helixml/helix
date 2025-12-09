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

### Key Finding

The provider lookup is **case-sensitive**. If the model prefix is `Nebius` but the provider is stored as `nebius`, it won't match. This could cause routing failures.

---

## Issue 1: Provider Access Control When No Prefix Provided

### Problem

When a user sends a model name **without** a provider prefix (e.g., `gpt-4o`), we need to:
1. Find which provider(s) offer this model
2. Select one that the user has access to
3. **Ensure we don't route to a provider the user can't access**

### Observed Bugs

**Bug A: Helix Sessions UI - No client found**

When trying to use a "global" inference provider created by another user via the Helix Sessions UI:

```
failed to get client: failed to get client: no client found for provider| Qwen_Mrek, available providers: [helix]
```

This shows that:
1. User A created a provider called `Qwen_Mrek` (intended to be globally available)
2. User B tries to use it via the model picker
3. Provider manager only sees `[helix]` as available
4. Request fails - the "global" provider isn't actually visible to other users

**Bug B: Qwen in Zed - Model not found**

When using Qwen Code agent with a correctly prefixed model:

```
error getting model: not found
```

This shows that:
1. Model is set to `Qwen_Achraf/Qwen/Qwen3-Coder` (correctly prefixed)
2. Parses correctly: provider=`Qwen_Achraf`, model=`Qwen/Qwen3-Coder`
3. Routes to `Qwen_Achraf` provider
4. But model `Qwen/Qwen3-Coder` isn't in the provider's model list

Possible causes:
- Provider's cached model list doesn't include this model
- Model name mismatch (e.g., provider lists it as `Qwen3-Coder` not `Qwen/Qwen3-Coder`)
- Provider model list refresh issue

### Questions to Investigate

1. How are "global" user-created providers supposed to work?
2. Is there a flag to make a user-created provider visible to all users?
3. When iterating through available providers to find a model match, are we filtering by user access?
4. Could a request accidentally route to another user's personal provider endpoint?
5. Is there proper isolation between:
   - Global providers (available to all) - hardcoded: openai, anthropic, etc.
   - System providers (available to all, configured via env)
   - User providers (only available to the owner)
   - "Shared" user providers (created by user, visible to all?) - does this exist?

### Desired Behavior

```
User sends: model="gpt-4o" (no prefix)

1. Get list of providers user can access:
   - Global providers (openai, anthropic, etc.)
   - System providers (from env vars)
   - User's own custom providers

2. For each accessible provider, check if it offers "gpt-4o"

3. Select first matching provider (or use priority/preference)

4. NEVER route to another user's personal provider
```

### Files to Investigate

- `api/pkg/server/openai_chat_handlers.go` - Main routing logic
- `api/pkg/controller/inference.go` - Provider selection
- `api/pkg/openai/manager/provider_manager.go` - Provider client management
- `api/pkg/store/store.go` - Provider endpoint queries

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

### Files to Investigate

- Frontend model picker component
- `api/pkg/server/provider_handlers.go` - Model list endpoints
- `api/pkg/types/models.go` - Model types

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

### Recommendation

Lean towards **Option A** or **Option B**. The `::` separator is unambiguous and won't conflict with HF model IDs. Option B is more backwards-compatible but requires model existence checks.

### Files to Investigate

- `api/pkg/model/models.go` - `ParseProviderFromModel`
- `api/pkg/server/openai_chat_handlers.go` - Routing logic

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

## Next Steps

1. [ ] Investigate provider access control in model routing (Issue 1)
2. [ ] Audit `isKnownProvider` for case sensitivity issues
3. [ ] Design model picker UI changes (Issue 2)
4. [ ] Ensure model list API returns provider information
5. [ ] Decide on disambiguation strategy for HF model IDs vs provider prefixes (Issue 3)
6. [ ] Deploy `fix/multi-provider-model-routing` after testing
