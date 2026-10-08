# Desktop root isolation — closing "full desktop allows root into the SaaS runner"

**Date:** 2026-10-08
**Status:** investigation complete + first hardening slice implemented behind a
default-off flag. The final image `dd5b27` was validated on Prime in a
disposable `ubuntu-desktop`; the Go HostConfig and the image capability drops
were exercised live. Nothing here flips production behaviour on its own (flag
defaults off).

**Why this doc exists:** it is the first written record of the finding. Priya
Samuel raised it (Oct 6, relayed from a Slack exchange outside the indexed
channels); Karolis Rusenas named the core: "the main one is full desktop
allowing root into the saas runner ofc". A sweep of all 51 accessible channels
(Aug 1 → Oct 7, full history + threads) found no prior written discussion. The
standalone evidence digest is in `#security-n-privacy` (ts 1791410945.007319).

**Related trust-boundary work:**
`design/2026-08-30-sandbox-egress-hardening.md` (network side of the same
boundary), `design/2026-09-24-untrusted-bot-mode.md` §"Why removing tools is
not enough" item 6 ("Desktop containers are privileged"),
`design/2026-09-27-org-bot-instance-isolation.md`,
`design/2026-10-07-api-key-list-secrets.md` (cross-org metadata disclosure —
adjacent, same "what a sandboxed tenant can see" family).

## 1. The gap, root-caused

Desktop runtimes are launched with Docker `--privileged`:

- User-facing sandboxes: `builtinDesktop.Privileged = true`
  (`api/pkg/sandbox/runtimes.go`), carried into the create request in
  `controller_provision.go`.
- Spec-task / agent desktops: `externalAgentIsolation()` returns
  `{privileged: true}` for any non-headless container
  (`api/pkg/external-agent/hydra_executor.go`).

Both land in `DevContainerManager.buildHostConfig`
(`api/pkg/hydra/devcontainer.go`), where `req.Privileged` sets:

```go
hostConfig.Privileged = true
hostConfig.SecurityOpt = []string{"seccomp=unconfined", "apparmor=unconfined"}
```

Docker `--privileged` additionally grants the **full capability set**, an
**allow-all device cgroup**, and bind access to **every host device node**. The
desktop image then runs the session as root with passwordless sudo
(`Dockerfile.ubuntu-helix`: `retro ALL=(ALL) NOPASSWD:ALL`).

### What root inside a desktop can actually reach (verified live)

Probed read-only from inside a running `ubuntu-desktop` sandbox on 2026-10-08:

| Check | Result |
|---|---|
| `sudo -n true` | succeeds (passwordless root) |
| `CapBnd` | `000001ffffffffff` — every capability |
| `Seccomp` / `NoNewPrivs` in `/proc/self/status` | `0` / `0` (unconfined) |
| `/proc/self/attr/current` | `runc (unconfined)` — AppArmor off |
| `/var/run/docker.sock` | present (`root:docker`, inner dockerd) |
| Host block devices in `/dev` | `/dev/nvme0n1p1` (runner root fs), `/dev/mem`, `/dev/kmsg`, `/dev/zfs`, and **125 `/dev/zd*` ZFS zvols** |

The `/dev/zd*` nodes are the per-sandbox `docker-data-<id>` zvols of **other
tenants on the same runner**. With `--privileged` the device cgroup permits
opening them and AppArmor is disabled, so a root desktop user can:

- `mount /dev/nvme0n1p1 /mnt` → read the runner host's root filesystem: its
  credentials, Hydra's local registry, every container's image layers.
- `mount /dev/zdN /mnt` → read a sibling tenant's workspace / docker data.
- write `/dev/mem` or load a kernel module (`CAP_SYS_MODULE`) → own the kernel
  → own the runner host → own all tenants.

This is exactly "full desktop allowing root into the SaaS runner": the desktop
container is not a tenant boundary at all, it is host-root with extra steps.

