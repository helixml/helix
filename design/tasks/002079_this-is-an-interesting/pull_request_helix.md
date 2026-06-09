# Add cron stream transport for scheduled worker triggering

## Summary

Adds `KindCron` as a fifth stream transport. A cron stream has no
external producer; a new in-process scheduler publishes a system-emitted
event on the configured cadence and the existing dispatcher fans it out
to every subscribed Worker via the normal activation queue. From the
dispatcher down, a cron tick is indistinguishable from a `publish` MCP
call — same code path, same fan-out, same Worker behaviour.

Why this shape: the user already has the Stream/Subscription primitive
and the dispatcher fan-out. Making the schedule a *transport* on a
Stream means no new subscription model, no new dispatcher path, no new
worker-side wiring — just a fifth file in `transport/` and one new
scheduler.

## Changes

### Backend
- `api/pkg/org/domain/transport/cron.go` — new `KindCron` transport with
  `CronConfig.Validate` that expands `@daily`-style aliases, parses
  with `robfig/cron/v3`'s standard 5-field parser, and rejects any
  schedule whose next-fire gap is `< 90s` (matches the existing app-cron
  floor at `api/pkg/trigger/cron/trigger_cron.go:311`).
- `api/pkg/org/infrastructure/streamcron/scheduler.go` — `gocron/v2`
  scheduler reconciled every 10s against `Streams.ListByTransportKind`.
  Each tick builds a canonical `{"kind":"scheduled",…}` message,
  appends the event, notifies the hub, and calls
  `Dispatcher.Dispatch` — same sequence as `tools/publish.go`. Wrapped
  in panic recovery so one bad tick can't take down the goroutine.
- `Streams.ListByTransportKind` added to the org store interface plus
  both gorm and memory implementations — used only by the scheduler
  for its cross-org enumeration; per-tenant request paths stay on
  `Get` / `List`.
- Wired into `initHelixOrgHandler`; `Start(ctx)` is launched from
  `registerRoutes` so the scheduler runs for the lifetime of the API
  process.

### Frontend
- `frontend/src/pages/HelixOrgStreams.tsx` — `cron` joins the
  transport dropdown. Selecting it renders a Schedule field with the
  syntax helper text and quick-pick chips (Hourly, Daily, Weekly,
  Weekdays, Weekends, Mon 09:00, Fri 18:00). Server-side validation
  errors are surfaced verbatim by the existing snackbar path.

### Tests
- `api/pkg/org/domain/transport/cron_test.go` — alias expansion,
  standard cron, `CRON_TZ=…` prefix, sub-90s rejection, per-second
  rejection, malformed input.
- `api/pkg/org/domain/transport/transport_test.go` — drift-prevention
  test asserting every Kind in `kindOrder` has a `strategies` entry;
  end-to-end Transport.Validate cases for cron.
- `api/pkg/org/infrastructure/streamcron/scheduler_test.go` — integration
  coverage against memorystore + recording dispatcher: fire path,
  reconcile add/update/remove, panic recovery, defensive
  invalid-schedule skip.

## Test plan
- [x] `go test ./api/pkg/org/domain/transport/` — green
- [x] `go test ./api/pkg/org/infrastructure/streamcron/` — green
- [x] `go test ./api/pkg/org/infrastructure/persistence/memory/` — green
- [x] `go build ./api/...` — clean
- [x] `tsc --noEmit` on the frontend — clean
- [x] End-to-end in inner Helix: create cron stream via UI → scheduler picked it up within 10s (see api logs) → try `* * * * *` → server rejected with `1m30s minimum` error, surfaced in snackbar

## Screenshots
![cron transport selected in New Stream dialog](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002079_this-is-an-interesting/screenshots/01-new-stream-cron-selected.png)
![cron stream appears in the streams list](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002079_this-is-an-interesting/screenshots/02-cron-stream-in-list.png)
![server-side 90s minimum surfaces in the UI](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002079_this-is-an-interesting/screenshots/03-validation-rejects-every-minute.png)

## Out of scope / follow-ups
- Per-stream "last fired at" / "next fire" badges in the streams list —
  the kind column already says `cron`; richer surfacing belongs on the
  detail page.
- Dedicated `org_stream_cron_executions` audit table — v1 logs every
  fire at info level. Reintroduce when the UI wants to render history.
- Multi-leader scheduling — same single-process caveat as the existing
  app-cron at `api/pkg/trigger/cron`; both schedulers want the same
  leader-election story when N>1 replicas land.
