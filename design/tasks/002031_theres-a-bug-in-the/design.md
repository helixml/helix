# Design: Honour Explicit Unlimited for Max Concurrent Desktops

## Problem Recap

`api/pkg/server/handlers.go:117-123` treats an explicit `0` in
`SystemSettings.MaxConcurrentDesktops` as "field unset, fall back to free
tier", but the System Settings UI label promises `0 = unlimited`. The two
meanings collide on the same value, so the user-visible behaviour silently
contradicts the help text.

## Chosen Approach: `-1` Means Unlimited

Per colleague feedback, **use `-1` as the explicit sentinel for "unlimited"**.
This keeps the field as a plain `int` (no nullable column / migration) and
aligns with the existing convention in `api/pkg/quota/quota.go:58`/`129`/`268`,
where `MaxConcurrentDesktops: -1` already means "no limit".

### Value Semantics (target state)

| Stored value | Meaning                                          | Effective `MaxConcurrentDesktops` returned by `/api/v1/config` |
|--------------|--------------------------------------------------|----------------------------------------------------------------|
| `-1`         | Admin explicitly chose **Unlimited**             | `-1`                                                           |
| `0`          | Unset — fall back to server default              | `Cfg.SubscriptionQuotas.Projects.Free.MaxConcurrentDesktops` (currently `2`, env `PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS`) |
| `N > 0`      | Explicit finite cap                              | `N`                                                            |

### Why this option, not the alternatives

**`-1` sentinel (chosen)** ✅
- No schema change — column stays a plain `int`.
- Reuses the convention already used by `quota.go`, so the "unlimited" path
  is consistent end-to-end.
- Existing rows storing `0` keep their current behaviour after upgrade
  (free-tier default) — no silent flip to unlimited. Admins who actually want
  unlimited explicitly re-select it.
- Cost: requires the UI to be updated so admins can produce the `-1` value
  (today they can only enter `>= 0`), and the displayed help text / chip need
  updating.

**`*int` pointer (previously proposed, now rejected)**
- More mechanical work (DB migration to allow NULL, type change, request /
  store glue) for the same observable outcome.
- Three meanings (unset / `0` / `>0`) mapped onto `nil` / `*int(0)` / `*int(N)`
  works but adds nullability to a column that doesn't otherwise need it.

**Drop the fallback entirely; treat `0` as unlimited**
- Silently flips existing instances that have `0` saved from "capped at 2" to
  "unlimited" on upgrade — resource-exposure risk on shared deployments.
  Rejected on safety grounds.

**Change only the UI label to "use server default"**
- Closes the bug by removing the feature. Rejected — admins legitimately want
  an "Unlimited" mode.

## Architecture Changes

### 1. Resolution helper (`api/pkg/types/system_settings.go`)

Add a method next to the existing `EffectiveMaxConcurrentHeadlessSandboxes` /
`EffectiveMaxConcurrentDesktopSandboxes` helpers (`system_settings.go:219-231`):

```go
// EffectiveMaxConcurrentDesktops resolves the three states.
// Returns -1 to mean unlimited (consistent with quota.go convention).
func (s *SystemSettings) EffectiveMaxConcurrentDesktops(freeTierDefault int) int {
    if s.MaxConcurrentDesktops < 0 {
        return -1
    }
    if s.MaxConcurrentDesktops == 0 {
        return freeTierDefault
    }
    return s.MaxConcurrentDesktops
}
```

Field type stays `int`. **No DB migration.** **No change to
`SystemSettingsRequest.MaxConcurrentDesktops`** (it's already `*int`, which
correctly distinguishes "patch with -1" from "field absent from request").

### 2. Handler change (`api/pkg/server/handlers.go:117-123`)

Replace the inline fallback with the helper:

```go
config.MaxConcurrentDesktops = systemSettings.EffectiveMaxConcurrentDesktops(
    apiServer.Cfg.SubscriptionQuotas.Projects.Free.MaxConcurrentDesktops,
)
```

Delete the old branch entirely (`NO FALLBACKS` rule from `helix/CLAUDE.md` —
one path, fix properly).

### 3. Response mapping (`SystemSettings.ToResponseWithSource`)

`SystemSettingsResponse.MaxConcurrentDesktops` should reflect the **stored
raw value** (`-1` / `0` / `N`), not the resolved one. Admin UI needs to know
the raw stored intent to render the three distinct states correctly. The
*effective* resolved value lives on `ServerConfigForFrontend` and that's what
consumers like helix-org read.

So `ToResponseWithSource` simply passes `s.MaxConcurrentDesktops` through —
no change to current behaviour beyond accepting `-1` as a valid value.

### 4. Frontend (`frontend/src/components/dashboard/SystemSettingsTable.tsx`)

