# One rootless Docker engine for SaaS sandboxes

Date: 2026-10-15
Status: proposed implementation, not released; last successful SaaS check was Birding-only

## Decision proposed

Use one image-owned rootless Docker engine for every SaaS sandbox that needs
an inner engine. The proposed implementation covers
external-agent headless and Helix-in-Helix sandboxes, Ubuntu/GNOME desktops,
Sandbox API Ubuntu, and persistent web services. Sway remains gated until its
live image and display tests pass.

The target has no Podman. `dockerd-rootless.sh` and RootlessKit run inside the
existing sandbox lifecycle. This is one selected engine for a sandbox, not an
extra engine, sidecar, or supervisor beside Podman.

The outer Docker used by Hydra to launch and manage sandbox containers remains
runner infrastructure. It is not a tenant's inner engine and is not removed by
this proposal.

The existing flags and allowlist semantics are unchanged:

```text
HELIX_SANDBOX_DESKTOP_ROOTLESS=<true|false>
HELIX_SANDBOX_DESKTOP_ROOTLESS_PROJECT_IDS=<comma-separated project IDs>
```

With the master flag on and no non-empty allowlist, desktop and web-service
projects use rootless Docker. With a non-empty allowlist, only listed projects
use rootless Docker. The flag-off path continues to use rootful Docker. In the
proposed implementation, headless and Helix-in-Helix engine sessions also select
rootless Docker through their existing engine selection; they do not need a new
flag. Released SaaS still uses rootless Podman for those sessions until this
code is released.

The last successful SaaS check found rootless desktop mode off with only
Birding allowlisted. The API was recreated and verified after that rollback.
Existing sandboxes were not recreated. SSH read-only verification from this
environment is currently unavailable, so the live flag state is not confirmed.
Birding still depends on the current rootless Podman path until this
implementation is released and Birding is replaced.

## Current and target architectures

| Sandbox type | Current inner engine | Target inner engine |
| --- | --- | --- |
| External-agent headless / Helix-in-Helix | Rootless Podman | Rootless Docker |
| Ubuntu/GNOME desktop | Rootful Docker by default; Podman when rootless is enabled | Rootless Docker |
| Sway desktop | Rootful Docker; rootless Sway is rejected today | Rootless Docker after display gates pass |
| Sandbox API Ubuntu | Rootful Docker unless its rootless policy is selected | Rootless Docker |
| Persistent web service | FindAI is old rootful; new allowlisted services use Podman | Rootless Docker |
| Engine-free org bot | No inner engine | No inner engine |
| Generic headless/custom runtime | No Helix-managed inner engine | No Helix-managed inner engine |

```text
Current

Runner infrastructure: outer Hydra Docker
  +-- FindAI web service: privileged outer sandbox -> rootful Docker
  +-- Birding web service: unprivileged outer sandbox -> rootless Podman
  +-- rootless Ubuntu desktop where enabled -> rootless Podman
  +-- Sway desktop -> rootful Docker
  +-- headless/H-in-H where enabled -> rootless Podman
  +-- engine-free bot -> no inner engine
```

```text
Target, after migration

Runner infrastructure: outer Hydra Docker
  +-- every engine-bearing sandbox: unprivileged outer sandbox
        +-- dockerd-rootless.sh + RootlessKit
              +-- BuildKit / Buildx
              +-- headless or H-in-H containers
              +-- Ubuntu/GNOME or Sway desktop
              +-- Sandbox API or web-service containers
  +-- engine-free bot: no inner engine
  +-- generic headless/custom runtime: no Helix-managed inner engine
```

## What exists today

The proposed implementation changes the rootless branch in
`desktop/shared/17-start-dockerd.sh` to start image-owned
`dockerd-rootless.sh` as `retro`. It exposes
`/run/user/1000/docker.sock`, stores data under
`/home/retro/.local/share/docker`, and uses Docker's default Buildx builder.
The released SaaS image still starts the old Podman branch. The proposed
implementation keeps old Podman socket, storage, and BuildKit references only in
recovery and cache cleanup code so existing Podman sessions can drain safely.

Hydra's current rootless desktop policy limits capabilities and devices,
requires `/dev/fuse` and `/dev/net/tun`, and does not pass the host Docker
socket. The Ubuntu desktop path has display, input, GPU, D-Bus, PipeWire, and
sudo requirements. Sway is currently rootful because the rootless desktop
validation does not support it.

