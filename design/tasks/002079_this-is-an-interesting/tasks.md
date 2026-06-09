# Implementation Tasks: Cron Stream Transport for Scheduled Worker Triggering

## Backend: transport kind

- [ ] Create `api/pkg/org/domain/transport/cron.go` with `KindCron`, `CronConfig`, and a `Strategy` implementation modelled on `local.go`
- [ ] Implement `CronConfig.Validate()` — expand aliases (`@hourly`, `@daily`, `@weekly`, `@weekdays`, `@weekends`), parse the resulting cron string, and reject intervals < 90s
- [ ] Register `KindCron` in `transport.go`: add to `kindOrder` (line 65) and to the `strategies` map (line 72)
- [ ] Add a unit test in `transport_test.go` asserting every `Kind` in `kindOrder` has an entry in `strategies` (prevents drift)
- [ ] Unit-test `CronConfig.Validate()` for: each alias, valid 5-field cron, `CRON_TZ=…` prefix, sub-90s rejection, malformed input
- [ ] Explicit DoS-prevention tests: `* * * * *` (60s — rejected as < 90s minimum), per-second formats rejected as unparseable, aliases that would resolve to sub-90s are rejected, error message clearly names the 90s limit

## Backend: scheduler

- [ ] Create `api/pkg/org/infrastructure/streamcron/scheduler.go` modelled on `api/pkg/trigger/cron/trigger_cron.go`
- [ ] Implement `Scheduler.reconcile()` — list `KindCron` streams, diff against current `gocron.Job`s, add/update/remove
- [ ] Implement `Scheduler.fire(streamID, orgID)` — build event via `streaming.NewMessageEvent` with `kind:"scheduled"` body, call `Store.Events.Append`, `Hub.Notify`, `Dispatcher.Dispatch`
- [ ] Wrap `fire()` in panic recovery so a single bad tick can't poison the schedule
- [ ] Start the scheduler in the API bootstrap alongside the existing app-cron (`api/cmd/serve.go` or equivalent)
- [ ] Integration test: create a cron stream, subscribe a fake Worker, advance time, assert the Worker's activation queue received a `TriggerEvent`

## Backend: audit

- [ ] Decide: new `org_stream_cron_executions` table vs reuse `trigger_executions` (inspect existing schema; prefer reuse if shape fits)
- [ ] Implement the audit write inside `fire()` — record `fired_at`, `event_id`, `status`, `error`
- [ ] Add store method to fetch "last N executions" for a given stream (used by the UI)

## Frontend: stream creation

- [ ] Add `{ value: 'cron', label: 'cron', help: … }` to `TRANSPORT_KINDS` in `frontend/src/pages/HelixOrgStreams.tsx` (line 53)
- [ ] In the create-stream dialog, render a **Schedule** input when `cron` is selected
- [ ] Add quick-pick preset buttons: Hourly, Daily, Weekly, Weekdays, Weekends, Mon 09:00, Fri 18:00
- [ ] Show inline "next fire: …" preview (server endpoint OR shared client-side parser — pick one during implementation)
- [ ] Surface server-side validation errors clearly on the schedule field

## Frontend: stream list

- [ ] For cron streams, show **schedule**, **last fired at**, and **next fire** columns/badges
- [ ] Wire up the existing edit/delete flows for cron-kind streams (verify TransportConfig update is supported end-to-end)

## Docs & cleanup

- [ ] Update any developer-facing doc that lists transport kinds (e.g. `api/pkg/org/domain/transport/transport.go` doc comment, README sections under `docs/`)
- [ ] Note the single-leader limitation in operational docs (same as existing app cron)
- [ ] Manual verification checklist: create cron stream → subscribe a Worker → confirm activation on tick → edit schedule → confirm new cadence within 10s → delete stream → confirm no further fires
