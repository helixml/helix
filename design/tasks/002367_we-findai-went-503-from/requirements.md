# Requirements: Graceful Nested-Stack Shutdown and Non-Flapping Paging for Hosted Web Services

## Background

On 2026-07-28 the paying-customer site **we-find.ai** (plus `www.we-find.ai` and the baseline
`find-ai.apps.helix.ml`) was found serving HTTP 503. The Find AI project
(`prj_01kvz0e7b401545376fyyfxtta`) runs a nested `docker compose` stack — including its own
Postgres — inside its web-service sandbox (`sbx-01ky6z4njscab10294hkze09vh` on runner
`code.helix.ml`, compose dir `/workspace/find-ai`).

Postgres died with a corrupt checkpoint:

```
LOG:  database system was interrupted; last known up at 2026-07-23 07:42:09 UTC
PANIC:  could not locate a valid checkpoint record
startup process (PID 30) was terminated by signal 6: Aborted
```

→ `find-ai-db-1` never starts → `find-ai-app-1` stuck `Created` → nothing on `:8080` →
`vhost_proxy.go:107 web service upstream unavailable` → 503. Disk was 84% — **not** ENOSPC.
Around 2026-07-23 the sandbox/DinD/host was restarted and the running Postgres was hard-killed
with no clean shutdown, corrupting its WAL checkpoint.

Service was restored manually (volume backup + `pg_resetwal -f` + `docker compose up`); all three
domains return 200. That was a band-aid and is not repeated here.

Prior write-up to reference from the design: `design/2026-07-08-we-find-ai-custom-domain-prod-cutover.md`.

### Already shipped — explicitly out of scope

The 2026-07-09 hardening (commits `818cd3a44`, `72ec07f3b`, `14b774c8b`, `ee73bec4d`,
`3a307ed20`) is in `api/pkg/webservice/` and must **not** be rebuilt or edited: rollback to
last-known-good SHA, `restart=unless-stopped` self-heal, `HealthMonitor` (30s probe, recover
after 3 fails, exponential backoff capped at 30 min, orphan cleanup), the looping-recovery log
line + `helix_webservice_consecutive_recovery_failures` gauge, the friendly 503 page,
deploy-log surfacing, and the 502-leak fix.

Two gaps remain. This task fixes only those two.

## Code evidence gathered during planning

| Finding | Location |
|---|---|
| Sandbox teardown stops the sbx container with a **2-second** timeout ("disposable dev containers … no need to wait for graceful shutdown"). That SIGKILLs the container's inner dockerd and every nested customer container, live Postgres included. | `api/pkg/hydra/devcontainer.go:1789-1793` |
| The redeploy path stops only the **`startup.sh` process group** (pidfile + `setsid`, `kill -- -PID`). Nested compose containers are children of the inner dockerd, not of that group, so they are never stopped. The package doc's "single writer on `/data`" guarantee is therefore not enforced. | `deployScript`, `api/pkg/webservice/controller.go:718-777` |
| `RecoverWebService`'s recreate branch calls `sandboxes.Delete` with no drain. | `api/pkg/webservice/controller.go:458-464` |
| `sandbox.Controller.Delete` calls `hydraClient.DeleteDevContainer` directly; no nested drain, no `Purpose` special-casing. | `api/pkg/sandbox/controller.go:232-264` |
| The sandbox container's PID 1 is `exec tail -f /dev/null` with **no signal trap**. `docker stop helix-sandbox-app` (host reboot, `SANDBOX_TAG` bump via `scripts/deploy-prod.sh`) kills PID 1 immediately → outer dockerd dies → every nested container is SIGKILLed. Hydra never receives the signal. | `sandbox/startup-app.sh`, `sandbox/overlay/entrypoint.sh` |
| Hydra already stamps a `helix.persistent=true` label on web-service containers and knows each container's docker socket — the authority needed to drain them exists. | `api/pkg/hydra/devcontainer.go:469-479` |
| The proof that draining from the side that knows works: `applyRestartPolicy` already execs `docker ps -q` inside the sandbox via `RunSandboxCommand`. | `api/pkg/webservice/controller.go:253-271` |
| **`helix_webservice_up` flaps while a service is continuously down.** On every tick an in-flight deploy sets the gauge to **1** and clears failure counters. Auto-recovery redeploys every ~11 min (cooldown + backoff), and each redeploy is in-flight for up to `readinessWait` = 10 min. So a permanently-503 site produces a square wave, not a flat 0. | `api/pkg/webservice/health_monitor.go:108-112`, `controller.go:95`, `health_monitor.go:42` |
| There is **no** Prometheus rule, Alertmanager config, ServiceMonitor or PrometheusRule anywhere in the `helix` repo. | grep of `charts/`, `docker-compose*.yaml`, `scripts/` |
| The `/metrics` listener is opt-in and off by default; it only runs when `HELIX_METRICS_LISTEN` is set. | `api/pkg/config/config.go:846`, `api/pkg/server/metrics_listener.go:20-24` |
| The vhost proxy serves the holding page on upstream failure but **emits no metric**, so "customers are actually receiving 503s" is not observable anywhere. | `api/pkg/server/vhost_proxy.go:97-110` |
| An alert-delivery primitive already exists and is unused by `webservice`: Slack webhook via `janitor.SendMessage` (`JANITOR_SLACK_WEBHOOK_URL`) + admin email. | `api/pkg/notification/admin_alerts.go`, `api/pkg/janitor/janitor.go:78` |

