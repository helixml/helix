# Design: Graceful Nested-Stack Shutdown and Non-Flapping Paging for Hosted Web Services

Incident context, code evidence and acceptance criteria are in `requirements.md`. Prior write-up
of the same platform gap: `design/2026-07-08-we-find-ai-custom-domain-prod-cutover.md`. Sibling
spec for the same incident with a wider scope: `design/tasks/002365_a-paying-customer-web/`
(see Open Question 4).

Everything in `api/pkg/webservice/` from the 2026-07-09 hardening — rollback, backoff,
`restart=unless-stopped`, orphan cleanup, the friendly 503 page — is **untouched**.

## 1. Problem shape

```
GAP 1 — teardown is a hard kill              GAP 2 — the down signal flaps, so the page resolves
  three layers stop the customer's DB          probe fails            → up=0   ALERT FIRES
  without ever asking it to stop:              recovery redeploys     → up=1   ALERT RESOLVES ✅
    redeploy → kills startup.sh pgroup only    deploy fails (10 min)  → up=0   ALERT FIRES
    sbx delete → docker stop -t 2              backoff (≤30 min)      → up=0
    host/SANDBOX_TAG → PID 1 dies, no trap     recovery redeploys     → up=1   ALERT RESOLVES ✅
  → nested postgres gets SIGKILL               … for five days …
  → corrupt WAL checkpoint → PANIC             → operator sees "down, then resolved" = a blip
```

The two are independent and both are needed. Gap 1 stops us *creating* the corruption. Gap 2
stops us *hiding* it — and, per the operator, Gap 2 is the one that actually cost five days.

## 2. Architecture

```
 API process
 ├─ webservice.Controller
 │   deployInPlace      → drainNested() BEFORE the pgroup kill              ◀── new (US-1)
 │   RecoverWebService  → drainNested() before sandboxes.Delete             ◀── new (US-1)
 │                        (skipped when dockerd is unresponsive)
 │   waitForReady fail  → classifyDeployFailure(log tail) → specific error  ◀── new (US-3)
 │
 ├─ webservice.HealthMonitor
 │   downSince map + helix_webservice_unhealthy_since_seconds gauge         ◀── new (US-2)
 │   crosses downAlertThreshold → AdminAlerter (Slack + admin email), once  ◀── new (US-2)
 │
 ├─ server.vhost_proxy  ErrorHandler → helix_webservice_upstream_errors_total ◀── new (US-2)
 │
 └─ sandbox.Controller.Delete
     Purpose == web-service → DrainNestedContainers() → hydra delete        ◀── new (US-1)

 Runner host — helix-sandbox-app
 ├─ startup-app.sh: trap SIGTERM → POST hydra /api/v1/drain → exit          ◀── new (US-1)
 ├─ hydra: drains containers labelled helix.persistent=true                 ◀── new (US-1)
 └─ compose: stop_grace_period: 120s                                        ◀── new (US-1)

 deploy/monitoring/  (new dir, this repo)
 ├─ helix-webservice.rules.yml        4 alerting rules incl. dead-man       ◀── new (US-2)
 ├─ helix-webservice.rules.test.yml   promtool tests replaying the outage   ◀── new (US-2)
 └─ README.md                          where to install, what to set        ◀── new (US-2)
```

## 3. Gap 1 — graceful nested shutdown

### 3.1 The drain primitive

One helper, used by every path that ends a web-service app stack. It lives in the `sandbox`
package because `webservice` imports `sandbox` (defining it in `webservice` and calling it from
`sandbox.Delete` would be an import cycle); `webservice` reaches it through the existing
`c.sandboxes.HydraClient(sb)` accessor.

```go
// DrainNestedContainers stops every container inside the sandbox's inner dockerd
// with a bounded grace period, honouring each image's STOPSIGNAL so stateful
// workloads (Postgres fast-shutdown on SIGINT) checkpoint cleanly. Best-effort
// and bounded: a wedged container must never block teardown.
func DrainNestedContainers(ctx context.Context, hc *hydra.RevDialClient, sandboxID string, grace time.Duration) error
```

