# Requirements: Honour Explicit Unlimited for Max Concurrent Desktops

## Background

The System Settings admin page exposes a field **"Max Concurrent Desktops"**
with the help text:

> Maximum number of concurrent desktop sessions per user (0 = unlimited)

However, the API's `/api/v1/config` endpoint does **not** treat 0 as unlimited.
It falls back to the Free-tier default (typically `2`) whenever the saved value
is 0. So an admin who deliberately sets the slider to `Unlimited` (which today
stores `0`) silently gets capped at 2 desktop sessions — the opposite of what
the UI promises.

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
(`api/pkg/types/system_settings.go:37`), so "user explicitly chose unlimited"
and "field never set / use free-tier default" are forced through the same
value (`0`) and cannot be told apart.

## Chosen Direction

Use **`-1` as the explicit "unlimited" sentinel**, consistent with how the
quota system already represents unlimited
(`api/pkg/quota/quota.go:58`/`129`/`268`). `0` keeps its existing meaning of
"unset / fall back to server default", which preserves behaviour for every
deployment that has never touched the setting. Positive integers continue to
mean a finite cap.

## Value Semantics (target state)

| Stored value | Meaning                                              | Effective behaviour                                            |
|--------------|------------------------------------------------------|----------------------------------------------------------------|
| `-1`         | Admin explicitly chose **Unlimited**                  | No quota gate on desktop sessions                              |
| `0`          | Unset — fall back to server default                  | Free-tier default (`PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS`, currently `2`) |
| `N > 0`      | Explicit finite cap                                  | `N`                                                            |

## User Stories

### Story 1: Admin sets desktops to Unlimited

**As an** admin operator of a Helix instance
**I want** selecting `Unlimited` in the admin UI to actually grant unlimited
desktops
**So that** the UI and runtime agree, and I can host demos / large teams
without hitting an unexpected `2/2` quota wall.

**Acceptance Criteria**
- Selecting `Unlimited` in the System Settings admin UI stores `-1` in the
  `system_settings.max_concurrent_desktops` column.
- `GET /api/v1/config` then returns `max_concurrent_desktops: -1`.
- Opening additional desktop sessions via helix-org's pre-flight check
  (`helix-org/helix/helixclient/client.go:560` / `HasDesktopRoom`) succeeds
  regardless of how many sessions are already active.
- `StartDesktop` on the API side does not reject new sessions for quota
  reasons.
- The admin UI displays the chip / row as `Unlimited` when the stored value
  is `-1`.

### Story 2: Admin sets a finite cap

**As an** admin operator
**I want** setting a positive value (e.g. `5`) to enforce that exact cap
**So that** I can still constrain resource use on shared deployments.

**Acceptance Criteria**
- Saving `max_concurrent_desktops = 5` causes `GET /api/v1/config` to return
  `max_concurrent_desktops: 5`.
- The 6th attempted desktop session is rejected with a clear "quota reached"
  error.

### Story 3: Admin has never touched the setting (fresh install / upgrade)

**As an** operator of a brand-new instance, or one upgrading from before this
change
**I want** sensible defaults so that I'm not accidentally exposed to unlimited
desktops out of the box
**So that** behaviour stays predictable across upgrades.

**Acceptance Criteria**
- On a fresh install where the admin has not touched the setting (DB value
  `0`), the effective cap remains
  `Cfg.SubscriptionQuotas.Projects.Free.MaxConcurrentDesktops` (currently
  `2`, override via `PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS`).
- Existing deployments where the admin previously saved `0` (mistakenly
  believing it would unlock unlimited) see **no behavioural change** after
  upgrading — they still get the free-tier default. If they truly want
  unlimited, they must explicitly re-save `Unlimited` (which now stores
  `-1`).
- The admin UI clearly distinguishes the three states so that the difference
  between "Unlimited" and "Use server default" is unambiguous.

## Out of Scope

- Re-architecting per-tier (Free / Pro / Enterprise) quota resolution.
- Per-user or per-org overrides of `MaxConcurrentDesktops` (today it's
  effectively system-wide).
- Changing the way `helix-org` provisions one desktop per Worker chat — that's
  a separate, larger refactor.
- Auto-migrating existing `0` rows to `-1` (rejected: silent flip to
  unlimited is a resource-exposure risk on shared instances; admins who want
  unlimited can re-save explicitly).