### Why nobody was paged (revised root cause)

The operator reports that prod **is** wired to Prometheus + Grafana + Alertmanager, and that an
alert about a customer website being down did fire — **and then showed as resolved**. That is
consistent with the flapping gauge above: any rule shaped `helix_webservice_up == 0 for: <short>`
fires during the down phase and **auto-resolves on every recovery attempt**, so Alertmanager
sends a RESOLVED notification roughly every 11 minutes. A site that was 100% 503 to customers
for five days looked like a self-healing blip.

So Gap 2 is not only "no rule exists in the repo". The underlying signal is unfit to alert on,
and a rule written against it would page and un-page indefinitely. The signal must stop flapping
before any rule is worth installing.

## User Stories

### US-1 — Stateful customer apps survive platform lifecycle events
**As** a customer running a database inside my web-service compose stack,
**I want** Helix to stop my stack gracefully whenever it redeploys, recovers, or tears down my
sandbox,
**so that** my database checkpoints and exits cleanly instead of being corrupted.

Acceptance criteria:
- AC-1.1 Before a web-service sandbox is redeployed, recovered-by-recreate, or deleted, every
  container inside it is stopped with a bounded graceful timeout (default 60s, configurable),
  honouring each image's `STOPSIGNAL` — the official Postgres image declares `SIGINT`, which is
  Postgres *fast shutdown*.
- AC-1.2 A redeploy drains the nested stack **before** starting the new one, so the
  single-writer-on-`/data` invariant in the `webservice` package doc becomes true rather than
  aspirational.
- AC-1.3 Stopping or recreating the outer sandbox host container (`helix-sandbox-app` — host
  reboot, `SANDBOX_TAG` bump) drains nested containers before the outer dockerd dies, and the
  outer compose service is given a `stop_grace_period` long enough for that drain to complete.
- AC-1.4 Draining is bounded and never blocks teardown: after the grace period the platform
  proceeds and logs that it had to force.
- AC-1.5 The drain is skipped, with a log line, when there is nothing to drain through — e.g.
  `RecoverWebService`'s "sandbox dockerd unresponsive" branch.
- AC-1.6 Verified concretely: after a platform-initiated redeploy and after a platform-initiated
  teardown of a sandbox running Postgres, the DB log shows `database system is shut down` and
  `pg_controldata` reports `Database cluster state: shut down` (not `in production`).
- AC-1.7 Non-web-service sandboxes (spec tasks, ephemeral dev containers) keep the existing fast
  2s teardown. No global slowdown of sandbox deletion.

### US-2 — A down customer site pages a human and stays paged
**As** the on-call operator,
**I want** one unambiguous, non-flapping signal that a hosted site is down, wired to the pager,
**so that** a 503 customer site cannot resolve itself in Alertmanager while still being 503.

Acceptance criteria:
- AC-2.1 A new gauge records **when the current down-streak began**, e.g.
  `helix_webservice_unhealthy_since_seconds{project_id}`: set to now on the first failed probe of
  a streak, and cleared **only** by a genuinely successful probe. An in-flight deploy — including
  a recovery-triggered redeploy — does **not** clear it.
