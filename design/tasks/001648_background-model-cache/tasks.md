# Implementation Tasks

- [x] Read `provider_handlers.go:210-250` (the list endpoint handler) and confirm that `getProviderModels` is called for user-created providers when `?with_models=true` is used, and that the TTL cache write at line 317 fires correctly for them
- [x] If the on-demand path is broken (e.g., user providers are skipped or their cache key collides), fix that specific gap — do NOT use `All: true` in `refreshAllProviderModels` (on-demand path is working correctly, no fix needed)
- [~] Warm on create: after `CreateProviderEndpoint` succeeds, trigger a single async `getProviderModels` call for the new endpoint to pre-populate the cache
- [x] Do NOT change the background refresh query scope — system/global-only polling is correct for scale reasons
