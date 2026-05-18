# Requirements: Remove the Phantom Max Concurrent Desktops Setting

## Background

The System Settings admin page exposes a field **"Max Concurrent Desktops"**
with the help text:

> Maximum number of concurrent desktop sessions per user (0 = unlimited)

After investigation, this setting is a **phantom**: it has no effect on
real enforcement.

### Proof

A full grep across `api/` for `SystemSettings.MaxConcurrentDesktops` shows
exactly one production read:

```
api/pkg/server/handlers.go:118
    config.MaxConcurrentDesktops = systemSettings.MaxConcurrentDesktops
```

That single read populates the `MaxConcurrentDesktops` field on the
`GET /api/v1/config` response, which is consumed by the frontend and by the
helix-org pre-flight check (`helix-org/helix/helixclient/client.go:560`).

The **actual desktop quota enforcement** lives elsewhere:

```
api/pkg/external-agent/hydra_executor.go:checkLimits (line 1448-1471)
    → quotaManager.LimitReached(Resource: ResourceDesktop)
api/pkg/quota/quota.go:LimitReached
    → getOrgQuotas / getUserQuotas
    → getFreeQuotas / getProQuotas
    → reads cfg.SubscriptionQuotas.Projects.{Free,Pro}.MaxConcurrentDesktops
```

The quota manager **never reads `SystemSettings.MaxConcurrentDesktops`**.
The only source of truth for the actual cap is the pair of env vars:

```go
// api/pkg/config/config.go:551-562
Projects struct {
    Free struct {
        MaxConcurrentDesktops int `envconfig:"PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS" default:"2"`
        ...
    }
    Pro struct {
        MaxConcurrentDesktops int `envconfig:"PROJECTS_PRO_MAX_CONCURRENT_DESKTOPS" default:"30"`
        ...
    }
}
```

Whether a caller gets the Free or Pro cap depends on whether their wallet
has an active Stripe subscription (user wallet for user-context sessions,
org wallet for org-context sessions). This is set at the env-var layer per
deployment.

### Consequence

The admin slider in the UI does change what `/api/v1/config` returns, which
in turn changes what helix-org's pre-flight check rejects. But it does **not**
change what the server itself rejects in `StartDesktop`. If anything bypasses
the pre-flight, the env-var cap is what bites. Two layers, two answers — and
the admin-visible knob is the wrong one.

The colleague's earlier instinct (use `-1` for unlimited) is correct as a
convention, but the cleaner fix is to **delete the phantom setting entirely**
and rely on the env-var tiers, which are already the source of truth.

## Chosen Direction

1. **Remove `SystemSettings.MaxConcurrentDesktops`.** The field, the DB
   column read, the System Settings UI row, the request/response wiring.
2. **Have `/api/v1/config` get its `MaxConcurrentDesktops` number from the
   quota manager** for the calling user, so the displayed cap matches what
   enforcement will actually do.
3. **Document the env-var control surface.** To raise the cap for everyone,
   set `PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS` (and/or `PROJECTS_PRO_…`).
   Setting either to `-1` yields unlimited (the quota manager already
   short-circuits `limit < 0` at `api/pkg/quota/quota.go:268`).
4. **Fix the per-user / per-org confusion** by updating the help text /
   documentation for the env vars and the `/api/v1/config` response: the
   cap is enforced per-org when the session has an `OrganizationID`,
   per-user otherwise.

## User Stories

### Story 1: Operator wants unlimited desktops

**As an** operator of a Helix deployment
**I want** a documented, working way to allow unlimited desktops
**So that** I can run demos / large teams without hitting an unexpected
quota wall.

**Acceptance Criteria**
- Setting `PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS=-1` (and optionally
  `PROJECTS_PRO_MAX_CONCURRENT_DESKTOPS=-1`) and restarting the API
  container causes `GET /api/v1/config` to return
  `max_concurrent_desktops: -1` for affected users.
- Opening any number of desktop sessions for those users succeeds — no
  rejection from `StartDesktop` and no pre-flight rejection from
  helix-org.
- Verification step in the docs: `curl /api/v1/config | jq
  '.max_concurrent_desktops'` returns `-1`.

### Story 2: Operator wants a specific cap

**As an** operator
**I want** to set a finite cap that applies consistently
**So that** what helix-org's pre-flight rejects matches what the server
itself rejects.

**Acceptance Criteria**
- Setting `PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS=N` makes
  `/api/v1/config` return `N` for Free-tier callers, and the `(N+1)`th
  `StartDesktop` is rejected with `desktop limit reached (N)`.
- Same applies for Pro-tier with `PROJECTS_PRO_MAX_CONCURRENT_DESKTOPS=N`.

### Story 3: Admin opening System Settings

**As an** admin of an existing deployment
**I want** the misleading "Max Concurrent Desktops" row removed from the
System Settings UI
**So that** I'm not tempted to set a value that the server ignores.

**Acceptance Criteria**
- The System Settings page no longer shows a "Max Concurrent Desktops" row.
- No request to `PATCH /api/v1/system_settings` carries a
  `max_concurrent_desktops` field after the change.
- Upgrade does not break for instances that previously saved a value: the
  DB column may remain as orphan data; nothing in the app reads it.

### Story 4: Per-user / per-org semantics are explicit

**As an** operator reading the docs / response schema
**I want** the description of the cap to say that enforcement is per-org
when the session has an org, otherwise per-user
**So that** I can predict behaviour in mixed environments.

**Acceptance Criteria**
- The Swagger description for `max_concurrent_desktops` on
  `ServerConfigForFrontend` (and `QuotaResponse`) says: "Cap on concurrent
  desktop sessions, enforced per organisation when the session has an org,
  per user otherwise. -1 = unlimited."
- The env vars in `api/pkg/config/config.go` have a matching one-line
  comment.

## Out of Scope (but related — flag for follow-up)

- **`SystemSettings.MaxConcurrentHeadlessSandboxes` and
  `SystemSettings.MaxConcurrentDesktopSandboxes`.** These are *real* — they
  are read and enforced by `api/pkg/quota/quota.go:97-104` for the newer
  sandbox-API. They have a related but separate ambiguity (`<= 0 →
  default 10`, no way to express "unlimited"). Not touched in this fix —
  worth a separate spec.
- Migrating away from a per-user / per-org cap split, per-tier quotas, or
  any subscription-billing changes.
- The way helix-org provisions one desktop per Worker chat — separate
  refactor.
- Auto-dropping the orphan DB column. GORM AutoMigrate only adds, never
  removes; an explicit migration would be needed if we wanted clean
  schema. Acceptable to leave as orphan for now.
