# Implementation Tasks

## Bug 1 — stale-while-revalidate for provider model list

- [x] In `api/pkg/server/provider_handlers.go`, introduce two cache key helpers (`freshModelCacheKey` and `staleModelCacheKey`) so fresh and stale entries live under separate keys with separate TTLs. Stale TTL: 1 hour (`staleModelCacheTTL` constant); fresh TTL stays at `ModelsCacheTTL`
- [x] Update `getProviderModels` so that on `ListModels` error (after the existing static-`Models` synthesis branch is exhausted) it falls back to the stale cache before returning the error. Return signature changed to `(models []types.OpenAIModel, staleErr error, err error)` — `err` for hard failure, `staleErr` non-nil when serving from stale cache
- [x] Update `listProviderEndpoints` to read the new `staleErr` return: when non-nil, set `Status = ProviderEndpointStatusError` and `Error` to the underlying upstream error string, but leave `available_models` populated from the stale entry
- [x] On every successful fresh fetch in `getProviderModels`, write to BOTH the fresh key (short TTL) and the stale key (long TTL)
- [x] Update `invalidateProviderModelCache` to delete both fresh and stale keys so a rename/edit/delete doesn't leave a stale entry alive for the long TTL
- [x] Verified: `StartModelCacheRefresh` and `refreshAllProviderModels` work unchanged — they call `getProviderModels`, so the new write-both-keys behaviour handles them automatically. Updated 3 other call sites for the new 3-return-value signature (warm-after-create + 2 in refreshAllProviderModels)
- [x] Add unit tests in `api/pkg/server/provider_handlers_test.go`: (a) `TestGetProviderModels_FallsBackToStaleOnUpstreamError` — fresh expired + stale present + `ListModels` returns error → handler returns stale models with non-nil staleErr; (b) `TestGetProviderModels_PopulatesBothFreshAndStaleOnSuccess` — fresh fetch writes both keys; (c) `TestInvalidateProviderModelCache_ClearsBothFreshAndStale` — invalidation drops both. Existing test for hard-fail (no fresh, no stale, error) still passes
- [x] Ran `CGO_ENABLED=1 go test -v -run TestProviderHandlersSuite ./pkg/server/ -count=1` — all 16 tests pass including the 3 new ones

## Bug 2 — org lookup returns 404 (not 500) when org doesn't exist

- [~] Update `lookupOrg` in `api/pkg/server/wallet_handlers.go` to detect `errors.Is(err, store.ErrNotFound)` and return a more helpful wrapped error (`organization %q not found: %w`) that preserves the sentinel for callers to detect
- [ ] Update each caller of `lookupOrg` to map ErrNotFound to HTTP 404 instead of 500. Call sites: `provider_handlers.go:80`, `project_handlers.go:75`, `wallet_handlers.go:38,157,242,290`, `quota_handlers.go:25`. Real errors continue to 500
- [ ] If `system.NewHTTPError404` does not already exist in `api/pkg/system/`, add it next to the existing 400/403/500 helpers (one-liner). Otherwise use the existing helper
- [ ] Add a short Go unit test on `lookupOrg` that uses the gomock store: ErrNotFound from store → returned error satisfies `errors.Is(err, store.ErrNotFound)`; generic store error → does not
- [ ] Run `cd api && go build ./pkg/server/...` to confirm Go still compiles

## Combined verification

- [ ] Run `cd frontend && yarn build` (no FE changes expected, but CLAUDE.md requires the check)
- [ ] In inner Helix: configure a provider endpoint with a deliberately unreachable `BaseURL` (e.g. `https://127.0.0.1:1`) and visit Projects / Onboarding / App settings. Confirm pages don't stall for 3 s per navigation after the first successful warm-up; confirm the bogus provider appears with an error status badge but the model list elsewhere is not empty
- [ ] In inner Helix: hit `GET /api/v1/projects?organization_id=org_doesnotexist` (or `?organization_id=nonexistentslug`) with a valid session cookie via the browser DevTools network tab or `curl`. Confirm response is HTTP 404 with the new message wording, not 500
- [ ] Take a screenshot of the providers table showing the degraded provider with status badge, save under `screenshots/01-degraded-provider.png`
- [ ] Open PR with conventional commit message `fix(api): stale-while-revalidate provider models, 404 on missing org` and link this spec
