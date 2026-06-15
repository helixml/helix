# Implementation Tasks: Durable Web Service State and Runner Pinning

## Data model
- [ ] Add `HostDeviceID string` to `ProjectWebServiceState` (`api/pkg/types/vhost.go`) and a GORM AutoMigrate column `host_device_id`.
- [ ] Add `HostDeviceID string` and `Purpose string` to `Sandbox` and `CreateSandboxRequest` (`api/pkg/types/sandbox.go`).
- [ ] Add a store setter to persist the pinned runner onto `project_web_service_state` (and read it back).

## Sandbox provisioning
- [ ] In `buildMounts` (`controller_provision.go`), when `sandbox.Purpose == "web-service"`, bind-mount `<workspaceDir>/webservice/<ProjectID>/data` at `/data` (read-write), keyed by project ID so it is stable across deploys.
- [ ] In `pickHostForSandbox`, honour a requested `HostDeviceID` on first-time placement; if that runner is offline, return a clear error instead of falling back to another runner.
- [ ] In `Create` (`controller.go`), copy `HostDeviceID` and `Purpose` from the request onto the sandbox row.
- [ ] Ensure `Delete`/cleanup never removes `<workspaceDir>/webservice/<projectID>/data`.

## Web service controller
- [ ] `provisionSandbox`: set `Purpose: "web-service"` and pass the project's pinned `HostDeviceID` (empty on first deploy).
- [ ] `runDeploy`: after a successful first deploy, record the chosen runner onto `ProjectWebServiceState.HostDeviceID`.
- [ ] `runDeploy`: if the pinned runner is offline, fail the deploy with a clear message and leave the current live service untouched (do not cut over).
- [ ] `runBootstrap`: export `HELIX_WEB_SERVICE_DATA_DIR=/data` to `.helix/startup.sh`.

## API / UI surfacing
- [ ] Expose the pinned runner and data-volume location on the web-service GET endpoint and in `WebServiceTab.tsx`.

## Docs
- [ ] Document the `HELIX_WEB_SERVICE_DATA_DIR=/data` convention (apps must write durable state there).
- [ ] Document that pinning protects against reschedule/reboot but NOT permanent runner-disk loss; note backups/replication as out of scope.

## Tests
- [ ] Unit: `buildMounts` adds the `/data` bind only for `Purpose=web-service` and keyed by project ID.
- [ ] Unit: `pickHostForSandbox` returns requested host when online; errors (no fallback) when offline.
- [ ] Controller: first deploy records pin; second deploy reuses it; deploy with offline pinned runner fails without cutover.
- [ ] Integration: app writes to a DB under `/data`, redeploy, data still present.
