# Design: Cron Stream Transport for Scheduled Worker Triggering

## Summary

Add `KindCron` as a fifth transport kind for Streams. A new in-process
**stream cron scheduler** watches `org_streams` rows of this kind and, using
`go-co-op/gocron/v2`, publishes a `streaming.Event` to each stream on the
configured schedule. The existing dispatcher then fans the event out to all
subscribed Workers via the existing per-Worker activation queue.

**Who fires the cron?** A new singleton goroutine in the API process —
`api/pkg/org/infrastructure/streamcron/scheduler.go` — modelled directly on
the existing `api/pkg/trigger/cron/trigger_cron.go`. Same library, same
reconcile-every-10s pattern, same single-leader caveat.

## Where this slots into the existing model

```
                  ┌──────────────────────────────────┐
                  │  streamcron.Scheduler  (NEW)     │
                  │  reconciles every 10s            │
                  │  gocron.Job per KindCron stream  │
                  └─────────────────┬────────────────┘
                                    │  on tick
                                    ▼
        Store.Events.Append() ──► Hub.Notify() ──► Dispatcher.Dispatch()
                                                          │
                                                          ▼
                                    per-Worker activation queues
                                    (existing behaviour, unchanged)
```

This is the same call sequence already used by
`api/pkg/org/application/tools/publish.go:102-111` for the `publish` MCP
tool, so cron-driven events look identical to tool-driven events downstream.

## Key Decisions

### D1: Cron is a Stream transport, not a new trigger type
The user's framing — *"a new stream which is basically a cron trigger"* — is
the right shape. Reusing the Stream/Subscription model means: no new
endpoints for subscribing, no new UI for managing Workers, no new code path
in the dispatcher. The schedule becomes a property of the stream's transport
config, and everything else is free.

Rejected alternative: extend the existing `trigger.cron` app-trigger to publish
to a stream. That trigger is wired to *app sessions* and carries app/agent
identity through its execution path; bending it to also publish events would
double the surface area. A separate scheduler is cleaner.

### D2: Use the same library and pattern as the existing app cron
`go-co-op/gocron/v2` is already a dependency and already used by
`api/pkg/trigger/cron/trigger_cron.go`. Reusing it gives us:
- The same supported syntax (5-field cron + `CRON_TZ=` prefix).
- The same 10-second reconcile loop pattern, which handles add/update/delete
  uniformly without per-operation hooks.
- The same single-process limitation (acceptable: today's app cron has the
  same constraint; deferring distributed scheduling to a separate task).

### D3: Aliases expand to standard cron strings at parse time
The user listed cases like "daily", "weekends only", "9am Monday". Rather
than carry alias strings through to gocron, expand them at `Validate()` time
into standard cron expressions. This keeps the runtime path simple and
unifies all scheduling on one syntax.

| Alias        | Expands to     |
| ------------ | -------------- |
| `@hourly`    | `0 * * * *`    |
| `@daily`     | `0 0 * * *`    |
| `@weekly`    | `0 0 * * 0`    |
| `@weekdays`  | `0 0 * * 1-5`  |
| `@weekends`  | `0 0 * * 0,6`  |

The stored `TransportConfig` keeps the original user-entered string so the UI
can round-trip what the user typed; only the runtime evaluation uses the
expansion.

### D4: 90-second minimum interval, same as app cron
`trigger_cron.go:311` enforces a 90s minimum. Match it exactly — both for
consistency and to avoid stream-cron becoming a DoS vector against the
dispatcher.

### D5: Missed ticks during downtime are skipped
gocron's default behaviour. Avoiding back-fill keeps the design simple and
prevents a thundering herd of activations after a restart. Matches today's
app cron. Documented in requirements.md US-5.

### D6: System-emitted events use empty `Source`
The streaming event model already supports system-emitted events with empty
`Source` (`event.go:58-63`). Reuse this. The dispatcher's existing logic —
that AI Workers may deprioritise events from other AI Workers, etc. — needs
no change; an empty Source is treated as system, not AI.

### D7: Event body is a small canonical message
On each tick, append an event with `Body` set to a short JSON message such as
`{"kind":"scheduled","firedAt":"<RFC3339>","streamId":"<id>"}`. Workers that
care about timing can read this; Workers that don't can ignore it. Keep the
shape stable so downstream tooling can match on `kind:"scheduled"`.

## Component Changes

### Backend

**New file: `api/pkg/org/domain/transport/cron.go`**
- `const KindCron Kind = "cron"`
- `type CronConfig struct { Schedule string }`
- `CronConfig.Validate()` — expand aliases, then parse with
  `cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour |
  cron.Dom | cron.Month | cron.Dow | cron.Descriptor)` to validate. Reject
  if next-fire-interval < 90s.
- `type cron struct{}` implementing `transport.Strategy`.
- Add `KindCron` to `kindOrder` in `transport.go:65`.
- Add the strategy to the `strategies` map in `transport.go:72`.

