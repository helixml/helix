# Durable web service state and runner pinning

## Summary
Project web services now keep their state (databases, uploads, …) across
deploys and reboots, and each service is pinned to the runner that holds its
data. Previously every deploy provisioned a brand-new sandbox with an empty
volume, so a redeploy wiped the database; and app data lived under `/workspace`
(ephemeral), so it didn't even survive a container restart.

The crux: a web app that owns an on-disk database cannot have two processes
open the same data dir at once (Postgres refuses; SQLite corrupts). So deploys
are now **in-place** on a single long-lived sandbox — the old app is stopped
before the new one starts — guaranteeing a single writer of the durable `/data`
dir. This costs a brief restart-window of downtime, which is the right trade for
data safety. Zero-downtime blue/green / scaling is explicitly delegated to an
external Kubernetes cluster.

## Changes
- **Durable `/data`:** `buildMounts` bind-mounts `<workspaceDir>/webservice/<projectID>/data`
  at `/data` for `Purpose=web-service` sandboxes — keyed by project (not sandbox
  id) so it survives redeploys and reboots, and is never deleted on teardown.
- **In-place deploys:** rewrote `webservice/controller.go`. `ensureSandbox`
  get-or-creates one `Persistent=true, Purpose=web-service` sandbox and reuses
  it. `deployScript` stops the previous app (pidfile + `setsid` process group)
  before fetch/checkout/relaunch, and exports `HELIX_WEB_SERVICE_DATA_DIR=/data`
  + `HELIX_WEB_SERVICE_PORT`.
- **Rollback:** on readiness failure, redeploy the previously-live commit so the
  site returns to last-known-good against intact `/data`.
- **Runner pin:** the web-service sandbox is persistent, so the existing
  scheduler guard pins it to its runner and refuses to relocate (which would
  orphan local-disk data). The bound runner is recorded on
  `ProjectWebServiceState.HostDeviceID` and surfaced in the UI.
- **Types/store:** `Sandbox.Purpose`, `ProjectWebServiceState.HostDeviceID`
  (GORM AutoMigrate), `store.SetWebServiceHostDeviceID`.
- **UI:** new "Storage & runner" section in `WebServiceTab` showing the `/data`
  convention, pinned runner, and the single-writer/downtime note.
- **Tests:** `/data` mount (only for web-service, project-keyed); deploy script
  stops-before-start + env + checkout; rollback baseline selection. Pinned-runner
  -offline failure is covered by the existing placement suite.

## Note
Single-runner local disk is the storage substrate — pinning protects against
reschedule/reboot but not permanent loss of that runner's disk. Backups are the
user's responsibility (documented). Integration test (real DB under `/data`
across a redeploy) is manual, not automated in this PR.
