# Hosted web services flapping: root cause and fix (2026-09-29)

we-find.ai and birding.live — two customers' production sites on the London
control plane — have been going down repeatedly. The worst episode ran ~28 hours
across 2026-09-27/28. Customers saw a bare `502 active sandbox not found: not
found`.

This is the third incident in this family (see
`design/2026-07-08-we-find-ai-custom-domain-prod-cutover.md` and the 2026-07-28
nested-Postgres corruption). The previous two were treated as app-level
problems. This one is not: **the control plane destroys healthy customer
containers when it cannot reach them.**

## Evidence that it is not the customer's app

- `find-ai-app-1` carries `restart: unless-stopped`, `RestartCount=0`, both its
  containers healthy, clean startup log. It has never crash-looped.
- **birding.live goes down at exactly the same timestamps.** 19 of 20 outage
  episodes over 14 days are simultaneous to the minute. Two unrelated codebases
  do not fail in lockstep; shared infrastructure does.
- Both web services are pinned to the same runner, `code-for-app`
  (= code.helix.ml), which also hosts the spec-task agent desktops.
- Of 47 kernel OOM kills on that host since 2026-09-20, **zero** were in the
  `sbx-*` web-service containers. 39 were `chrome`, 7 `zed`, 1
  `chrome-devtools` — all inside `ubuntu-external-*` agent desktops. Chrome
  raises its renderers' `oom_score_adj` to 200–300, so it is always the
  preferred victim; the web-service containers are 2 GiB and use ~490 MiB.

The customer stacks were alive. What broke was our ability to *reach* them, and
then our reaction to that.

## The chain

1. **Host memory exhaustion.** `code.helix.ml` has 62 GiB of RAM and runs
   **292 GiB of committed container memory limits**: 12 agent desktops at 24 GiB
   each (`HELIX_SPEC_TASK_SANDBOX_DEFAULT_MEMORY_MB` default `24576`, 12 vCPUs)
   plus 2 web services at 2 GiB. `MAX_SANDBOXES=10` is set on the runner but
   nothing enforces it: `MaxSandboxes` is read only by `pkg/sandbox/compute/`,
   the cloud autoscaler, which is disabled for self-registered hosts.
   `helix-sandbox-app` itself has no memory limit. Result: repeated
   `global_oom`.

2. **The runner stops heartbeating.** `sandbox-heartbeat` is a single blocking
   loop on a 30 s ticker; each pass does
   `GET /containers/json?all=true&size=true` — a full layer-size walk over 16
   large DinD containers, 30 s timeout — plus disk collection. Under memory and
   I/O pressure it slips. Past `HELIX_SANDBOX_STALE_THRESHOLD` (5 m) the
   instance reaper flips `sandbox_instances.status` to `offline`.

3. **Probes fail, for the wrong reason.** `HealthMonitor.probe` →
   `Controller.Probe` → `HydraClient(sb).ProbeDevContainerPort` travels RevDial
   to that runner. Three consecutive failures (~90 s) fire `doRecover`.

4. **Recovery misdiagnoses it.** `RecoverWebService` re-probes (same path,
   fails), then calls `sandboxDockerAlive`, which runs `docker info` — *also
   over the same RevDial path*. All 3 attempts fail, so `recreate = true`,
   reason `"sandbox dockerd unresponsive"`.

   **This is the core defect.** Both signals that authorise destruction ride the
   single transport whose failure is exactly what a stressed runner produces.
   They cannot distinguish "the container is broken" from "I cannot reach the
   runner". The disambiguating signal — the runner's own heartbeat state — was
   available and free, and was not consulted.

5. **A healthy container is deleted.** `c.sandboxes.Delete(ActiveSandboxID)`
   soft-deletes the row.

6. **The replacement cannot be placed.** `Redeploy` → `ensureSandbox` →
   `sandboxes.Create` → the web-service runtime is
   `SandboxRuntimeUbuntuDesktop` (it needs a Docker daemon), so
   `spec.RequiresDisplay` is true → `HasDisplayCapableHost` requires a host with
   `status == "online"` → none → `ErrNoDisplayCapableHost`, surfaced as
   *"this deployment has no sandbox host with a display/render node"*.

   That message is misleading and cost real debugging time. code.helix.ml has an
   NVIDIA GPU; `sandbox-heartbeat` never even sends `render_node`, so the column
   is empty and the render-node check never fails here. The real reason was "the
   only runner is marked offline".

7. **The route dangles.** `active_sandbox_id` still points at the deleted row —
   it is only rewritten by `SetActiveWebServiceSandbox` at the very *end* of a
   successful `runDeploy`, after provision, bootstrap, checkout, compose up
   **and** the readiness probe. Every request meanwhile hits
   `dispatchProjectWebService` → `GetSandbox` (which filters
   `deleted_at IS NULL`) → `http.Error("active sandbox not found: not found",
   502)`, bypassing `writeUnavailablePage` entirely. That is the string the
   customer saw, for 28 hours.