- AC-2.2 Given the find-ai failure pattern (probe fails → recovery redeploy in-flight up to 10
  min → fails → backoff up to 30 min → repeat), the down alert **fires once and keeps firing**
  for the whole outage. It must not resolve during a recovery attempt.
- AC-2.3 A customer-truth counter is incremented where the vhost proxy actually fails a request
  (`vhost_proxy.go` error handler), so "customers are receiving 503s" is observable independently
  of the deploy state machine. This is a metric increment only — no change to the holding page.
- AC-2.4 Prometheus alerting rules are delivered **in this repo** covering, at minimum:
  site down past a threshold (AC-2.1 gauge); recovery looping
  (`helix_webservice_consecutive_recovery_failures` past `loopingAlertThreshold`); sustained
  vhost 503s (AC-2.3); and a **dead-man rule** that fires when the web-service metrics stop being
  scraped at all — a silent scrape failure must not become a silent outage.
- AC-2.5 The rules are proven by `promtool test rules` unit tests committed alongside them. At
  least one test replays the find-ai square wave (up flapping 0→1→0 every ~11 min for hours) and
  asserts the down alert stays firing throughout and never resolves.
- AC-2.6 Documentation states exactly where the rules must be installed, what label set routes
  them to the pager, and that `HELIX_METRICS_LISTEN` must be set on the prod controlplane and
  scraped — the rules are inert without it.
- AC-2.7 Independent of Prometheus: when a project crosses the down threshold, the platform
  delivers an alert directly through the existing `notification.AdminAlerter` (Slack webhook via
  janitor + admin email), **once per down-streak**, cleared when the service recovers. This path
  works even if scraping, Prometheus, or Alertmanager is broken or misconfigured.
- AC-2.8 The alert payload identifies the project, its hosted domain(s), how long it has been
  down, and the stored deploy error, so the operator can triage without SSHing to a runner.

### US-3 — An unstartable nested database produces an actionable error
**As** an operator or project owner,
**I want** "your nested Postgres will not start: corrupt checkpoint record" instead of an opaque
`web service upstream unavailable` 503,
**so that** it is immediately clear this is a data problem, not a code problem.

Acceptance criteria:
- AC-3.1 On readiness failure the platform captures the tail of `/data/.helix-webservice.log`
  (already surfaced by `DeployLog`) and matches it against known fatal signatures — at minimum
  `could not locate a valid checkpoint record`, `invalid resource manager ID`, `PANIC:`,
  `dependency failed to start`, `container ... is unhealthy`.
