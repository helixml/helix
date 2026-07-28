# Requirements: Graceful Web-Service Shutdown and Stop-Loop Alerting for Hosted Deploys

## Background

On 2026-07-28 the paying-customer domain **we-find.ai** (plus `www.we-find.ai` and
`find-ai.apps.helix.ml`) served HTTP 503 for days. Root cause, already diagnosed and
mitigated live by the operator:

- The Find AI project (`prj_01kvz0e7b401545376fyyfxtta`) runs its web-service backend in a
  Helix web-service sandbox on the `code.helix.ml` runner, which runs a nested
  `docker compose` stack (`find-ai-app-1` + `find-ai-db-1`, Postgres 15).
- Something hard-killed Postgres at 2026-07-23 07:42 UTC with no clean shutdown. Postgres
  then refused to start: `invalid resource manager ID in primary checkpoint record` →
  `PANIC: could not locate a valid checkpoint record`.
- `db` never became healthy → `app` stayed in `Created` → nothing bound `:8080` → the vhost
  proxy (`api/pkg/server/vhost_proxy.go:107`) served the unavailable page.
- The health monitor re-ran the same failing deploy roughly every 11 minutes, forever, with
  no alert and no operator-visible signal. `rollback()` re-ran the *same* broken startup, so
  it failed identically.

Mitigation applied manually (backup corrupt volume → `pg_resetwal -f` → `docker compose up -d`);
all three domains verified 200. **That was a band-aid.** This task is the durable platform fix.

