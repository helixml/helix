# Requirements: Durable Web Service State and Runner Pinning

## Background

Helix recently added **project web service hosting** (PR #2603, spec
`002096_so-sandboxes-aka-new`): a project can serve a live web app on a
custom/default domain. Each deploy is orchestrated by
`api/pkg/webservice/controller.go`, which **provisions a brand-new sandbox**,
clones the repo into it, runs `.helix/startup.sh`, cuts the vhost over to the
new container, then deletes the previous sandbox.

The user asked two questions about this feature:

1. Does state (e.g. a database) **survive between upgrades** (redeploys) and
   **persist across reboots** of the sandbox?
2. Can we **pin a production web service to a specific runner** (sandbox host),
   so that if it comes back up on a different runner it doesn't lose its data?

### What the code does today (the honest answer)

- **State does NOT survive a redeploy.** `provisionSandbox()` creates a *new*
  sandbox with a *new* ID on every deploy. The persistent workspace mount is
  keyed by sandbox ID (`<workspaceDir>/persist/<sandboxID>` →
  `/home/retro/work`), so a new sandbox always starts with an empty volume.
  Any database written by the previous deploy is left orphaned and unused.
- **State does NOT even survive a sandbox/container reboot in the common case.**
  The bootstrap clones the repo into `/workspace` and runs the app from there,
  but only `/home/retro/work` is the persistent mount. Data written under
  `/workspace` (or e.g. `/var/lib/postgresql`) lives on the container's
  ephemeral filesystem and is lost when the container is recreated.
- **Runner pinning is partial.** The sandbox scheduler
  (`pickHostForSandbox`) *is* sticky: a persistent sandbox re-binds to its
  original runner (`HostDeviceID`) and refuses to move to another runner when
  the original is offline (explicit data-loss guard). But the web-service layer
  does **not** carry a runner choice across deploys — each new deploy's sandbox
  is placed freshly via `FindAvailableSandboxInstance` and may land on a
  different runner. There is no project-level pin.
- **Persistent data lives on the runner's local disk.** If a runner is
  permanently lost, the data is gone — there is no replication/remote storage.

This feature closes those gaps for **production web services**.

## User Stories

### US-1: Database survives redeploys
As a project owner running a web app with a database, when I push a new commit
and Helix redeploys my web service, I want my database (and any other persisted
files) to be intact afterwards, so an upgrade does not wipe customer data.

**Acceptance criteria**
- Data written by the app to the designated persistent data directory is
  present and unchanged after a successful redeploy.
- A failed deploy never destroys the existing data volume.
- The persistent data directory is at a stable, documented in-container path
  that is the same across every deploy.
- **At most one app/database instance accesses the durable data directory at
  any instant.** A deploy must stop the old instance before the new one opens
  the data dir — two database processes (e.g. two Postgres) must never run
  against the same files concurrently. (This rules out concurrent blue/green
  cutover for stateful services; deploys are in-place with a brief restart
  window. See design.md.)

### US-2: Database survives reboots
As a project owner, when my web service's container or its runner restarts,
I want the database to come back with its data intact.

**Acceptance criteria**
- Data in the persistent data directory survives container recreation.
- Data survives a runner reboot (it is re-mounted from the runner's local disk
  and the service re-binds to the same runner).

### US-3: Pin a web service to a specific runner
As an operator, I want a project's production web service pinned to a chosen
runner, so its local-disk data volume is always reachable and a redeploy never
silently moves it to a different runner and loses data.

**Acceptance criteria**
- A project's web service records the runner (`HostDeviceID`) it is bound to.
- Every deploy for that project provisions its sandbox on the pinned runner.
- If the pinned runner is offline, a deploy **fails loudly** with a clear
  message rather than placing the new sandbox elsewhere on an empty volume.
- The first deploy (no pin yet) records whichever runner it lands on as the
  pin.

### US-4: Operator visibility & honest limits
As an operator, I want to see which runner a web service is pinned to and to
understand that pinning protects against reschedules/reboots but **not** against
permanent loss of that single runner's disk.

**Acceptance criteria**
- The pinned runner and the data volume location are surfaced (API/UI/docs).
- Docs clearly state that durability across *permanent* runner loss requires
  backups or replicated storage, which is out of scope for this change.

## Out of Scope
- Replicated/remote/networked storage for web-service data (single-runner
  local disk remains the storage substrate; only pinning + a stable volume are
  added here).
- Automated backups/snapshots of the data volume (note as a follow-up).
- Multi-runner / horizontally-scaled web services.
