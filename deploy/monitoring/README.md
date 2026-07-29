# Monitoring and alerting for hosted web services

These rules page a human when a customer's hosted web service goes down and the
platform cannot fix it itself.

## Why this directory exists

On 2026-07-28 the paying-customer site **we-find.ai** served HTTP 503 for five
days. An alert did fire — and then showed as **resolved**, repeatedly — so the
operator reasonably read it as a self-healing blip and nobody investigated until
the customer complained.

The cause was the signal, not the rule. `helix_webservice_up` is set to `1`
whenever a deploy is in flight (`health_monitor.go`), and auto-recovery redeploys
a broken service roughly every 11 minutes. A permanently-down site therefore
produces a **square wave**, not a flat zero:

```
up:   0 ──11m── 1 ──10m── 0 ──11m── 1 ──10m── 0 ...
             ▲                    ▲
             └─ ALERT RESOLVES ───┘   ...while every request returns 503
```

`deploy/monitoring/helix-webservice.rules.test.yml` replays that exact waveform
and asserts both behaviours: the new rule stays firing, and the old
`helix_webservice_up == 0` shape resolves repeatedly.

**Never write an alert against `helix_webservice_up`.** It is a dashboard signal.
Page on `helix_webservice_unhealthy_since_seconds`, which records when the
current down-streak began and is cleared *only* by a genuinely successful probe.

## Installation

These files are **not** loaded by anything in this repo. Helix does not ship
Prometheus. Install them into your monitoring stack:

1. Copy `helix-webservice.rules.yml` to the Prometheus rules directory (in the
   infra repo — see "Where this must be installed" below) and reference it from
   `prometheus.yml`:

   ```yaml
   rule_files:
     - /etc/prometheus/rules/helix-webservice.rules.yml
   ```

2. **Enable the metrics listener on the controlplane.** It is off by default.
   Without it none of the `helix_webservice_*` series exist and every rule here
   is inert:

   ```bash
   # in the controlplane .env
   HELIX_METRICS_LISTEN=0.0.0.0:9110
   ```

   The listener is deliberately kept off the public app port
   (`api/pkg/server/metrics_listener.go`). **Firewall port 9110 to your
   Prometheus scraper** — it exposes process and runtime internals.

3. Add the scrape target:

   ```yaml
   scrape_configs:
     - job_name: helix-controlplane
       static_configs:
         - targets: ['helix-cloud-london:9110']
   ```

4. Reload Prometheus and confirm the series exist:

   ```bash
   curl -s 'http://prometheus:9090/api/v1/query?query=helix_webservice_up' | jq .
   promtool check rules helix-webservice.rules.yml
   ```

5. Confirm the labels below actually route to your pager.

## Alert routing

Every rule is labelled `severity: page` and `team: platform`. Your Alertmanager
`route` must select on those, or the rules fire into the void exactly like the
log line did:

```yaml
route:
  routes:
    - matchers: [severity="page", team="platform"]
      receiver: platform-oncall     # PagerDuty / Slack / whatever pages a human
      repeat_interval: 1h
```

If your route matches on different labels, change the labels in
`helix-webservice.rules.yml` to match and re-run the tests.

## The rules

| Alert | Fires when | Why it exists |
|---|---|---|
| `HelixWebServiceDown` | A service has been continuously down > 15 min | The primary page. Cannot self-resolve during a recovery redeploy. |
| `HelixWebServiceRecoveryLooping` | ≥ 3 consecutive failed auto-recoveries for 15 min | Auto-recovery re-runs the same commit; a persistent failure is environmental and needs a human. |
| `HelixWebServiceServing503s` | The vhost proxy has failed requests continuously for 15 min | Customer truth — independent of the platform's own health state machine. |
| `HelixWebServiceMetricsMissing` | No `helix_webservice_up` series for 30 min | **Dead-man switch — do not delete.** |

### About the dead-man switch

Every other rule depends on this process being scraped. If `HELIX_METRICS_LISTEN`
is unset, the scrape target breaks, or the API cannot start, they all go silent —
and silence is indistinguishable from health. Without
`HelixWebServiceMetricsMissing`, the failure mode of this entire directory is the
failure mode it was written to fix.

## Belt and braces: the in-code alert path

Prometheus is not the only path. `webservice.HealthMonitor` also pages directly
through `notification.AdminAlerter` (Slack via `JANITOR_SLACK_WEBHOOK_URL`, plus
email to admin users) once a service has been down past
`webservice.DownAlertThreshold` — one alert per down-streak, plus a recovery
notice.

This is deliberate duplication. The two paths fail independently:

- Prometheus covers rate-based and dead-man conditions the API cannot self-report
  (including "the controlplane itself is down").
- The in-code path works even if scraping, Prometheus, or Alertmanager is broken,
  misconfigured, or — as we could not rule out during this incident — never
  wired to these metrics at all.

Keep `DownAlertThreshold` in `api/pkg/webservice/health_monitor.go` and the
`> 900` threshold in `HelixWebServiceDown` in step.

## Running the tests

```bash
cd deploy/monitoring
promtool check rules helix-webservice.rules.yml
promtool test rules helix-webservice.rules.test.yml
```

Both run in CI (`prometheus-rules-test` in `.drone.yml`).

`testdata/naive-rule-for-comparison.yml` is the **wrong** rule, kept only so the
tests can demonstrate the flapping-resolve bug. Do not install it.

## Where this must be installed

**Unresolved at time of writing.** The Helix repo has no Prometheus, Alertmanager
or Grafana configuration; the prod stack lives elsewhere (infra repo / prod
Alertmanager), which the author of this change did not have access to. Before
this is considered done, an operator must:

- [ ] copy `helix-webservice.rules.yml` into the prod Prometheus rules directory
      and record the path here;
- [ ] confirm `HELIX_METRICS_LISTEN` is set on `helix-cloud-london` and scraped;
- [ ] confirm `severity: page` / `team: platform` matches the Alertmanager route;
- [ ] identify and **delete** any existing rule based on
      `helix_webservice_up == 0`, which is the one that resolved itself
      throughout the we-find.ai outage;
- [ ] force a test alert and confirm a human receives it.

Until every box is ticked, the in-code AdminAlerter path above is the only thing
that will actually reach a person.