Implementation: a single `RunSandboxCommand` exec of
`ids=$(docker ps -q); [ -n "$ids" ] && docker stop --time <grace> $ids || true`, with the exec
timeout set to `grace + slack`. It returns the exec result so callers can log whether the drain
completed or was forced. This is the same mechanism `applyRestartPolicy`
(`controller.go:253-271`) already uses successfully, so the path is proven.

**Why `docker stop` rather than `docker compose down`.** The brief allows either
("`docker compose down` (or SIGTERM the stack and wait a bounded grace period)"). `docker stop`
is chosen because:
- It is generic. The startup mechanism is deliberately *not* assumed to be docker-compose (see
  the `DeployLog` comment in `controller.go`); `docker ps` works regardless of how the app was
  launched. `compose down` needs us to locate the right compose file and project directory.
- It sends each image's declared `STOPSIGNAL`. The official Postgres image sets `SIGINT` =
  Postgres *fast shutdown*, which is exactly the clean checkpoint we need.
- It does not remove containers, networks or volumes, so nothing the customer expects to persist
  is destroyed and the next `compose up` is fast.

`docker stop` also marks containers as explicitly stopped, so the shipped
`restart=unless-stopped` policy will not race us by restarting them mid-drain. That interaction
is the reason `unless-stopped` was chosen over `always` and it works in our favour here.

### 3.2 Call sites

| Path | Change |
|---|---|
| `webservice.Controller.deployInPlace` (`controller.go:558`) | Drain before running `deployScript`. The pidfile/process-group kill **stays** — it stops the `startup.sh` supervisor; the drain is what actually stops the app containers. Together they make the package doc's single-writer claim true. |
| `webservice.Controller.RecoverWebService` (`controller.go:458`) | Drain before `c.sandboxes.Delete(...)` in the recreate branch. Skipped, with a log line, when the reason is `sandbox dockerd unresponsive` — there is nothing to drain through a dead dockerd (AC-1.5). |
| `sandbox.Controller.Delete` (`controller.go:232`) | Drain when `sandbox.Purpose == types.SandboxPurposeWebService`, before `hydraClient.DeleteDevContainer`. Other purposes keep the existing fast path (AC-1.7). |
| `sandbox/startup-app.sh` + hydra + compose | Outer-container stop path — see 3.3. |

### 3.3 The outer-container stop path (the one that actually caused this)

This is the layer the sibling spec and the brief both flag, and it is the most likely trigger of
the 2026-07-23 hard kill. Today:

- `sandbox/overlay/entrypoint.sh` ends with `exec gosu "${UNAME}" /opt/gow/startup-app.sh`, and
  `startup-app.sh` ends with `exec tail -f /dev/null`. So **PID 1 is `tail`**, which has no
  SIGTERM handler and dies instantly.
- `docker stop helix-sandbox-app` therefore kills PID 1 at once, the container's dockerd dies with
  it, and every nested `sbx-*` container — and every customer container inside those — is
  SIGKILLed. Hydra is never signalled at all.

Fix, in three parts:

1. **`startup-app.sh` traps SIGTERM.** Replace `exec tail -f /dev/null` with a backgrounded
   `tail` + `wait`, and a `trap` that calls hydra's drain endpoint before exiting. Backgrounding
   plus `wait` is required — a trap cannot run while the shell is blocked in `exec`.
2. **A hydra drain endpoint.** `POST /api/v1/drain?grace=<seconds>` on the existing hydra unix
   socket, invoked from the trap with
   `curl --unix-socket /var/run/hydra/hydra.sock -X POST 'http://localhost/api/v1/drain?grace=60'`.
   Hydra is the right authority: it already knows every dev container, its per-scope docker
   socket, and stamps `helix.persistent=true` on web-service containers
   (`devcontainer.go:469-479`). The handler iterates persistent containers and, for each, execs
   the same `docker stop --time` drain *inside* it, in parallel, bounded by `grace`.
   Hydra's own SIGTERM handler (`api/cmd/hydra/main.go:96`) also calls the drain, so the path
   works whichever process gets signalled first; the drain is idempotent.
3. **`stop_grace_period: 120s`** on the sandbox services in `docker-compose.yaml` /
   `docker-compose.dev.yaml`, so Docker actually waits for the drain instead of SIGKILLing at the
   10s default. The prod runner's compose lives outside this repo
   (`/opt/HelixML/upgrade-sandbox-app.helix.ml.sh`, per `scripts/deploy-prod.sh:46`) — the
   matching change there must be documented in the PR and applied by an operator.

