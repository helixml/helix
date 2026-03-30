# Requirements: Background Model Cache Refresh for User-Created Providers

## Problem

`refreshAllProviderModels()` in `api/pkg/server/provider_handlers.go:854` only queries providers with `owner=system` or `endpoint_type=global`. User-created provider endpoints (e.g., personal Ollama instances) are owned by a user ID and have `endpoint_type=user`, so they are never included in the background cache refresh.

As a result, model listings for user-created providers are never pre-cached, causing slower or missing model availability for those endpoints.

## User Story

As a user who has added a custom Ollama provider endpoint, my provider's models should be cached in the background refresh cycle so that model lookups are fast and available without on-demand fetching.

## Acceptance Criteria

- [ ] `refreshAllProviderModels()` fetches and caches models for all provider endpoints in the database, including user-created ones (`endpoint_type=user` with any owner).
- [ ] Errors for individual providers are still logged and skipped without stopping the refresh for other providers.
- [ ] No regression for system/global providers — they continue to be cached as before.
- [ ] The existing global (env-var) provider refresh logic is unchanged.
