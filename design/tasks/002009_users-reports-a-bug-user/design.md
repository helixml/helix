# Design: provider-resilience + org-lookup status code

Two unrelated fixes shipping together because the user reported them in the same session. Both small.

## Bug 1: stale-while-revalidate for provider model lists

### What's there today

`api/pkg/server/provider_handlers.go`:

- `listProviderEndpoints` (line 71) builds the list of providers, then for each one in a goroutine calls `getProviderModels`. `wg.Wait()` blocks until they all finish or fail. Each goroutine sets `Status = ProviderEndpointStatusError` on failure and continues.
- `getProviderModels` (line 278):
  1. Cache hit → return cached models. Cache is `s.cache` (an in-memory ristretto-style cache) keyed by `modelCacheKey(name, owner)`. TTL is `s.Cfg.WebServer.ModelsCacheTTL` (default 1 minute).
  2. Cache miss → singleflight on `BaseURL`, then `provider.ListModels(fetchCtx)` with a 3-second deadline.
  3. On `ListModels` error: if a static `Models` list is configured on the endpoint, synthesise from that. Otherwise return the error.
- `StartModelCacheRefresh` (line 949) runs a background goroutine that re-calls `getProviderModels` for every known provider on the same `ModelsCacheTTL` cadence. Errors are logged at debug.

### Where it breaks

The default `ModelsCacheTTL` is short (1 minute). When a provider goes down for, say, 90 seconds:

- `t=0s`: cache populated, UI fast.
- `t=60s`: cache TTL expires.
- `t=60s..150s`: every UI page that calls `useListProviders({loadModels: true})` triggers a cache miss → `ListModels` → 3-second timeout → `ProviderEndpointStatusError` with empty `available_models`. Background refresh hits the same wall.
- `t=150s`: provider recovers, but the cache is still empty. The next foreground request repopulates it.

The user observed the same shape: their provider is intermittent, the UI hangs, sending "Hi" via chat warms the upstream and the very next providers call repopulates the cache.

A 3-second wait per page-load isn't great; the bigger pain is the empty `available_models` in the response, which makes Onboarding (`Onboarding.tsx:358-363` `hasAnyEnabledModels`) and AdvancedModelPicker show "no models available" UI states that shouldn't happen for a transient blip.

### Fix: keep a stale copy

Two cache entries per provider instead of one:

- **Fresh** entry, TTL `ModelsCacheTTL` (1 min default). Same as today.
- **Stale** entry, TTL much longer (e.g. 1 hour). Written every time we successfully populate fresh.

`getProviderModels` flow becomes:

```
fresh, ok := cache.Get(freshKey(name, owner))
if ok { return fresh, nil }

result, err, _ := singleflight.Do(BaseURL, func() {
    if fresh, ok := cache.Get(freshKey); ok { return fresh, nil } // double-check
    models, fetchErr := provider.ListModels(fetchCtx)
    if fetchErr != nil {
        // Static-list synthesis path stays as-is (handles "endpoint doesn't expose /v1/models").
        if len(providerEndpoint.Models) > 0 { ... return synthesised, nil }
        // NEW: fall back to stale cache before bubbling error.
        if stale, ok := cache.Get(staleKey(name, owner)); ok {
            return stale, nil  // err is silently dropped — caller already got a successful response previously
        }
        return nil, fetchErr
    }
    enrichWithModelInfo(models, fetchCtx)
    cache.SetWithTTL(freshKey, models, fresh TTL)
    cache.SetWithTTL(staleKey, models, stale TTL)
    return models, nil
})
```

The error-status surfacing in `listProviderEndpoints` needs a small adjustment. Today: error → `Status = error`, no models. After: if we served from stale cache, `Status` should still be set so the FE knows the underlying upstream is down — but `available_models` is populated. Easiest way: have `getProviderModels` return `(models, servedStale bool, err error)` so the caller can set `Status = ProviderEndpointStatusError` (or a new `ProviderEndpointStatusStale` if we want to be precise) without losing the model list.

For v1 prefer the boolean + reuse `Status = error`. Adding a new enum value means propagating through OpenAPI + generated client + frontend types — out of scope for a bug fix.

### Cache invalidation

`invalidateProviderModelCache` (line 274) currently does a single `cache.Del` on the unified key. It now needs to delete both fresh and stale entries — otherwise renaming/editing a provider could leave a stale entry alive for an hour. One-line change inside that helper.

### Tests

`api/pkg/server/provider_handlers_test.go` (file may need to be created — verify before assuming):

- Test 1 (already-passing behaviour): cache hit returns cached models; `ListModels` not called.
- Test 2 (new): seed both fresh and stale cache, expire fresh, mock `provider.ListModels` to return error, assert handler returns the seeded models AND `Status = ProviderEndpointStatusError`.
- Test 3 (new): no fresh, no stale, `ListModels` errors → existing behaviour: error returned to caller, `Status = error`, empty `available_models`.
- Test 4 (regression): `invalidateProviderModelCache` clears both fresh and stale entries.