### 3.4 Why not just raise hydra's 2-second stop timeout

Considered and rejected as the *only* fix, for a reason that matters: raising it does nothing.
SIGTERM to the `sbx-*` container goes to *its* PID 1, which does not stop the containers running
on its inner dockerd. A 60-second timeout would just mean waiting 60 seconds before SIGKILLing
the customer's Postgres. The drain has to be executed *inside* the sandbox, which is what 3.1
does. `devcontainer.go`'s `timeout: 2` therefore stays as-is — after a successful drain it applies
to an already-empty dockerd, which is fine.

## 4. Gap 2 — a signal worth paging on, then the page

The operator's report ("an alert fired, then I saw it resolve") is the key evidence. Adding a rule
to today's metrics would reproduce exactly that. So Gap 2 is fixed in four layers: make the signal
honest, add a customer-truth signal, ship rules that are tested against this outage's shape, and
add a delivery path that does not depend on Prometheus at all.

### 4.1 Stop the flap — `helix_webservice_unhealthy_since_seconds`

`helix_webservice_up` is set to `1` whenever a deploy is in flight
(`health_monitor.go:108-112`), which is correct for a dashboard — a first deploy genuinely is not
"down" — but fatal for alerting, because auto-recovery redeploys mean a permanently-broken service
spends much of its life "deploying".

New gauge, maintained by `HealthMonitor`:

```go
metricUnhealthySince = promauto.NewGaugeVec(prometheus.GaugeOpts{
    Name: "helix_webservice_unhealthy_since_seconds",
    Help: "Unix timestamp when this web service's current down-streak began; absent when healthy. Alert on time() - this.",
}, []string{"project_id"})
```

Rules:
- Set to `time.Now().Unix()` on the **first** failed probe of a streak; left alone on subsequent
  failures, so `time() - gauge` grows monotonically.
- Cleared (series deleted) **only** on a genuinely successful probe.
- An in-flight deploy — including a recovery-triggered redeploy — does **not** clear it. This is
  the whole point: a redeploy is an *attempt* to fix, not evidence of health.
- `forgetProjectMetrics` clears it when a project stops being an active web service.

`helix_webservice_up` keeps its current meaning and its dashboard role. It is **not** redefined —
existing panels keep working — but the docs state plainly that it must never be used for paging.
Considered and rejected: making `up` itself honest by suppressing the deploying-is-up rule. That
would page on every slow first deploy (`readinessWait` is 10 min) and change a shipped metric's
meaning under whatever dashboards already consume it.

Restart behaviour, stated honestly: this gauge is in-memory, so an API restart resets the streak
and the down alert re-arms from zero (it would re-fire ~15 min later). Persisting it is Open
Question 4 / task 002365's scope. For a five-day outage this is immaterial; for a
30-minute outage straddling a CD upgrade it could delay the page by one threshold window.

### 4.2 Customer truth — `helix_webservice_upstream_errors_total`

Everything above infers health from the platform's own state machine. The one thing that cannot
lie is a request a customer made that failed. `vhost_proxy.go`'s `ErrorHandler` (line 97) already
runs on exactly that event; it just does not count it.

```go
metricUpstreamErrors = promauto.NewCounterVec(prometheus.CounterOpts{
    Name: "helix_webservice_upstream_errors_total",
    Help: "Requests to a hosted web service that could not be proxied to its upstream (holding page served).",
}, []string{"project_id"})
```

One `.Inc()` in the error handler. No behaviour change, no change to the holding page — that is
shipped code and stays as it is. Cardinality is bounded by project id, same as the existing
gauges. This gives an alert that says "customers are being served 503s right now", which is
immune to any bug in the deploy state machine.

### 4.3 The rules — `deploy/monitoring/helix-webservice.rules.yml`

