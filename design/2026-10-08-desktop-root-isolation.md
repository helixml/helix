# Desktop root isolation — closing "full desktop allows root into the SaaS runner"

**Date:** 2026-10-08
**Status:** investigation complete + first hardening slice implemented behind a
default-off flag, and **validated live** on the inner Helix (2026-10-08): a
rootless `ubuntu-desktop` sandbox boots under the hardened HostConfig and the
escalation paths are gone. See "Live validation". Nothing here flips production
behaviour on its own (flag defaults off).

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
  which rootless Podman needs to create its subordinate user namespace. The
  effective bounding set drops from the full privileged 40 caps to 13 —
  critically **`SYS_MODULE`, `SYS_RAWIO`, `MKNOD` and `NET_ADMIN` are gone**.
  So `SYS_ADMIN`'s residual mount power has no host block device to target
  (device allow-list), raw disk I/O is denied (`SYS_RAWIO`), a device node
  cannot be fabricated to reach one (`MKNOD`), and the kernel cannot be
  modified (`SYS_MODULE`). These two controls together are what make the mode
  non-escalating, and they are portable — they do not depend on the host LSM.
- **AppArmor is a bonus, not the boundary.** On a runner with AppArmor
  enabled, dropping `apparmor=unconfined` lets Docker's default profile deny
  `mount` as well. But a nested runner (helix-sandbox) exposes no AppArmor to
  delegate, so the container shows `runc (unconfined)` there — exactly as the
  privileged desktop already did. The design therefore does **not** rely on
  AppArmor; the device allow-list and capability drops stand alone.
- `IpcMode: private`, a private IPC namespace (unchanged from today's desktop).
- `seccomp=unconfined` is retained for the rootless-engine posture only
  (Podman needs syscalls the default profile blocks); matches the
  already-shipping headless rootless path.

Net effect: the desktop keeps GPU, input, its own inner container engine
(rootless Podman) and sudo, while losing every path to the runner host and to
sibling tenants.

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
  (`DesktopRootless` + `Privileged`, `DesktopRootless` on a headless type).
- `config.Sandboxes.DesktopRootless` (`HELIX_SANDBOX_DESKTOP_ROOTLESS`, default
  **false**) plumbed through the runtime registry → `provision()` for
  user-facing desktops, and through `HydraExecutorConfig` →
  `externalAgentIsolation()` for spec-task desktops. When the flag is off,
  desktops are byte-for-byte what they are today.

Image side (validated live, see below): a rootless desktop sets
`HELIX_ROOTLESS_CONTAINER_ENGINE=1` **and** `HELIX_DESKTOP_ROOTLESS=1`
(`buildEnv`). The `17-start-dockerd.sh` rootless-Podman branch runs as before,
but three desktop-image paths are gated so the full GNOME-as-root session with
sudo survives rather than taking the headless privilege-drop path:

- the `Dockerfile.ubuntu-helix` entrypoint `setpriv --nnp` drop branch,
- the `startup-app.sh` zed-symlink FATAL (a rootless desktop has sudo and
  creates the symlink itself) and the workspace `chown`,
- the `99-startdbus.sh` `setpriv` dbus branch.

The rootless engine's storage volume is mounted at
`/home/retro/.local/share/containers` (Podman), not `/var/lib/docker`, in both
`sandbox/controller_provision.go` and `external-agent/hydra_executor.go`
buildMounts. The CPU-tier setup (`16-cpu-tiers.sh`) self-skips in an
unprivileged container (cgroup2 is read-only), so a rootless desktop does not
get the display/agent CPU prioritisation — an accepted tradeoff for now.

## 4. What desktop users can and cannot reach (documentation)

With `HELIX_SANDBOX_DESKTOP_ROOTLESS=true`:

**Can:** be root via sudo inside the container; `apt install`; run containers
through the inner rootless Podman/BuildKit; use the GPU, keyboard/mouse input
and the display; reach the public internet and the Helix API proxy per the
egress policy; read/write their own `/home/retro/work` and workspace volumes.

**Cannot:** open any host block device (none are in the device cgroup —
runner root fs, other tenants' `docker-data` zvols); fabricate a device node to
reach one (`MKNOD` dropped); raw disk I/O (`SYS_RAWIO` dropped); write
`/dev/mem` or load kernel modules (`SYS_MODULE` dropped); see the full host
capability set; reach sibling sandboxes or the runner host's
sockets/credentials.

## Live validation (2026-10-08, inner Helix)

Built the desktop image with the gating above, redeployed hydra with the
`buildHostConfig` change, set `HELIX_SANDBOX_DESKTOP_ROOTLESS=true`, and created
an `ubuntu-desktop` sandbox. Observed:

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
- **Not covered:** the sandbox-API screenshot endpoint returned 503 ("desktop
  bridge not connected") — a pre-existing sandbox-API-desktop limitation
  (CLAUDE.md flags sandbox-API desktops as not wired for streaming),
  independent of this change: the compositor and GPU stack are demonstrably
  running. Full GPU-stream frame capture should be re-confirmed on the
  **spec-task** desktop path (which wires the desktop-bridge) before enabling
  the flag in production, along with an `apt install` through the public egress
  and a build on the inner rootless engine.

The flag ships **false**. With it off, desktops are byte-for-byte unchanged
(verified: `TestBuildHostConfigPrivilegedDesktopUnchanged`,
`TestProvisionDesktopBuildsFullEnvAndMounts`). The Go boundary is covered by
unit tests in `api/pkg/hydra/devcontainer_test.go` and
`api/pkg/sandbox/controller_*_test.go`.
