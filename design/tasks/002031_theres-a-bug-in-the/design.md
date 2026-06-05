# Design: Remove the Phantom Max Concurrent Desktops Setting

## Problem Recap (one paragraph)

`SystemSettings.MaxConcurrentDesktops` is a phantom: it's persisted, edited
in the admin UI, and surfaced on `GET /api/v1/config`, but **no enforcement
code ever reads it**. Real desktop quota enforcement
(`api/pkg/external-agent/hydra_executor.go:checkLimits` →
`api/pkg/quota/quota.go:LimitReached`) reads only the env-var tier values
(`PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS` / `PROJECTS_PRO_…`). The two layers
can disagree, and the admin-visible knob is the wrong one.

## Chosen Approach: Delete the Setting

Per colleague feedback: there's no point patching a setting that does
nothing. Delete it. The Free/Pro env vars are already the source of truth;
expose them honestly and accept `-1` for unlimited (which the quota manager
at `api/pkg/quota/quota.go:268` already short-circuits via `if limit < 0`).

### What to delete

1. `SystemSettings.MaxConcurrentDesktops` field — `api/pkg/types/system_settings.go:37`
2. `SystemSettingsRequest.MaxConcurrentDesktops` field — `api/pkg/types/system_settings.go:79`
3. `SystemSettingsResponse.MaxConcurrentDesktops` field — `api/pkg/types/system_settings.go:131`
4. The mapping in `ToResponseWithSource` — `api/pkg/types/system_settings.go:190`
5. The `if req.MaxConcurrentDesktops != nil` patch path in
   `api/pkg/store/store_system_settings.go:86-88`
6. The log-line key `max_concurrent_desktops_updated` in
   `api/pkg/server/system_settings_handlers.go:97`
7. The `MaxConcurrentDesktops` row in the System Settings UI
   (`frontend/src/components/dashboard/SystemSettingsTable.tsx`, the
   `handleSaveMaxDesktops` handler and lines ~731-790 — verify exact range
   when editing)
8. Regenerated swagger artefacts (`./stack update_openapi`) — drop the
   field from the request/response schemas

### What to change (not delete)

**`api/pkg/server/handlers.go:117-123`** — the only place that read the
phantom. Replace the inline fallback with a call to the quota manager so
the `/api/v1/config` response reflects what enforcement will actually
return:

```go
quotas, err := apiServer.quotaManager.GetQuotas(ctx, &types.QuotaRequest{
    UserID:         user.ID,
    OrganizationID: orgID, // empty string if no org context
})
if err != nil {
    return types.ServerConfigForFrontend{}, system.NewHTTPError500(err.Error())
}
config.MaxConcurrentDesktops = quotas.MaxConcurrentDesktops
```

**Caveat**: `getConfig` is called via the lightweight `config` wrapper at
`handlers.go:144`. Need to verify the caller's `user` is available in
context at that point. If extracting it from `req.Context()` is invasive,
fall back to returning the **Free-tier env value as a floor** (most callers
are unsubscribed); this still removes the phantom and is honest about
where the number comes from. Document whichever path is taken.

### What stays untouched

- The Free/Pro tier env vars in `api/pkg/config/config.go:548-562`.
- The quota resolver in `api/pkg/quota/quota.go` (it already does the
  right thing — Free vs Pro based on wallet subscription, per-user vs
  per-org based on `OrganizationID`).
- The `-1`-means-unlimited convention at
  `api/pkg/quota/quota.go:268`. Setting either env var to `-1` already
  yields unlimited enforcement today — no code change required to enable
  this.
- `MaxConcurrentHeadlessSandboxes` and `MaxConcurrentDesktopSandboxes` —
  those are real, enforced, and out of scope here.

## Truth Table (the env-var-based resolver — spec for the test)

Inputs:
- `enforce` — `SystemSettings.EnforceQuotas` (bool)
- `freeEnv` — `PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS` int (e.g. `2`, `5`,
  `-1`)