**New file: `api/pkg/org/infrastructure/streamcron/scheduler.go`**
- Modelled on `api/pkg/trigger/cron/trigger_cron.go`.
- `Scheduler` struct holding the `gocron.Scheduler` plus a `map[StreamID]gocron.Job`.
- `Start(ctx)` — launches a 10s ticker; each tick calls `reconcile()`.
- `reconcile()` — lists all streams with `TransportKind == "cron"`, diffs
  against current jobs, adds/updates/removes to match.
- `fire(streamID, orgID)` — the gocron task callback:
  1. Build the canonical event (`streaming.NewMessageEvent` with a
     `kind:"scheduled"` body).
  2. `Store.Events.Append(ctx, evt)`.
  3. `Hub.Notify(streamID)`.
  4. `Dispatcher.Dispatch(ctx, evt)`.
  Wrap in panic recovery; on error, log and continue (do not re-throw to
  gocron — a single bad tick must not poison the schedule).

**Wire-up: `api/cmd/serve.go` (or wherever the existing trigger.cron scheduler
is started)**
- Construct the `streamcron.Scheduler` with deps `(Store, Dispatcher, Hub)`
  and call `Start(ctx)` alongside the existing trigger cron.

**Audit: `org_stream_cron_executions` table (or reuse existing trigger
execution table if shape fits)**
- Columns: `id`, `org_id`, `stream_id`, `fired_at`, `event_id`, `status`,
  `error`.
- Used by the UI to show "last fired at" and recent errors.

### Frontend

**`frontend/src/pages/HelixOrgStreams.tsx`**
- Add `{ value: 'cron', label: 'cron', help: 'Periodic schedule fires events on this stream.' }` to `TRANSPORT_KINDS` (line 53).
- When `cron` is selected in the create dialog, show a **Schedule** input
  with:
  - A free-text field accepting cron strings or aliases.
  - Quick-pick buttons: Hourly · Daily · Weekly · Weekdays · Weekends ·
    Mon 09:00 · Fri 18:00.
  - Inline validation (call a small `/api/v1/streams/validate-schedule`
    endpoint — or do client-side parse with a shared lib — to show
    "next fire: <human-readable>" feedback).
- For an existing cron stream, show **last fired at** and **next fire**
  in the stream list.

## Data Model

`org_streams.transport_config` already stores JSON. For cron streams:

```json
{ "schedule": "0 9 * * 1" }
```

or with timezone:

```json
{ "schedule": "CRON_TZ=Europe/London 0 9 * * 1" }
```

No new column on `org_streams`. One new table `org_stream_cron_executions`
for audit, or reuse the existing `trigger_executions` table if its shape is
compatible (decide during implementation).

## Operational Notes

- **Single-leader**: same as existing app cron. If/when Helix runs the API
  with N>1 replicas, the *same* leader-election story applies to both. Out
  of scope for this task; documented as a known limitation.
- **Restart**: the 10s reconcile loop re-registers all jobs from DB on
  startup. No in-memory state to persist.
- **Observability**: log every fire at info level with stream ID, org ID,
  and subscriber count. Failures at error level.

## Implementation Notes

Captured during the build for future agents who'll work on similar transports.

### Final shape of the changes

**Files added:**
- `api/pkg/org/domain/transport/cron.go` — `KindCron`, `CronConfig`, `CronConfig.Validate`, `ExpandCronSchedule` helper, `Transport.CronConfig()` typed accessor.
- `api/pkg/org/domain/transport/cron_test.go` — alias/parse/DoS-floor unit tests.
- `api/pkg/org/infrastructure/streamcron/scheduler.go` — gocron-driven Scheduler with `Start` (blocking on ctx), `reconcile`, `fire`, and the panic-protected `fireFn`.
- `api/pkg/org/infrastructure/streamcron/scheduler_test.go` — integration tests against memorystore + recording dispatcher (fire, reconcile add/update/remove, panic recovery, invalid-schedule defensive skip).

**Files modified:**
- `api/pkg/org/domain/transport/transport.go` — added `KindCron` to `kindOrder` and `strategies`.
- `api/pkg/org/domain/transport/transport_test.go` — extended `KindValues` test, added drift-prevention test, added end-to-end Transport.Validate cases for cron.
- `api/pkg/org/domain/store/store.go` — added `Streams.ListByTransportKind(ctx, kind)` to the interface.
- `api/pkg/org/infrastructure/persistence/gorm/stream.go` — implemented `ListByTransportKind` as a single cross-org `WHERE transport_kind = ?` query.
- `api/pkg/org/infrastructure/persistence/memory/memorystore.go` — implemented the same in the memory store.
- `api/pkg/server/helix_org.go` — instantiated `streamcron.Scheduler` inside `initHelixOrgHandler` and added it to `helixOrgHandlers`.
- `api/pkg/server/server.go` — renamed `_ context.Context` → `ctx` in `registerRoutes`; launched `orgHandlers.streamCron.Start(ctx)` in a goroutine right after init.
- `frontend/src/pages/HelixOrgStreams.tsx` — added `cron` to `TRANSPORT_KINDS`, added `CRON_PRESETS`, added the conditional schedule field + preset chips, added cron-handling in `submit()`, gated the JSON config box.

