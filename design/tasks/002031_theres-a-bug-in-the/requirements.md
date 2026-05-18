# Requirements: Make Desktop Quota Settings Coherent and Honest

## Background

The System Settings admin page exposes a field **"Max Concurrent Desktops"**
with the help text:

> Maximum number of concurrent desktop sessions per user (0 = unlimited)

Three things are wrong with this:

1. **`0` does not mean unlimited.** The API treats `0` as "fall back to the
   Free-tier env-var default", which is `2`. So an admin who picks the
   "Unlimited" option silently gets capped at 2.
2. **"Per user" is at best half-true.** The real enforcement path
   (`api/pkg/external-agent/hydra_executor.go:checkLimits` → `api/pkg/quota/quota.go`)
   counts and caps **per-org when the session has an `OrganizationID`**, and
   only falls back to per-user when it doesn't. The help text never says
   this.
3. **The system setting is effectively dead in real enforcement.** It is
   only read by `/api/v1/config` (`api/pkg/server/handlers.go:117-123`),
   which the UI and the helix-org pre-flight check consume. The actual
   `LimitReached` call in `api/pkg/quota/quota.go:32-37` looks up the cap
   from `Cfg.SubscriptionQuotas.Projects.{Free,Pro}.MaxConcurrentDesktops`
   based on the Stripe subscription on the wallet, and **never consults
   `SystemSettings.MaxConcurrentDesktops`**. So an admin tweaking the
   System Settings slider changes what helix-org's pre-flight rejects, but
   never what `StartDesktop` itself enforces. The two layers can disagree.

## Where the "Free" cap comes from

Located in `api/pkg/config/config.go:548-562`:

```go
Projects struct {
    Enabled bool `envconfig:"PROJECTS_ENABLED" default:"true"`
    Free    struct {
        MaxConcurrentDesktops int `envconfig:"PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS" default:"2"`
        MaxProjects           int `envconfig:"PROJECTS_FREE_MAX_PROJECTS" default:"3"`
        MaxRepositories       int `envconfig:"PROJECTS_FREE_MAX_REPOSITORIES" default:"3"`
        MaxSpecTasks          int `envconfig:"PROJECTS_FREE_MAX_SPEC_TASKS" default:"500"`
    }
    Pro struct {
        MaxConcurrentDesktops int `envconfig:"PROJECTS_PRO_MAX_CONCURRENT_DESKTOPS" default:"30"`
        MaxProjects           int `envconfig:"PROJECTS_PRO_MAX_PROJECTS" default:"50"`
        MaxRepositories       int `envconfig:"PROJECTS_PRO_MAX_REPOSITORIES" default:"100"`
        MaxSpecTasks          int `envconfig:"PROJECTS_PRO_MAX_SPEC_TASKS" default:"50000"`
    }
}
```

So:
- **Free tier default = 2 desktops**, override via env
  `PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS`.
- **Pro tier default = 30 desktops**, override via env
  `PROJECTS_PRO_MAX_CONCURRENT_DESKTOPS`.
- Whether you get Free or Pro is decided by whether your **wallet has an
  active Stripe subscription** (`api/pkg/quota/quota.go:64`/`134`). User
  wallet for user-context sessions; org wallet for org-context sessions.

## How this setting relates to the other "desktop" settings

There are **three different "desktop"-ish caps** in System Settings. They
look similar but mean very different things — admins reasonably get confused.

