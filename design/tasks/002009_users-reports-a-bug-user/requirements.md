# Requirements: UI hangs when upstream provider is intermittently unavailable

## Bug report

> "I found a little bug that hangs the UI does not load projects (providers endpoint can't be loaded). If I go to chats and say Hi, I get answer from my provider, and UI instantly works again. Bear in mind that our provider is NOT always available for Helix (so it could be related). Sometimes it's up sometimes it's down."

The user also reports an apparently separate 500 response — they note it "might be completely unrelated":

```
{
    "StatusCode": 500,
    "Message": "failed to lookup org: failed to get organization: not found"
}
```

The two reports are kept together in this spec because they both surface in the same UI session and likely compound the user's experience of "the UI is broken." They get separate fixes (one is a provider-resilience issue, one is a status-code mapping issue) but ship together.

## Bug 1: providers endpoint blocks the UI when an upstream is slow/unavailable

### Root cause (one-line)

`GET /api/v1/provider-endpoints?with_models=true` blocks until **every** configured provider's `ListModels` call has returned or hit its 3-second timeout. When the user's upstream is unreachable AND the in-memory model cache has expired (TTL ≈ 1 minute), every cache-miss UI page that reads providers stalls for the full timeout window. The endpoint never serves the previously cached model list as a fallback.

### Why "say Hi in chat fixes it"

A successful `/v1/chat/completions` call requires the provider to be reachable, so it warms up DNS / TCP / TLS and proves the provider is back. The next `getProviderModels` call inside `provider_handlers.go:listProviderEndpoints` then succeeds and repopulates the cache. From there the providers endpoint returns instantly and the dependent UI hooks (`useListProviders` in `Onboarding`, `useApp`, `AdvancedModelPicker`, `TokenUsageDisplay`, `ProjectSettings`, `NewSpecTaskForm`, `AgentSelectionModal`, `CreateProjectDialog`) unblock.

### User stories

- **As a Helix user behind an intermittently available LLM provider**, when my provider is briefly down I want the UI to keep loading using the last-known model list, so navigation doesn't hang and I can still create projects, see settings, and reach the chat page that would actually warm the provider back up.
- **As the same user**, I want a small visible indicator that one provider is degraded (rather than silent fallback), so I'm not surprised when the model list looks slightly stale.

### Acceptance criteria

1. **Stale-while-revalidate for `getProviderModels`.** When the in-memory cache entry for a provider has expired AND the upstream `ListModels` call fails or times out, the handler returns the **previously cached** model list (if one was ever populated) instead of returning an error for that provider. The cache entry is kept under a separate "stale" record with a longer TTL (e.g. 1 hour) so a single dead minute doesn't wipe the model list.
2. **Per-provider error is surfaced to the FE without blocking the response.** The endpoint already records `Status = ProviderEndpointStatusError` and `Error = err.Error()` for failed providers — that behaviour is preserved. With criterion 1, the response now includes `available_models` from the stale cache AND `Status = ProviderEndpointStatusError` so the frontend can render a small "degraded" badge.
3. **Overall request latency budget.** The handler's wall-clock time stays bounded: with N providers all in error state, the response returns within ~3 seconds (the existing `provider.ListModels` timeout), not N×3s. (This is already the case today via parallel goroutines — but the test suite should pin it so a future refactor doesn't reintroduce a serial loop.)
4. **No behavioural change when providers are healthy.** Cache hit path is unchanged. Fresh fetch path is unchanged. Stale fallback only kicks in on `ListModels` error after a cache miss.
5. **Background refresh continues to attempt fresh fetches.** `StartModelCacheRefresh` keeps polling on its `ModelsCacheTTL` interval, so as soon as the provider recovers the fresh list reappears without requiring a chat round-trip.
6. **Test coverage.** A Go unit test exercises the stale-fallback path: seed the cache, expire the fresh entry, make the provider's `ListModels` return an error, assert the handler returns the seeded models with `Status = error` and a non-empty `Error` field.

## Bug 2: org lookup returns 500 instead of 404 with a confusing message

### Root cause

`s.lookupOrg` in `api/pkg/server/wallet_handlers.go:200-216` returns `store.ErrNotFound` (literal text `"not found"`) wrapped as `fmt.Errorf("failed to get organization: %w", err)`. Every handler that calls `lookupOrg` then wraps it again as `"failed to lookup org: <err>"` and **always returns HTTP 500**, regardless of whether the underlying error is "not found" or a real database failure.

Callers verified: `provider_handlers.go:80`, `project_handlers.go:75`, `wallet_handlers.go:38,157,242,290`, `quota_handlers.go:25`. All of them use `system.NewHTTPError500` / `http.StatusInternalServerError`.

### User stories

- **As a Helix user with a stale `/org/<slug>/...` URL** (the org was deleted, renamed, or my membership was revoked), I want a clear 404 with an actionable message, not a generic 500 that looks like the server is broken.

### Acceptance criteria

1. **Org-not-found returns HTTP 404, not 500.** All eight callers of `lookupOrg` distinguish `errors.Is(err, store.ErrNotFound)` and respond with HTTP 404. Other errors continue to map to 500.
2. **The 404 body carries an actionable message.** The message should name the org reference the user supplied (the slug or id from the URL) and tell them the org doesn't exist or they no longer have access. Don't repeat the doubled `"failed to lookup org: failed to get organization: not found"` wrapping.
3. **No frontend churn.** The frontend's existing 404 handling in `useOrganizations.loadOrganization` (`hooks/useOrganizations.ts:188`) already clears the org state on 404. With the 500→404 mapping, that path triggers cleanly when an org-scoped endpoint returns 404, so the user is bumped out of the dead org context naturally on the next render.

## Out of scope

- Adding a frontend warning banner / snackbar for the degraded-provider state. Surfacing `Status = error` per-row in the providers table is enough for v1 — banner UX is a separate piece of work.
- Implementing a stale cache for `/v1/chat/completions` routing. Chat routing has different semantics (it picks one provider per request, not a list); a separate spec if it becomes an issue.
- Adding circuit-breaker / exponential backoff to the per-provider client. The 3-second per-call timeout plus the existing background refresh is sufficient — overkill here would just add config surface.
- Reworking the React Query retry/backoff defaults for `useListProviders`. Once the API stops returning errors for the slow provider (criterion 1), retry doesn't matter.
