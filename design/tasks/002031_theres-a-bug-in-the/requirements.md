# Requirements: Honour "0 = Unlimited" for Max Concurrent Desktops

## Background

The System Settings admin page exposes a field **"Max Concurrent Desktops"** with
the help text:

> Maximum number of concurrent desktop sessions per user (0 = unlimited)

However, the API's `/api/v1/config` endpoint does **not** treat 0 as unlimited.
It falls back to the Free-tier default (typically `2`) whenever the saved value
is 0. So an admin who deliberately sets the slider to `Unlimited` (which stores
`0`) silently gets capped at 2 desktop sessions — the opposite of what the UI
promises.

This blocks legitimate use cases such as recording the helix-org manufacturing
demo, where more than 2 worker desktops are needed concurrently.

## Root Cause (already diagnosed)

`api/pkg/server/handlers.go:117-123`:

```go
config.MaxConcurrentDesktops = systemSettings.MaxConcurrentDesktops
// If system settings doesn't have a max, fall back to the free tier config
if config.MaxConcurrentDesktops == 0 {
    config.MaxConcurrentDesktops = apiServer.Cfg.SubscriptionQuotas.Projects.Free.MaxConcurrentDesktops
}
```

`SystemSettings.MaxConcurrentDesktops` is a plain `int`
(`api/pkg/types/system_settings.go:37`), so "user explicitly set 0" and "field
never set" are indistinguishable from each other and from "use free-tier
default". The downstream UI (`SystemSettingsTable.tsx:739`) and quota code
(`api/pkg/quota/quota.go:268`, which uses `-1` for unlimited) already interpret
0 / non-positive values as unlimited, so it's only this one handler that lies.

## User Stories

### Story 1: Admin sets desktops to Unlimited

**As an** admin operator of a Helix instance
**I want** setting Max Concurrent Desktops to `0` (UI label: `Unlimited`) to
actually grant unlimited desktops
**So that** the UI help text matches the runtime behaviour and I can host
demos / large teams without hitting an unexpected `2/2` quota wall.

**Acceptance Criteria**
- Setting `max_concurrent_desktops = 0` via the System Settings admin UI causes
  `GET /api/v1/config` to return a value that callers interpret as "no limit"
  (the existing convention is `-1`; see `quota.go`).
- Opening a new desktop session via helix-org's pre-flight check
  (`helix-org/helix/helixclient/client.go:560` / `HasDesktopRoom`) succeeds
  regardless of how many sessions are already active.
- `StartDesktop` on the API side does not reject the new session for quota
  reasons.

### Story 2: Admin sets a finite cap

**As an** admin operator
**I want** setting a non-zero positive value (e.g. `5`) to enforce that exact
cap
**So that** I can still constrain resource use on shared deployments.

**Acceptance Criteria**
- Saving `max_concurrent_desktops = 5` causes `GET /api/v1/config` to return
  `max_concurrent_desktops: 5`.
- The 6th attempted desktop session is rejected with a clear "quota reached"
  error.

### Story 3: Admin has never touched the setting (fresh install)

**As an** operator of a brand-new instance
**I want** sensible defaults so that I'm not accidentally exposed to unlimited
desktops out of the box
**So that** a fresh Helix install behaves the same as today for unconfigured
deployments.

**Acceptance Criteria**
- On a fresh install where the admin has not touched the setting, the
  effective cap remains the value of
  `Cfg.SubscriptionQuotas.Projects.Free.MaxConcurrentDesktops` (the
  environment-driven default — currently `2`, override via
  `PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS`).
- Existing deployments that have never edited the setting see no behavioural
  change after upgrading.

## Out of Scope

- Re-architecting per-tier (Free / Pro / Enterprise) quota resolution.
- Per-user or per-org overrides of `MaxConcurrentDesktops` (today it's
  effectively system-wide).
- Changing the way `helix-org` provisions one desktop per Worker chat — that's
  a separate, larger refactor.
- Modifying the UI label itself (the label is correct; the server must be
  fixed to match).