This is the second write-up of the same hardening gap. The first
(`design/2026-07-08-we-find-ai-custom-domain-prod-cutover.md`, section "Platform bug — make it
not happen again", options 1 + 2) was never shipped. That doc is not present in the current
`helix` checkout — see Open Questions.

## Code evidence gathered during planning

| Finding | Location |
|---|---|
| Sandbox teardown stops the container with a **2-second** timeout, commented "disposable dev containers … no need to wait for graceful shutdown". This SIGKILLs the nested dockerd and every nested container, including a live Postgres. | `api/pkg/hydra/devcontainer.go:1790-1793` |
| The redeploy path only SIGTERM/SIGKILLs the **`startup.sh` process group**. Nested compose containers are children of the inner dockerd, not of that group — so they are *not* stopped. The package doc's "single writer on /data" guarantee is not actually enforced. | `deployScript` in `api/pkg/webservice/controller.go:718` |
| `HealthMonitor.recovFails` (backoff + looping alert) is an **in-memory map**. Every API restart / CD upgrade resets it, so the destructive retry loop never converges and the looping log line effectively never fires in prod. | `api/pkg/webservice/health_monitor.go:31,260` |
| `maxRecoveryBackoff` is 30 min and recovery is *never* stopped — "still retried periodically rather than abandoned". | `api/pkg/webservice/health_monitor.go:42` |
| The looping condition only writes a `log.Error()` line. There is no alert delivery. | `api/pkg/webservice/health_monitor.go:264-268` |
| `rollback()` re-runs `deployInPlace` with the previous SHA and never checks readiness afterwards, so an environmental failure (corrupt DB) rolls back into the identical failure and is logged as if it worked. | `api/pkg/webservice/controller.go:651-660` |
| An alerting primitive already exists (Slack via `janitor.SendMessage`, plus admin email). Nothing in `webservice` uses it. | `api/pkg/notification/admin_alerts.go` |
| Sandbox container startup scripts install no SIGTERM handler, so stopping/recreating `helix-sandbox-app` (e.g. a `SANDBOX_TAG` bump from `scripts/deploy-prod.sh`) hard-kills every nested container. | `sandbox/*.sh` |
| `ProjectWebServiceState` has no failure/paused columns; deploy statuses are only `pending/building/live/failed/superseded`. | `api/pkg/types/vhost.go:54-86` |

## User Stories

### US-1 — Stateful customer apps survive platform lifecycle events
**As** a customer running a database in my web-service compose stack,
**I want** Helix to stop my stack gracefully whenever it redeploys, recovers, or tears down my
sandbox,
**so that** my database flushes and checkpoints cleanly instead of being corrupted.

Acceptance criteria:
- AC-1.1 Before a web-service sandbox is deleted or recreated by the platform, every nested
  container is stopped with a bounded graceful timeout (default 60s, configurable), honouring
  each image's `STOPSIGNAL` (Postgres uses `SIGINT` = fast shutdown).
- AC-1.2 A redeploy stops the nested app stack gracefully **before** starting the new one — not
  merely killing the `startup.sh` process group. The single-writer-on-`/data` invariant in the
  package doc becomes true rather than aspirational.
- AC-1.3 Stopping or recreating the outer sandbox host container (`helix-sandbox-app`) drains
  nested containers before the inner dockerd dies, and the outer compose service is given a
  `stop_grace_period` long enough for that drain.
- AC-1.4 Draining is bounded: a wedged nested container cannot block teardown indefinitely; the
  platform proceeds after the grace period and logs that it had to force.
- AC-1.5 Verified concretely: after a platform-initiated redeploy/teardown of a sandbox running
  Postgres, `pg_controldata` reports `Database cluster state: shut down` (not
  `in production`).

### US-2 — A failed deploy does not take down a working site
**As** a project owner,
**I want** a deploy that cannot possibly succeed to be rejected before my running stack is
touched,
**so that** a bad commit or a broken startup script never converts a healthy site into a 503.

Acceptance criteria:
- AC-2.1 `runDeploy` validates the *new* revision before stopping the running app: the target
  SHA is fetchable/checkoutable, `.helix/startup.sh` exists on the project's `helix-specs`
  branch, and any compose file the startup script references passes `docker compose config -q`.
- AC-2.2 Validation runs without disturbing the running stack (scratch worktree; no writes to
  the live code dir, no container operations).
- AC-2.3 If validation fails, the deploy is marked `failed` with the validation error, the
  previously-live stack keeps running, and the site keeps serving.
- AC-2.4 If validation passes but readiness fails afterwards, rollback runs **and its readiness
  is checked**. A rollback that also fails readiness is recorded as such — never logged as
  success.

### US-3 — A persistently failing service stops looping and pages a human
**As** the on-call operator,
**I want** the platform to stop re-running a destructive redeploy once it is clearly not working,
and to tell me,
**so that** a customer site cannot sit at 503 for days with zero signal.

Acceptance criteria:
- AC-3.1 Consecutive recovery failures are **persisted per project** (survive API restart / CD
  upgrade), not held only in memory.
- AC-3.2 After N consecutive failed recoveries (default 5, configurable), auto-recovery for that
  project is **paused**: the health monitor stops triggering redeploys until a human resumes.
- AC-3.3 On entering the paused state, an alert is delivered exactly once through the existing
  `notification.AdminAlerter` (Slack via janitor + admin email), containing: project id/name,
  the hosted domain(s), consecutive-failure count, the stored deploy error, and the tail of the
  deploy log.
- AC-3.4 The web service's health becomes `errored` (distinct from `unhealthy`, which means
  "down but still being auto-recovered") and the API surfaces the stored deploy error and the
  paused reason.
- AC-3.5 A manual redeploy (or an explicit resume) clears the paused state and the persisted
  failure counter; a subsequent successful probe also clears them.
- AC-3.6 Pausing is per project. Other projects' auto-recovery is unaffected.
- AC-3.7 The existing Prometheus metrics keep working, and a new gauge exposes paused state.

### US-4 — An unstartable nested database produces an actionable error
**As** a project owner or operator,
**I want** "your Postgres cannot start because its checkpoint record is corrupt" instead of an
opaque 503,
**so that** the real problem is visible without SSHing into a runner.

Acceptance criteria:
- AC-4.1 When readiness fails, the platform captures the tail of `/data/.helix-webservice.log`
  and matches it against known fatal signatures (at minimum:
  `could not locate a valid checkpoint record`, `invalid resource manager ID`,
  `PANIC:`, `dependency failed to start`, `container ... is unhealthy`).