- `proEnv` — `PROJECTS_PRO_MAX_CONCURRENT_DESKTOPS` int
- `subscribed` — whether the wallet has an active Stripe subscription
- `ctx` — `"user"` (req has no `OrganizationID`) or `"org"` (req has one)
- `active` — count of the caller's currently-active desktops (per-user
  count if `ctx == "user"`, per-org count if `ctx == "org"`)

Outputs:
- `effective_limit` — value the quota manager will return in
  `QuotaResponse.MaxConcurrentDesktops`
- `allowed` — whether the next `StartDesktop` call succeeds

| #  | enforce | freeEnv | proEnv | subscribed | ctx  | active | → effective_limit | → allowed | notes |
|----|---------|---------|--------|------------|------|--------|-------------------|-----------|-------|
| 1  | false   | 2       | 30     | false      | user | 99     | `-1`              | yes       | EnforceQuotas off short-circuits |
| 2  | false   | 2       | 30     | true       | org  | 99     | `-1`              | yes       | EnforceQuotas off short-circuits |
| 3  | true    | 2       | 30     | false      | user | 1      | `2`               | yes       | Free, per-user, under cap |
| 4  | true    | 2       | 30     | false      | user | 2      | `2`               | no        | Free, per-user, at cap |
| 5  | true    | 2       | 30     | false      | org  | 1      | `2`               | yes       | Free, per-org, under cap |
| 6  | true    | 2       | 30     | false      | org  | 2      | `2`               | no        | Free, per-org, at cap |
| 7  | true    | 2       | 30     | true       | user | 29     | `30`              | yes       | Pro, per-user, under cap |
| 8  | true    | 2       | 30     | true       | user | 30     | `30`              | no        | Pro, per-user, at cap |
| 9  | true    | 2       | 30     | true       | org  | 30     | `30`              | no        | Pro, per-org, at cap |
| 10 | true    | `-1`    | 30     | false      | user | 999    | `-1`              | yes       | Free env unlimited |
| 11 | true    | 2       | `-1`   | true       | user | 999    | `-1`              | yes       | Pro env unlimited |
| 12 | true    | 5       | 30     | false      | user | 4      | `5`               | yes       | Custom Free cap, under |
| 13 | true    | 5       | 30     | false      | user | 5      | `5`               | no        | Custom Free cap, at |
| 14 | true    | 2       | 50     | true       | org  | 49     | `50`              | yes       | Custom Pro cap, per-org |
| 15 | true    | 0       | 30     | false      | user | 0      | `0`               | no        | Pathological: env set to 0 → no desktops at all. Document this is a misconfiguration; operator should set `-1` for unlimited or a positive integer |

The new `TestQuotaDesktopResolution` (in `api/pkg/quota/quota_test.go`) should
implement all 15 rows. The existing tests at `quota_test.go:121-650` already
cover several of these — extend or refactor as needed; don't duplicate.

## Per-User / Per-Org Behaviour (documentation)

This isn't being changed — it's just being made explicit. Code already
behaves this way:

- `api/pkg/quota/quota.go:32-37` (`GetQuotas`) branches on
  `req.OrganizationID`: non-empty → `getOrgQuotas`, empty →
  `getUserQuotas`.
- `getActiveConcurrentDesktopsByOrg`
  (`quota.go:228-237`) counts sessions matching `session.OrganizationID`.
- `getActiveConcurrentDesktopsByUser`
  (`quota.go:188-197`) counts sessions matching `session.UserID`.
- `hydra_executor.go:checkLimits` (`hydra_executor.go:1461-1465`) passes
  both `agent.UserID` and `agent.OrganizationID` into `LimitReached`, so
  whichever is set determines the path.

Update:

- The Swagger description of `QuotaResponse.MaxConcurrentDesktops` and
  `ServerConfigForFrontend.MaxConcurrentDesktops` to say "Cap on concurrent
  desktop sessions. Enforced per organisation when the session has an org,
  per user otherwise. -1 = unlimited."
