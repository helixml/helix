# Design: Durable Web Service State and Runner Pinning

## Problem recap

Production web-service state is lost on redeploy and (under `/workspace`) on
reboot, and the web service is not pinned to a runner. See `requirements.md`
for the exact current-code behaviour. Two root causes:

1. **Data is keyed to the per-deploy sandbox ID.** Each deploy = new sandbox =
   new empty `<workspaceDir>/persist/<sandboxID>` volume. The app's data dir
   (`/workspace`) isn't even on the persistent mount.
2. **No project-level runner pin.** Deploys place freshly; the sticky guard in
   `pickHostForSandbox` only protects an *existing* sandbox, and we throw the
   old sandbox away every deploy.

## Approach

Decouple the **data volume** and the **runner** from the per-deploy sandbox,
binding both to the **project** instead. Keep the existing blue/green cutover
(provision new → readiness check → flip vhost → delete old) — it already gives
near-zero-downtime deploys — and mount a shared, project-scoped persistent
volume into every web-service sandbox, always on the same pinned runner.

```
project web service
  ├─ pinned runner (HostDeviceID)         ── chosen on first deploy, sticky
  └─ project data volume (per-project)    ── lives on that runner's local disk
        │  mounted at a stable path (/data) into EVERY deploy's sandbox
        ▼
   deploy N (sandbox A)  ──flip──►  deploy N+1 (sandbox B)
   both mount the SAME /data volume on the SAME runner
```

### Key decisions

**1. Per-project data volume, not per-sandbox.**
Add a host-side data directory keyed by project:
`<workspaceDir>/webservice/<projectID>/data`, bind-mounted into every
web-service sandbox at a stable in-container path **`/data`**. Because it is
keyed by project (not sandbox ID), deploy N+1 sees exactly what deploy N wrote.
This directory is **never** deleted by `sandbox.Controller.Delete` (which only
touches the container and the per-sandbox `persist/ephem` dirs), so deleting the
superseded sandbox after cutover does not touch web-service data.

`buildMounts` (`api/pkg/sandbox/controller_provision.go:241`) gains a branch:
when a sandbox carries a "web service data volume" marker, add the
`/data` bind mount. The marker is passed via the create request / sandbox row
(see decision 4) so the generic sandbox path stays unaware of web-service
semantics beyond "mount this extra dir".

**2. App writes durable state under `/data`.**
The bootstrap (`runBootstrap`) keeps cloning code into `/workspace` (code is
disposable — re-cloned every deploy) but exports `HELIX_WEB_SERVICE_DATA_DIR=/data`
to `.helix/startup.sh`. Convention: apps put their database / uploads / durable
files under `$HELIX_WEB_SERVICE_DATA_DIR`. Document this clearly; it is the
single contract that makes US-1/US-2 work. (For a SQLite/Postgres-in-container
app, point its data path at `/data`.)

**3. Pin the web service to a runner at the project level.**
Add `HostDeviceID` to `project_web_service_state`. On deploy:
- If the state has no `HostDeviceID` yet → let the scheduler pick, then record
  the chosen runner onto the state (first-deploy pin).
- If the state has a `HostDeviceID` → force the new sandbox onto that runner.
  If that runner is offline → **fail the deploy** with a clear message
  (mirrors the existing persistent-sandbox guard), leaving the current live
  service untouched.

Mechanism: `CreateSandboxRequest` gains an optional `HostDeviceID` (preferred
host). `pickHostForSandbox` already honours a sandbox's recorded `HostDeviceID`;
we extend first-time placement to honour a requested host and to error if that
specific host is offline. The web-service controller passes the pinned host on
every `provisionSandbox`.

**4. Plumb the two new bits through the create path.**
`CreateSandboxRequest` (and the `Sandbox` row) gain:
- `HostDeviceID string` — preferred/pinned runner (empty = scheduler picks).
- `WebServiceProjectID string` (or a boolean `WebServiceData bool` + reuse
  `ProjectID`) — tells `buildMounts` to add the `/data` bind for that project.

Using the existing `ProjectID` (already on the sandbox) plus a
`Purpose="web-service"` marker is the lightest option: `buildMounts` mounts
`<workspaceDir>/webservice/<ProjectID>/data` when `Purpose == "web-service"`.
This avoids a redundant field.

**5. Reboot semantics (what the user asked).**
- *Container recreate / sandbox reboot:* `/data` is a host bind mount → data
  survives. (Today's `/workspace`-only data does not — this fix addresses it.)
- *Runner reboot:* the data dir is on the runner's local disk; the project is
  pinned to that runner, so the next deploy/restart re-mounts it. Survives.
- *Permanent runner loss:* data is gone — local disk only. We surface the
  pinned runner and document that backups/replication (out of scope) are the
  only protection. The pin deliberately **refuses** to relocate rather than
  silently bring the service up empty on another runner.

### Alternative considered: reuse the active sandbox in place
Instead of new-sandbox-per-deploy, re-run bootstrap inside the existing active
sandbox (`git fetch && checkout && restart startup.sh`). This gets durability
"for free" (same sandbox ID → same persist dir, sticky runner). Rejected as the
primary design because it sacrifices blue/green: a failed/slow restart takes the
live site down, and there is no atomic cutover. The per-project volume keeps the
existing safe cutover while still being durable. (The in-place path could be a
future "fast restart" optimisation.)

## Affected code
- `api/pkg/types/vhost.go` — add `HostDeviceID` to `ProjectWebServiceState`.
- `api/pkg/types/sandbox.go` — add `HostDeviceID` and `Purpose` to `Sandbox`
  and `CreateSandboxRequest`.
- `api/pkg/sandbox/controller_provision.go` — `buildMounts` adds `/data` for
  web-service sandboxes; `pickHostForSandbox` honours a requested host on
  first placement and errors clearly if it is offline.
- `api/pkg/sandbox/controller.go` — `Create` copies `HostDeviceID`/`Purpose`
  onto the row.
- `api/pkg/webservice/controller.go` — `provisionSandbox` sets `Purpose` and
  the pinned `HostDeviceID`; `runDeploy` records the pin on first deploy and
  fails when the pinned runner is offline; `runBootstrap` exports
  `HELIX_WEB_SERVICE_DATA_DIR=/data`.
- Store: migration adding `host_device_id` to `project_web_service_state`;
  `SetActiveWebServiceSandbox` / a new setter to persist the pin.
- Docs (`docs/`): document `HELIX_WEB_SERVICE_DATA_DIR` and the
  single-runner-loss caveat.

## Testing
- Unit: `buildMounts` includes the `/data` bind only for `Purpose=web-service`
  and is keyed by project ID (stable across sandbox IDs).
- Unit: `pickHostForSandbox` returns the requested host when online; errors
  when the requested host is offline (no fallback).
- Controller: first deploy records the pin; second deploy reuses it; deploy
  with offline pinned runner fails and leaves the previous live service active.
- Manual/integration: deploy a tiny app that writes a row to a SQLite DB under
  `/data`, redeploy, confirm the row is still present.
