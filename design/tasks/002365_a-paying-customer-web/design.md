# Design: Graceful Web-Service Shutdown and Stop-Loop Alerting for Hosted Deploys

Incident context and code evidence are in `requirements.md`. Prior write-up of the same gap:
`design/2026-07-08-we-find-ai-custom-domain-prod-cutover.md` (options 1 + 2, never shipped —
see Open Question 1).

## 1. Problem shape

Two independent defects combined to keep a paying customer's site down for five days:

```
A. teardown is a hard kill            B. failure handling is a silent loop
   sandbox stopped with timeout=2s       readiness fails
   → inner dockerd SIGKILLed             → rollback re-runs the SAME broken startup
   → nested Postgres killed mid-write    → health monitor retries every ~11 min, forever
   → corrupt checkpoint record           → recovFails counter is in-memory, reset every restart
   → db never starts again               → looping condition only writes a log line
                                         → nobody is told; site is 503 for days
```

Fix A stops us from *creating* the corruption. Fix B stops us from *hiding* it. Both are needed:
A alone still leaves any other persistent failure invisible; B alone still corrupts databases.

## 2. Architecture

```
 API process
 ├─ webservice.Controller
 │   runDeploy:  validateRevision ──fail──▶ mark failed, OLD STACK UNTOUCHED   ◀── new (US-2)
 │               gracefulStopApp  ─────────▶ docker stop --time 60 (inner)     ◀── new (US-1)
 │               deployInPlace → waitForReady
 │               on fail: classifyFailure (log-tail signatures)                ◀── new (US-4)
 │                        rollback → waitForReady → record real outcome        ◀── fixed (US-2)
 │   RecoverWebService: gracefulStopApp before sandbox recreate                ◀── new (US-1)
 │
 ├─ webservice.HealthMonitor
 │   failure counters persisted on ProjectWebServiceState                      ◀── new (US-3)
 │   ≥ N consecutive failures → pause auto-recovery + alert once               ◀── new (US-3)
 │   └─▶ notification.AdminAlerter (Slack via janitor + admin email)
 │
 └─ sandbox.Controller.Delete
     Purpose == web_service → drain nested containers, then hydra delete       ◀── new (US-1)

 Runner host
 └─ helix-sandbox-app
     SIGTERM handler drains inner containers before dockerd dies               ◀── new (US-1)
     compose stop_grace_period raised to cover the drain
```

## 3. Root cause A — graceful nested shutdown

### 3.1 The drain primitive

One helper, used by every path that ends a web-service app stack:

```go
// drainNestedContainers stops every container inside the sandbox's inner
// dockerd with a bounded grace period, honouring each image's STOPSIGNAL so
// stateful workloads (Postgres fast-shutdown on SIGINT) checkpoint cleanly.
// Best-effort and bounded: a wedged container must never block teardown.
func drainNestedContainers(ctx context.Context, hc hydra.Client, sandboxID string, grace time.Duration) error
```

Implementation: one exec via `hydra.RunSandboxCommand`,
`docker ps -q | xargs -r docker stop --time <grace>`, with the exec timeout set to
`grace + slack`. Returns the exec result so callers can log whether the drain completed or was
forced.

**Why `docker stop` rather than `docker compose down`:**
- Generic. The startup mechanism is deliberately not assumed to be docker-compose (see the
  `DeployLog` comment in `controller.go`); `docker ps` works regardless.
- `docker stop` sends the image's declared `STOPSIGNAL`. The official Postgres image sets
  `SIGINT`, which is Postgres *fast shutdown* — exactly what we need. `compose down` would also
  do this, but only for containers it owns, and only if we can locate the right compose file and
  project directory.
- No risk of removing volumes or networks the customer expects to persist.

### 3.2 Call sites

