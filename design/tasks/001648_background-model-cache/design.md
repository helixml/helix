# Design: Background Model Cache Refresh for User-Created Providers

## Root Cause

`refreshAllProviderModels()` (`provider_handlers.go:854`) only queries:

```sql
(owner = 'system' AND endpoint_type = 'user') OR endpoint_type = 'global'
```

User-created providers have `owner = <user_id>` and `endpoint_type = 'user'` — excluded.

## Scaling Concern: Don't Naively Refresh All

`ListProviderEndpointsQuery` has an `All: true` flag, but using it indiscriminately is wrong at scale.

- Refresh interval = `ModelsCacheTTL` (default 1 minute)
- Each provider HTTP call has a 3-second timeout
- With 100s of user providers polled sequentially: 100 × 3s = 300s per cycle — longer than the interval itself
- Most user providers (personal Ollama instances) are idle most of the time; polling them continuously wastes their resources and Helix's

## Correct Approach: On-Demand Caching for User Providers

`getProviderModels()` already implements cache-then-fetch with TTL (`ModelsCacheTTL`). When a user calls any endpoint that needs model listings, the result is cached automatically. This is sufficient for user-created providers:

- First request: slow (live fetch + cache write)
- Subsequent requests within TTL: instant (cache hit, no HTTP)
- After TTL expires with no activity: cache evicted; next request re-fetches — acceptable, since an idle provider has no waiting users

Background refresh exists for a specific reason (comment in `StartModelCacheRefresh`): ensuring model names are pre-parsed for routing logic (e.g., HuggingFace IDs like `Qwen/Qwen3-Coder`). This is a system-level concern that only applies to providers Helix routes jobs to automatically. User-created providers aren't part of that routing path.

## Fix

**Keep background refresh scoped to system/global providers** — the current query scope is actually correct for background polling. The real fix is to ensure `getProviderModels` is called on-demand for user providers at the right points in the request path, and that those results are cached with TTL.

Investigate where model listings for user providers are consumed (e.g., `GET /api/v1/provider-endpoints?with_models=true`) and confirm that the on-demand caching path is working correctly there. If the cache is being populated on first user request and served from cache on subsequent ones, the behavior is correct.

If there is a specific code path where user provider models are needed before any user request has occurred (unlikely), a targeted fix would be: after a user *creates* a provider endpoint, immediately trigger a single background cache warm for that endpoint only — not a recurring poll.

## Decision

Do NOT use `All: true` in the background refresh loop. The current refresh scope (system/global only) is intentional. Fix any specific on-demand caching gaps instead.

## Codebase Notes

- `getProviderModels()` at `provider_handlers.go:251` handles cache check + TTL write
- Cache TTL set via `s.Cfg.WebServer.ModelsCacheTTL`; refresh interval matches this value
- `ListProviderEndpointsQuery.All` at `store.go:96` — exists but not appropriate here
- On-demand model fetch for user providers happens at `provider_handlers.go:224` (inside list endpoint handler)
