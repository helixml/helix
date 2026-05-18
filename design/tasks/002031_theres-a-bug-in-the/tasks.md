# Implementation Tasks: Make Desktop Quota Settings Coherent and Honest

## Backend — type helpers

- [ ] Add `EffectiveMaxConcurrentDesktops(tierDefault int) int` on `SystemSettings` in `api/pkg/types/system_settings.go` — returns `-1` for negative input (unlimited), `tierDefault` for `0` (unset), and the input value otherwise
- [ ] Refactor `EffectiveMaxConcurrentHeadlessSandboxes` and `EffectiveMaxConcurrentDesktopSandboxes` (`system_settings.go:219-231`) to the same three-state convention: `< 0 → -1` (unlimited), `== 0 → DefaultMaxConcurrent…Sandboxes`, `> 0 → value`
- [ ] Grep all callers of the two `Effective…Sandboxes` helpers to confirm none rely on the old `<= 0 → default` behaviour for negative inputs (any caller saving negative values today is a bug regardless)

## Backend — quota resolver

- [ ] Add `getTierDesktopCap(isPro bool) int` helper in `api/pkg/quota/quota.go` that returns the Pro env-var value when `isPro`, Free env-var value otherwise
- [ ] In `getOrgQuotas` and `getUserQuotas` (`quota.go:39-166`), after the tier is decided, set `quotas.MaxConcurrentDesktops = systemSettings.EffectiveMaxConcurrentDesktops(getTierDesktopCap(isPro))` — replaces the direct `getFreeQuotas` / `getProQuotas` use for this field. Other tier-default fields (`MaxProjects`, etc.) stay untouched
- [ ] Keep the `!EnforceQuotas → -1` short-circuit at the top of both functions intact

## Backend — `/api/v1/config` handler

- [ ] In `api/pkg/server/handlers.go:117-123`, replace the inline `MaxConcurrentDesktops` fallback with a call to the quota manager for the calling user/org so the response reflects what enforcement will actually do. If the caller user is not currently in scope in `getConfig`, extract from `req` (called via `config` at line 144) or document a fallback to Free-tier resolution
- [ ] Delete the dead `if config.MaxConcurrentDesktops == 0 { … }` branch entirely (no fallbacks per `helix/CLAUDE.md`)

## Backend — verification (no change unless needed)

- [ ] Verify `api/pkg/external-agent/hydra_executor.go:checkLimits` (`hydra_executor.go:1448-1471`) needs no change — it already routes through `quotaManager.LimitReached(ResourceDesktop)`
- [ ] Verify `api/pkg/quota/quota.go:268` `limit < 0` guard still handles the new resolved-`-1` value (it does)
- [ ] Verify `helix-org/helix/helixclient/client.go:560` `HasDesktopRoom` still treats `Max <= 0` as unlimited; no change if so

## Frontend — Max Concurrent Desktops row

- [ ] Update `handleSaveMaxDesktops` in `frontend/src/components/dashboard/SystemSettingsTable.tsx:113-132` to accept `-1` as a valid value (current guard `value < 0` rejects it). Preferred UX: replace the free-form `TextField` with a three-mode picker (`Unlimited` / `Use server default` / `Custom cap N`) that writes `-1` / `0` / `N`
- [ ] Update the chip display at `SystemSettingsTable.tsx:738-742` to render three distinct states: `Unlimited` for `< 0`, `Default` for `== 0`, `Limit: N` for `> 0`
- [ ] Update the help text at `SystemSettingsTable.tsx:731-734` from "Maximum number of concurrent desktop sessions per user (0 = unlimited)" to "Cap on concurrent desktop sessions per organisation (or per user if the session has no org). Overrides the subscription-tier default. -1 = unlimited, 0 = use server default."

## Frontend — Headless Sandbox Limit and Desktop Sandbox Limit rows

- [ ] Apply the same three-mode picker / `-1`-accepting validator to `handleSaveSandboxLimit` (`SystemSettingsTable.tsx` around line 217)
- [ ] Update both chip displays (`SystemSettingsTable.tsx:1024-1028`, `1088-1092`) to render the three-state model
- [ ] Update both help texts (`SystemSettingsTable.tsx:1019-1021`, `1083-1085`) to mention the new `-1` / `0` / `N` convention

## Tests

- [ ] Add `TestEffectiveMaxConcurrentDesktops` in `api/pkg/types/system_settings_test.go` (create if needed) — table-test covering: `-1` / `0` / `N` × tier defaults `2` / `10` / `30`
- [ ] Add `TestEffectiveSandboxLimits` in the same file covering the refactored helpers (both headless and desktop variants), `-1` / `0` / `N` × defaults
- [ ] Add `TestQuotaDesktopResolution` in `api/pkg/quota/quota_test.go` — table-test covering all 17 rows of the truth table in `design.md` (`enforce`, `sys`, `tier`, `ctx`, `active` → `effective_limit`, `allowed`)
- [ ] Add a handler-level test that `GET /api/v1/config` returns the **resolved** `max_concurrent_desktops` for the calling user (cases: unset Free, unset Pro, admin-overridden, admin-unlimited)

## End-to-end (per `helix/CLAUDE.md`)

- [ ] In inner Helix at `http://localhost:8080`: register / log in, save `Unlimited` → DB shows `-1`, `/api/v1/config` returns `-1`, can open more than 2 desktops without rejection
- [ ] Save `1` → second desktop attempt rejected with `desktop limit reached (1)`
- [ ] Save `0` (or "Use server default") → DB shows `0`, cap reverts to Free-tier default (`2`), third desktop attempt rejected
- [ ] Repeat the same trio with an **org-scoped** session and confirm the cap is enforced per-org (a second user in the same org counts toward the same limit)

## Ship

- [ ] Run `go build ./pkg/server/ ./pkg/store/ ./pkg/types/ ./pkg/quota/ ./pkg/external-agent/` and `cd frontend && yarn build` before push
- [ ] Push, open PR with full URL format per `helix/CLAUDE.md` (`https://github.com/helixml/helix/pull/<n>`), reference this spec task
- [ ] Check CI green via `gh pr checks <n>` or Drone MCP tools per `helix/CLAUDE.md`
