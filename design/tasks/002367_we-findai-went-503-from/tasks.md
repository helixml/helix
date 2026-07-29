# Implementation Tasks: Graceful Nested-Stack Shutdown and Non-Flapping Paging for Hosted Web Services

> Order note: Gap 2 (paging) is implemented first. The operator's judgement is that not being
> paged was the biggest failure of this incident — corruption is recoverable, five silent days
> are not. Gap 1 follows.

## Gap 2 — make the alert reach a human and stay fired

- [x] Add `helix_webservice_unhealthy_since_seconds{project_id}` to `webservice/metrics.go` and to `forgetProjectMetrics`
- [x] Maintain it in `HealthMonitor`: set once on the first failed probe of a streak, never refreshed while down, cleared only by a successful probe, NOT cleared by an in-flight deploy
- [x] Add `helix_webservice_upstream_errors_total{project_id}` and increment it in the `vhost_proxy.go` error handler (metric only — no change to the holding page)
- [x] Wire `notification.AdminAlerter` into `HealthMonitor`; send one alert per down-streak once past the 15-minute threshold, plus one recovery message; no-op with a warn log when neither Slack nor admin email is configured
- [x] Alert payload: project id/name, hosted domains, down duration, consecutive recovery failures, classified deploy error, deploy-log URL
- [x] Unit-test: down-since set once and not refreshed; not cleared by an in-flight deploy; cleared on success; alert sent exactly once per streak
- [x] Create `deploy/monitoring/helix-webservice.rules.yml` with `HelixWebServiceDown`, `HelixWebServiceRecoveryLooping`, `HelixWebServiceServing503s`, and the `HelixWebServiceMetricsMissing` dead-man rule, each with `keep_firing_for`
- [x] Create `deploy/monitoring/helix-webservice.rules.test.yml`: replay the find-ai square wave and assert the down alert fires and never resolves; assert a naive `up == 0` rule would have resolved; slow-first-deploy produces no page; looping and dead-man rules fire
- [x] Wire `promtool test rules` into CI
- [x] Write `deploy/monitoring/README.md`: where the rules must be installed, the label set that routes to the pager, and that `HELIX_METRICS_LISTEN` must be set on the prod controlplane and scraped or every rule is inert

## Gap 1 — graceful shutdown of the nested app stack

- [x] Add `DrainNestedContainers(ctx, hc, sandboxID, grace)` to `api/pkg/sandbox/` — one exec of `docker ps -q` + `docker stop --time <grace>`, exec timeout `grace + slack`, returns whether the drain completed or was forced
- [x] Unit-test the helper: expected exec issued, grace bound respected, no-containers case, wedged-container case does not block
- [x] Drain in `webservice.Controller.deployInPlace` before running `deployScript` (keep the existing pidfile/process-group kill)
- [x] ~~Drain in `webservice.Controller.RecoverWebService`~~ — **not needed**: the recreate branch calls `sandboxes.Delete`, which now drains web-service sandboxes itself. A second call site would be duplicate logic (see design.md Implementation Notes)
- [x] Drain in `sandbox.Controller.Delete` when `Purpose == types.SandboxPurposeWebService`, before `hydraClient.DeleteDevContainer`; leave other purposes on the fast 2s path
- [x] Add hydra `POST /api/v1/drain?grace=<seconds>` — drains all containers labelled `helix.persistent=true` in parallel, bounded, idempotent
- [x] Call the drain from hydra's existing SIGTERM handler (`api/cmd/hydra/main.go`)
- [x] Replace `exec tail -f /dev/null` in `sandbox/startup-app.sh` with backgrounded `tail` + `wait` + a SIGTERM trap that POSTs the hydra drain endpoint before exiting
- [x] Set `stop_grace_period: 120s` on the sandbox services in `docker-compose.yaml` and `docker-compose.dev.yaml`
- [ ] Document the matching prod-runner compose change (`/opt/HelixML`, outside this repo) in the PR description
- [x] Unit-test each call site: drain runs before teardown; dockerd-unresponsive branch skips it

## Gap 1 follow-on — actionable deploy error

- [~] Add `classifyDeployFailure(logTail)` with signatures for corrupt checkpoint / invalid resource manager ID, `PANIC:`, `dependency failed to start`, `container ... is unhealthy`
- [ ] Call it on readiness failure, store the specific error on the deploy row, fall back to the existing generic error on no match
- [ ] Include the classified error in the AdminAlerter payload
- [ ] Table-test the classifier including the no-match case

## End-to-end verification in the inner Helix

- [ ] Stand up a project web service whose compose runs a real `postgres:15` with its volume under `/data`
- [ ] Control run on `main`: redeploy, capture the unclean shutdown and `pg_controldata` → `in production`
- [ ] With the change: redeploy and delete, capture `database system is shut down` + `pg_controldata` → `shut down`, then restart and confirm no recovery/PANIC
- [ ] Force the service down and capture `/metrics` showing `helix_webservice_unhealthy_since_seconds` pinned across a recovery redeploy while `helix_webservice_up` flaps
- [ ] Capture the AdminAlerter firing once against a test webhook, and `helix_webservice_upstream_errors_total` incrementing on a holding-page hit
- [ ] Record every result in the PR; mark anything not run as **NOT tested** with the reason

## Ship

- [ ] `go build ./...`, `go vet`, run the affected package tests
- [ ] Open the PR against `https://github.com/helixml/helix` referencing this incident and `design/2026-07-08-we-find-ai-custom-domain-prod-cutover.md`, with full URLs and the operator actions needed (runner compose `stop_grace_period`, rules install, `HELIX_METRICS_LISTEN`)
- [ ] Check CI yourself and drive it green