- AC-4.2 A matched signature is stored on the deploy row as a specific, human-readable error
  (e.g. "nested Postgres will not start: corrupt checkpoint record — data volume needs
  recovery") rather than `readiness: timeout`.
- AC-4.3 The captured error and log tail are included in the AC-3.3 alert and returned by the
  web-service health/status API.
- AC-4.4 Signature matching never causes a false *pass*: an unmatched failure still fails the
  deploy with the generic readiness error.

### US-5 — Proven end-to-end, not asserted
**As** a reviewer,
**I want** the behaviour demonstrated in the inner Helix against a real stack that will not come
healthy,
**so that** we do not ship a second unverified write-up of this same gap.

Acceptance criteria:
- AC-5.1 In the inner Helix at `http://localhost:8080`, a project web service is created with a
  compose stack whose `db` never becomes healthy.
- AC-5.2 Demonstrated: a failed redeploy leaves the previously-good stack serving (US-2).
- AC-5.3 Demonstrated: after N failures the loop stops, state is `errored`/paused, and the alert
  path fires (captured in logs or via a stubbed sender).
- AC-5.4 Demonstrated: a platform-initiated teardown of a sandbox running Postgres leaves a
  clean `pg_controldata` shutdown state (US-1).
- AC-5.5 Anything not verified end-to-end is stated explicitly as **NOT tested**, per repo
  CLAUDE.md. No quantified confidence claims.

## Non-Goals

- Blue/green or zero-downtime deploys. The single-writer-on-`/data` design is deliberate
  (`api/pkg/webservice/controller.go` package doc; design doc 002107). "Keep old until new is
  healthy" here means *validate before teardown*, not run two stacks against one data dir.
- Automatic repair of a corrupted Postgres volume. `pg_resetwal` is a destructive, judgement
  call and stays a human decision. We detect and report; we do not auto-run it.
- Backups of customer web-service data volumes (worth doing — out of scope here).
- Any change to the prod find-ai deployment itself; it is already restored.

## Constraints

- Real production, paying customer. No band-aids, no fallback code paths, clean up dead code
  (repo `CLAUDE.md`).
- Go: wrap errors, no `map[string]interface{}` API responses, GORM AutoMigrate only, gomock.
- The `webservice` package imports `sandbox`; the drain-on-delete hook must not create an import
  cycle.
- PR against `https://github.com/helixml/helix`, full URLs in any write-up.

## Open Questions

1. **The prior design doc is missing.** `design/2026-07-08-we-find-ai-custom-domain-prod-cutover.md`
   does not exist in `/home/retro/work/helix` on `main`, and `git log --all` finds no commit that
   ever added it. This spec reconstructs options 1 + 2 from the brief's description. Can you
   point at where that doc lives (another branch, prod filestore, another repo), or confirm the
   reconstruction is faithful?
2. **Alert channel.** I plan to reuse `notification.AdminAlerter` (Slack via `janitor.SendMessage`
   + email to admin users), since it already exists and the waitlist alert uses it. Is there a
   dedicated ops/incident Slack channel or PagerDuty route that should be used for
   customer-site-down instead of the generic admin alert?
3. **Pause threshold.** Assumed N = 5 consecutive failed recoveries (≈ the first hour, given the
   existing exponential backoff up to 30 min). Too aggressive, too slow, or should it be
   time-based ("down for 30 minutes") rather than count-based?
4. **Resume UX.** Assumed: paused state clears on a manual redeploy from the project UI/API. Do
   you want a separate explicit "resume auto-recovery" control, and should it be admin-only or
   available to the project owner?
5. **Drain grace period.** Assumed 60s for nested containers and a 120s `stop_grace_period` on
   the outer sandbox service. Postgres fast shutdown is normally sub-second, but a large busy DB
   can take longer. Are those bounds acceptable on the prod runner, given they extend how long a
   sandbox teardown blocks?
6. **Scope of graceful drain.** Assumed the drain applies to sandboxes with
   `Purpose == SandboxPurposeWebService` only, leaving spec-task/dev sandboxes on the existing
   fast 2s teardown. Should *any* persistent sandbox get the graceful path?
7. **Where the 2026-07-23 hard kill came from.** The most likely candidate is a prod deploy
   bumping `SANDBOX_TAG` on `code.helix.ml` (recreating `helix-sandbox-app`), which US-1/AC-1.3
   covers. Do you have host-side evidence (a reboot, an OOM kill, a `docker compose up` on the
   runner around 2026-07-23 07:42 UTC) that would confirm or point elsewhere? If it was an OOM
   kill of the nested Postgres specifically, that is a third root cause needing memory limits,
   and this spec does not currently address it.
