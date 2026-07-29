# Fix the remaining root causes of the we-find.ai 503 outage

## Summary

On 2026-07-28 the paying-customer site **we-find.ai** was found serving HTTP 503. Its nested
Postgres had been hard-killed around 2026-07-23 and came back with a corrupt WAL checkpoint
(`PANIC: could not locate a valid checkpoint record`), so `find-ai-db-1` never started,
`find-ai-app-1` stayed `Created`, nothing bound `:8080`, and `vhost_proxy.go:107` served the
holding page for five days.

The 2026-07-09 deploy hardening (rollback, backoff, `restart=unless-stopped`, `HealthMonitor`,
the friendly 503 page) is **untouched** by this PR. It closes the two gaps that hardening did not
cover:

1. **Nothing gracefully stopped the customer's nested container stack** before the platform killed
   its sandbox — so a live database was SIGKILLed mid-write.
2. **The looping-recovery signal never reached a human.**

### The alerting finding

An alert *did* fire during this outage — and then showed as **resolved**, repeatedly. That was not
a missed page; the signal was unfit to alert on.

`helix_webservice_up` is set to `1` whenever a deploy is in flight
(`health_monitor.go:108-112`), and auto-recovery redeploys a broken service roughly every 11
minutes. A permanently-503 site therefore produced a **square wave**, and any rule shaped
`helix_webservice_up == 0` fired and auto-resolved on every recovery attempt. Worse than
momentary: `deployInProgress` stays true for up to `DeployBuildTimeout` (15 min), so the gauge sat
at `1` for 15-minute stretches. Confirmed live — see Verification.

## Changes

### Gap 2 — a signal worth paging on, and a page that arrives

- **`helix_webservice_unhealthy_since_seconds`** (new): records when the current down-streak
  began; cleared **only** by a genuinely successful probe. An in-flight recovery deploy does not
  clear it, so `time() - gauge` grows monotonically for as long as the site is actually down.
  `helix_webservice_up` keeps its dashboard meaning and is documented as never-page-on-this.
- **`helix_webservice_upstream_errors_total`** (new): incremented in the vhost proxy's error
  handler — requests real customers made that we failed. Metric only; the holding page is
  unchanged.
- **`deploy/monitoring/`** (new): four Prometheus rules — down, recovery-looping, sustained 503s,
  and a **dead-man switch** (`absent(helix_webservice_up)`), because a broken scrape would
  otherwise be indistinguishable from health. With `promtool` unit tests that replay this
  outage's waveform and assert the new rule never resolves — and that the naive `up == 0` rule
  does. Wired into CI as `prometheus-rules-test`.
- **Direct page path**: `HealthMonitor` now pages through the existing
  `notification.AdminAlerter` (Slack via `JANITOR_SLACK_WEBHOOK_URL` + admin email) once a service
  passes `DownAlertThreshold` (15 min) — once per down-streak, plus a recovery notice. Deliberate
  duplication of the Prometheus path: the two fail independently, and we cannot currently confirm
  prod scrapes these metrics at all (see Operator actions).

### Gap 1 — graceful shutdown of the nested app stack

- **`sandbox.DrainNestedContainers`** (new): execs a bounded `docker stop --time 60` *inside* the
  sandbox. `docker stop` (not `compose down`) is deliberate — it is generic, it honours each
  image's `STOPSIGNAL` (the Postgres image declares `SIGINT` = fast shutdown), and it destroys
  no containers, networks or volumes.
- Called before **redeploy** (`deployInPlace`) and before **sandbox delete** for
  `Purpose == web-service`. Ephemeral spec-task/dev sandboxes keep the existing fast 2s teardown.
- **`sandbox/startup-app.sh` now traps SIGTERM.** It ended in `exec tail -f /dev/null`, so **PID 1
  was `tail`** — `docker stop helix-sandbox-app` (host reboot, `SANDBOX_TAG` bump) killed it
  instantly, dockerd went with it, and every nested container was SIGKILLed. A trap cannot run
  while a shell is blocked in `exec`, so it is now `tail … & wait $!` plus a trap that calls a new
  hydra `POST /api/v1/drain`.
