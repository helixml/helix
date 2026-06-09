# Implementation Tasks: Cron Stream Transport for Scheduled Worker Triggering

## Backend: transport kind

- [x] Create `api/pkg/org/domain/transport/cron.go` with `KindCron`, `CronConfig`, and a `Strategy` implementation modelled on `local.go`
- [x] Implement `CronConfig.Validate()` — expand aliases (`@hourly`, `@daily`, `@weekly`, `@weekdays`, `@weekends`), parse the resulting cron string, and reject intervals < 90s
- [x] Register `KindCron` in `transport.go`: add to `kindOrder` and to the `strategies` map
- [x] Add a unit test in `transport_test.go` asserting every `Kind` in `kindOrder` has an entry in `strategies` (prevents drift)
- [x] Unit-test `CronConfig.Validate()` for: each alias, valid 5-field cron, `CRON_TZ=…` prefix, sub-90s rejection, malformed input
- [x] Explicit DoS-prevention tests: `* * * * *` (60s — rejected as < 90s minimum), per-second formats rejected as unparseable, aliases that would resolve to sub-90s are rejected, error message clearly names the 90s limit

## Backend: store

- [x] Add `ListByTransportKind(ctx, kind) ([]Stream, error)` to the `Streams` store interface — needed by the scheduler to enumerate cron streams across all orgs
- [x] Implement `ListByTransportKind` in `gorm/stream.go` (single cross-org `WHERE transport_kind = ?` query)
- [x] Implement `ListByTransportKind` in `memory/memorystore.go`

## Backend: scheduler

- [x] Create `api/pkg/org/infrastructure/streamcron/scheduler.go` modelled on `api/pkg/trigger/cron/trigger_cron.go`
- [x] Implement `Scheduler.reconcile()` — list `KindCron` streams, diff against current `gocron.Job`s, add/update/remove
- [x] Implement `Scheduler.fire(streamID, orgID)` — build event via `streaming.NewMessageEvent` with `kind:"scheduled"` body, call `Store.Events.Append`, `Hub.Notify`, `Dispatcher.Dispatch`
- [x] Wrap `fire()` in panic recovery so a single bad tick can't poison the schedule
- [x] Start the scheduler in the helix-org bootstrap (`api/pkg/server/helix_org.go`), driven by the server's `ctx` so shutdown is clean
- [~] Integration test: create a cron stream, subscribe a fake Worker, advance time, assert the Worker's activation queue received a `TriggerEvent`

## Backend: audit

- [ ] **Defer to follow-up task.** v1 logs every fire at info level. A dedicated `org_stream_cron_executions` table can be added later when the UI surfaces "last fired at"; for now, log + Drone metrics are enough and we avoid coupling v1 to a UI we haven't shipped.

## Frontend: stream creation

- [ ] Add `{ value: 'cron', label: 'cron', help: … }` to `TRANSPORT_KINDS` in `frontend/src/pages/HelixOrgStreams.tsx` (line 53)
- [ ] In the create-stream dialog, render a **Schedule** input when `cron` is selected
- [ ] Add quick-pick preset buttons: Hourly, Daily, Weekly, Weekdays, Weekends, Mon 09:00, Fri 18:00
- [ ] Show inline "next fire: …" preview (server endpoint OR shared client-side parser — pick one during implementation)
- [ ] Surface server-side validation errors clearly on the schedule field

## Frontend: stream list

- [ ] **Defer.** v1 doesn't surface schedule/last-fired in the list — the existing kind column shows "cron" which is enough; per-cron-stream UI lives on the detail page in a follow-up. Existing edit/delete flows work unchanged because the transport_config update path is generic.

## Docs & cleanup

- [ ] Update any developer-facing doc that lists transport kinds (e.g. `api/pkg/org/domain/transport/transport.go` doc comment, README sections under `docs/`)
- [ ] Note the single-leader limitation in operational docs (same as existing app cron)
- [ ] Manual verification checklist: create cron stream → subscribe a Worker → confirm activation on tick → edit schedule → confirm new cadence within 10s → delete stream → confirm no further fires