### Why headless is not affected

Headless agents already run unprivileged (`RootlessContainerEngine`: rootless
Podman, `CapAdd` only `SYS_ADMIN` for the trusted init, Docker-default
AppArmor, an explicit two-device allow-list of `/dev/fuse` + `/dev/net/tun`).
Org bot instances add `no_new_privs`. Desktops are the **sole** remaining
privileged runtime — the headless path is the template this design copies.

## 2. The isolation boundary

Goal: root-inside-a-desktop must be non-escalating — it may be root within an
unprivileged container, but must not reach runner-host resources or sibling
tenants. Decisions (confirmed with the operator 2026-10-08):

1. **Engine:** reuse the existing rootless Podman path. No new container
   runtime, no new daemon (CLAUDE.md forbids new sidecars without approval).
2. **Sudo:** kept, contained. Root inside an unprivileged container is fine;
   `apt install` still works. The container no longer carries `--privileged`.

The hardened desktop HostConfig (produced by `buildHostConfig` when the new
`DesktopRootless` flag is set) is:

- `Privileged: false`.
- **Device allow-list — the load-bearing control.** The device cgroup gets
  exactly: GPU (`/dev/dri/card*`, `/dev/dri/renderD*`, NVIDIA nodes via
  `configureGPU`), input (`hidraw`/`input` majors via `getDeviceCgroupRules`,
  plus `/dev/uinput` and `/dev/input/event*` via the GOW
  `GOW_REQUIRED_DEVICES` path), and `/dev/fuse` + `/dev/net/tun` for the
  rootless engine. **No** block devices — no `/dev/nvme*`, `/dev/zd*`,
  `/dev/mem`, `/dev/zfs`, `/dev/kmsg`. There is simply nothing host-side left
  to open or mount.
- **Capabilities.** `CapDrop` the host-level set
  (`SYS_NICE,SYS_PTRACE,NET_RAW,MKNOD,NET_ADMIN`); `CapAdd` only `SYS_ADMIN`,
  which rootless Podman needs during trusted init to create its subordinate
  user namespace. The desktop image then uses `setpriv --bounding-set=-sys_admin
  --` at the user startup and explicit command handoff, so the desktop session
  does not retain `SYS_ADMIN`. The remaining capabilities critically exclude
  **`SYS_MODULE`, `SYS_RAWIO`, `MKNOD` and `NET_ADMIN`**. Raw disk I/O is denied
  (`SYS_RAWIO`), a device node cannot be fabricated to reach one (`MKNOD`), and
  the kernel cannot be modified (`SYS_MODULE`). These controls close the
  direct host-block-device path without depending on a host LSM. The residual
  boundary of the rootless engine processes still needs review.
- **AppArmor is not available on the Prime nested runner.** The container
  shows `runc (unconfined)` there, so the rootless boundary must not claim that
  AppArmor denies `mount`. A pre-fix rootless desktop could mount a container
  local `tmpfs` even while `mknod` was denied and host block devices were
  absent. The boundary is the device allow-list and capability drops: there is
  no host block device to mount, and the session loses `SYS_ADMIN` at handoff.