- **`--stop-timeout 120`** on the prod runner's `docker run` in `install.sh` (the prod compose file
  does not define sandbox services), and `stop_grace_period: 120s` on the four dev sandbox
  services — otherwise Docker SIGKILLs us mid-drain.

### Actionable failure diagnosis

- **`classifyDeployFailure`** turns "the app never started listening on port 8080" into "the app's
  nested database will not start: its write-ahead log has a corrupt checkpoint record. This is a
  DATA problem, not a code problem — redeploying the same commit cannot fix it." Ordered signature
  table (the specific corrupt-checkpoint diagnosis beats the generic `PANIC:`); an unmatched
  failure keeps the existing generic error. Rollback, backoff and the holding page are untouched.

## Verification

All of the following was run; nothing here is inferred.

**Gap 2, end-to-end in the inner Helix** (real API, real health monitor, real `AdminAlerter`, local
HTTP catcher standing in for the Slack webhook):

```
16:18:52 up=0 unhealthy_since=1.785341867e+09
16:20:53 up=1 unhealthy_since=1.785341867e+09   <-- recovery redeploy in flight
                                                    (the instant the OLD alert sent RESOLVED)
16:35:53 up=0 unhealthy_since=1.785341867e+09
```

`up` flapped 0→1→0; `unhealthy_since` held exactly one value across 70+ samples. The page was
delivered at **15m30s, while `up` was still 1** — the window in which the old alert showed
resolved:

```
🚨 Hosted web service DOWN — Find AI (e2e) (prj_findai_e2e)
Domains: we-find.ai, www.we-find.ai
Down for: 15m30s
Consecutive failed auto-recoveries: 1 — auto-recovery cannot fix this on its own
Last deploy error: the app never started listening on port 8080
```

Exactly **1** page across ~35 minutes of ticks.

**Gap 1**, two sandboxes differing only in PID 1, each running a nested compose Postgres with
dirty buffers written after the last checkpoint, then `docker stop`:

| | before | after |
|---|---|---|
| `docker stop` | **120s** (full timeout, then SIGKILL) | **0s** |
| drain ran | **no** | yes |
| Postgres log | *(none — killed)* | `checkpoint complete: shutdown immediate`, `database system is shut down` |
| `pg_controldata` | **`in production`** | **`shut down`** |
| next start | — | clean, no recovery/PANIC, all 400k rows intact |

Note: plain `docker:dind` is **not** a faithful harness — its PID 1 *is* dockerd, which stops
containers gracefully and hides the bug entirely. The harness above backgrounds dockerd and makes
PID 1 `tail`, matching the real sandbox.

Also: `go build ./...`, package tests, and `promtool test rules` all pass. The regression test for
the flapping gauge was verified to fail when the fix is sabotaged.

### NOT tested

- The recovery ("back up") notification — unit-tested only; needs a service that comes back.
- The admin-email arm (no SMTP in the inner Helix) and `helix_webservice_upstream_errors_total`
  incrementing live (needs a runner-backed sandbox to proxy to).
- `DrainNestedContainers` and hydra's `/api/v1/drain` through a live RevDial sandbox — the drain
  script and the failure mode were proven in the harness, and the Go wiring is unit-tested, but
  not exercised together. Needs `./stack build-sandbox` and a runner restart.
- `install.sh --stop-timeout 120` — syntax-checked only.

## Operator actions required

This PR does not page anyone on its own:

- [ ] Install `deploy/monitoring/helix-webservice.rules.yml` into the prod Prometheus rules dir.
- [ ] Confirm `HELIX_METRICS_LISTEN` is set on the controlplane **and scraped** — it is off by
      default, and every rule is inert without it.
- [ ] Confirm `severity: page` / `team: platform` matches the Alertmanager route.
- [ ] **Find and delete any existing rule based on `helix_webservice_up == 0`** — that is the one
      that resolved itself throughout this outage.
- [ ] Add `--stop-timeout 120` (or `stop_grace_period`) to the runner's compose/run on
      `code.helix.ml`, which lives outside this repo (`/opt/HelixML`).

See `deploy/monitoring/README.md`.

Incident: we-find.ai 503, 2026-07-28. Related design doc:
`design/2026-07-08-we-find-ai-custom-domain-prod-cutover.md`.
