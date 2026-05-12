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

- [x] Updated `lookupOrg` in `api/pkg/server/wallet_handlers.go` to detect `errors.Is(err, store.ErrNotFound)` and return a more helpful wrapped error (`organization %q not found: %w`) that preserves the sentinel for callers to detect
- [x] Updated callers of `lookupOrg` to map ErrNotFound to HTTP 404 instead of 500: `provider_handlers.go:80` (writeErrResponse), `project_handlers.go:73` (NewHTTPError404), `quota_handlers.go:23` (http.Error 404), `wallet_handlers.go:38` (NewHTTPError404), `question_set_handlers.go:69` (refined existing 404 to be ErrNotFound-specific). Real errors continue to 500
- [x] **Scope note:** the 3 wallet handlers using `(string, error)` returns (`createTopUp:158`, `subscriptionCreate:249`, `subscriptionManage:297`) go through `system.DefaultWrapper` which always 500s. Updating those would require changing handler signatures + route bindings — out of scope for the user's reported bug (those endpoints are billing actions a user with a stale org slug is unlikely to invoke). Better lookupOrg error message still flows through. The `sandbox.go` caller already returned 404 — no change needed
- [x] `system.NewHTTPError404` already existed at `api/pkg/system/http.go:80` — no new helper needed
- [x] Added `api/pkg/server/wallet_handlers_test.go` with `TestLookupOrgSuite`: ErrNotFound preserved as sentinel + named in message; real DB error not confused with ErrNotFound; org_/slug routing regressions guarded
- [x] Ran `go build ./pkg/server/...` and `CGO_ENABLED=1 go test -v -run TestLookupOrgSuite ./pkg/server/ -count=1` — both pass

## Combined verification

- [x] Run `cd frontend && yarn build` (no FE changes expected, but CLAUDE.md requires the check) — passes
- [x] In inner Helix: configured `broken-provider` with `base_url=https://127.0.0.1:1` (connection-refused). Verified `GET /api/v1/provider-endpoints?with_models=true&org_id=…` returns in **~20 ms over 4 calls** (not 3 s × 4), broken provider shows `status=error` with the dial-tcp error inline, **other providers (helix, openai, claude) still load and surface their models**. Projects page at `/orgs/testorg` renders instantly. See `screenshots/01-degraded-provider.png` and `screenshots/02-projects-loads-with-broken-provider.png`
- [x] In inner Helix: tested all four high-traffic org-scoped endpoints with a stale slug. `/api/v1/projects?organization_id=org_doesnotexist` → **404** `organization "org_doesnotexist" not found: not found`. `/api/v1/projects?organization_id=nonexistentslug` → **404**, same wording. `/api/v1/provider-endpoints?org_id=org_doesnotexist` → **404** with the new wording. `/api/v1/quotas?org_id=ghostorg` → **404**. (Wallet returns 400 because billing is disabled — short-circuits before lookupOrg.) None return the doubled `failed to lookup org: failed to get organization: not found` 500 anymore
- [x] Took screenshots of degraded provider state (`01-degraded-provider.png`) and Projects page loading cleanly with the broken provider configured (`02-projects-loads-with-broken-provider.png`)
- [x] Pushed `feature/002009-ui-hangs-when-upstream` (after merging latest `origin/main`); platform will pick up the branch automatically. PR description in `pull_request_helix.md`