### Files touched

| File | Change |
|------|--------|
| `api/pkg/server/provider_handlers.go` | Two cache keys, stale-fallback in `getProviderModels`, return tuple includes `servedStale`. ~30 lines. |
| `api/pkg/server/provider_handlers_test.go` | New tests for stale fallback + invalidation covering both keys. |

No frontend change required. No new types. No OpenAPI regeneration.

### Why not …

- **Why not just bump `ModelsCacheTTL` to 1 hour?** It hides the bug for fewer users but creates a new one: cache stays stale for an hour after any legitimate model-list change (provider adds a new model, user edits the endpoint). Stale-while-revalidate gives us both: 1-minute freshness when healthy, hour-long resilience when not.
- **Why not a circuit breaker?** Adds config and complexity for behaviour the cache pattern already handles. Circuit breakers shine for high-rate callers; we're talking about a once-per-minute background refresh plus on-demand UI fetches.
- **Why not retry inside `ListModels`?** The 3-second per-call timeout is already the right shape for "provider is briefly unreachable." Retry stacks badly with the existing cache refresh interval.

## Bug 2: org lookup returns 404 on not-found

### What's there today

`api/pkg/server/wallet_handlers.go:200-216`:

```go
func (s *HelixAPIServer) lookupOrg(ctx context.Context, orgStr string) (*types.Organization, error) {
    query := &store.GetOrganizationQuery{}
    if strings.HasPrefix(orgStr, "org_") { query.ID = orgStr } else { query.Name = orgStr }
    org, err := s.Store.GetOrganization(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("failed to get organization: %w", err)
    }
    return org, nil
}
```

`s.Store.GetOrganization` returns `store.ErrNotFound` (text: `"not found"`) when the row doesn't exist (`api/pkg/store/organizations.go:113-117`). Every caller of `lookupOrg` then maps any error to HTTP 500.

The doubled wrapping produces the user-visible `"failed to lookup org: failed to get organization: not found"`.

### Fix

Map `errors.Is(err, store.ErrNotFound)` to HTTP 404 with a clearer message at the eight call sites. Keep 500 for other errors (real DB failures).

Two equally fine shapes:

- **Option A — fix at the helper.** `lookupOrg` returns a sentinel that callers can detect (or returns the unwrapped store error). Simpler text in the wrapper (drop the `"failed to get organization"` middle segment so the user-visible message is `organization "<slug>" not found`).
- **Option B — fix at every call site.** Each caller does `if errors.Is(err, store.ErrNotFound) { return 404 } else { return 500 }`.

Pick **Option A** for less duplication. `lookupOrg` becomes:

```go
func (s *HelixAPIServer) lookupOrg(ctx context.Context, orgStr string) (*types.Organization, error) {
    ...
    org, err := s.Store.GetOrganization(ctx, query)
    if err != nil {
        if errors.Is(err, store.ErrNotFound) {
            return nil, fmt.Errorf("organization %q not found: %w", orgStr, store.ErrNotFound)
        }
        return nil, fmt.Errorf("failed to get organization: %w", err)
    }
    return org, nil
}
```

Then update the eight callers (the list is in `requirements.md`) to use:

```go
org, err := s.lookupOrg(ctx, orgRef)
if err != nil {
    if errors.Is(err, store.ErrNotFound) {
        return nil, system.NewHTTPError404(err.Error())
    }
    return nil, system.NewHTTPError500(err.Error())
}
```

For handlers that don't use `system.NewHTTPError` (e.g. `provider_handlers.go:80` uses `writeErrResponse`), do the equivalent with `http.StatusNotFound`. Keep the wording short — the user already sees the org reference in their URL.

