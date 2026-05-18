# Design: Honour "0 = Unlimited" for Max Concurrent Desktops

## Problem Recap

`api/pkg/server/handlers.go:117-123` treats an explicit `0` in
`SystemSettings.MaxConcurrentDesktops` as "field unset, fall back to free
tier", but the System Settings UI label promises `0 = unlimited`. The two
meanings collide on the same value, so the user-visible behaviour silently
contradicts the help text.

This must be resolved so that the field's three meaningful states map to
distinct, predictable runtime behaviour.

## State Model

| UI / DB value           | Intended meaning                                | Effective `MaxConcurrentDesktops` returned by `/api/v1/config` |
|-------------------------|-------------------------------------------------|----------------------------------------------------------------|
| `0` (UI: `Unlimited`)   | Admin explicitly chose unlimited                | `-1` (the project-wide convention for unlimited; see `quota.go:268`) |
| `N > 0`                 | Admin chose a finite cap                        | `N`                                                            |
| Field never set / fresh install | Use the operator-configured Free-tier default | `Cfg.SubscriptionQuotas.Projects.Free.MaxConcurrentDesktops` (default `2`, env `PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS`) |

The challenge: `SystemSettings.MaxConcurrentDesktops` is a plain `int`, so
states #1 and #3 currently look identical to the server.

## Decision: Promote `MaxConcurrentDesktops` to `*int`

Choose **Option A** below: change the field to a nullable pointer so the three
states become distinguishable. Then update the handler to map them as in the
table above.

### Why this option, not the alternatives

**Option A — `*int` on `SystemSettings.MaxConcurrentDesktops`** ✅ *chosen*

- Pros: matches `SystemSettingsRequest.MaxConcurrentDesktops` which is already
  `*int` (`system_settings.go:79`); aligns with how the rest of the codebase
  (`quota.go`) already treats "unlimited" as a distinct concept; preserves
  upgrade behaviour for instances that never touched the setting.
- Cons: requires a DB column shape that allows NULL, a small migration, and a
  one-time touch of the persistence + response-mapping code. JSON shape of the
  response is unaffected (`int` field still serialises as a number).

**Option B — Use a sentinel like `-1` for unlimited**

- Pros: no schema change.
- Cons: the **UI already saves `0`** as the "unlimited" sentinel, so we'd be
  redefining the semantics of values currently in production DBs and changing
  the frontend storage model. Two sentinels (`0` and `-1`) competing for the
  same meaning is worse than the current bug.

**Option C — Drop the fallback entirely; treat any `0` as unlimited**

- Pros: one-line change in `handlers.go`.
- Cons: breaks instances that have never touched System Settings — they'd
  silently flip from "2 desktops" to "unlimited" on upgrade. This is a
  meaningful regression for shared deployments. Rejected on safety grounds.

**Option D — Change only the UI label to "0 = use server default"**

- Pros: zero server change.
- Cons: closes the bug by giving up on the feature. There is genuine demand
  for "unlimited" — that's why the help text was written that way in the first
  place. Rejected as user-hostile.

## Architecture Changes

### 1. Type change (`api/pkg/types/system_settings.go`)

```go
// before
MaxConcurrentDesktops int `json:"max_concurrent_desktops,omitempty"`

// after
MaxConcurrentDesktops *int `json:"max_concurrent_desktops,omitempty"`
```

Update:
- `SystemSettings.MaxConcurrentDesktops` → `*int`.
- `SystemSettingsResponse.MaxConcurrentDesktops` already serialises an `int`;
  derive it from a helper that returns the *resolved effective value* (one of
  the three rows above). This way the admin UI shows what the server actually
  enforces, not the raw DB value, which keeps the displayed `Unlimited` chip
  honest.

### 2. Resolution helper

Add a single method on `SystemSettings` (parallel to the existing
`EffectiveMaxConcurrentHeadlessSandboxes` / `…DesktopSandboxes` helpers at
`system_settings.go:219-231`):

```go
// EffectiveMaxConcurrentDesktops resolves the three states to the int the
// rest of the system should enforce. Returns -1 to mean unlimited.
func (s *SystemSettings) EffectiveMaxConcurrentDesktops(freeTierDefault int) int {
    if s.MaxConcurrentDesktops == nil {
        return freeTierDefault
    }
    if *s.MaxConcurrentDesktops == 0 {
        return -1 // unlimited
    }
    return *s.MaxConcurrentDesktops
}
```