8. **Nothing paged, and it never backed off.** `Redeploy` ends with
   `go c.runDeploy(...)` and returns `nil`, so `doRecover` recorded **success**
   for every failed deploy: `recovFails` cleared, backoff never engaged,
   `helix_webservice_consecutive_recovery_failures` pinned at 0. Prometheus
   confirms `max_over_time(...[14d]) == 0` across the whole outage, so
   `HelixWebServiceRecoveryLooping` — added after the July incident for exactly
   this — could never fire. 22–24 failed deploys per hour, for 28 hours.

Separately, **every prod release flaps the sites for 5–8 minutes**: the tag
build's `deploy-prod` recreates `helix-sandbox-app` on the runner (2.12.23 at
22:31 UTC → `/opt/HelixML` touched 22:48 → alert 22:51, resolved 22:56), which
genuinely stops everything inside it. Same pattern on 2.12.22 and 2.12.18. Not
addressed here — see follow-ups.

## The fix

**`sandbox.Controller.HostReadyForReplacement`** — can a replacement for this
sandbox actually be placed right now? The pinned host must be online, within the
dispatch staleness bound, able to host the runtime, and still advertising its
image. It **fails closed**: any lookup error means "not ready", because "I don't
know" must never authorise a delete.

The staleness bound is `DefaultSandboxDispatchStaleThreshold` (90 s), matching
`FindAvailableSandboxInstance`, deliberately tighter than the reaper's 5 m
mark-offline. Between 90 s and 5 m the row still reads "online" while the host
is already undispatchable — and that gap is exactly where healthy containers
were being destroyed.

**`RecoverWebService` gates its destructive branch on it.** When the runner
cannot take a replacement we log loudly and leave the existing container alone.
Its inner containers carry `restart=unless-stopped`, so the service returns by
itself the moment the runner recovers — with no deploy at all. The refusal is
returned as an **error**, so it counts as a failed recovery: backoff grows and
the looping gauge climbs, paging a human for what is an infrastructure problem
a redeploy cannot fix.

This does not lose any capability. `pickHostForSandbox` already refuses to
relocate a persistent sandbox (its data is on that host's disk), so recreating
on a dead runner was never going to work; deleting the row only destroyed the
record while stranding the data.

**The vhost middleware serves the holding page** instead of a raw 502 leaking
`active sandbox not found`. `serveHoldingPage` already picks "starting up" vs
"temporarily unavailable" from deploy state. Also applied to the
`ActiveSandboxID == ""` path.

**Recovery accounting reflects the deploy, not the kickoff.**
`RecoverWebService` returns the deploy it started and `doRecover` waits for a
terminal status via `awaitDeploy`. A deploy superseded by a newer one is not
counted as this recovery's failure. The backoff calculation moved into
`backoffFor` so it is testable.

## What this changes on 2026-09-27's timeline

The host would still have hit OOM, and the sites would still have blipped for
the length of each pressure window. But the container would not have been
destroyed, the route would not have dangled, the customer would have seen the
branded page rather than an internal error string, and
`HelixWebServiceRecoveryLooping` would have paged within minutes. 28 hours
becomes minutes.

## Monitoring (shipped separately, helixml/infra#79)

`code.helix.ml` had **no metrics at all** while hosting customer production.
Now: node-exporter installed and firewalled to the bunker subnet, added to
`node-exporter-all` as `node=code` (inheriting disk/IO/node-down alerts), plus a
`host-memory` group — `HostMemoryCritical` (available < 10% for 10 m) and
`HostOOMKilling` (> 2 OOM kills in 30 m). Both would have fired on 2026-09-27;
neither fires on the current steady state.

## Follow-ups, not in this change

1. **Capacity.** 292 GiB of limits on 62 GiB is the actual cause. Either move
   customer web services off the agent-desktop runner, or enforce a real
   capacity check at placement (`MaxSandboxes` is currently advisory on
   self-registered hosts), or size desktops against host RAM.
2. **Helix exposes no runner-health metric.** `/metrics` carries only the four
   `helix_webservice_*` series, so "the runner went offline" — the actual
   trigger — is invisible to Prometheus. A `helix_sandbox_instance_*` family
   (online, heartbeat age, active vs max) plus its alert rule.
3. **`ErrNoDisplayCapableHost` is returned for host-offline and
   stale-heartbeat.** It should say what actually failed.
4. **Web services are gated on the desktop runtime's display requirement**
   although they run headless. They need `RequiresDisplay: false` with the
   Docker-capable image, or a dedicated runtime.
5. **Releases flap hosted sites.** `deploy-prod` recreating
   `helix-sandbox-app` stops every container on the runner. Either don't
   recreate it when only the tag moved, or drain and restore web services
   around the roll.
6. **`sandbox-heartbeat` should not block on Docker disk accounting.** The
   `size=true` walk belongs on a slower, separate cadence than the liveness
   beat, so a busy host stays *visible* even when it is slow.
