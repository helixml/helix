# Implementation Tasks

## Bug 1 — stale-while-revalidate for provider model list

- [~] In `api/pkg/server/provider_handlers.go`, introduce two cache key helpers (e.g. `freshModelCacheKey` and `staleModelCacheKey`) so fresh and stale entries live under separate keys with separate TTLs. Add a constant or config field for the stale TTL (default 1 hour; reuse `ModelsCacheTTL` for fresh)
- [ ] Update `getProviderModels` so that on `ListModels` error (after the existing static-`Models` synthesis branch is exhausted) it falls back to the stale cache before returning the error. Change its return signature to `(models []types.OpenAIModel, servedStale bool, err error)` so the caller can flag degraded status without losing the model list
- [ ] Update `listProviderEndpoints` to read the new `servedStale` return value: when `servedStale` is true, set `Status = ProviderEndpointStatusError` and `Error` to the underlying upstream error string, but leave `available_models` populated from the stale entry
- [ ] On every successful fresh fetch in `getProviderModels`, write to BOTH the fresh key (short TTL) and the stale key (long TTL)
- [ ] Update `invalidateProviderModelCache` (line 274 today) to delete both fresh and stale keys so a rename/edit/delete doesn't leave a stale entry alive for the long TTL
- [ ] Verify `StartModelCacheRefresh` and `refreshAllProviderModels` keep working unchanged — they call `getProviderModels`, so the new write-both-keys behaviour inside that function handles them automatically (no separate code path)
- [ ] Add unit tests in `api/pkg/server/provider_handlers_test.go` (create the file if it doesn't exist): (a) cache hit returns cached models without calling `ListModels`; (b) fresh expired + stale present + `ListModels` returns error → handler returns stale models with `Status = error`; (c) no fresh + no stale + `ListModels` errors → existing error behaviour preserved; (d) `invalidateProviderModelCache` clears both keys
- [ ] Run `CGO_ENABLED=1 go test -v -run TestProviderHandlers ./pkg/server/ -count=1` and confirm all new tests pass

## Bug 2 — org lookup returns 404 (not 500) when org doesn't exist

- [ ] Update `lookupOrg` in `api/pkg/server/wallet_handlers.go` to detect `errors.Is(err, store.ErrNotFound)` and return a more helpful wrapped error (`organization %q not found: %w`) that preserves the sentinel for callers to detect
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
