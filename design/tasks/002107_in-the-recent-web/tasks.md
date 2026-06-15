# Implementation Tasks: Durable Web Service State and Runner Pinning

## Data model
- [x] Add `Purpose string` to `Sandbox` and `CreateSandboxRequest` (`api/pkg/types/sandbox.go`). Added `SandboxPurposeWebService` const.
- [x] Add `HostDeviceID string` to `ProjectWebServiceState` (`api/pkg/types/vhost.go`) for visibility/UI; GORM AutoMigrate column `host_device_id`.
- [x] Add a store setter `SetWebServiceHostDeviceID` (+ mock) to record the bound runner onto `project_web_service_state`.

## Sandbox provisioning
- [ ] In `buildMounts` (`controller_provision.go`), when `sandbox.Purpose == "web-service"`, bind-mount `<workspaceDir>/webservice/<ProjectID>/data` at `/data` (read-write), keyed by project ID.
- [ ] In `Create` (`controller.go`), copy `Purpose` from the request onto the sandbox row.
- [ ] Ensure `Delete`/cleanup never removes `<workspaceDir>/webservice/<projectID>/data`.
- [ ] Confirm the existing persistent-sandbox sticky guard in `pickHostForSandbox` already pins the single web-service sandbox to its runner and fails loudly when that runner is offline (no code change expected; add a regression test).

## Web service controller (in-place recreate, single writer)
- [ ] Replace new-sandbox-per-deploy: `Redeploy`/`runDeploy` get-or-create the project's single `Persistent=true, Purpose=web-service, TimeoutSeconds=-1` sandbox and reuse it across deploys.
- [ ] Deploy steps: exec `git fetch && git checkout <sha>` in `/workspace`; **stop the running app**; then start `.helix/startup.sh`. Guarantee the old app is stopped before the new one starts (single writer of `/data`).
- [ ] `runBootstrap`/start: export `HELIX_WEB_SERVICE_DATA_DIR=/data` to the startup script.
- [ ] Record the previously-live SHA; on readiness failure, roll back to it and restart so the site returns to last-known-good against intact `/data`.
- [ ] After first provision, record the bound runner onto `ProjectWebServiceState.HostDeviceID`.

## API / UI surfacing
- [ ] Expose the pinned runner and `/data` location on the web-service GET endpoint and in `WebServiceTab.tsx`.

## Docs
- [ ] Document the `HELIX_WEB_SERVICE_DATA_DIR=/data` convention (apps must write durable state there).
- [ ] Document the single-writer constraint: exactly one app/DB instance runs at a time; deploys cause a brief restart-window of downtime.
- [ ] Document that pinning protects against reschedule/reboot but NOT permanent runner-disk loss; note backups/replication as out of scope.

## Tests
- [ ] Unit: `buildMounts` adds the `/data` bind only for `Purpose=web-service`, keyed by project ID.
- [ ] Controller: a redeploy reuses the same sandbox ID; assert the old app is stopped before the new one starts (no overlap → no two writers on `/data`).
- [ ] Controller: failed readiness rolls back to the previous SHA; service stays up on intact `/data`.
- [ ] Controller: deploy with the pinned runner offline fails loudly and leaves data untouched.
- [ ] Integration: app writes to Postgres/SQLite under `/data`, redeploy, data still present and only one DB process ran.