Multiple changes required — the current implementation is the source of the
"saved `0` thinking it meant unlimited" trap.

**`handleSaveMaxDesktops` (line 113-132)**: current guard rejects negatives:

```ts
if (isNaN(value) || value < 0) { snackbar.error('…non-negative number'); return }
```

Update to allow `-1` (and only `-1`) as a negative value, since that is now
the unlimited sentinel. Alternative (better UX): replace the free-form
number input with a small mode picker: `Unlimited` / `Use server default` /
`Custom cap (N)`, where the picker writes `-1` / `0` / `N` respectively. The
admin never needs to know that `-1` is the sentinel.

**Chip display (line 738-742)**:

```ts
label={settings?.max_concurrent_desktops ? `Limit: ${settings.max_concurrent_desktops}` : 'Unlimited'}
```

This is broken under the new model. Rewrite as:

```ts
const v = settings?.max_concurrent_desktops ?? 0
const label =
  v < 0  ? 'Unlimited' :
  v === 0 ? `Default (${serverDefault})` :
            `Limit: ${v}`
```

(`serverDefault` is the effective free-tier number — either expose it from
the backend or, simpler, drop the parenthetical and just display
`'Default'`.)

**Help text (line 733-734)**: rewrite from `0 = unlimited` to something like
`-1 = unlimited, 0 = use server default` — or, if the picker UX is adopted,
the help text becomes redundant.

### 5. Quota / desktop enforcement path

Verify (do not change) that `StartDesktop` and any quota gate handle `-1`
correctly. `api/pkg/quota/quota.go:268` already does:

```go
if limit < 0 { return &QuotaLimitReachedResponse{LimitReached: false, …} }
```

So passing `-1` through is harmless. **Spot-check there isn't a separate
enforcement path that reads `ServerConfigForFrontend.MaxConcurrentDesktops`
and does its own `>= limit` check without the `< 0` guard.** If one exists,
add the guard.

### 6. helix-org client (`helix-org/helix/helixclient/client.go`)

Already correct — `HasDesktopRoom` (per earlier investigation) treats
`Max <= 0` as unlimited. **Re-verify during implementation** in case the
field semantics or name have drifted. Note: with our new model, `Max == 0`
should never appear on the wire (the resolver always converts `0` → the
free-tier default before returning), so the `<= 0` check is more permissive
than strictly needed. Leave it as-is — it's harmlessly tolerant.

## Compatibility & Migration

- **DB**: no migration. Column stays `int NOT NULL DEFAULT 0`.
- **Existing rows with `0`**: continue to mean "fall back to free-tier
  default". Behaviour preserved across upgrade.
- **API JSON shape**: unchanged. `int` fields. The only new wire value is
  `-1`, which is just a number.
- **helix-org**: unchanged. `HasDesktopRoom` already tolerates `<= 0`.

## Testing Plan

Targeted Go table-test on `EffectiveMaxConcurrentDesktops`:

| Input | Free-tier default | Expected output |
|-------|-------------------|-----------------|
| `-1`  | `2`               | `-1`            |
| `-1`  | `10`              | `-1`            |
| `0`   | `2`               | `2`             |
| `0`   | `10`              | `10`            |
| `5`   | `2`               | `5`             |
| `5`   | `10`              | `5`             |

Handler-level test confirming `GET /api/v1/config` returns the resolved value
for each of the three states.

End-to-end in the inner Helix at `http://localhost:8080`
(per `helix/CLAUDE.md` testing rules):

1. Log in, save `Unlimited` in the admin UI; confirm DB stores `-1`.
2. `curl /api/v1/config | jq '.max_concurrent_desktops'` returns `-1`.
3. Open more than 2 desktop sessions; confirm none are rejected for quota.
4. Save a positive number (say `1`); confirm 2nd session is rejected.
5. Save `0` (or "Use server default"); confirm cap reverts to the free-tier
   default.

## Notes for the Implementing Agent

- **Do not migrate existing `0` rows** to `-1`. That would silently flip
  capped instances to unlimited.
- The `quota.go` code already uses `-1` for unlimited — reuse the convention,
  don't invent a new sentinel.
- `SystemSettingsRequest.MaxConcurrentDesktops` is already `*int`, so it
  correctly distinguishes "patch with -1" (admin chose unlimited) from
  "field absent from request" (don't change). No change needed there.
- Frontend changes are the largest surface area in this fix — don't ship the
  backend change without the matching UI update, or admins will be stuck
  again (this time unable to express the `-1` state through the UI at all).
- Test end-to-end in the inner Helix per `helix/CLAUDE.md` — do not rely on
  unit tests alone.
- Use full PR URL (`https://github.com/helixml/helix/pull/<n>`) per
  `helix/CLAUDE.md` communication rules.
