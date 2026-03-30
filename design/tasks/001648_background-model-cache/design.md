# Design: Background Model Cache Refresh for User-Created Providers

## Root Cause

`refreshAllProviderModels()` (`provider_handlers.go:854`) makes two passes:

1. **Global env-var providers** — iterates `providerManager.ListProviders()` (system-owned). Correct.
2. **Database providers** — calls `store.ListProviderEndpoints` with `{Owner: "system", WithGlobal: true}`.

The `ListProviderEndpoints` SQL for that query is:
```sql
(owner = 'system' AND endpoint_type = 'user') OR endpoint_type = 'global'
```

User-created providers have `owner = <user_id>` and `endpoint_type = 'user'` — neither condition matches, so they are silently excluded.

## Fix

`ListProviderEndpointsQuery` already has an `All bool` field. When `All: true`, the store returns every endpoint with no WHERE clause (`query.Find(&providerEndpoints)`).

Change the DB query in `refreshAllProviderModels()` from:
```go
s.Store.ListProviderEndpoints(ctx, &store.ListProviderEndpointsQuery{
    Owner:      string(types.OwnerTypeSystem),
    WithGlobal: true,
})
```
to:
```go
s.Store.ListProviderEndpoints(ctx, &store.ListProviderEndpointsQuery{
    All: true,
})
```

This is safe for a background refresh: the function already tolerates errors per-provider. Caching more providers adds load only on the provider endpoints themselves (Ollama etc.), not on Helix internals.

## Considered Alternatives

- **Per-user refresh**: query distinct owners, iterate. More complex, no benefit since `All: true` exists.
- **Change WithGlobal logic**: would require changing store semantics, affects other callers.

## Codebase Notes

- `ListProviderEndpointsQuery.All` is defined in `api/pkg/store/store.go:93`.
- The store implementation is in `api/pkg/store/store_provider_endpoints.go:88`.
- The background refresh loop is in `provider_handlers.go:823–916`.
- `getProviderModels()` is already designed to handle any `*types.ProviderEndpoint` owner.