- AC-3.2 A matched signature is stored on the deploy row as a specific, human-readable error
  (e.g. "nested Postgres will not start: corrupt checkpoint record — the data volume needs
  recovery") instead of the generic readiness timeout.
- AC-3.3 The classified error is included in the US-2 alert payload.
- AC-3.4 Classification never causes a false pass: an unmatched failure still fails the deploy
  with the existing generic readiness error. Rollback, backoff, and the holding page are
  untouched.

### US-4 — Proven end-to-end in the inner Helix, not asserted
**As** a reviewer,
**I want** both fixes demonstrated against a real stack,
**so that** we do not ship a second unverified write-up of this gap.

Acceptance criteria:
- AC-4.1 In the inner Helix (`http://localhost:8080`) a project web service is stood up whose
  compose runs a real Postgres.
- AC-4.2 Demonstrated: a platform-initiated redeploy/stop drains that stack cleanly — clean
  shutdown lines in the DB log, `pg_controldata` shows `shut down`, and the DB starts again with
  no recovery/PANIC.
- AC-4.3 Demonstrated (control): the pre-fix behaviour produced an unclean shutdown, so the
  change is shown to be the cause of the improvement.
- AC-4.4 Demonstrated: with the service forced down, the down-since gauge climbs monotonically on
  `/metrics` across a recovery redeploy (it does not reset), the `promtool` rule tests pass, and
  the AdminAlerter path fires (captured via a stubbed sender or a test Slack webhook).
- AC-4.5 Anything not verified end-to-end is stated explicitly as **NOT tested**, per repo
  `CLAUDE.md`. No quantified confidence claims.

## Non-Goals

- Re-implementing or editing any of the shipped 2026-07-09 hardening: rollback-to-last-known-good,
  exponential backoff, `restart=unless-stopped`, orphan cleanup, the friendly 503 page.
- Automatic repair of a corrupted Postgres volume. `pg_resetwal` is destructive and stays a human
  decision. We detect and report; we never auto-run it.
- Pausing auto-recovery, or persisting the recovery-failure counters across API restarts (the
  counters are in-memory today). Related but separate — see Open Question 4.
- Blue/green or zero-downtime deploys. The single-writer-on-`/data` design is deliberate; a brief
  restart window is accepted. The requirement is that the restart is *graceful*.
- Backups of customer web-service data volumes. Worth doing, out of scope here.
- Any further change to the prod find-ai deployment; it is already restored.

## Constraints

- Real production, paying customer. Root cause properly — no band-aids or fallback code paths,
  clean up dead code (repo `CLAUDE.md`).
- Go: wrap errors, structs not `map[string]interface{}` for API responses, GORM AutoMigrate only,
  gomock not testify/mock.
- `webservice` imports `sandbox`, so a drain hook on the delete path must not create an import
  cycle.
- Test end-to-end in the inner Helix. PR against `https://github.com/helixml/helix`, full URLs in
  any write-up.

## Open Questions

1. **Which alert fired and resolved?** You saw a customer-website-down alert fire and then
   resolve. I need its name and `expr`, and whether it is driven by `helix_webservice_up`, a
   blackbox probe against `we-find.ai`, or something else. If it is a rule on
   `helix_webservice_up`, this spec's US-2 replaces it and the old rule should be deleted. If it
   is a blackbox probe, the flap has a second cause worth finding (a probe against a 503 should
   not have resolved), and I would want that rule too.
2. **Infra repo / Alertmanager access.** I have no infra repo on this machine (only `helix`,
   `helix-next`, `docs`, `qwen-code`, `zed`, plus two Kodit-indexed repos) and no Alertmanager
   access. To ship a rule that actually reaches the pager I need either read access to the infra
   repo or the redacted Alertmanager `route`/`receivers` block, so I use the label set your route
   matches on. Absent that, the design assumes `severity: page` + `team: platform` and documents
   the install path as TBD. Please do not paste credentials into this task.
3. **Is `HELIX_METRICS_LISTEN` set on the prod controlplane, and is that port scraped?** It is
   off by default (`config.go:846`). If prod does not set it, no `helix_webservice_*` series exist
   in Prometheus at all and every rule here is inert — which would also explain the alert you saw
   coming from somewhere else entirely. This is the single most important thing to confirm.
4. **Relationship to task 002365.** `design/tasks/002365_a-paying-customer-web/` was written for
   this same incident on the same day, with a wider scope (pre-teardown revision validation,
   persisted failure counters, pausing auto-recovery). This spec is deliberately narrowed to the
   two gaps in the brief; the drain design is compatible with 002365's. Does 002367 supersede
   002365, or should both ship — and if both, which one owns the drain helper?
5. **Drain grace period.** Assumed 60s for nested containers and a 120s `stop_grace_period` on the
   outer sandbox service. Postgres fast shutdown is normally sub-second, but a large busy DB can
   take longer. Acceptable on the prod runner, given it extends how long a teardown blocks?
6. **Scope of the drain.** Assumed it applies to `Purpose == SandboxPurposeWebService` (and, on
   outer-container stop, to containers labelled `helix.persistent=true`, which today is the same
   set). Should *any* persistent sandbox get the graceful path?
7. **Down threshold before paging.** Assumed 15 minutes continuously down. Given a cold deploy can
   legitimately take ~10 min (`readinessWait`), anything shorter risks paging on a slow first
   deploy. Too slow for a paying customer's site?
8. **Pager route.** Assumed Slack for the in-code AdminAlerter path (`JANITOR_SLACK_WEBHOOK_URL`,
   already wired) and `severity: page` for Prometheus. Is there a PagerDuty route that
   customer-site-down should hit instead of, or in addition to, Slack?
9. **What actually hard-killed Postgres on 2026-07-23?** The strongest candidate is a
   `SANDBOX_TAG` bump / host restart recreating `helix-sandbox-app`, which AC-1.3 covers. Do you
   have host-side evidence (reboot, OOM kill, `docker compose up` on `code.helix.ml` around
   07:42 UTC)? If it was an OOM kill of the nested Postgres specifically, that is a third root
   cause needing memory limits, and this spec does not address it.