Persistent web services receive `restart=unless-stopped` on the outer sandbox
when Hydra creates them. The web-service controller also has a separate
best-effort update for inner containers after deployment.

## Why FindAI exposed a gap

FindAI uses Compose. PostgreSQL has a health check and the app waits for
`service_healthy`. In the rootless Podman rehearsal, PostgreSQL accepted
connections but its status stayed `starting`; the app remained `Created`.
Running `podman healthcheck run` manually made the app start.

Podman 5.4.2 creates its automatic health-check timers only when real systemd
is available. The SaaS sandbox has a custom PID 1. See the [Podman health-check source](https://github.com/containers/podman/blob/v5.4.2/libpod/healthcheck_linux.go#L20-L58)
and [systemd gate](https://github.com/containers/podman/blob/v5.4.2/libpod/healthcheck_linux.go#L159-L174).

Birding came up because it runs direct `npm` startup and has no Compose
database health gate. The Docker proof covered only FindAI's web service. It
did not prove any desktop, headless, Sway, or nested Helix path.

## Proposed engine contract

```text
Existing Hydra lifecycle
        |
        +-- create unprivileged sandbox
        +-- start image-owned dockerd-rootless.sh
        +-- RootlessKit creates the user namespace
        +-- wait for the user-owned Docker socket
        +-- Hydra uses the Docker-compatible API
        +-- stop and clean the same engine on shutdown
```

The image must contain a supported Docker rootless installation. This is not
an ad hoc production copy of binaries from the disposable PoC. The startup
path must provide the socket and API operations used by Hydra, Compose,
deploy readiness, logs, exec, events, restart, and recovery.

## Migration scope

### Startup, socket, BuildKit, and Buildx

The proposed implementation replaces the Podman startup branch with rootless Docker
while preserving the `retro` user boundary, RootlessKit setup, socket
readiness, environment handoff, Buildx, and clean shutdown. Rootless Docker's
documented model runs both the daemon and its containers in a user namespace. See the
[Docker rootless documentation](https://docs.docker.com/engine/security/rootless/)
and [rootless tips](https://docs.docker.com/engine/security/rootless/tips/).

### Storage and golden caches

Podman and Docker storage formats are incompatible. The implementation uses a
separate Docker per-session engine directory and cache identity:
`rootless-docker-data-<session>` mounted at
`/home/retro/.local/share/docker`, with the `rootless-docker` golden and ZFS
cache kind. It must not mount `/home/retro/.local/share/containers` as Docker
storage or reuse a Podman golden or ZFS cache.

The migration covers the new volume identity, image and BuildKit caches,
golden-cache promotion, stale-runtime archives, restart cleanup, and session
deletion. Old Podman cache and recovery identities remain until all live
Podman sessions are drained. Persistent workspace and `/data` mounts are
customer data and must not be removed with engine storage.

### Security, devices, and desktop behavior

Every engine-bearing target must pass:

- `Privileged=false`, no host Docker socket, and no broad `/dev` access;
- negative tests for host block devices, sibling storage, fabricated devices,
  `--privileged` child containers, and cloud metadata access;
- rootless child user namespaces and network isolation;
- the required fuse and tun devices for the engine;
- GPU, display, input, D-Bus, PipeWire, and sudo behavior for Ubuntu/GNOME;
- equivalent display and input behavior for Sway; and
- no loss of Hydra's resource and cleanup boundaries.

Prime exposed one extra security decision. The host's `docker-default` AppArmor
profile denied RootlessKit's mount setup. The sandbox image now includes a
separate `helix-rootless-docker` profile and loads it before Hydra starts when
AppArmor is enabled. Hydra selects this profile only for rootless engine
containers. The profile is based on Docker's default profile and keeps its
proc, sys, firmware, powercap, and AppArmor securityfs restrictions.

The profile makes the smallest changes needed by RootlessKit and runc:

- allow mount, unmount, and `pivot_root` operations needed inside the user
  namespace;
- leave namespaced network sysctls writable, including the detached procfs
  network sysctls exposed below `/sys/net` by the runc version in the image;
- keep the Docker default restrictions for the remaining `/proc` and `/sys`
  paths, firmware, powercap, and securityfs; and
- retain `Privileged=false`, the existing capability drops including
  `SYS_ADMIN`, `NET_ADMIN`, and `MKNOD`, and only the limited engine devices.

The profile also explicitly denies reads and writes to Docker's default masked
paths: `/proc/asound`, `/proc/acpi`, `/proc/scsi`, `/proc/keys`,
`/proc/latency_stats`, `/proc/timer_list`, `/proc/timer_stats`, and
`/proc/sched_debug`, alongside the existing `/proc/kcore` deny. The
`/sys/firmware` and `/sys/devices/virtual/powercap` denies include the
directories themselves and their contents. Prime enforcement verified that
reading `/proc/keys` returns `EACCES`.

The rootless branch still uses `seccomp=unconfined` because the tested
RootlessKit setup needs those syscalls. A four-case Prime matrix tested Docker's
default `MaskedPaths` and `ReadonlyPaths` settings: leaving both defaults in
place failed, clearing either one alone still failed to mount proc, and only
clearing both allowed the rootless daemon to start. Hydra therefore clears both
lists for rootless engine containers. Those Docker path defaults are replaced
by the confined AppArmor profile and the existing capability and device
boundary. This is a smaller exception than `apparmor=unconfined`, but it still
needs explicit security review before release.

Prime validation loaded the profile in enforce mode and passed rootless Docker,
hello-world, Buildx, Compose health gates, and a published HTTP port. Host
mount, proc sysctl, and nested privileged mount attempts remained blocked.

### Headless and nested Helix

External-agent headless sandboxes and Helix-in-Helix must run the same
rootless Docker engine without desktop-only devices. They must build and run
nested Compose workloads, reach the intended API proxy, and retain project
isolation.

The existing Podman probes showed that nested cgroups and network policy
cannot be assumed from a working engine socket. Docker must be tested for CPU,
memory, pids, network denial, nested user namespaces, and H-in-H startup.

Engine-free org bots remain engine-free. Generic headless and custom runtimes
must not receive an engine merely because Docker is available in the image.

### Restart and recovery

Test sandbox reboot, engine restart, child restart, stale-runtime cleanup,
image-cache reuse, and Hydra recovery for every engine-bearing type. The outer
persistent sandbox restart policy is set at creation. The inner engine must
return its socket before Hydra or the web-service health monitor acts, and
recovery must not open two database writers against one data directory.

## Removing Podman after parity

There is no permanent dual-engine target. Podman remains only during the
transition so current Birding and existing rootless sessions can keep running.
The proposed implementation deliberately retains Podman cache and recovery paths for
that transition.

Removal is a later phase, after Docker parity is proven and all live Podman
sessions have been drained or deliberately replaced:

1. Replace the rootless Podman path for every engine-bearing type and verify
   new sessions, restarts, recovery, and cache cleanup.
2. Drain or replace Birding and every other live rootless Podman session.
3. After the live drain, remove the legacy Podman cache and recovery detection
   paths, including the Podman golden-cache kind, ZFS/cache migration code,
   stale-runtime cleanup paths, and their tests. The proposed implementation already
   removes the Podman startup branch and package from new images.
4. Retire old Podman data only after live-session inventory, backup checks,
   and rollback review confirm that no tenant depends on it.
5. Keep `HELIX_SANDBOX_DESKTOP_ROOTLESS` and
   `HELIX_SANDBOX_DESKTOP_ROOTLESS_PROJECT_IDS` as the rootless Docker rollout
   controls. The allowlist is not removed by this migration.

This phase must not begin merely because a Docker test container starts.

## FindAI data cutover

The engine change does not make project data disposable. FindAI's startup
sets `PGDATA` below `HELIX_WEB_SERVICE_DATA_DIR`, which defaults to `/data`,
creates a named volume backed by that directory, and runs production Compose.
The current PostgreSQL path is fixed at `/data/postgres`.

```text
/data
  +-- postgres                 durable project data
  +-- uploads and other files  durable project data

Rootless Docker storage        images, networks, containers, cache
```

For a FindAI replacement:

1. Take fresh logical, physical, and non-PostgreSQL `/data` backups. Copy
   them off the runner and verify checksums and restores. An online copy is
   rehearsal only.
2. Record app and specs revisions, sandbox ID, runner, and public domains.
3. Stop or quiesce the app and database writer. Take the final consistent copy
   only after writes stop.
4. Restore it under `/data` in a staging directory. Test rootless Docker
   PostgreSQL and remap ownership there if needed. Preserve the original
   `/data/postgres` directory for rollback.
5. Replace the sandbox while no old writer runs. Perform a controlled atomic
   path swap so the staged directory becomes `/data/postgres`; the current
   startup script does not select an arbitrary Compose data path.
6. Start the new sandbox and verify database access, domains, app readiness,
   health monitoring, restart behavior, and the next release.

Never run two PostgreSQL containers against one directory. If the new service
accepts writes, rollback is a data migration: stop all writers, preserve the
new directory, and restore the agreed backup or perform a reviewed forward
migration before starting the old service.

## Verification completed locally and on Prime

The exact rootless Docker image was built in an isolated Prime validation
environment. The
following checks passed locally and on Prime:

- `hello-world` through the rootless Docker socket;
- Docker Buildx;
- a Compose health-gate stack; and
- a published port check.

Focused Go tests also pass. Prime enforced the outer cgroup limits of 4 GiB
memory, 2 CPUs, and 512 pids. The inner rootless Docker daemon reported
`CgroupDriver=none`; nested `--cpus`, `--memory`, and `--pids` flags were
ignored without cgroup delegation or systemd, as described in Docker's
[rootless tips](https://docs.docker.com/engine/security/rootless/tips/). This
matches the old Podman limitation. GPU and live desktop streaming, full
headless/H-in-H behavior, ZFS-backed storage, SaaS rollout, and live Birding
and FindAI replacements remain untested.

Restarting the disposable outer container on Prime restored rootless Docker,
the existing inner web container with `restart=unless-stopped`, its published
HTTP response, and the previously built image cache. Docker logged restore
warnings, but the service was up in 6 seconds. This does not verify a host
reboot or SaaS recovery.

## Acceptance gates

Before any production migration:

1. Build the image-owned rootless Docker path. Do not use ad hoc PoC binaries.
2. Load and verify the confined `helix-rootless-docker` AppArmor profile on
   every runner that can create rootless engine containers. Complete explicit
   security review of its mount, `pivot_root`, network-sysctl, and
   `seccomp=unconfined` exceptions.
3. Test headless and Helix-in-Helix startup, BuildKit, nested Compose,
   cgroups, network denial, storage isolation, reboot, and recovery. Verify
   that outer Hydra limits still enforce CPU, memory, and pids bounds when the
   inner rootless daemon reports `cgroup driver=none`.
4. Test Ubuntu/GNOME and Sway desktops, including GPU, display, input,
   D-Bus, PipeWire, sudo, and desktop recovery.
5. Test Sandbox API Ubuntu and persistent web-service startup, including
   Compose health checks, direct `npm`, retries, failures, and no-health-check
   stacks.
6. Verify Hydra operations, socket readiness, restart policies, cache cleanup,
   and public readiness through the production API path.
7. Use an internally owned or explicitly authorized canary. Nocknock remains
   out of scope without its owner's permission.
8. Take and verify final backups immediately before any live replacement.
9. Replace only the canary sandbox, preserve `/data/postgres`, and verify
   database, domains, health monitor, and rollback before expanding scope.

If any gate fails, keep the current Podman path and Birding-only allowlist. Do
not flip the SaaS flag globally or silently start both engines.

## Status and limits

The last successful SaaS check found global rootless off with only Birding
allowlisted; existing sandboxes were not recreated. FindAI remains on its old
privileged rootful sandbox. SSH read-only verification from this environment is
currently unavailable, so the live flag state is not confirmed. The local and
Prime Docker smoke tests, focused Go tests, outer cgroup enforcement, confined
AppArmor profile load, and security negative tests pass. The profile is
host-global, so an API-first release can create a
rootless container before the runner image has loaded the profile; that launch
will fail until the runner is upgraded. GPU and live desktop streaming, full
headless/H-in-H behavior, ZFS, SaaS rollout, and live Birding and FindAI
replacements remain unproven. The inner daemon still reports
`CgroupDriver=none`, so per-child CPU, memory, and pids limits are not enforced
inside the rootless daemon. Sway rootless is still rejected by validation.
Legacy Podman named volumes also need inventory and migration before replacing
services that store data there. Podman cleanup is explicitly deferred until
parity and live-session drain are complete.

Isolated Prime validation used uniquely tagged images and disposable
containers; the shared Prime Helix stack and production/live customer
sandboxes were not changed.