| Alert | Expression (shape) | For | Severity |
|---|---|---|---|
| `HelixWebServiceDown` | `time() - helix_webservice_unhealthy_since_seconds > 900` | 5m | page |
| `HelixWebServiceRecoveryLooping` | `helix_webservice_consecutive_recovery_failures >= 3` | 15m | page |
| `HelixWebServiceServing503s` | `rate(helix_webservice_upstream_errors_total[5m]) > 0` | 15m | page |
| `HelixWebServiceMetricsMissing` | `absent(helix_webservice_up)` | 30m | page |

Notes on the shapes:
- The threshold is 15 min *inside* the expression rather than a long `for:`, so the alert survives
  a scrape gap without re-arming its `for:` window.
- Every rule gets `keep_firing_for: 15m` (Prometheus ≥ 2.42) as belt-and-braces against
  resolve-flapping — no rule here should ever send a RESOLVED while the site is still 503.
- `HelixWebServiceMetricsMissing` is the dead-man switch, and it is not optional. If
  `HELIX_METRICS_LISTEN` gets unset, the scrape target breaks, or the API can't start, every other
  rule in this file goes silent — which is indistinguishable from "everything is fine". Absent the
  dead-man, the failure mode of this entire deliverable is the failure mode we are fixing.
- Annotations carry the project id and a runbook line pointing at the Web Service tab's deploy log
  and the AC-3.2 classified error.
- Labels are assumed `severity: page` + `team: platform` pending Open Question 2 — these must
  match whatever the prod Alertmanager `route` selects on, or the rules fire into the void exactly
  like the log line did.

### 4.4 Proving the rules against this outage — `promtool test rules`

Committed next to the rules, run in CI. This is how we earn the claim "this would have paged for
this exact outage" without prod access:

- **Test 1 (regression, the important one).** A `helix_webservice_up` series replaying the find-ai
  square wave — `0` for ~11 min, `1` for ~10 min, repeating for 3 hours — alongside a
  `helix_webservice_unhealthy_since_seconds` series pinned at t0. Asserts `HelixWebServiceDown` is
  firing at every sample from t0+20m onward and **never** resolves. Also asserts that a naive
  `helix_webservice_up == 0 for: 5m` rule would have resolved during the up phases — documenting
  the bug the alert you saw was suffering from.
- **Test 2.** Healthy service with a slow 10-minute first deploy → no alert (no false page).
- **Test 3.** `helix_webservice_consecutive_recovery_failures` climbing to 3 → looping alert fires.
- **Test 4.** Series disappear entirely → `HelixWebServiceMetricsMissing` fires.

### 4.5 The delivery path that does not depend on Prometheus

A rules file in this repo pages nobody until an operator installs it, the metrics listener is
enabled, and the scrape works. Open Question 3 is unresolved, and the honest reading of this
incident is that we do not currently know the alerting pipeline reaches these metrics at all. So
the in-code path is not a nice-to-have:

`HealthMonitor` gains an `alerter *notification.AdminAlerter` (already constructed in
`server.go:249`, already has Slack-via-janitor + admin email, already used by the waitlist and
disk-space alerts). When a project's down-streak crosses `downAlertThreshold` (15 min, matching
the rule), it sends **once per streak**:

```
🚨 Hosted web service DOWN — <project name> (<project_id>)
Domains: we-find.ai, www.we-find.ai, find-ai.apps.helix.ml
Down for: 17m   Consecutive failed recoveries: 3
Last deploy error: nested Postgres will not start: corrupt checkpoint record — data volume needs recovery
Deploy log: <app url>/projects/<id>/web-service
```

A `alerted map[string]bool` guard means one message per down-streak, cleared on recovery — no
per-tick spam. A recovery sends a single "✅ back up after Nm" so the operator knows it closed. If
no Slack webhook and no admin email is configured this is a no-op with a warn log, same as the
existing admin alerts.

Trade-off, stated: this is a second alerting path with its own thresholds, which is duplication of
a kind the repo's CLAUDE.md would normally push back on. It is justified here because the two
paths fail independently — Prometheus covers rate-based and dead-man conditions the API cannot
self-report (including "the API is down"), and the in-code path covers a mis-wired or unscraped
monitoring stack. The shared threshold constant lives in one place in Go and is referenced by a
comment in the rules file.

## 5. Gap 1 follow-on — actionable failure classification (US-3)

`waitForReady` currently fails with a generic "the app never started listening on port N". For a
corrupt DB that is technically true and operationally useless.