- Add a one-line comment on the two env vars in `api/pkg/config/config.go`
  reflecting the same.

## Compatibility & Migration

- **DB column**: `system_settings.max_concurrent_desktops` becomes orphan
  data. GORM AutoMigrate (the project's only allowed migration mode per
  `helix/CLAUDE.md`) does not drop columns; leaving the column is safe and
  read by nothing. Dropping cleanly would require an explicit migration
  step — out of scope here.
- **API JSON shape**: `SystemSettingsRequest`, `SystemSettingsResponse`,
  and the System Settings PATCH payload lose the
  `max_concurrent_desktops` field. The generated frontend types and any
  external clients consuming the system-settings API need to handle the
  field's absence. The field on `ServerConfigForFrontend` stays — but its
  value now comes from `QuotaManager.GetQuotas`.
- **Existing admins**: anyone who had previously saved a non-zero value
  loses that "control" — but the value was never enforced anyway, so
  no behavioural regression. If they relied on it to gate helix-org's
  pre-flight, they switch to the env vars (announce in changelog).
- **Demo unblock today**: still works as `PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS=20`
  via `.env` + `docker compose up -d api`. Document `-1` as the unlimited
  value going forward.

## Testing Plan

### Go unit tests

1. **`TestQuotaDesktopResolution`** in `api/pkg/quota/quota_test.go` —
   table-test covering all 15 rows of the truth table above. Reuses /
   extends the existing `quota_test.go` test fixtures (gomock store +
   mocked external-agent executor for `ListSessions`).
2. **Handler test** for `getConfig` confirming
   `ServerConfigForFrontend.MaxConcurrentDesktops` matches the resolved
   value from the quota manager for the calling user, in: unsubscribed
   user with default Free env, subscribed user with default Pro env, and
   env set to `-1` (unlimited).

### Frontend

3. The "Max Concurrent Desktops" row is gone from the System Settings
   page (visual check + no console errors after rebuild).

### End-to-end (per `helix/CLAUDE.md`)

4. In inner Helix at `http://localhost:8080`: log in, confirm the System
   Settings page no longer shows "Max Concurrent Desktops".
5. Edit `.env` to set `PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS=-1`,
   `docker compose -f docker-compose.dev.yaml up -d api`, confirm
   `/api/v1/config` returns `max_concurrent_desktops: -1` and that
   opening multiple desktops is not rejected.
6. Set `PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS=1`, restart, confirm second
   desktop attempt is rejected with `desktop limit reached (1)`.

## Notes for the Implementing Agent

- **Delete is cheaper than redesign.** Resist the urge to add a new
  System Settings field that does the same thing as the env var; the env
  var is already the source of truth. If a future need for a DB-backed
  override emerges, that's a separate spec.
- **Sandbox limits stay.** Don't touch `MaxConcurrentHeadlessSandboxes` or
  `MaxConcurrentDesktopSandboxes` in `SystemSettings` — those are
  enforced and out of scope.
- **`getConfig` user context.** Verify whether `getConfig`
  (`handlers.go:74`) currently has the caller's user in scope. If yes,
  pass it to `quotaManager.GetQuotas`. If no and extraction is invasive,
  document the floor-fallback (Free env value) and ship that. Don't
  block the fix on plumbing.
- **`-1` already works in `quota.go`.** No code change in `quota.go` is
  required for `-1`-means-unlimited; the env vars flow through to
  `QuotaResponse.MaxConcurrentDesktops` directly, and
  `LimitReached:268` short-circuits negative limits.
- **Regenerate swagger after type edits**: `./stack update_openapi`
  (per `helix/CLAUDE.md`).
- **GORM AutoMigrate is additive**: the DB column for the deleted field
  becomes orphan data. That's acceptable; don't write a custom migration
  unless asked.
- **Test end-to-end in inner Helix** per `helix/CLAUDE.md` — not just
  unit tests.
- **Full PR URL** in any references (`https://github.com/helixml/helix/pull/<n>`).