- `IpcMode: private`, a private IPC namespace (unchanged from today's desktop).
- `seccomp=unconfined` is retained for the rootless-engine posture only
  (Podman needs syscalls the default profile blocks); matches the
  already-shipping headless rootless path.
- **Engine process boundary.** The long-running Podman API service runs inside
  Podman's subordinate user namespace. Dropping host `SYS_ADMIN` before that
  namespace exists prevents `newuidmap` from creating it. On Prime, the outer
  Podman parent had `CapEff=0` in the initial UID map; the service child ran in
  its subordinate map (`0 -> 1000`, `1 -> 100000/65536`). BuildKit showed a
  similar residual namespace boundary and retains its existing RootlessKit
  setup.

Net effect: the tested HostConfig closes the direct host-block-device path while
keeping GPU, input, the rootless Podman engine and sudo. It does not yet prove
that every rootless engine process path is non-escalating.

### Credential / network domain (ties into cross-org disclosure)

Unchanged by this slice but recorded as the standing boundary: desktops reach
the control plane only through Hydra's fixed API proxy
(`helix-api.internal:18080`), on the isolated `helix-sandboxes` bridge, with
private ranges rejected (egress hardening doc). The ephemeral sandbox API key
minted per desktop (`ensureSandboxAPIToken`) is the only credential in the
container and is revoked on delete. Reducing what that key can see org-wide is
tracked separately under the cross-org metadata disclosure finding.

## 3. Implementation (this change)

The durable enforcement boundary is in Go and is fully unit-tested here:

- `CreateDevContainerRequest.DesktopRootless` (new, `api/pkg/hydra/types.go`).
- `buildHostConfig` treats a desktop with `DesktopRootless` as the unprivileged
  rootless-engine posture above, and rejects the illegal combinations
  (`DesktopRootless` + `Privileged`, non-Ubuntu container types, and golden
  builds). The external-agent launch path rejects unsupported desktop types,
  custom images, and golden builds before provisioning. Headless agents and
  no-container-engine browser sandboxes keep their existing handling because
  they do not use the desktop rootless engine path.
- `config.Sandboxes.DesktopRootless` (`HELIX_SANDBOX_DESKTOP_ROOTLESS`, default
  **false**) plumbed through the runtime registry → `provision()` for
  user-facing desktops, and through `HydraExecutorConfig` →
  `externalAgentIsolation()` for spec-task desktops. When the flag is off,
  Hydra forces `HELIX_DESKTOP_ROOTLESS=0` after removing duplicate caller
  values, so project or image environment cannot select the desktop rootless
  startup path.

Image side (validated with Prime image `dd5b27`): a rootless desktop sets
`HELIX_ROOTLESS_CONTAINER_ENGINE=1` **and** `HELIX_DESKTOP_ROOTLESS=1`
(`buildEnv`). The `17-start-dockerd.sh` rootless-Podman branch is extended, and
the desktop image has four relevant paths so the full GNOME session runs as
`retro` via `gosu` with sudo available rather than taking the headless
privilege-drop path:

- the `Dockerfile.ubuntu-helix` entrypoint user startup and explicit CMD
  handoff now drop `SYS_ADMIN` without `no_new_privs`,
- the `startup-app.sh` zed-symlink FATAL (a rootless desktop has sudo and
  creates the symlink itself) and the workspace `chown`,
- the `99-startdbus.sh` and virtio scanout `17-scanout-setup.sh` system D-Bus
  branches. Headless rootless sessions retain the `SYS_ADMIN` drop with
  `no_new_privs`; rootless desktops drop `SYS_ADMIN` without `no_new_privs` so
  the GNOME session keeps sudo.
- the long-running Podman API service in `17-start-dockerd.sh`, which enters a
  subordinate user namespace before exposing its socket. The RootlessKit
  preflight and BuildKit launch retain their existing rootless boundary.

Flag-off desktops keep their existing startup behavior. The virtio scanout
D-Bus change is gated by the desktop rootless flag.

The rootless engine's storage volume is mounted at
`/home/retro/.local/share/containers` (Podman), not `/var/lib/docker`, in both
`sandbox/controller_provision.go` and `external-agent/hydra_executor.go`
buildMounts. The CPU-tier setup (`16-cpu-tiers.sh`) self-skips in an
unprivileged container (cgroup2 is read-only), so a rootless desktop does not
get the display/agent CPU prioritisation — an accepted tradeoff for now.

When `CONTAINER_DOCKER_PATH` is configured, Hydra redirects legacy
`/var/lib/docker` volumes by destination and the DesktopRootless Podman volume
by its stable `docker-data-<session>` source name. DesktopRootless sessions use
a separate `podman` data directory and are never seeded from the legacy Docker golden
cache; Docker and Podman storage formats are incompatible. Existing headless
rootless sessions keep their named-volume storage. This gives DesktopRootless
sessions their own persistent storage across restarts without importing
privileged-engine state.

## 4. What desktop users can and cannot reach (documentation)

With `HELIX_SANDBOX_DESKTOP_ROOTLESS=true`:

**Can:** be root via sudo inside the container; `apt install`; run containers
through the inner rootless Podman/BuildKit; use the GPU, keyboard/mouse input
and the display; reach the public internet and the Helix API proxy per the
egress policy; read/write their own `/home/retro/work` and workspace volumes.
The final image denied both container-local `tmpfs` mounts and `mknod`; the
device allow-list and capability drops remain the isolation boundary.

**Cannot via the tested direct path:** open any host block device (none are in the device cgroup -
runner root fs, other tenants' `docker-data` zvols); fabricate a device node to
reach one (`MKNOD` dropped); raw disk I/O (`SYS_RAWIO` dropped); write
`/dev/mem` or load kernel modules (`SYS_MODULE` dropped); or see the full host
capability set. The tested HostConfig has no host Docker socket mounts. The
Podman API service is confined to its subordinate user namespace; BuildKit
retains its existing RootlessKit boundary.

## Live validation (2026-10-08, Prime, image `dd5b27`)

A disposable `ubuntu-desktop` booted GNOME with the hardened HostConfig and
the generated D-Bus runtime check present and passing. The final image smoke
checks recorded:

- PID 1, GNOME, and system D-Bus had `CapBnd=a00405fb`; `SYS_ADMIN` was absent.
- `sudo` remained available; `nvidia-smi` reported **NVIDIA GeForce RTX 4090**.
- Rootless `docker run` succeeded. `mount` and `mknod` were denied.
- `docker inspect` reported `Privileged=false`, no AppArmor profile, and no
  host block devices.
- The sandbox heartbeat advertised image `dd5b27`.
- Three older `9b480c` desktops stayed running. One autonomous desktop was
  restarted onto `dd5b27`.

The Podman parent had `CapEff=0` in the initial UID map. Its API service ran
in the subordinate map (`0 -> 1000`, `1 -> 100000/65536`). BuildKit showed a
similar residual namespace boundary.

The prior image `89fca5` passed `apt update`, Buildx build and run, and Compose
up, ps, and down. The final `dd5b27` image had only the `docker run` smoke
check; those earlier workflow results must not be attributed to `dd5b27`.

Earlier inner-Helix validation (not Prime, and using the earlier image)
recorded the following HostConfig and in-container checks:

- **HostConfig** (`docker inspect`): `Privileged=false`,
  `SecurityOpt=["seccomp=unconfined"]` (no `apparmor=unconfined`),
  `CapAdd=["SYS_ADMIN"]`, `CapDrop=["SYS_NICE","SYS_PTRACE","NET_RAW","MKNOD","NET_ADMIN"]`,
  devices = `fuse, tun, nvidiactl, nvidia-uvm{,-tools}, nvidia-modeset,
  nvidia0, renderD128, card1` — no block devices.
- **Boot:** rootless Podman + BuildKit come up; `gnome-shell --headless
  --virtual-monitor 1920x1080@30` and `pipewire` run; container stays up.
- **In-container escalation checks:**
  - `id` → `uid=0`; `sudo -n id` → root (sudo retained).
  - `sudo mknod /tmp/x b 259 6` → **Operation not permitted**.
  - `ls /dev` for `zd*|nvme*|mem|kmsg|zfs|sd*` → **count 0**.
  - `/var/run/docker.sock`, `/run/docker.sock` → **absent**; no host socket
    mounts.
  - `CapBnd=00000000a02405fb` (13 caps) — decodes without `sys_module`,
    `sys_rawio`, `mknod`, `net_admin`, `sys_ptrace`; contrast the privileged
    desktop's `000001ffffffffff` (all 40).
  - Inner `docker info` → engine rootless, works **without** sudo.
  - GPU usable: `nvidia-smi` reports "NVIDIA RTX 2000 Ada Generation";
    `renderD128` present.
- The sandbox-API screenshot endpoint returned 503 ("desktop bridge not
  connected") — a pre-existing sandbox-API-desktop limitation, covered by the
  spec-task run below.

### Spec-task desktop (hydra baked via `./stack build-sandbox`)

A `ubuntu-desktop` spec task on the forked sample project, flag on:

- HostConfig identical to the above; mounts include
  `/home/retro/.local/share/containers`; env carries
  `HELIX_ROOTLESS_CONTAINER_ENGINE=1`, `HELIX_DESKTOP_ROOTLESS=1` and the Podman
  `DOCKER_HOST`.
- Full desktop: GNOME, Zed with the agent thread, Chrome showing the project's
  dev server started by the project startup script.
- `helix spectask screenshot` works; `helix spectask benchmark --duration 20`:
  1167 frames, 25–61 fps, 97% of target, 0 gaps >50 ms.
- Agent runs as `retro`; `sudo` → root; `sudo apt-get install` succeeds;
  `docker build`/`docker run` on the inner rootless engine succeeds without
  sudo. `CapBnd` 13 caps, zero host block devices, `mknod` denied, no host
  Docker socket.
- `spectask stop` removes the container, the `docker-data-<session>` volume and
  the session's API keys.
- The HelixCursor "Socket not ready (30/30)" log is unrelated: the extension
  only retries its *initial* cursor send, and the desktop-bridge creates the
  socket when the first stream client connects.

### Residual risk: rootless engine process boundary

The final image removed `SYS_ADMIN` from PID 1, GNOME, and system D-Bus, and
the Podman API service ran in its subordinate UID map. BuildKit retains a
similar namespace-specific residual boundary. The engine paths therefore
remain the part requiring deeper validation; the Prime runner exposes no
AppArmor profile, and seccomp remains unconfined for rootless engine support.

The flag ships **false**. With it off, desktop behavior is unchanged, and the
canonical `HELIX_DESKTOP_ROOTLESS=0` prevents caller or image environment from
selecting the rootless desktop path (verified:
`TestBuildHostConfigPrivilegedDesktopUnchanged`,
`TestProvisionDesktopBuildsFullEnvAndMounts`). The Go boundary is covered by
unit tests in `api/pkg/hydra/devcontainer_test.go` and
`api/pkg/sandbox/controller_*_test.go`.

The review fixes are covered by unit tests for source-keyed Podman storage
redirects, isolation from the Docker golden cache, duplicate environment
variable removal, and fail-closed launch validation for unsupported desktop
types, custom images, and golden builds. The ZFS-backed rootless storage path
has not been exercised on a live runner yet.

No normal web-service replacement was tested. The Prime run left existing
`9b480c` desktops in place and restarted one autonomous desktop onto `dd5b27`.

The Prime image validation above predates the review-fix changes in this
section. It therefore does not count as live validation of the new ZFS storage
redirect or the fail-closed launch checks.

## SaaS migration semantics

`HELIX_SANDBOX_DESKTOP_ROOTLESS` is evaluated when a new container HostConfig
is built. Enabling it does not mutate existing desktop sessions or existing
web-service sandboxes; those containers remain privileged. A normal web-service
redeploy reuses its existing sandbox and therefore remains privileged as well.
A true sandbox replacement provisions the new container with the rootless
HostConfig when the flag is enabled. Fresh unsupported launches fail closed
before provisioning rather than falling back to privileged mode. The launch
validation runs after existing-session reuse and runtime resolution, so an
already-running container is not retroactively replaced or hardened. A live
web-service compose replacement has not been tested.