| System Settings field                       | Help text says                                                          | What's actually capped                                                                    | Currently enforced where                                                                                          | Default if unset |
|---------------------------------------------|-------------------------------------------------------------------------|-------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------|------------------|
| `MaxConcurrentDesktops`                     | "concurrent desktop sessions **per user** (0 = unlimited)"              | Classic external-agent / Zed desktop sessions tied to a chat session                      | **NOWHERE in real enforcement.** Only displayed via `/api/v1/config`, consumed by UI and helix-org pre-flight.   | env `PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS` (2) via the `handlers.go` fallback that the spec is fixing |
| `MaxConcurrentHeadlessSandboxes`            | "Maximum concurrent headless and custom-image sandboxes **per organization**" | Newer sandbox-API headless containers (`api/pkg/sandbox/`)                                | `api/pkg/quota/quota.go:97-104` (org path only)                                                                  | 10 (`DefaultMaxConcurrentHeadlessSandboxes`) |
| `MaxConcurrentDesktopSandboxes`             | "Maximum concurrent ubuntu-desktop sandboxes **per organization**"      | Newer sandbox-API `ubuntu-desktop` runtime containers (a separate path from #1)           | `api/pkg/quota/quota.go:97-104` (org path only)                                                                  | 10 (`DefaultMaxConcurrentDesktopSandboxes`) |

Observations the implementing agent needs to keep in mind:

- The other two (`MaxConcurrentHeadlessSandboxes` / `MaxConcurrentDesktopSandboxes`) are correctly described as **per-organisation**, and the code agrees.
- They use a `<= 0` → default convention (see `EffectiveMaxConcurrentHeadlessSandboxes` / `EffectiveMaxConcurrentDesktopSandboxes` at `api/pkg/types/system_settings.go:219-231`), which conflicts with the convention being introduced here (`-1` = explicit unlimited). Resolving this consistently across all three settings is **explicitly in scope**.
- The "per user" label on `MaxConcurrentDesktops` is wrong: real enforcement is "per org when session has an `OrganizationID`, otherwise per user".

## Chosen Direction

1. **Make `SystemSettings.MaxConcurrentDesktops` actually enforce.** Wire it
   through `quota.go` as the authoritative operator-wide cap. The Free/Pro
   subscription tiers from env vars become defaults that the operator can
   override from the UI.
2. **`-1` means explicit unlimited.** Consistent with the convention already
   used by `api/pkg/quota/quota.go:58/129/268`. `0` continues to mean "unset
   — fall back to the subscription-tier default". `N > 0` means an explicit
   finite cap that overrides the subscription tier.
3. **Fix the help text** to say "per user, or per organisation if the
   session belongs to one". Same correction applies to the chip and tooltip.
4. **Apply the same `-1`/`0`/`N` convention to `MaxConcurrentHeadlessSandboxes`
   and `MaxConcurrentDesktopSandboxes`** so admins don't have to memorise
   three different conventions. (Today they use `<= 0` → default-of-10; new
   model: `-1` → unlimited, `0` → default-of-10, `N` → explicit cap.)

## Value Semantics (target state, all three settings)

| Stored value | Meaning                                          | Effective cap returned by the resolver                            |
|--------------|--------------------------------------------------|-------------------------------------------------------------------|
| `-1`         | Admin explicitly chose **Unlimited**             | `-1`                                                              |
| `0`          | Unset — fall back to operator default            | For `MaxConcurrentDesktops`: subscription-tier value (Free or Pro env var). For sandbox limits: `Default{Headless,Desktop}Sandboxes` constant (10) |
| `N > 0`      | Explicit finite cap, overrides tier              | `N`                                                               |

## User Stories

### Story 1: Admin sets desktops to Unlimited

**As an** admin operator
**I want** selecting `Unlimited` for Max Concurrent Desktops to actually
allow unlimited desktops, in both the pre-flight check AND the server-side
enforcement
**So that** the UI and the server agree, and large demos / teams aren't
unexpectedly blocked.

**Acceptance Criteria**
- Selecting `Unlimited` stores `-1` in `system_settings.max_concurrent_desktops`.
- `GET /api/v1/config` returns `max_concurrent_desktops: -1`.
- `POST` to start a new desktop session via `StartDesktop` is **not** rejected
  for desktop quota reasons, regardless of how many sessions are active, for
  both user-context and org-context sessions.
- helix-org's `HasDesktopRoom` pre-flight permits the open.

### Story 2: Admin sets a finite cap (overrides Free / Pro tier)

**As an** admin
**I want** setting a positive value (e.g. `5`) to enforce that cap regardless
of whether the user is on Free or Pro tier
**So that** I have a single operator-controlled cap that I can reason about,
rather than two layers fighting each other.

**Acceptance Criteria**
- Saving `5` makes `GET /api/v1/config` return `max_concurrent_desktops: 5`.
- A 6th `StartDesktop` call for the same user (no org) fails with `desktop
  limit reached (5)`.
- Same applies in an org context: 6th desktop in the same org fails.
- A user on a Pro subscription (default tier limit 30) still gets the cap of
  `5`, because the admin override wins.

### Story 3: Admin has never touched the setting

**As an** operator of a brand-new or upgraded instance
**I want** sensible defaults that match the existing Free / Pro tier
behaviour
**So that** upgrading does not silently change my enforcement.

**Acceptance Criteria**
- Fresh install (DB value `0`): cap = Free tier env var
  (`PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS`, default 2) for users without an
  active Stripe subscription; Pro tier env var
  (`PROJECTS_PRO_MAX_CONCURRENT_DESKTOPS`, default 30) for those with one.
- Existing installs upgrading with the value previously left at `0` see the
  same behaviour they had before the fix.
- Existing installs that previously saved `0` *thinking it meant unlimited*
  continue to be capped (no silent flip to unlimited — admin must
  re-select `Unlimited` to make `-1` real).

### Story 4: Per-user vs per-org semantics are honest

**As an** admin reading the help text
**I want** the text to explain that the cap is per-org if the session has
an org, otherwise per-user
**So that** I can predict how the cap will apply in mixed environments.

**Acceptance Criteria**
- The help text under "Max Concurrent Desktops" reads (or equivalent): "Cap
  on concurrent desktop sessions per organisation (or per user if the
  session has no org). Overrides the subscription-tier default."
- The same correction is applied to any associated tooltip / chip.

### Story 5: All three desktop-related settings use the same convention

**As an** admin who has more than one desktop-ish limit to tune
**I want** `MaxConcurrentDesktops`, `MaxConcurrentHeadlessSandboxes`, and
`MaxConcurrentDesktopSandboxes` to use the same value convention
(`-1` / `0` / `N`)
**So that** I don't have to memorise different rules for each.

**Acceptance Criteria**
- All three settings accept `-1` for unlimited.
- All three settings interpret `0` as "use operator default" (sub-tier env
  var for `MaxConcurrentDesktops`; constant `10` for the sandbox limits).
- Their UI controls and help text use the same wording pattern.

## Out of Scope

- Re-architecting Free / Pro / Enterprise tier resolution.
- Per-user or per-org *individual overrides* of the operator cap (today
  it's effectively system-wide).
- Changing the way `helix-org` provisions one desktop per Worker chat —
  that's a separate, larger refactor.
- Auto-migrating existing `0` rows to `-1` (rejected: silent flip to
  unlimited is a resource-exposure risk on shared instances).
