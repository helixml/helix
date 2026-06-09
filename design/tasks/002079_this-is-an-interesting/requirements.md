# Requirements: Cron Stream Transport for Scheduled Worker Triggering

## Background

Helix already has the concept of **Streams** (`api/pkg/org/domain/streaming/`)
to which Workers `subscribe` and from which they are activated when a
`streaming.Event` is published. Today, events arrive from external transports:
`local` (in-process publish from a tool), `webhook` (HTTP), `email`, and
`github`.

Separately, Helix has a **cron trigger system** for *apps/agents*
(`api/pkg/trigger/cron/trigger_cron.go`, using `go-co-op/gocron/v2`), but that
runs an app session — it does **not** publish to a stream and therefore can't
fan out to every Worker subscribed to the stream.

The user wants the periodic-trigger and the stream/subscription model to be the
same primitive: **a Stream whose "transport" is a schedule.** When the schedule
fires, an event is published to the stream and the existing dispatcher fans it
out to all subscribed Workers.

## User Stories

### US-1: Create a stream that fires on a schedule
**As a** Helix user
**I want to** create a stream whose transport kind is `cron`
**So that** every Worker subscribed to the stream is activated at the
configured cadence — no external producer required.

**Acceptance criteria**
- The Streams UI (`HelixOrgStreams.tsx`) lists `cron` alongside
  `local / webhook / email / github` in the transport-kind selector.
- Selecting `cron` exposes a **Schedule** input plus quick-pick presets.
- Creating the stream succeeds via the existing `create_stream` MCP tool with
  `transport: { kind: "cron", config: { schedule: "<spec>" } }`.
- The schedule is validated server-side; invalid input returns a clear error
  before the stream is persisted.

### US-2: Express the schedule in familiar ways
**As a** user picking a cadence
**I want to** specify the schedule using either a cron string or a short
human alias
**So that** I don't need to memorise cron syntax for common cases.

**Acceptance criteria** — the following all parse and validate:
- Aliases: `@hourly`, `@daily`, `@weekly`, `@weekdays`, `@weekends`.
- 5-field cron: `0 9 * * 1` (9:00 Monday), `0 18 * * 5` (18:00 Friday),
  `0 0 * * 0,6` (weekends), `0 0 * * 1-5` (weekdays).
- Optional timezone prefix: `CRON_TZ=Europe/London 0 9 * * 1` (matches the
  existing app-cron behaviour at `trigger_cron.go:311`).
- Minimum interval is enforced at **90 seconds** to match the existing
  app-cron limit; shorter intervals are rejected.

### US-3: Subscribed Workers are activated on each tick
**As a** Worker subscribed to a cron stream
**I want to** receive a normal activation each time the schedule fires
**So that** my periodic work runs without me needing to implement my own
scheduler.

**Acceptance criteria**
- On each fire, the system appends a `streaming.Event` to the stream with
  `Source = ""` (system-emitted, mirroring `event.go:58-63`).
- The existing dispatcher (`dispatch/dispatcher.go`) fans the event out via
  the per-Worker activation queue exactly as for any other published event.
- If no Workers are subscribed, the tick is a no-op (event is still appended
  for audit purposes, no error).
- The event `Body` contains a small canonical message indicating the source
  was a scheduled tick (e.g. `"Scheduled trigger fired at <RFC3339>"`).

### US-4: Edit and delete a cron stream
**As a** user
**I want to** change the schedule or delete the stream
**So that** I can adjust the cadence without recreating subscriptions.

**Acceptance criteria**
- Updating `TransportConfig` for a cron stream reschedules the underlying job
  within one reconcile cycle (≤ 10 s, matching `reconcileCronApps` cadence).
- Deleting the stream removes the underlying `gocron.Job` on the next
  reconcile; no further events are published.
- Firing a Worker still drops its subscription as today (see
  `subscription.go`) — cron stream is unaffected; remaining Workers continue
  to receive ticks.

### US-5: Operational behaviour matches existing crons
**As an** operator
**I want** cron streams to follow the same operational pattern as the
existing app cron
**So that** observability, restart behaviour, and missed-fire policy don't
diverge.

**Acceptance criteria**
- Scheduler is in-process in the API (single-leader semantics deferred —
  same as today's app cron).
- On API restart, the next reconcile cycle (≤ 10 s) re-registers all jobs
  from DB — no in-memory state required.
- Missed fires while the API was down are **skipped**, not back-filled.
- Each fire writes a `TriggerExecution`-style audit row (or the simplest
  equivalent that fits the streams model) so the UI can show "last fired at".

## Out of Scope

- Multi-leader / distributed scheduling (today's app cron is also
  single-process; deferring to a separate task).
- Catch-up / back-fill of missed ticks.
- Per-Worker schedules (the schedule is a property of the *stream*; if a
  Worker wants a different cadence, it subscribes to a different stream).
- Replacing the existing app-cron trigger system; the two will coexist. App
  crons run an app/agent end-to-end; cron streams activate Workers via the
  stream/subscription model. Migration is a separate decision.
