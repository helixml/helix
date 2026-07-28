# Implementation Tasks: Graceful Web-Service Shutdown and Stop-Loop Alerting for Hosted Deploys

## 0. Setup

- [ ] Create a feature branch off `main` in `/home/retro/work/helix`
- [ ] Confirm the inner Helix is up (`docker compose -f docker-compose.dev.yaml ps`, `curl localhost:8080` → 200); wait up to 10 min before concluding otherwise
- [ ] Register `test@helix.ml` / `helixtest` in the inner Helix and complete onboarding (org → project)
- [ ] Reproduce the "before" state: run a Postgres in a web-service sandbox, trigger a platform teardown, and record `pg_controldata` showing `Database cluster state: in production` (the corruption precondition)

## 1. Root cause A — graceful nested shutdown

- [ ] Add `drainNestedContainers(ctx, hydraClient, sandboxID, grace)` to `api/pkg/sandbox/controller.go` — exec `docker ps -q | xargs -r docker stop --time <grace>` inside the sandbox, bounded, best-effort, logging whether the drain completed or was forced
- [ ] Make the grace period configurable (default 60s) via the sandbox config, not a magic number
- [ ] Call the drain in `sandbox.Controller.Delete` when `sandbox.Purpose == types.SandboxPurposeWebService`, before `hydraClient.DeleteDevContainer`
- [ ] Call the drain in `webservice.Controller.deployInPlace` before the existing `startup.sh` process-group kill
- [ ] Call the drain in `webservice.Controller.RecoverWebService` before `c.sandboxes.Delete(...)` on the recreate path; skip it when the recreate reason is "sandbox dockerd unresponsive"
- [ ] Add a SIGTERM handler to the sandbox startup scripts (`sandbox/`) that drains inner containers before the inner dockerd exits
- [ ] Raise `stop_grace_period` on the sandbox service in the outer compose files so Docker actually waits for that drain
- [ ] Update the `webservice` package doc comment so the single-writer-on-`/data` statement matches what the code now enforces

## 2. Root cause B1 — validate before teardown

- [ ] Add `Controller.validateRevision(ctx, sb, repo, sha)`: fetch/checkout the target SHA into a scratch worktree, assert `.helix/startup.sh` exists on the `helix-specs` branch, run `docker compose config -q` on any referenced compose file, then clean up the worktree
- [ ] Call `validateRevision` in `runDeploy` **before** `deployInPlace`; on failure mark the deploy failed with the validation error and return without stopping the running stack
- [ ] Verify validation performs no writes to the live code dir and no container operations

## 3. Root cause B2 — honest rollback and failure classification

- [ ] Change `rollback()` to return an error and to wait for readiness after redeploying the previous SHA
- [ ] Have `runDeploy` record the combined outcome when rollback also fails readiness, instead of logging rollback as successful
- [ ] Add `classifyDeployFailure(logTail) (message, matched)` with the signature table from the design doc (corrupt checkpoint / unhealthy dependency / generic DB PANIC / ENOSPC)
- [ ] On readiness failure, read the deploy log tail via the existing `DeployLog` path, classify it, and store the actionable message on the deploy row; fall back to the generic readiness error when unmatched

## 4. Root cause B3 — persisted stop-loop plus alert

- [ ] Add `ConsecutiveRecoveryFailures`, `RecoveryPausedAt`, `RecoveryPausedReason`, `LastRecoveryError` to `types.ProjectWebServiceState` (GORM AutoMigrate)
- [ ] Add store methods to read/update the new columns and regenerate the store mocks
- [ ] Replace `HealthMonitor.recovFails` with the persisted counter — delete the in-memory map, don't shadow it
- [ ] Pause auto-recovery for a project once the counter reaches `recoveryPauseThreshold` (default 5, configurable): set the paused fields and stop triggering recovery
- [ ] Keep probing paused projects so `helix_webservice_up` stays accurate and external healing auto-clears the pause
- [ ] Clear counters and paused fields on a successful probe and at the start of a manual `Redeploy`
- [ ] Add `AdminAlerter.SendWebServiceDownAlert` (+ email template) reusing Slack-via-janitor and admin email
- [ ] Fire the alert exactly once on entering the paused state, including project, domains, failure count, classified error, and deploy log tail
- [ ] Wire the alerter into `NewHealthMonitor` at `api/pkg/server/server.go:752`; keep it optional (nil = log only)
- [ ] Add the `helix_webservice_recovery_paused` gauge in `metrics.go` and clear it in `forgetProjectMetrics`

## 5. Surfacing

- [ ] Add `"errored"` to `Controller.Health`, returned when recovery is paused (distinct from `"unhealthy"`)
- [ ] Return `LastRecoveryError` and `RecoveryPausedReason` from the web-service status handler as typed struct fields
- [ ] Add swagger annotations and run `./stack update_openapi`
- [ ] Show the paused banner, classified error, and a redeploy-as-resume action in the project web-service UI, using the generated API client

## 6. Tests

- [ ] Unit: `classifyDeployFailure` across every signature plus the unmatched case
- [ ] Unit: health monitor — N-1 failures still recover; N failures pause and alert exactly once; success clears counters and paused state; paused projects are still probed
- [ ] Unit: `runDeploy` never calls `deployInPlace` when validation fails (asserted at the gomock boundary)
- [ ] `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` and `cd frontend && yarn build`

## 7. End-to-end verification in the inner Helix

- [ ] Create a project web service with a compose stack whose `db` never becomes healthy
- [ ] Prove a failed redeploy leaves the previously-good stack serving (AC-2.3)
- [ ] Prove the loop stops after N failures: state `errored`, classified error visible, alert fired (AC-3.2–3.4)
- [ ] Prove a manual redeploy clears the pause and resumes recovery (AC-3.5)
- [ ] Prove a platform-initiated teardown of a sandbox running Postgres leaves `pg_controldata` reporting `Database cluster state: shut down` (AC-1.5), compared against the "before" recording from task 0
- [ ] Record exactly what was and was not verified end-to-end; write "NOT tested: <what/why>" for any gap, with no quantified confidence claims

## 8. Ship

- [ ] Write `design/2026-07-28-we-find-ai-postgres-corruption-redeploy-loop.md` referencing this task and `design/2026-07-08-we-find-ai-custom-domain-prod-cutover.md`
- [ ] Remove any dead code the change orphans (per CLAUDE.md)
- [ ] Commit in conventional-commit format and open a PR against `https://github.com/helixml/helix` with full URLs and the e2e evidence in the description
- [ ] Check CI yourself (`gh pr checks` / Drone MCP tools), fix failures, and re-check until green