### Things I discovered during build

- The frontend's `TRANSPORT_KINDS` array already lists `postmark` rather than `email` (the backend constant is `KindEmail = "email"`). Pre-existing inconsistency; left alone.
- `streaming.NewStream` calls `Transport.Validate` — there's no path to construct a Stream domain value with an invalid transport via the constructor. To test the scheduler's defensive `Validate` inside `reconcile`, I mutate the row through `Streams.Update` (which doesn't re-validate the transport) after creating with a valid schedule.
- `gocron/v2`'s `CronJob` constructor with `withSeconds=false` insists on the 5-field syntax — and our `cronv3.ParseStandard` also rejects 6-field. That's the design choice from D1/R1: closing the syntactic door is stronger than checking gap intervals.
- The `Repository` generic in `gorm/repository.go` exposes `db` as an unexported field. Since `streamsRepo` embeds `*Repository`, the existing code in `stream.go` already reaches the embedded `r.db` directly through Go's field promotion. `ListByTransportKind` follows the same pattern.
- `HELIX_ORG_ENABLED` defaults to `false` in `docker-compose.dev.yaml` (envconfig fallback). For end-to-end UI testing I had to `down api` + `HELIX_ORG_ENABLED=true docker compose -f docker-compose.dev.yaml up -d api` (a plain `restart` won't pick up new env). Helix-org also requires an admin user to exist before initHelixOrgHandler will wire itself up.
- The `chrome-devtools__fill` MCP tool concatenates onto existing values rather than replacing — used a small `evaluate_script` to set the schedule field cleanly via the native input value setter.

### Manual verification

End-to-end exercised in the inner Helix at http://localhost:8080:
- Registered `test@helix.ml` (auto-admin in dev), created `testorg`, granted the `helix-org` alpha feature, opened `/orgs/testorg/helix-org/streams`.
- Clicked "New Stream" → "Transport" dropdown: `cron` appears as the 5th option (alongside local, webhook, github, postmark).
- Selecting cron renders the Schedule field with the helper text and the seven preset chips.
- Clicked "Mon 09:00" → schedule auto-fills `0 9 * * 1` → submit → success snackbar.
- ~10 seconds later, api logs show `streamcron: scheduled stream … schedule="0 9 * * 1"` — the scheduler picked up the new row on the next reconcile.
- Tried submitting `* * * * *` → server rejected with `schedule "* * * * *" fires more often than the 1m30s minimum` → snackbar surfaced the verbatim error.

Screenshots in `screenshots/`:
- `01-new-stream-cron-selected.png` — cron transport selected, Schedule field + preset chips visible.
- `02-cron-stream-in-list.png` — created cron stream appears in the streams table.
- `03-validation-rejects-every-minute.png` — server-side 90s minimum enforcement surfaced in the UI.

## Risks

- **R1: Runaway sub-minute schedules.** Standard cron syntax allows up to
  `* * * * *` (every minute); gocron's parser with the `SecondOptional`
  descriptor permits per-second schedules; a determined user could even
  abuse aliases or `CRON_TZ` constructs. A 1-second tick on a stream with
  many subscribers would saturate the dispatcher, blow up activation costs,
  and effectively DoS the cluster.
  **Mitigation (mandatory, server-side):** `CronConfig.Validate()` rejects
  any schedule whose next-fire-interval is shorter than the configured
  minimum. Default minimum is **90 seconds**, matching the existing
  app-cron limit at `trigger_cron.go:311`. Concretely:
  1. Parse the schedule (do not enable `cron.SecondOptional` — only the
     standard 5-field parser, so per-second specs are unparseable).
  2. Compute the gap between the next two fire times via `Schedule.Next`.
  3. Reject if `gap < 90s` with a clear error message naming the limit.
  This validation runs on both create and update; the frontend mirrors it
  for fast feedback but the backend is authoritative. Unit-test specifically
  with `* * * * *` (60s — accepted), `*/30 * * * * *` (rejected as
  unparseable), and any alias that resolves to sub-90s.
- **R2: Thundering herd on common cadences.** If many streams use `@daily`
  (`0 0 * * *`), they all fire at the same instant. gocron handles this
  serially per-scheduler, and the dispatcher fan-out is per-Worker, so the
  blast radius is bounded — but it's worth noting. Mitigation: optionally
  add jitter at fire time (a few seconds). Defer unless it becomes an issue.
- **R3: Drift between transport.go `kindOrder` and `strategies` map.** Both
  need updating; add a unit test that asserts every Kind in `kindOrder` has
  an entry in `strategies` to catch this in CI.
- **R4: Mis-parsed schedule silently never fires.** Server-side validation
  on create/update is mandatory. The UI's "next fire: …" preview also gives
  the user a sanity check before save.
