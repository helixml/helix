# Desktop root isolation — closing "full desktop allows root into the SaaS runner"

**Date:** 2026-10-08
**Status:** investigation complete + first hardening slice implemented behind a
default-off flag. The flag's enablement is gated on a desktop-image rebuild and
a live GPU-stream validation that this change does **not** yet carry out — see
"Validation gate". Nothing here flips production behaviour on its own.

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
- **No `apparmor=unconfined`** — Docker's default AppArmor profile applies,
  which denies `mount` outright. This is the primary reason even a process
  holding `CAP_SYS_ADMIN` cannot mount a sibling zvol or the host root fs.
- Capabilities: `CapDrop` the host-level set
  (`SYS_NICE,SYS_PTRACE,NET_RAW,MKNOD,NET_ADMIN`); `CapAdd` only `SYS_ADMIN`,
  which rootless Podman needs to create its subordinate user namespace. Not
  the full privileged set.
- **Device allow-list only.** The device cgroup gets exactly: GPU
  (`/dev/dri/card*`, `/dev/dri/renderD*`, NVIDIA nodes via `configureGPU`),
  input (`hidraw`/`input` majors via `getDeviceCgroupRules`, plus `/dev/uinput`
  and `/dev/input/event*` via the GOW `GOW_REQUIRED_DEVICES` path), and
  `/dev/fuse` + `/dev/net/tun` for the rootless engine. **No** block devices —
  no `/dev/nvme*`, `/dev/zd*`, `/dev/mem`, `/dev/zfs`, `/dev/kmsg`. There is
  nothing host-side left to mount even where AppArmor is absent.
- `IpcMode: private`, a private IPC namespace (unchanged from today's desktop).
- `seccomp=unconfined` is retained for the rootless-engine posture only
  (Podman needs syscalls the default profile blocks); this matches the
  already-shipping headless rootless path. seccomp is the secondary control;
  the device allow-list + AppArmor mount denial are the load-bearing ones.

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

Image side (coded, **not yet live-validated** — see gate): when the desktop
runs rootless, the entrypoint must start rootless Podman (the existing
`HELIX_ROOTLESS_CONTAINER_ENGINE=1` branch in `17-start-dockerd.sh`) **without**
taking the headless `setpriv --nnp` privilege-drop branch in
`Dockerfile.ubuntu-helix`, so sudo survives (decision 2). The drop branch is
therefore gated to the headless path explicitly.

## 4. What desktop users can and cannot reach (documentation)

With `HELIX_SANDBOX_DESKTOP_ROOTLESS=true`:

**Can:** be root via sudo inside the container; `apt install`; run containers
through the inner rootless Podman/BuildKit; use the GPU, keyboard/mouse input
and the display; reach the public internet and the Helix API proxy per the
egress policy; read/write their own `/home/retro/work` and workspace volumes.

**Cannot:** open or mount any host block device (runner root fs, other tenants'
`docker-data` zvols); `mount` at all (AppArmor); write `/dev/mem` / load kernel
modules; see the full host capability set; reach sibling sandboxes or the
runner host's sockets/credentials.

## Validation gate (before the flag is turned on anywhere)

This change is safe with the flag off and ships that way. Turning it on
requires, and is explicitly **not** done by this change:

1. `./stack build-ubuntu` (desktop image carries the entrypoint gating).
2. Live desktop session with the flag on: GNOME + Zed start, the stream
   renders a frame, GPU is visible inside, sudo works, inner rootless Podman
   builds and runs an image.
3. Negative checks from inside the hardened desktop (each must fail): enumerate
   `/dev` for `zd*`/`nvme*`/`mem` (absent), `mount /dev/<host-dev> /mnt`
   (denied), read `/proc/self/attr/current` (an AppArmor profile, not
   `unconfined`), `CapBnd` reduced.

Until that matrix passes on a rebuilt image, `HELIX_SANDBOX_DESKTOP_ROOTLESS`
stays false. The Go layer is the enforced boundary and is covered by unit tests
in `api/pkg/hydra/devcontainer_test.go`.
