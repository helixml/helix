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

## The single-writer constraint (why blue/green is out)

A web app that owns a database keeps that DB *on disk*. Two processes must
**never** open the same on-disk database concurrently — Postgres refuses to
start a second postmaster against the same data dir (`postmaster.pid` lock),
and a filesystem-level DB like SQLite will corrupt under two writers.

The current deploy model is blue/green: provision the new sandbox, run it and
readiness-check it **while the old one is still live**, then flip the vhost and
delete the old one. A readiness check against a DB-backed app means the new
instance *opens the database* — so for a window both the old and new instances
hold the same data directory. With one shared `/data` volume that is exactly the
"two Postgres on one filesystem" disaster.

**Conclusion:** durability via a shared mutable data dir is fundamentally
incompatible with concurrent blue/green. The durable data must be accessed by
**at most one running instance at a time**. This design therefore makes
stateful deploys **in-place / recreate** (exclusive handoff), not blue/green.

## Approach

One long-lived, **runner-pinned** web-service sandbox per project owns the
durable data dir. Deploys happen **in place**: fetch new code, stop the old app
process, start the new one. Only one process ever touches the data.

```
project web service
  └─ ONE pinned sandbox (Persistent=true, Purpose=web-service)
       ├─ pinned to a runner via HostDeviceID  ── existing sticky guard enforces it
       └─ /data  ── durable dir on that runner's local disk, keyed by project

   deploy:  git fetch + checkout <sha>
            │
            ├─ stop running app (releases /data)
            └─ start .helix/startup.sh  ── new app opens /data  (exclusive)
   exactly one writer of /data at any instant
```

### Key decisions

**1. One persistent web-service sandbox per project; deploys are in-place.**
Instead of new-sandbox-per-deploy, the project keeps a single
`Persistent=true, Purpose=web-service, TimeoutSeconds=-1` sandbox. A deploy
execs into it: `git fetch && git checkout <sha>` in `/workspace`, **stop the
current app**, then run `.helix/startup.sh`. Because the old process is stopped
before the new one starts, the database is opened by exactly one process. This
also makes runner pinning automatic (decision 3).

Trade-off accepted: a deploy incurs a short restart-window of downtime, and a
broken build can leave the app down. We mitigate with rollback (decision 4).
Data safety outranks zero-downtime for stateful services — and zero-downtime
was never actually safe here, it was the source of the corruption risk.

**2. Per-project durable data dir at `/data`.**
Bind-mount a host dir keyed by project — `<workspaceDir>/webservice/<projectID>/data`
— into the web-service sandbox at the stable in-container path **`/data`**.
Keying by project (not sandbox ID) means the data survives even if the single
sandbox row is ever rebuilt, and it is **never** deleted by
`sandbox.Controller.Delete` (which only touches the container and the
per-sandbox `persist/ephem` dirs). Code lives in `/workspace` (disposable,
re-cloned); durable state lives in `/data`.

`buildMounts` (`api/pkg/sandbox/controller_provision.go:241`) adds the `/data`
bind when `sandbox.Purpose == "web-service"`, using the sandbox's existing
`ProjectID`. No redundant field needed.

**3. App writes durable state under `/data`.**
`runBootstrap` exports `HELIX_WEB_SERVICE_DATA_DIR=/data` to `.helix/startup.sh`.
Convention: apps put their database / uploads / durable files under
`$HELIX_WEB_SERVICE_DATA_DIR`. This is the single contract that makes US-1/US-2
work — e.g. a Postgres-in-container app sets its data directory to `/data`,
SQLite apps put the `.db` file there. **Exactly one such app instance runs at a
time** (decision 1), so a single Postgres on `/data` is safe.