`system.NewHTTPError404` exists (verify before relying on it; if it doesn't, use `http.StatusNotFound` + `writeErrResponse` consistently with how the file already writes errors).

### Tests

A single Go test exercising `lookupOrg` against a mock store:

- `GetOrganization` returns `store.ErrNotFound` → `errors.Is(returnedErr, store.ErrNotFound)` is true.
- `GetOrganization` returns a generic error → `errors.Is(returnedErr, store.ErrNotFound)` is false.

That guards the helper. Don't write a separate test per call site — the type-system + `errors.Is` discipline does that work.

### Files touched

| File | Change |
|------|--------|
| `api/pkg/server/wallet_handlers.go` | `lookupOrg`: distinguish ErrNotFound; reword wrapper. |
| `api/pkg/server/provider_handlers.go` | `listProviderEndpoints`: map ErrNotFound → 404. |
| `api/pkg/server/project_handlers.go` | `listOrganizationProjects`: same. |
| `api/pkg/server/wallet_handlers.go` (other handlers) | Four other call sites in this file: same. |
| `api/pkg/server/quota_handlers.go` | One call site: same. |
| `api/pkg/server/wallet_handlers_test.go` (or new) | One unit test on `lookupOrg`. |

No frontend change required. The existing 404 handling in `useOrganizations.loadOrganization:188` already does the right thing (clears the dead org from state).

### Notes for the implementer

- `store.ErrNotFound` is the sentinel — confirm its exact symbol path in `api/pkg/store/store.go` before importing.
- `system.NewHTTPError404` may not exist — check `api/pkg/system/`. If only `400`/`403`/`500` helpers exist, either (a) add `NewHTTPError404` (one-liner alongside the others) or (b) use `writeErrResponse(rw, err, http.StatusNotFound)` directly. Either is fine; pick whatever matches the surrounding handler's style.
- Don't change the 500 path. Real DB errors still need to bubble as 500 so on-call sees them.

## Test plan (combined)

1. **Go unit tests** as listed above. Run via `CGO_ENABLED=1 go test -v -run TestProviderHandlers... ./pkg/server/ -count=1` per CLAUDE.md.
2. **Inner-Helix manual** (per CLAUDE.md, prefer over isolated harnesses):
   - Configure a custom provider endpoint with a deliberately bogus `BaseURL` (e.g. `https://127.0.0.1:1` — connection refused).
   - Visit Projects, Onboarding, App settings — confirm they don't hang for 3 s on every navigation; the bogus provider shows up in the providers table with a status badge but the model list is non-empty (after one successful warm-up).
   - Hit `/api/v1/projects?organization_id=org_doesnotexist` directly via `curl` with the user's session cookie — confirm 404 with the new wording, not 500.
3. **Build checks**: `cd api && go build ./pkg/server/...` and `cd frontend && yarn build` (the latter only because CLAUDE.md mandates it; no FE changes here).

## Implementation notes (added during implementation)

- **Final return signature for `getProviderModels` is `(models, staleErr, err error)`** rather than the `(models, servedStale bool, err error)` proposed in the original design. The bool flag created an awkward callsite: when serving stale, you want the error context in the response (`Status = error` + `Error = <upstream msg>`), not just a flag. Returning the error directly avoids carrying it as a side-channel.
- **`store.ErrNotFound` is at `api/pkg/store/store.go:189`** as a `errors.New("not found")` sentinel. Both `errors.Is` and string comparison work; we use `errors.Is`.
- **`system.NewHTTPError404` already exists** at `api/pkg/system/http.go:80` — no new helper needed.
- **9 lookupOrg call sites, not 8.** The grep for `lookupOrg(` in the original design missed `sandboxes_api_handlers.go:911` (already returns 404 — good, no change needed) and `question_set_handlers.go:69` (was returning 404 unconditionally — refined to be ErrNotFound-specific so real DB errors still 500).
- **3 wallet handlers (`createTopUp`, `subscriptionCreate`, `subscriptionManage`) intentionally not migrated to 404.** They return `(string, error)` and go through `system.DefaultWrapper` which always 500s. Fixing them would require changing the handler signature + the `authRouter.HandleFunc` route binding, a larger refactor outside the spirit of "small bug fix." The error message is still cleaner via the new `lookupOrg` wrapping.
- **Cache type is `*ristretto.Cache[string, string]`** (typed-key version) — the second `1` argument in `SetWithTTL` is the cost (each entry costs 1).
- **Inner-Helix verification** showed the `with_models=true` providers endpoint completes in ~20 ms even with a broken provider configured, confirming the parallel-goroutine + 3-second-cap layout still bounds wall-clock time to ~3 s worst case.
- **The `quota_handlers.go` caller has a pre-existing latent bug**: it calls `lookupOrg` even when `orgID == ""`, which produces a "id or name not specified" error. Out of scope; the fix here only changes the error→404 mapping for ErrNotFound, leaving that branch unchanged.
- **`frontend/dist` had root ownership** (probably from Docker bind-mount), preventing `yarn build` until `chown`'d back to `retro`. Per CLAUDE.md, never `rm -rf` it — `chown -R retro:retro frontend/dist` was the right fix.

## Notes & gotchas

- The cache being process-local means each API replica will have its own stale entry. Fine for the current single-API deploy; if Helix later scales horizontally we may want to share the stale snapshot via Postgres or Redis. Out of scope.
- The background refresh (`StartModelCacheRefresh`) needs to keep writing the stale entry too. Easiest: have it call `getProviderModels` exactly as today and let the new write-both-keys behaviour inside `getProviderModels` handle it. No special path required.
- Don't conflate `ProviderEndpointStatusError` with "models are wrong." A stale-but-correct response with `Status = error` is the *intended* signal. The frontend already renders that status (search `ProviderEndpointsTable.tsx` for `status === 'error'`).