New `classifyDeployFailure(logTail string) string` in `webservice`: a table of
`{signature, humanMessage}` pairs matched against the tail of `/data/.helix-webservice.log`
(fetched with the existing `DeployLog` path). First match wins; no match returns "" and the
existing generic error is used unchanged (AC-3.4). Signatures to start with:

| Signature in the log | Stored error |
|---|---|
| `could not locate a valid checkpoint record` / `invalid resource manager ID` | nested Postgres will not start: corrupt checkpoint record — the data volume needs recovery, not a redeploy |
| `PANIC:` | nested database PANIC on startup — see deploy log |
| `dependency failed to start` / `container ... is unhealthy` | a dependency container never became healthy — see deploy log |

This deliberately does **not** change rollback, backoff, or the holding page. A tempting next step
— skipping rollback when the failure is classified as data-level, since re-running the same commit
cannot fix a corrupt volume — is out of scope by the brief's explicit instruction not to edit
rollback code. Flagged for 002365 or a follow-up.

## 6. Testing

Unit (gomock, table-driven, per repo convention):
- `DrainNestedContainers` issues the expected exec, respects the grace bound, and returns without
  error when there are no containers.
- Each call site drains before it tears down; the dockerd-unresponsive branch skips the drain.
- `HealthMonitor`: down-since is set once and not refreshed on subsequent failures; **not** cleared
  by an in-flight deploy; cleared by a successful probe; the alert sends once per streak.
- `classifyDeployFailure` table incl. the no-match case.
- `promtool test rules` on the rules file, wired into CI.

End-to-end in the inner Helix (`http://localhost:8080`) — this is the evidence that counts, and
per repo `CLAUDE.md` anything not run is reported as **NOT tested**:
1. Register/onboard, create a project, enable its web service, push a `.helix/startup.sh` whose
   compose runs `postgres:15` with a volume under `/data`.
2. Control run on `main`: redeploy, then `docker logs` the DB — expect no clean-shutdown line, and
   `pg_controldata` showing `in production` (AC-4.3).
3. With the change: redeploy and delete, then check for
   `received fast shutdown request` / `database system is shut down` and `pg_controldata` →
   `Database cluster state: shut down`; restart the stack and confirm no recovery/PANIC (AC-4.2).
4. Break the app (bind no port), watch `/metrics` on `HELIX_METRICS_LISTEN`: confirm
   `helix_webservice_unhealthy_since_seconds` stays pinned across a recovery redeploy while
   `helix_webservice_up` flaps 0→1→0 — the bug, demonstrated and fixed side by side (AC-4.4).
5. Confirm the AdminAlerter fires once with a test webhook, and `helix_webservice_upstream_errors_total`
   increments when a browser hits the holding page.

## 7. Decisions summary

| Decision | Rationale | Alternative rejected |
|---|---|---|
| `docker stop --time` inside the sandbox | Generic, honours `STOPSIGNAL` (Postgres → SIGINT fast shutdown), preserves volumes/networks | `docker compose down` — needs the compose project dir, removes containers/networks |
| Drain helper lives in `sandbox` | `webservice` imports `sandbox`; avoids an import cycle, one definition | Duplicating it in both packages |
| Hydra owns the outer-stop drain, triggered by a PID 1 trap | Hydra already knows every container, its socket, and the `helix.persistent` label; PID 1 is `tail` and must be trapped or nothing else gets a chance to run | Raising hydra's 2s stop timeout — does nothing, SIGTERM never reaches nested containers |
| New down-since gauge instead of redefining `helix_webservice_up` | Alerting needs a monotonic, non-flapping signal; `up` is correct for dashboards and is shipped | Suppressing deploying-is-up — pages on slow first deploys, changes a shipped metric |
| Dead-man rule is mandatory | Without it, a broken scrape is indistinguishable from health — this deliverable's own worst failure mode | Trusting the scrape |
| Both a Prometheus rule *and* an in-code AdminAlerter page | The two fail independently; we cannot currently confirm prod scrapes these metrics (OQ 3) | Rules only — inert if unscraped; in-code only — silent if the API is down |
| Classify failures, don't act on them | Brief scopes rollback/backoff out | Skipping rollback on data-level failures (flagged for follow-up) |