| Path | Change |
|---|---|
| `webservice.Controller.deployInPlace` | Drain before the existing process-group kill. The process-group kill stays (it stops the `startup.sh` supervisor); the drain is what actually stops the app containers. This is what makes the package doc's single-writer claim true. |
| `webservice.Controller.RecoverWebService` (recreate branch) | Drain before `c.sandboxes.Delete(...)`. Skipped when the reason is `sandbox dockerd unresponsive` — there is nothing to drain through a dead dockerd. |
| `sandbox.Controller.Delete` | Drain when `sandbox.Purpose == types.SandboxPurposeWebService`, before `hydraClient.DeleteDevContainer`. |
| `sandbox/` startup scripts + outer compose | SIGTERM handler drains the inner dockerd's containers; `stop_grace_period` raised so Docker actually waits. |

**Import-cycle note.** `webservice` imports `sandbox`, so the delete-path drain lives in the
`sandbox` package (it already holds the hydra client and the `Purpose` field). The helper is
therefore defined in `sandbox` and called by `webservice` through the existing
`sandboxes.HydraClient(sb)` accessor — one definition, no cycle, no duplication.

### 3.3 Why not just raise hydra's timeout from 2s

Considered and rejected as the *only* fix. `api/pkg/hydra/devcontainer.go:1790` uses `timeout: 2`
deliberately for ephemeral dev containers, and raising it globally would make every spec-task
sandbox teardown slow. Hydra also does not know a container's `Purpose` — plumbing that through
the hydra API to special-case a stop timeout is more surface area than draining from the side
that already knows. Draining *before* the hydra call means the 2s stop then applies to an
already-empty dockerd, which is fine.

Hydra's 2s stays as-is. If review disagrees, the alternative is adding `StopTimeoutSeconds` to
the hydra delete request — noted, not chosen.

## 4. Root cause B — validate first, then stop looping

### 4.1 Pre-teardown validation (option 1)

New `Controller.validateRevision(ctx, sb, repo, sha) error`, run in `runDeploy` **before**
`deployInPlace`. It execs a read-only script in the sandbox that:

1. fetches the target SHA and checks it out into a **scratch worktree** (never the live code dir),
2. asserts `.helix/startup.sh` exists on the project's `helix-specs` branch,
3. greps the startup script for a compose file and, if found, runs `docker compose -f <file> config -q`,
4. removes the scratch worktree.

On failure: `markFailed(deployID, …, "validation: <err>")` and **return without touching the
running stack**. Because nothing has been stopped, the live site keeps serving. This is the
honest form of "keep old until new is healthy" under a single-writer data model — see Non-Goals.

Validation is cheap (seconds) and adds no downtime, since it precedes the stop.

### 4.2 Rollback tells the truth

`rollback()` currently redeploys the previous SHA and returns, logging success. It never checks
readiness, and when the failure is environmental (corrupt DB) it fails identically. Change it to
return an error, wait for readiness after the redeploy, and have `runDeploy` record the combined
outcome — `"readiness: <err>; rollback also failed readiness"` — so the deploy error stored on
the row reflects reality and feeds the alert.

### 4.3 Failure classification (US-4)

New `verifier`-adjacent helper `classifyDeployFailure(logTail string) (string, bool)`: an ordered
table of signature → actionable message.

| Signature | Message |
|---|---|
| `could not locate a valid checkpoint record` / `invalid resource manager ID` | nested Postgres will not start: corrupt checkpoint record — the data volume needs recovery (backup then `pg_resetwal`), the platform will not do this automatically |
| `dependency failed to start` / `is unhealthy` | a compose dependency never became healthy — check the failing service's healthcheck |
| `PANIC:` / `FATAL:` (generic DB) | database refused to start — see deploy log |
| `no space left on device` | the sandbox disk is full |

Unmatched → keep the generic readiness error (AC-4.4). The log tail comes from the existing
`DeployLog` reader, so no new plumbing.

### 4.4 Persisted stop-loop state (option 2)

Add to `types.ProjectWebServiceState` (GORM AutoMigrate, nullable — no backfill needed):

