# Design: Make Desktop Quota Settings Coherent and Honest

## Problem Recap

Three intertwined defects in the desktop-quota story, all surfaced by the
demo-blocking 2/2 incident:

1. `SystemSettings.MaxConcurrentDesktops` set to `0` (UI labelled
   "Unlimited") falls back to the Free-tier env-var default (`2`) instead of
   actually unlimited.
2. The system setting is **only** read by `/api/v1/config` —
   `api/pkg/server/handlers.go:117-123` — and never consulted by the actual
   enforcement path (`api/pkg/external-agent/hydra_executor.go:checkLimits`
   → `api/pkg/quota/quota.go:LimitReached`). So tweaking it changes what
   helix-org's pre-flight rejects but not what the server itself enforces.
3. The help text claims "per user". The real enforcement is per-org when
   the session has an `OrganizationID`, otherwise per-user. Three sibling
   settings (`MaxConcurrentDesktops`, `MaxConcurrentHeadlessSandboxes`,
   `MaxConcurrentDesktopSandboxes`) use different conventions for "unset"
   and "unlimited".

## Where things live (map for the implementing agent)

| Concern | File |
|---|---|
| Subscription-tier env vars (Free / Pro) | `api/pkg/config/config.go:523-563` |
| System Settings type | `api/pkg/types/system_settings.go` |
| System Settings → response mapping | `api/pkg/types/system_settings.go:160-209` (`ToResponseWithSource`) |
| Sandbox-limit effective-value helpers | `api/pkg/types/system_settings.go:219-231` |
| `/api/v1/config` desktop fields | `api/pkg/server/handlers.go:112-141` |
| Quota resolver (user vs org) | `api/pkg/quota/quota.go:32-186` |
| Desktop quota enforcement | `api/pkg/external-agent/hydra_executor.go:1448-1471` (`checkLimits`) |
| System Settings admin UI | `frontend/src/components/dashboard/SystemSettingsTable.tsx` (3 separate rows) |
| helix-org pre-flight | `helix-org/helix/helixclient/client.go:560` (`HasDesktopRoom`) — external to this repo, do not edit unless cross-cutting needed |

## Chosen Approach: `-1` Means Unlimited, System Setting Becomes Authoritative

1. Use **`-1` as the explicit sentinel for "unlimited"**, consistent with the
   convention already in `api/pkg/quota/quota.go:58/129/268`.
2. **Wire `SystemSettings.MaxConcurrentDesktops` through the quota resolver**
   so the System Settings value actually enforces. Today the resolver only
   uses Free/Pro tier env vars; we make the system setting an override on
   top of that.
3. Apply the same `-1`/`0`/`N` convention to
   `MaxConcurrentHeadlessSandboxes` and `MaxConcurrentDesktopSandboxes` so
   admins don't have to memorise three different conventions.

### Value semantics (target state, all three settings)

| Stored | Meaning                                          | Effective cap                                            |
|--------|--------------------------------------------------|----------------------------------------------------------|
| `-1`   | Admin explicitly chose Unlimited                 | `-1`                                                     |
| `0`    | Unset — fall back to operator default            | For `MaxConcurrentDesktops`: subscription-tier value (Free env or Pro env depending on wallet). For sandbox limits: `Default{Headless,Desktop}Sandboxes` constant (10) |
| `N>0`  | Explicit finite cap, overrides tier              | `N`                                                      |

`EnforceQuotas == false` short-circuits everything to `-1` (unlimited)
regardless — preserves today's `quota.go` behaviour.

### Why not the alternatives

**Drop the system setting; just use env vars.** Cleanest architecturally but
removes admin self-service. Operators rightly expect to tweak this from the
UI. Rejected.