Putting it on the type keeps the resolution logic next to the data and means
every caller (current handler, future API endpoints, future quota code paths)
gets the same answer.

### 3. Handler change (`api/pkg/server/handlers.go:117-123`)

```go
config.MaxConcurrentDesktops = systemSettings.EffectiveMaxConcurrentDesktops(
    apiServer.Cfg.SubscriptionQuotas.Projects.Free.MaxConcurrentDesktops,
)
```

Delete the old branch entirely. The helper does the right thing.

### 4. Client-side gate (`helix-org/helix/helixclient/client.go`)

Already correct — `HasDesktopRoom` (per earlier investigation) treats
`Max <= 0` as unlimited. No change required, but **must be verified during
implementation** in case the field name or semantics drift.

### 5. Persistence layer (`api/pkg/store/store_system_settings.go`)

Inspect the column definition and read/write paths:
- GORM AutoMigrate will need to accept NULLs for the column.
- `Update` / `Patch` paths must preserve "explicit 0" (saved value) versus
  "field absent in request" (no change). Since `SystemSettingsRequest`
  already uses `*int`, the patch path likely just needs the destination type
  to match.
- `Get` defaults: when reading an existing row with no value, the pointer is
  `nil` — which the helper interprets as "fall back to free tier". This is
  the desired upgrade behaviour.

### 6. Test additions

Targeted Go table-test on `EffectiveMaxConcurrentDesktops`:

| Input (pointer)   | Free-tier default | Expected output |
|-------------------|-------------------|-----------------|
| `nil`             | `2`               | `2`             |
| `nil`             | `10`              | `10`            |
| ptr(`0`)          | `2`               | `-1`            |
| ptr(`5`)          | `2`               | `5`             |
| ptr(`5`)          | `10`              | `5`             |

Plus a handler-level test confirming the JSON returned by `/api/v1/config`
respects each case.

## Compatibility & Migration

- **DB migration**: change the column to nullable. Existing rows that have
  `0` get migrated to `NULL` *only if* we want to preserve their current
  behaviour (cap of 2). This is a judgement call — see "Open question" below.
- **API JSON shape**: unchanged for consumers. `ServerConfigForFrontend.MaxConcurrentDesktops`
  remains `int`; only its resolution changes.
- **helix-org**: no change needed; existing `HasDesktopRoom` already treats
  `≤ 0` as unlimited (verify during implementation).
- **Existing deployments**: must keep current "do nothing → cap of 2"
  behaviour. The `nil` pointer case handles this.

## Open Question for Implementation

When migrating existing rows: should every existing `0` value become `NULL`
(preserves today's "fall back to 2" behaviour) **or** stay as `0` (immediately
flips those instances to unlimited, matching the UI label they've been
staring at)?

Recommendation: **migrate `0` → `NULL`**. Reasoning: any admin who saw the
UI saying `Unlimited` and got 2 anyway has been living with the bug; they
have a workaround (set a high number). Flipping them silently to unlimited
on upgrade could open up resource exposure on shared instances. Preserving
the current effective behaviour is the safer default. If they actually want
unlimited, the chip will still say `Unlimited` post-upgrade (the displayed
value comes from the resolver), and they can re-save `0` to make it real.

Confirm this with the user before running the migration.

## Notes for the Implementing Agent

- The `quota.go` code already uses `-1` for unlimited — do not invent a new
  sentinel.
- The `SystemSettingsRequest.MaxConcurrentDesktops` field is *already* `*int`,
  so the API request shape needs no change.
- The frontend `SystemSettingsTable.tsx` chip logic `settings?.max_concurrent_desktops ? 'Limit' : 'Unlimited'`
  works correctly if the response returns `0` when the resolved cap is
  unlimited — but since we're returning the *effective* value, we should
  instead return `-1` for unlimited and update the frontend to display
  `Unlimited` for `≤ 0`. Or, simpler: have the resolver return `0` for
  unlimited in the response payload so the existing frontend conditional keeps
  working. Pick **whichever is the smaller diff** and document the choice in
  the PR.
- Test end-to-end in the inner Helix at `http://localhost:8080` per
  `helix/CLAUDE.md` testing rules: set Max Concurrent Desktops to 0 in the
  admin UI, then check `curl http://localhost:8080/api/v1/config | jq
  '.max_concurrent_desktops, .active_concurrent_desktops'` returns a value
  that downstream consumers (and an actual `StartDesktop` call) accept as
  unlimited.