```go
ConsecutiveRecoveryFailures int        `json:"consecutive_recovery_failures"`
RecoveryPausedAt            *time.Time `json:"recovery_paused_at,omitempty"`
RecoveryPausedReason        string     `json:"recovery_paused_reason,omitempty"`
LastRecoveryError           string     `json:"last_recovery_error,omitempty"`
```

**Why persisted rather than the existing in-memory map.** `HealthMonitor.recovFails` is reset on
every API restart. In prod, CD upgrades and restarts happen far more often than the 30-minute
max backoff, so the counter effectively never reached `loopingAlertThreshold` — which is a
concrete reason the existing looping alert never fired during a five-day outage. The in-memory
map is replaced by these columns, not shadowed by them (no duplicate state, no dead code).

Health-monitor logic:

- `onSuccess` → zero the counter, clear paused fields, log recovery.
- Failed recovery → increment the persisted counter; keep the existing exponential backoff.
- Counter ≥ `recoveryPauseThreshold` (default 5, configurable) → set `RecoveryPausedAt` /
  `RecoveryPausedReason`, fire the alert **once** (guarded by `RecoveryPausedAt == nil`), and
  stop triggering recovery for that project.
- `runOnce` skips paused projects for recovery but keeps probing them, so `helix_webservice_up`
  stays accurate and a service that heals externally auto-clears.
- `Controller.Redeploy` (manual) clears all four fields at the start — a human redeploy is the
  resume action.

### 4.5 Alerting

Add `AdminAlerter.SendWebServiceDownAlert(ctx, data)` alongside the existing disk-space and
waitlist alerts, reusing the same Slack-via-janitor + admin-email delivery. Payload: project id
and name, hosted domain(s), consecutive-failure count, classified error (§4.3), and the deploy
log tail. The `HealthMonitor` gets an optional alerter dependency (nil = log only, so tests and
minimal deployments are unaffected), wired at `api/pkg/server/server.go:752`.

### 4.6 Health and API surface

`Controller.Health` gains `"errored"`, returned when `RecoveryPausedAt != nil` — distinct from
`"unhealthy"` ("down, still being auto-recovered"). The web-service status API returns
`LastRecoveryError` and `RecoveryPausedReason` as typed struct fields (not a map), and the
project UI shows the paused banner plus a redeploy button that doubles as resume.

New Prometheus gauge `helix_webservice_recovery_paused{project_id}` (1 = paused), registered in
`metrics.go` and cleared in `forgetProjectMetrics`.

## 5. Key decisions

| Decision | Rationale | Rejected alternative |
|---|---|---|
| Drain via `docker stop` on all inner containers | Generic (compose is not assumed), honours per-image `STOPSIGNAL`, touches no volumes | `docker compose down` — needs to locate the compose project, and removes networks |
| Drain from the control plane / sandbox package, not hydra | Hydra doesn't know sandbox `Purpose`; dev sandboxes must stay fast to tear down | Raise hydra's 2s timeout globally — slows every spec-task teardown |
| Validate before teardown, not blue/green | Single-writer `/data` is a deliberate design constraint (package doc, design 002107) | Run new stack alongside old — corrupts the shared data dir, the exact failure we're fixing |
| Persist failure counters in `ProjectWebServiceState` | The in-memory map is wiped by every API restart; that is why the existing alert never fired | Keep in-memory and shorten the threshold — still lost on restart |
| Pause, don't abandon | Explicit paused state + alert + human resume beats infinite silent retry *and* beats permanent give-up | Keep retrying with a longer cap (status quo) |
| Detect, don't repair, a corrupt DB | `pg_resetwal` risks data loss and is a judgement call | Auto-run `pg_resetwal` on signature match |
| Reuse `notification.AdminAlerter` | Slack + admin email already exist and are already used for disk alerts | New alerting subsystem |

## 6. Files touched (expected)