**Keep the system setting "advisory-only" (today's behaviour) and just fix
the `0 = unlimited` semantics.** Doesn't resolve the deeper bug that the
two layers disagree on what the cap actually is. Admins would still be able
to set a value that the server quietly ignores in real enforcement.
Rejected.

**Pointer (`*int`) on the field to distinguish nil vs 0.** More mechanical
work (column nullability, migrations, request glue) for the same observable
behaviour as the `-1` convention. The `-1` convention is already in
`quota.go`. Rejected — pick one convention and use it everywhere.

## Truth Table (the spec for the new resolver)

Inputs:
- `enforce` — `SystemSettings.EnforceQuotas` (bool)
- `sys` — `SystemSettings.MaxConcurrentDesktops` (int: `-1` / `0` / `N>0`)
- `tier` — `"free"` or `"pro"` (decided by wallet's Stripe subscription)
- `ctx` — `"user"` or `"org"` (decided by whether the request has `OrganizationID`)
- `active` — current count of concurrent desktops (per-user OR per-org, by `ctx`)
- env: `PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS=2`, `PROJECTS_PRO_MAX_CONCURRENT_DESKTOPS=30`

Outputs:
- `effective_limit` — value returned in `QuotaResponse.MaxConcurrentDesktops`
- `allowed` — whether the next `StartDesktop` call succeeds

| # | enforce | sys  | tier | ctx  | active | → effective_limit | → allowed | notes |
|---|---------|------|------|------|--------|-------------------|-----------|-------|
| 1 | false   | any  | any  | any  | any    | `-1`              | yes       | EnforceQuotas off short-circuits |
| 2 | true    | `-1` | free | user | 0      | `-1`              | yes       | explicit unlimited, per-user |
| 3 | true    | `-1` | free | org  | 99     | `-1`              | yes       | explicit unlimited, per-org |
| 4 | true    | `-1` | pro  | user | 0      | `-1`              | yes       | unlimited beats Pro tier |
| 5 | true    | `0`  | free | user | 1      | `2`               | yes       | unset → Free-tier default; under cap |
| 6 | true    | `0`  | free | user | 2      | `2`               | no        | unset → Free-tier default; at cap |
| 7 | true    | `0`  | free | org  | 1      | `2`               | yes       | per-org count; under Free-tier cap |
| 8 | true    | `0`  | free | org  | 2      | `2`               | no        | per-org count; at Free-tier cap |
| 9 | true    | `0`  | pro  | user | 29     | `30`              | yes       | unset → Pro-tier default; under cap |
| 10| true    | `0`  | pro  | user | 30     | `30`              | no        | unset → Pro-tier default; at cap |
| 11| true    | `0`  | pro  | org  | 29     | `30`              | yes       | per-org count; Pro tier |
| 12| true    | `5`  | free | user | 4      | `5`               | yes       | admin override beats tier |
| 13| true    | `5`  | free | user | 5      | `5`               | no        | admin override; at cap |
| 14| true    | `5`  | pro  | user | 5      | `5`               | no        | admin override beats Pro |
| 15| true    | `5`  | free | org  | 5      | `5`               | no        | admin override, per-org |
| 16| true    | `5`  | pro  | org  | 4      | `5`               | yes       | admin override beats Pro, per-org |
| 17| true    | `100`| free | user | 50     | `100`             | yes       | admin can raise above Pro too |

This table is the source of truth for the table-test in
`api/pkg/quota/quota_test.go`.

## Architecture Changes

### 1. Resolver helper (`api/pkg/types/system_settings.go`)

Add a helper next to the existing `Effective…Sandboxes` ones at
`system_settings.go:219-231`:

```go
// EffectiveMaxConcurrentDesktops resolves the system-setting + tier-default
// into the cap the quota system should enforce. Returns -1 for unlimited.
//   sys == -1 → -1 (admin override: unlimited)
//   sys ==  0 → tierDefault (unset; fall back to subscription tier)
//   sys  >  0 → sys (admin override beats tier)
func (s *SystemSettings) EffectiveMaxConcurrentDesktops(tierDefault int) int {
    if s.MaxConcurrentDesktops < 0 {
        return -1
    }
    if s.MaxConcurrentDesktops == 0 {
        return tierDefault
    }
    return s.MaxConcurrentDesktops
}
```

Refactor the existing `EffectiveMaxConcurrentHeadlessSandboxes` and
`EffectiveMaxConcurrentDesktopSandboxes` (`system_settings.go:219-231`) to
match the same three-state convention (currently they do `<= 0 → default`):

```go
func (s *SystemSettings) EffectiveMaxConcurrentHeadlessSandboxes() int {
    if s.MaxConcurrentHeadlessSandboxes < 0 { return -1 }
    if s.MaxConcurrentHeadlessSandboxes == 0 { return DefaultMaxConcurrentHeadlessSandboxes }
    return s.MaxConcurrentHeadlessSandboxes
}
// (same for Desktop variant)
```

Behaviour change for the sandbox helpers: today, an admin who saves `-5`
gets `default 10`; under the new model they'd get `-1` (unlimited). This is
a deliberate alignment — no admin should be saving negative-but-not-`-1`
values today.

### 2. Quota resolver (`api/pkg/quota/quota.go`)

The two getter functions (`getFreeQuotas`, `getProQuotas`) currently read
straight from the tier env vars. Replace them with a single helper that
threads the system setting through:

```go
func (m *DefaultQuotaManager) getTierDesktopCap(isPro bool) int {
    if isPro {
        return m.cfg.SubscriptionQuotas.Projects.Pro.MaxConcurrentDesktops
    }
    return m.cfg.SubscriptionQuotas.Projects.Free.MaxConcurrentDesktops
}
```

In both `getOrgQuotas` and `getUserQuotas`, after deciding `isPro`, apply
the system-setting override:

```go
tierCap := m.getTierDesktopCap(isPro)
quotas.MaxConcurrentDesktops = systemSettings.EffectiveMaxConcurrentDesktops(tierCap)
```

Leave the `!EnforceQuotas → -1` short-circuit at the top intact. Keep the
rest of the tier-default-driven fields (`MaxProjects`, `MaxRepositories`,
`MaxSpecTasks`) untouched — they're out of scope for this fix.

The end result: `quota.go`'s `MaxConcurrentDesktops` field is now the same
number that `/api/v1/config` will return, because both go through the same
resolver.

### 3. `/api/v1/config` handler (`api/pkg/server/handlers.go:117-123`)

Replace the inline fallback with the resolver. But — and this is the
subtle bit — at this point the handler doesn't know which user is calling,
so it can't know whether to use Free or Pro tier. The cleanest fix:

- Make `/api/v1/config` return the **Free-tier-resolved** value (effectively
  the floor; most clients are unsubscribed users).
- Or, better: have the handler call the quota manager for the caller's
  user/org to get the actual cap they'd see. This is a slightly bigger
  change but eliminates the disagreement helix-org has been working around.

Recommendation: **go with the quota-manager call**. Code becomes:

```go
quotaReq := &types.QuotaRequest{
    UserID:         user.ID, // already available in this handler? — verify
    OrganizationID: orgID,    // if available
}
quotas, err := apiServer.quotaManager.GetQuotas(ctx, quotaReq)
if err != nil { ... }
config.MaxConcurrentDesktops = quotas.MaxConcurrentDesktops
```

If the handler doesn't have the user in scope (need to verify — the existing
`getConfig` is called via `config` at line 144 which receives `req`),
extract user from `req` context. If extraction is invasive, document the
floor-fallback compromise and pick the Free-tier resolver instead.

### 4. Frontend (`SystemSettingsTable.tsx`)

Three rows to update, all in the same file:

**Max Concurrent Desktops row** (`SystemSettingsTable.tsx:113-132, 731-790`):
- `handleSaveMaxDesktops` (line 116) currently rejects negative input. Update
  the validation to accept `-1` (and only `-1`) as a negative value. Better
  UX: replace the free-form input with a small selector
  (`Unlimited` / `Use server default` / `Custom cap N`) that writes
  `-1` / `0` / `N`.
- Chip / display logic (line 738-742) currently shows `Limit: N` for truthy
  and `Unlimited` for falsy. Rewrite as three-state: `Unlimited` for `< 0`,
  `Default` (or `Default (2)` if we expose the resolved tier value) for
  `== 0`, `Limit: N` for `> 0`.
- Help text (line 731-734) currently says "Maximum number of concurrent
  desktop sessions per user (0 = unlimited)". Replace with:
  > "Cap on concurrent desktop sessions per organisation (or per user if the
  > session has no org). Overrides the subscription-tier default. -1 =
  > unlimited, 0 = use server default."

