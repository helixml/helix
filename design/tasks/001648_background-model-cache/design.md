# Design: Background Model Cache for User-Created Providers

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

## On-Demand Caching Already Works for User Providers

Investigation confirmed (`provider_handlers.go:103-128`): the list endpoint queries with `Owner: user.ID` + `WithGlobal: true`, which correctly returns user-created providers. When `?with_models=true` is set, `getProviderModels` is called for all of them concurrently (lines 219-244), and the cache write at line 317 fires with TTL. The on-demand path is correct.

## Fix Implemented

**Warm on create**: after `CreateProviderEndpoint` succeeds, spin up a goroutine that calls `getProviderModels` with a 10-second detached context. This pre-populates the cache so the first `?with_models=true` after creating a provider is served from cache. Errors are logged at debug level — the provider may not be reachable at the moment of creation.

**Background refresh scope is unchanged** — system/global only, intentional for scale reasons.

## Codebase Notes

- `getProviderModels()` at `provider_handlers.go:251`: cache key = `"<name>:<owner>"`, checks cache first, then fetches live with 3s timeout, writes with TTL
- `refreshAllProviderModels()` at `provider_handlers.go:854`: intentionally scoped to system/global providers
- `ListProviderEndpointsQuery.All` at `store.go:96`: returns all endpoints, NOT appropriate for background polling
- On-demand fetch for user providers: `provider_handlers.go:219-244` (parallel goroutines, one per endpoint)
- Cache warm on create: `provider_handlers.go:403-411` (added in this implementation)