| File | Change |
|---|---|
| `api/pkg/sandbox/controller.go` | `drainNestedContainers` helper; call it in `Delete` for web-service sandboxes |
| `api/pkg/webservice/controller.go` | drain in `deployInPlace` + `RecoverWebService`; `validateRevision`; rollback returns/checks readiness; `Health` → `errored`; clear paused state on manual `Redeploy` |
| `api/pkg/webservice/verifier.go` (or new `failure_classify.go`) | `classifyDeployFailure` signature table |
| `api/pkg/webservice/health_monitor.go` | persisted counters replace `recovFails`; pause + alert-once; skip recovery when paused |
| `api/pkg/webservice/metrics.go` | `helix_webservice_recovery_paused` |
| `api/pkg/types/vhost.go` | four new `ProjectWebServiceState` columns |
| `api/pkg/store/` | setters for the new columns (+ mock regen) |
| `api/pkg/notification/admin_alerts.go` (+ template) | `SendWebServiceDownAlert` |
| `api/pkg/server/server.go` | wire the alerter into `NewHealthMonitor` |
| `api/pkg/server/webservice_handlers.go` | surface paused reason / last error |
| `frontend/src/…` project web-service panel | paused banner + error, via the generated API client |
| `sandbox/` scripts + outer compose | SIGTERM drain handler; `stop_grace_period` |
| `design/2026-07-28-we-find-ai-postgres-corruption-redeploy-loop.md` | incident write-up referencing this task and the 2026-07-08 doc |

## 7. Testing

Unit (gomock, table-driven where natural):
- `classifyDeployFailure` over each signature and the unmatched case.
- Health monitor: N-1 failures → still recovering; N failures → paused + alert fired exactly
  once; success → counters and paused state cleared; paused project still probed.
- `runDeploy`: validation failure → `deployInPlace` never called (this is the AC-2.3 guarantee,
  asserted at the mock boundary).

End-to-end in the inner Helix (`http://localhost:8080`) — **the primary evidence**, per repo
CLAUDE.md. Register `test@helix.ml` / `helixtest`, onboard, create a project with a web service
whose compose stack has a `db` that never becomes healthy:
1. Deploy a good revision → site serves. Deploy a revision with a broken compose file → deploy
   marked failed by validation, **old stack still serving** (AC-2.3).
2. Break the db so readiness fails persistently → observe the pause after N failures, `errored`
   in the UI, the classified error, and the alert firing (log or stubbed sender) (AC-3.2–3.4).
3. Manual redeploy → paused state clears, recovery resumes (AC-3.5).
4. Run a Postgres in a web-service sandbox, trigger a platform-initiated redeploy/teardown, then
   `pg_controldata` → `Database cluster state: shut down` (AC-1.5). Compare against the same
   check before the fix to show the difference.

Per CLAUDE.md: a unit test asserting a field was set is **not** evidence the feature works.
Anything not run end-to-end must be reported as "NOT tested: <what/why>" — no percentages, no
reasoning-by-analogy.

## 8. Notes for future agents

- The web-service package doc's "at most one instance ever touches /data" claim was **not
  enforced** before this change — `deployScript` kills only the `startup.sh` process group, and
  nested compose containers are children of the inner dockerd. If you're reasoning about data
  safety here, check what actually stops the containers, not what the comment says.
- Nested-docker (DinD) sandboxes do not propagate SIGTERM to inner containers. Any path that
  stops the outer container hard-kills everything inside it. Assume this whenever you touch
  sandbox lifecycle.
- `HealthMonitor`'s counters were in-memory; in prod, API restarts are frequent enough that
  in-memory failure counters never accumulate. Persist anything that gates an alert.
- `notification.AdminAlerter` is the existing platform alert path (Slack via `janitor.SendMessage`
  + admin email). Don't build a new one.
- The inner Helix is a full stack and takes 5–10 minutes to come up. `000`/connection-refused on
  `localhost:8080` early on means "still booting", not "broken".