**Headless Sandbox Limit / Desktop Sandbox Limit rows** (`SystemSettingsTable.tsx:1014-1135`):
- Same three-state picker treatment for parity.
- Update chip labels and help text to acknowledge `-1` / `0` / `N`.
- Validators must allow `-1`.

### 5. helix-org pre-flight client

`helix-org/helix/helixclient/client.go:560` (`HasDesktopRoom`): already
treats `Max <= 0` as unlimited. **Re-verify during implementation** — the
new server behaviour means `Max == 0` should never appear on the wire
(`/api/v1/config` now returns the resolved value, never `0`), but tolerant
is fine.

## Compatibility & Migration

- **DB**: no migration. All three columns stay `int NOT NULL DEFAULT 0`.
- **Existing rows with `0`**: continue to mean "fall back to operator
  default". Behaviour preserved on upgrade.
- **Existing rows where an admin saved `0` thinking it meant unlimited**:
  still capped post-upgrade. They must explicitly re-select `Unlimited` to
  store `-1`. This is a safer default than silently flipping them to
  unlimited.
- **`/api/v1/config` JSON shape**: unchanged. `MaxConcurrentDesktops` stays
  an `int`. The only new wire value is `-1`.
- **helix-org**: unchanged. `HasDesktopRoom` already tolerates `<= 0`.

