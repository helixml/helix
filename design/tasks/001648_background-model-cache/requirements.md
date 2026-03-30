# Requirements: Background Model Cache for User-Created Providers

## Problem

User-created provider endpoints (e.g., personal Ollama instances) are not included in the background model cache refresh. The initial bug report suggested querying all providers, but at scale (100s of users each with their own provider) this would create a polling loop that polls idle endpoints every minute — excessive load on both Helix and user infrastructure.

## Correct Goal

User-created providers should have their model lists cached so that API calls are fast, without Helix continuously polling all of them in the background.

The existing `getProviderModels()` function already caches results with a TTL on first request. This on-demand caching is the right mechanism for user providers. Background polling is only appropriate for system/global providers (small, known set) that Helix routes workloads to automatically.

## User Story

As a user who has added a custom Ollama provider, my provider's models should be cached after I first use the endpoint, so subsequent requests are fast — without Helix polling my endpoint every minute even when I'm not using it.

## Acceptance Criteria

- [ ] When a user requests model listings for their provider endpoint (e.g., via `?with_models=true`), the result is cached with TTL and returned from cache on subsequent requests within that TTL.
- [ ] User providers are NOT polled in the background refresh loop (prevents overload with many users).
- [ ] Optionally: when a user creates a new provider endpoint, a single cache warm is triggered immediately so the first `?with_models=true` request is fast.
- [ ] System/global provider background refresh is unchanged.
