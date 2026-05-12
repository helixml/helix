# fix(api): provider model cache stale fallback + org lookup 404

## Summary

Two small backend fixes for a user report where the UI hangs intermittently and a separate `failed to lookup org` 500 surfaces on stale URLs.

1. **`getProviderModels` now serves from a long-TTL stale cache when upstream errors.** When the user's `ModelsCacheTTL` (1 min) expires AND their provider is briefly unreachable, every UI page that consumes `/api/v1/provider-endpoints?with_models=true` was emptying out the model picker until a chat round-trip warmed the upstream back up. We now keep a separate hour-long stale entry and fall back to it on `ListModels` error, surfacing the failure as `Status = error` on the affected provider without dropping its models.
2. **`lookupOrg` returns HTTP 404 (not 500) when the org doesn't exist.** The previous wrapping produced `failed to lookup org: failed to get organization: not found` with a 500 status, which looked like a server bug to users on stale `/orgs/<slug>/...` URLs. We detect `store.ErrNotFound`, name the missing reference in the error, and map to 404 in the four high-traffic callers (`provider_handlers`, `project_handlers`, `quota_handlers`, `getWalletHandler`). The frontend's existing 404 handler in `useOrganizations.loadOrganization` already clears stale org state.

## Changes

- `api/pkg/server/provider_handlers.go`: Split the single model-cache key into `freshModelCacheKey` (1 min TTL) and `staleModelCacheKey` (1 hour TTL). `getProviderModels` returns `(models, staleErr, err)` so callers can render degraded status without losing the model list. Successful fetches write both keys; `invalidateProviderModelCache` deletes both.
- `api/pkg/server/wallet_handlers.go`: `lookupOrg` wraps `store.ErrNotFound` as `organization %q not found: %w` so callers can `errors.Is`-check it. `getWalletHandler` maps to 404.
- `api/pkg/server/{provider,project,quota,question_set}_handlers.go`: detect ErrNotFound and return 404 instead of 500.
- `api/pkg/server/provider_handlers_test.go`: 3 new tests for stale fallback + cache-write + invalidation.
- `api/pkg/server/wallet_handlers_test.go`: new `TestLookupOrgSuite` covering ErrNotFound sentinel preservation, real-error distinction, and id/slug routing.

The 3 wallet handlers using `(string, error)` returns (`createTopUp`, `subscriptionCreate`, `subscriptionManage`) keep their 500 behaviour because mapping them would require changing handler signatures + route bindings. They aren't part of the user's reported path. The error wording is still cleaner because lookupOrg's wrapping changed.

No frontend changes. No new types. No OpenAPI regeneration.

## Test plan

- [x] `CGO_ENABLED=1 go test -v -run TestProviderHandlersSuite ./pkg/server/ -count=1` — 16/16 pass
- [x] `CGO_ENABLED=1 go test -v -run TestLookupOrgSuite ./pkg/server/ -count=1` — 4/4 pass
- [x] `cd frontend && yarn build` — passes
- [x] Inner Helix: configured a provider with `base_url=https://127.0.0.1:1` and verified `/api/v1/provider-endpoints?with_models=true` returns in ~20 ms across 4 calls with the broken provider showing `status=error` inline; other providers still surface their models; Projects page loads instantly
- [x] Inner Helix: hit `/api/v1/projects?organization_id=org_doesnotexist` and the other three org-scoped endpoints; all return HTTP 404 with the new wording (no more `failed to lookup org: failed to get organization: not found` 500)

## Screenshots

![Degraded provider with inline error](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002009_users-reports-a-bug-user/screenshots/01-degraded-provider.png)
![Projects page loads cleanly with broken provider configured](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002009_users-reports-a-bug-user/screenshots/02-projects-loads-with-broken-provider.png)

Spec: `design/tasks/002009_users-reports-a-bug-user/`