**4. Runner pinning comes from the single persistent sandbox.**
Because there is one long-lived persistent sandbox, the *existing*
`pickHostForSandbox` guard already does the pinning we want: it re-binds the
sandbox to its original `HostDeviceID` and **refuses to move a persistent
sandbox to a different runner when the original is offline** (the data is on
that runner's local disk). We surface the bound runner on
`project_web_service_state.HostDeviceID` (copied from the sandbox) for
visibility/UI, but the enforcement is the sandbox guard — no new placement
logic, no fallback-to-another-runner path to get wrong.

If the pinned runner is offline at deploy time, the deploy **fails loudly**
(the guard already returns that error) and the existing data is left untouched
rather than a fresh empty service coming up elsewhere.

**5. Deploy safety / rollback (replaces blue/green's safety net).**
Record the previously-live SHA. On deploy: checkout new SHA → stop app → start
app → readiness-poll the container port. If readiness fails within the timeout,
**roll back**: checkout the previous SHA and restart, so the site returns to the
last-known-good code against the same intact `/data`. Record `live` / `failed`
on the `web_service_deploys` row as today. There is no second sandbox to clean
up.

**6. Reboot semantics (what the user asked).**
- *Container recreate / sandbox reboot:* `/data` is a host bind mount → data
  survives. (Today's `/workspace`-only data does not — this fix addresses it.)
- *Runner reboot:* the data dir is on the runner's local disk and the sandbox
  is pinned to that runner, so it re-mounts on restart. Survives.
- *Permanent runner loss:* data is gone — local disk only. The pin deliberately
  **refuses** to relocate rather than silently bring the service up empty
  elsewhere. Document that backups/replication (out of scope) are the only
  protection.

### Blue/green and scaling → external Kubernetes
Built-in web-service hosting is deliberately the simple case: one app plus its
database, on one pinned runner, with durable local-disk state. It is **not** a
general orchestrator, and we will not bolt blue/green or multi-replica scaling
onto the single-runner sandbox model — that path leads straight back to the
single-writer / shared-volume problems above.

Users who genuinely need zero-downtime blue/green deploys, rolling updates, or
horizontal scaling should point Helix at an **external Kubernetes cluster** they
configure; the cluster handles deploy strategy, replica management, and
storage (PVCs / managed DBs) properly. The two models are complementary:

- **Sandbox hosting (this design):** zero-config, single-runner, in-place
  deploys with a brief restart window, durable on the runner's local disk.
  Best for small apps, internal tools, prototypes-that-stuck.
- **External Kubernetes:** operator-managed, supports blue/green and scale,
  durability is the cluster's responsibility.

So "stateless apps could keep blue/green" is answered by *use Kubernetes for
that*, not by adding a fragile opt-in flag to the sandbox path.

### Alternative considered: shared `/data` + blue/green
Mount the same per-project `/data` into both the old and new sandboxes during a
blue/green cutover. **Rejected** — this is precisely the "two Postgres on one
filesystem" corruption case the single-writer constraint forbids. Making it
safe would require quiescing/stopping the old DB before the new one opens the
volume, which is just in-place recreate with extra sandboxes.

## Affected code
- `api/pkg/types/vhost.go` — add `HostDeviceID` to `ProjectWebServiceState`
  (for visibility/UI; mirrors the bound sandbox's host).
- `api/pkg/types/sandbox.go` — add `Purpose string` to `Sandbox` and
  `CreateSandboxRequest`.
- `api/pkg/sandbox/controller_provision.go` — `buildMounts` adds the `/data`
  bind for `Purpose == "web-service"` keyed by `ProjectID`. (No change needed
  to `pickHostForSandbox` — its persistent-sandbox sticky guard already pins.)
- `api/pkg/sandbox/controller.go` — `Create` copies `Purpose` onto the row;
  ensure `Delete`/cleanup never removes `<workspaceDir>/webservice/<projectID>/data`.
- `api/pkg/webservice/controller.go` — biggest change: `Redeploy`/`runDeploy`
  no longer provision-new-then-flip. Instead: get-or-create the project's single
  web-service sandbox; exec in-place fetch/checkout; stop app; start
  `.helix/startup.sh` with `HELIX_WEB_SERVICE_DATA_DIR=/data`; readiness-poll;
  roll back to the previous SHA on failure. Record the bound runner onto
  `ProjectWebServiceState.HostDeviceID` after first provision.
- Store: migration adding `host_device_id` to `project_web_service_state`.
- Docs (`docs/`): document `HELIX_WEB_SERVICE_DATA_DIR`, the single-writer
  constraint (one DB instance), the brief-downtime deploy model, and the
  single-runner-loss caveat.

## Testing
- Unit: `buildMounts` includes the `/data` bind only for `Purpose=web-service`,
  keyed by project ID (stable across sandbox restarts).
- Unit/controller: a redeploy reuses the same sandbox (same ID) rather than
  creating a new one; the old app is stopped before the new one starts (assert
  ordering so two instances never overlap).
- Controller: failed readiness rolls back to the previous SHA; the service
  stays up on intact `/data`.
- Controller: deploy when the pinned runner is offline fails loudly and leaves
  data untouched.
- Integration: deploy an app that writes a row to Postgres/SQLite under `/data`,
  redeploy, confirm the row is still present and only one DB process ran.