## Testing Plan

### Go unit tests

1. **`TestEffectiveMaxConcurrentDesktops`** in `api/pkg/types/system_settings_test.go`:
   table-test on the helper with inputs `-1` / `0` / `N` × tier defaults
   `2` / `10` / `30`.
2. **`TestQuotaDesktopResolution`** in `api/pkg/quota/quota_test.go`:
   table-test on `LimitReached(ResourceDesktop)` covering all 17 rows of
   the truth table above. Use the existing `quota_test.go` mocking pattern
   (gomock store + mock external-agent executor for `ListSessions`).
3. **`TestEffectiveSandboxLimits`** in `api/pkg/types/system_settings_test.go`:
   table-test for the refactored sandbox helpers covering `-1` / `0` / `N`
   for both headless and desktop sandbox variants.

### Handler / integration tests

4. Handler-level test that `GET /api/v1/config` returns the **resolved**
   value of `MaxConcurrentDesktops` for the calling user (Free vs Pro vs
   admin-overridden).

### End-to-end (per `helix/CLAUDE.md`)

5. Inner Helix at `http://localhost:8080`:
   - Log in as test user; save `Unlimited` in the admin UI → DB shows
     `-1`; `/api/v1/config` returns `-1`; opening more than 2 desktops is
     not rejected.
   - Save `1` → DB shows `1`; second desktop attempt is rejected.
   - Save `0` (or pick "Use server default") → DB shows `0`; cap reverts
     to Free-tier default.
   - Repeat the `Unlimited` / `1` / `0` sequence with an org-scoped
     session; confirm the cap is enforced per-org (a second user in the
     same org counts toward the same limit).

## Notes for the Implementing Agent

- **Do not auto-migrate existing `0` rows to `-1`.** Silent flip to
  unlimited is a resource-exposure risk on shared instances.
- Reuse the `-1` convention from `quota.go` — don't invent a new sentinel.
- `SystemSettingsRequest.MaxConcurrentDesktops` is already `*int`. No
  request shape change needed.
- The frontend changes are the largest surface area. Don't ship the backend
  without the matching UI updates, or admins will be unable to express `-1`
  through the UI at all.
- `EffectiveMaxConcurrentDesktopSandboxes` and
  `EffectiveMaxConcurrentHeadlessSandboxes` are also being changed
  (`<= 0 → default` becomes `< 0 → -1`, `== 0 → default`). Hunt for callers
  to confirm none rely on the old behaviour for negative values. There
  shouldn't be any, but search and confirm.
- The `getConfig` handler may not currently have the caller's user in
  scope. If extracting it is invasive, document and fall back to returning
  the Free-tier resolution (the floor); this still fixes the immediate
  reported bug.
- Test end-to-end in the inner Helix per `helix/CLAUDE.md` — unit tests are
  not a substitute.
- Use full PR URL (`https://github.com/helixml/helix/pull/<n>`) per
  `helix/CLAUDE.md` communication rules.
