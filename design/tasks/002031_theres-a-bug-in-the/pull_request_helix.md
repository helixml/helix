# refactor(api): remove phantom SystemSettings.MaxConcurrentDesktops

## Summary

The "Max Concurrent Desktops" field on `SystemSettings` was a phantom: it
was persisted, surfaced on `/api/v1/config`, and editable from the System
Settings admin UI, but no enforcement code ever read it. The real desktop
quota path (`hydra_executor.checkLimits` → `quota.LimitReached`) consults
only the Free/Pro env-var tier values
(`PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS`,
`PROJECTS_PRO_MAX_CONCURRENT_DESKTOPS`).

Worse, the help text claimed `0 = unlimited` while `handlers.go` treated
`0` as "fall back to free tier" (default `2`). Admins selecting Unlimited
got `2`. This blocked the helix-org manufacturing demo with a
`desktop quota reached on Helix (2/2 active)` error.

This change deletes the phantom rather than papering over it. To get
unlimited desktops, operators set `PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS=-1`
(and/or the PRO variant). `quota.go:268` already short-circuits negative
limits — no extra code path needed.

## Changes

### Backend deletions
- Drop `MaxConcurrentDesktops` from `SystemSettings`,
  `SystemSettingsRequest`, `SystemSettingsResponse`, and
  `ToResponseWithSource` (`api/pkg/types/system_settings.go`).
- Drop the patch branch in `api/pkg/store/store_system_settings.go`.
- Drop the `max_concurrent_desktops_updated` log key in
  `api/pkg/server/system_settings_handlers.go`.
- Drop the three assertions and two struct-literal fields in
  `api/pkg/store/store_system_settings_test.go`.

### `/api/v1/config` honesty
- Replace the inline fallback at `api/pkg/server/handlers.go:117-123` with
  the Free-tier env value. `/config` is registered on `insecureRouter`
  with no auth middleware, so the caller's user is never in context — we
  can't ask the quota manager for their resolved cap. Code comment
  documents why. Real enforcement still runs through the quota manager in
  `StartDesktop` and picks the correct Free/Pro tier per wallet.

### Documentation
- Add per-org-vs-per-user `-1=unlimited` doc comments on
  `PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS` /
  `PROJECTS_PRO_MAX_CONCURRENT_DESKTOPS` in `api/pkg/config/config.go`,
  on `ServerConfigForFrontend.MaxConcurrentDesktops` in
  `api/pkg/types/types.go`, and on `QuotaResponse.MaxConcurrentDesktops`
  in `api/pkg/types/quota.go`.

### Frontend
- Remove the "Max Concurrent Desktops" `<TableRow>`, the
  `handleSaveMaxDesktops` handler, and the
  `maxDesktopsValue` / `editingMaxDesktops` state hooks from
  `frontend/src/components/dashboard/SystemSettingsTable.tsx`.

### Tests
- Add `TestQuotaDesktopResolution` in `api/pkg/quota/quota_test.go`
  implementing the 15-row truth table from the spec (`enforce × freeEnv
  × proEnv × subscribed × ctx × active → effective_limit, allowed`).
  All 15 subtests pass.

### Generated
- Regenerated swagger JSON/YAML, `docs.go`, and frontend `api.ts` via
  `./stack update_openapi`.

## Compatibility

- **DB**: the `system_settings.max_concurrent_desktops` column becomes
  orphan data. GORM AutoMigrate doesn't drop columns; no read references
  it. Add an explicit drop migration later if `pg_dump` diffs become
  noisy.
- **API JSON shape**: `SystemSettingsRequest` / `SystemSettingsResponse`
  lose the field. `ServerConfigForFrontend.MaxConcurrentDesktops` stays
  but its source changes from "system setting" to "Free-tier env value".
- **Operators who relied on the System Settings slider**: switch to the
  env vars. `-1` = unlimited.

## Verification

- `go build ./...` clean.
- `go vet` clean on all touched packages.
- `go test ./pkg/quota/ -count=1` passes including
  `TestQuotaDesktopResolution` (15 subtests, all green).
- `yarn tsc` clean.
- End-to-end in inner Helix:
  - System Settings UI no longer shows the row (screenshot below).
  - `PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS=-1` → `/config` returns `-1`.
  - `PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS=1` → `/config` returns `1`.

## Screenshots

![System Settings after — Max Concurrent Desktops row removed](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002031_theres-a-bug-in-the/screenshots/01-system-settings-after.png)
