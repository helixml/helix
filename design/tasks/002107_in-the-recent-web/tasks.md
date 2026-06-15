# Implementation Tasks: Durable Web Service State and Runner Pinning

## Data model
- [x] Add `Purpose string` to `Sandbox` and `CreateSandboxRequest` (`api/pkg/types/sandbox.go`). Added `SandboxPurposeWebService` const.
- [x] Add `HostDeviceID string` to `ProjectWebServiceState` (`api/pkg/types/vhost.go`) for visibility/UI; GORM AutoMigrate column `host_device_id`.
- [x] Add a store setter `SetWebServiceHostDeviceID` (+ mock) to record the bound runner onto `project_web_service_state`.

## Sandbox provisioning
- [x] In `buildMounts` (`controller_provision.go`), when `sandbox.Purpose == "web-service"`, bind-mount `<workspaceDir>/webservice/<ProjectID>/data` at `/data` (read-write), keyed by project ID. Added `webServiceDataDir` helper.
- [x] In `Create` (`controller.go`), copy `Purpose` from the request onto the sandbox row.
- [x] Ensure `Delete`/cleanup never removes `<workspaceDir>/webservice/<projectID>/data`. Confirmed safe by construction: `DeleteDevContainer` only removes the container + `docker-data-<id>` volume; GC is keyed by sandbox/session ID and can never match the project-keyed `webservice/<projectID>/data` path. No code change needed.
- [x] Confirm the existing persistent-sandbox sticky guard in `pickHostForSandbox` already pins the single web-service sandbox to its runner and fails loudly when that runner is offline. Confirmed (`controller_provision.go:336+`): a `Persistent` sandbox with a recorded `HostDeviceID` re-binds to that host and returns an error if it is offline. Regression test added in Tests section.

## Web service controller (in-place recreate, single writer)
- [x] Replace new-sandbox-per-deploy: `ensureSandbox` get-or-creates the project's single `Persistent=true, Purpose=web-service, TimeoutSeconds=-1` sandbox (keyed via `state.ActiveSandboxID`) and reuses it. Fails loudly if an existing sandbox isn't running (pinned runner likely offline).
- [x] `deployScript`/`deployInPlace`: stop previous app via pidfile (`kill -- -PGID`, launched with `setsid`) BEFORE `git fetch`/`checkout` + relaunch — guarantees a single writer of `/data`. Exec is synchronous (app backgrounded) so readiness can be polled after.
- [x] Export `HELIX_WEB_SERVICE_DATA_DIR=/data` (and `HELIX_WEB_SERVICE_PORT`) to the startup script.
- [x] `lastLiveSHA` captures the previously-live commit; on readiness failure `rollback` redeploys it so the site returns to last-known-good against intact `/data`.
- [x] After first provision, record the bound runner onto `ProjectWebServiceState.HostDeviceID` (+ active sandbox id).

## API / UI surfacing
- [x] Expose the pinned runner and `/data` location on the web-service GET endpoint (via the new `host_device_id` field on `ProjectWebServiceState`, already returned by GET) and in `WebServiceTab.tsx` (new "Storage & runner" section). Regenerated swagger + TS client via `./stack update_openapi`.

## Docs
- [x] Document the `HELIX_WEB_SERVICE_DATA_DIR=/data` convention (apps must write durable state there). New page `content/helix/using-helix/web-service-hosting/index.md` with a worked `.helix/startup.sh` example.
- [x] Document the single-writer constraint: exactly one app/DB instance runs at a time; deploys cause a brief restart-window of downtime; external Kubernetes for blue/green.
- [x] Document that pinning protects against reschedule/reboot but NOT permanent runner-disk loss; note backups as the user's responsibility.

## Tests
- [ ] Unit: `buildMounts` adds the `/data` bind only for `Purpose=web-service`, keyed by project ID.
- [ ] Controller: a redeploy reuses the same sandbox ID; assert the old app is stopped before the new one starts (no overlap → no two writers on `/data`).
- [ ] Controller: failed readiness rolls back to the previous SHA; service stays up on intact `/data`.
- [ ] Controller: deploy with the pinned runner offline fails loudly and leaves data untouched.
- [ ] Integration: app writes to Postgres/SQLite under `/data`, redeploy, data still present and only one DB process ran.
