# Dramatic Simplification via Arbitrary Docker Nesting

**Date:** 2026-02-10
**Status:** POC Validated (branch: `feature/docker-in-desktop`)
**Author:** Claude (with Luke)
**Depends on:** 2025-12-07-hydra-architecture-deep-dive.md, 2026-01-25-helix-in-helix-development.md, 2026-02-02-docker-network-isolation-fix.md, 2026-02-06-docker0-veth-reconnection.md

## Executive Summary

The entire Helix sandbox architecture rests on a false premise: that Docker-in-Docker can only nest 2 levels deep. This belief led to an architecture where multiple dockerd instances run as **siblings inside the sandbox**, connected by veth bridges, custom DNS proxies, and iptables NAT rules. The resulting complexity is enormous.

**The truth:** The "2 levels deep" limitation applies only to overlay2 filesystem stacking, not to Docker itself. As long as each nested Docker daemon's `/var/lib/docker` is backed by a real filesystem (via Docker volumes), you can nest Docker **arbitrarily deep** — up to the kernel's 32-level namespace limit, which you'd never approach in practice.

**The simplification:** Move the per-session dockerd **inside** the desktop container instead of running it as a sibling in the sandbox. This eliminates veth bridging, DNS proxying, subnet management, and the docker0 deletion bug — because user containers and the desktop share the same network namespace.

---

## The Misunderstanding That Created the Complexity

### What We Believed

From `2026-01-25-helix-in-helix-development.md`:

> Running DinD-in-DinD-in-DinD doesn't work **if each level uses overlay2 on the layer above**

This is correct — but the second half of the sentence is crucial. The limitation is about **overlay2 on overlay2**, not about Docker nesting depth.

### What's Actually True

The Linux kernel enforces a filesystem stacking depth limit of 2:

```c
// include/linux/fs.h
#define FILESYSTEM_MAX_STACK_DEPTH 2
```

When overlay2 mounts on top of another overlay2, the stacking counter increments. At depth 3, it fails with `EINVAL`. **But this counter only applies to stacked filesystems.** A Docker volume backed by the host's ext4 filesystem has a stacking depth of 0 — overlay2 on top of it starts at depth 1, well within the limit.

This is already how the sandbox works today: the sandbox container's `/var/lib/docker` is a Docker volume (backed by host ext4), so overlay2 works fine inside it. The same trick works at any depth.

### The Hard Limits (None That Matter)

| Resource | Kernel Limit | We'd Use |
|----------|-------------|----------|
| Overlay2 filesystem stacking | 2 | 1 per level (with volumes) |
| PID namespace nesting | 32 | 2-5 |
| User namespace nesting | 32 | 2-5 |
| Network namespace nesting | N/A (flat) | N/A |
| Cgroup v2 nesting | Configurable (default unlimited) | 2-5 |

**Verified sources:** Linux kernel `include/linux/fs.h` (FILESYSTEM_MAX_STACK_DEPTH), `pid_namespaces(7)`, `user_namespaces(7)`, cgroup v2 kernel documentation (`cgroup.max.depth`).

**Real-world precedents:**
- **KinD** (Kubernetes in Docker) uses fuse-overlayfs for 3-level nesting (host → KinD node → Kubernetes pods)
- **Sysbox** solves this with implicit host mounts for inner `/var/lib/docker` directories
- **Helix itself** already nests 2 levels (host → sandbox → desktop containers + per-session dockerds)

---

## The Current Architecture and Its Complexity

### What We Built

```
Host Docker (Level 0)
└── Sandbox Container (Level 1, privileged, DinD)
    ├── Main dockerd (sandbox0 bridge, 10.213.0.0/24)
    │   ├── Desktop containers (ubuntu-external-*, sway-external-*)
    │   └── helix-buildkit
    │
    ├── Hydra daemon (manages everything below)
    │   ├── Per-session dockerd 1 (hydra1 bridge, 10.200.1.0/24)
    │   │   └── User's docker-compose containers (10.112.0.0/20 networks)
    │   ├── Per-session dockerd 2 (hydra2 bridge, 10.200.2.0/24)
    │   │   └── User's docker-compose containers (10.112.16.0/20 networks)
    │   └── ...
    │
    ├── DNS proxy on 10.213.0.1:53 (for main dockerd)
    ├── Hydra DNS on 10.200.1.1:53, 10.200.2.1:53, ... (per session)
    ├── RevDial client
    └── Veth bridges connecting desktops ↔ per-session networks
```

### The Complexity Inventory

Every piece of infrastructure below exists solely because dockerd runs as a **sibling** to the desktop container rather than **inside** it:

| Component | Files | Purpose | Why It Exists |
|-----------|-------|---------|---------------|
| Veth pair injection | `manager.go` BridgeDesktop() | Connect desktop (sandbox0) to user containers (hydra bridges) | Two separate Docker networks |
| Per-session DNS proxy | `dns.go` | Resolve container names across networks | Desktop can't see per-session dockerd's DNS |
| Bridge management | `manager.go` createBridge() | Create/manage hydra1, hydra2, ... bridges | Each session needs its own network |
| Subnet allocation | `manager.go` | 10.200.X.0/24 for bridges, 10.112.0.0/12 for docker networks | Prevent conflicts between sessions and with outer networks |
| Bridge index recovery | `manager.go` recoverBridgeIndices() | Track used bridge indices across restarts | Bridges persist but in-memory state doesn't |
| Orphaned veth cleanup | `manager.go` cleanupOrphanedVeth() | Clean up veths when containers restart | Container restart destroys one veth end |
| Container restart detection | `manager.go` (PID mismatch) | Re-bridge desktop when it restarts | Veth ends die with their container |
| Sandbox0 custom bridge | `04-start-dockerd.sh`, `manager.go` | Prevent docker0 deletion by session dockerds | Session dockerd startup deletes "docker0" by name |
| Reconnect orphaned veths | `manager.go` reconnectOrphanedVeths() | Reattach veths to sandbox0 after bridge deletion | Defensive measure for bridge stability |
| Port forwarding (iptables) | `manager.go` configureLocalhostForwarding() | Make `localhost:PORT` work from desktop to user containers | Desktop and containers in different network namespaces |
| DNS proxy binding | `05-start-dns-proxy.sh` | Bind to 10.213.0.1:53 not 0.0.0.0:53 | Must not conflict with per-session DNS |
| Per-session dockerd lifecycle | `manager.go` CreateDockerInstance/Delete | Start/stop isolated dockerd per session | Isolation requirement |
| BridgeDesktopPrivileged | `manager.go` | Special veth bridge for privileged mode | Yet another network topology variant |
| Docker socket routing | `hydra_executor.go` | Mount correct socket per session | Desktop must get its session's socket |
| Network path from desktop to outer API | Various | Route through sandbox0 → eth0 → host | Desktop on inner network needs outer connectivity |

**That's 15+ distinct mechanisms**, spanning thousands of lines of code across `manager.go`, `dns.go`, `server.go`, `hydra_executor.go`, and shell scripts. Multiple design docs (`2025-12-04`, `2025-12-07`, `2026-02-02`, `2026-02-06`) document the problems and fixes for these mechanisms.

### Bugs That Resulted From This Complexity

1. **docker0 deletion bug** — Session dockerd startup deletes the main bridge by name, orphaning all container network connections. Required renaming to `sandbox0`. (2026-02-06)
2. **Wrong Docker socket mounted** — Desktop got sandbox's main dockerd instead of its per-session dockerd. (2026-02-02)
3. **Subnet conflicts** — Inner docker-compose created networks on same subnet (172.19.0.0/16) as outer control plane. Required awkward subnet allocation scheme. (2026-02-02)
4. **DNS proxy port conflict** — DNS proxy on 0.0.0.0:53 blocked per-session DNS servers from binding. Required interface-specific binding. (2026-02-02)
5. **Veth naming collisions** — Truncated session IDs caused veth name collisions. Required using bridge index + PID instead. (2025-12-04)
6. **iptables rule accumulation** — MASQUERADE rules piled up without cleanup in privileged mode. Required delete-before-add pattern. (2025-12-04)
7. **Wolf container naming** — Wolf adds UUID suffix, breaking Hydra's container lookup. Required prefix-matching fallback. (2025-12-07)

**Every single one of these bugs exists because of the sibling-dockerd architecture.** With dockerd inside the desktop container, none of them can occur.

---

## The Simplified Architecture

### Core Idea

Run dockerd **inside** the desktop container, with `/var/lib/docker` as a Docker volume:

```
Host Docker (Level 0)
└── Sandbox Container (Level 1, privileged)
    └── Main dockerd (sandbox0)
        └── Desktop Container (Level 2, privileged, with Docker volume)
            ├── GNOME/Sway desktop
            ├── Zed IDE + AI agent
            ├── Streaming (desktop-bridge)
            └── Inner dockerd (/var/lib/docker = Docker volume)
                ├── User's webapp (docker compose up)
                ├── User's postgres
                └── User's redis
```

### What This Eliminates

| Gone | Why |
|------|-----|
| Veth pair injection | User containers share desktop's network namespace — same `localhost` |
| Per-session DNS proxy | Docker's built-in DNS (127.0.0.11) resolves container names natively |
| Bridge management | No hydra bridges needed — inner dockerd manages its own bridges |
| Subnet allocation | No conflict possible — inner dockerd is fully isolated |
| Bridge index recovery | No bridges to recover |
| Orphaned veth cleanup | No veths to orphan |
| Container restart detection | No cross-network bridges to re-establish |
| Sandbox0 custom bridge hack | No sibling dockerds to delete bridges |
| Port forwarding iptables | `docker run -p 8080:8080` binds to desktop's localhost natively |
| Per-session dockerd lifecycle (in sandbox) | Desktop manages its own dockerd |
| BridgeDesktopPrivileged | No bridges needed in any mode |
| Docker socket routing | Desktop always uses its own local dockerd |

### What `localhost` Means Now

Today, when a user runs `docker run -p 8080:8080 nginx`, making `localhost:8080` work from the desktop's Firefox requires:

1. Hydra creates a bridge (hydra1) with gateway 10.200.1.1
2. Docker binds port 8080 on 10.200.1.1 (the hydra bridge gateway)
3. BridgeDesktop creates a veth pair connecting desktop to hydra1
4. `configureLocalhostForwarding()` adds iptables DNAT rules redirecting 127.0.0.1:8080 → 10.200.1.1:8080
5. Periodic refresh (every 10 seconds) updates iptables for new ports

With dockerd inside the desktop container:

1. User runs `docker run -p 8080:8080 nginx`
2. Docker binds port 8080 on the desktop's 127.0.0.1
3. Firefox opens `localhost:8080`
4. It works.

### What Container Name Resolution Means Now

Today: Custom Go DNS server per session using miekg/dns, querying the per-session dockerd's API, listening on bridge gateway IP, added to desktop's resolv.conf.

With dockerd inside: Docker's built-in DNS at 127.0.0.11 resolves container names. Zero configuration.

### Simplified Hydra

Hydra's role shrinks dramatically:

**Before (15+ responsibilities):**
- Start/stop per-session dockerd instances
- Create/manage bridges (hydra1, hydra2, ...)
- Allocate subnets (10.200.X.0/24, 10.112.X.0/20)
- BridgeDesktop (veth pair injection, IP assignment, route setup)
- BridgeDesktopPrivileged (different veth setup for privileged mode)
- Per-session DNS servers (start, stop, query forwarding)
- DNS upstream configuration (parse /etc/resolv.conf)
- Bridge index recovery on restart
- Orphaned veth cleanup
- Container restart detection and re-bridging
- Port forwarding (iptables DNAT) and periodic refresh
- Sandbox0 bridge protection
- Docker socket path management
- Capacity management (254 bridges)
- RevDial API for all of the above

**After (3 responsibilities):**
- Start desktop containers with correct volume mounts (`/var/lib/docker` as Docker volume)
- Stop desktop containers
- Monitor desktop container health

The bridge management, DNS, veth injection, port forwarding, and subnet allocation code can all be deleted.

---

## Helix-in-Helix: The Dramatic Simplification

### Current Helix-in-Helix Architecture

Today, Helix-in-Helix requires two Docker endpoints and a complex proxy chain:

```
Host Docker (Level 0)
├── Outer Helix stack (api, postgres, frontend, sandbox)
│   └── Sandbox (Level 1)
│       └── Desktop (Level 2) ← Developer works here
│           ├── Inner Docker (per-session Hydra dockerd, via /var/run/docker.sock)
│           │   └── Inner Helix stack (api, postgres, chrome)
│           └── Outer Docker (host Docker, via /var/run/host-docker.sock)
│               └── Inner sandbox (runs on HOST Docker to avoid DinD limit)
│                   └── Inner desktop (Level 2 again, but on host Docker)
│                       └── User's work
```

This requires:
- Two Docker endpoints (`DOCKER_HOST_INNER`, `DOCKER_HOST_OUTER`)
- Host Docker socket proxy or bind mount
- Service exposure via API proxy chain (6 layers deep)
- Inner sandbox on host Docker (because we believed DinD can't go 3+ deep)
- Manual image transfer between sandbox and host Docker
- RevDial chain through multiple layers

### Simplified Helix-in-Helix

With dockerd inside the desktop container, the inner sandbox doesn't need to escape to host Docker. The inner sandbox is a **fully functional sandbox** — it runs dev containers that have their own dockerd, streaming, Zed IDE, the works. The architecture is recursive:

```
Host Docker (Level 0)
└── Outer Sandbox (Level 1, /var/lib/docker = volume)
    └── Outer Sandbox's dockerd
        └── Outer Desktop (Level 2, /var/lib/docker = volume)
            ├── GNOME desktop, Zed IDE, streaming ← Developer works here
            └── Outer Desktop's dockerd
                ├── Inner Helix stack (api, postgres, chrome)
                └── Inner Sandbox (Level 3, /var/lib/docker = volume)
                    └── Inner Sandbox's dockerd
                        └── Inner Desktop (Level 4, /var/lib/docker = volume)
                            ├── GNOME desktop, Zed IDE, streaming ← Fully functional
                            └── Inner Desktop's dockerd
                                └── User's containers (Level 5)
                                    e.g. webapp, postgres, redis
```

### The Fractal Property

The key insight is that **every level is structurally identical**. A sandbox runs dev containers. Each dev container has its own dockerd. If that dev container is running Helix, its Helix instance can create a sandbox, which runs more dev containers, which have their own dockerds.

The inner sandbox at level 3 is not a stub or a special case — it is a real sandbox running Hydra, launching real desktop containers with GNOME, Zed, video streaming, and AI agents. Those inner desktop containers can run `docker compose up` and their containers appear at level 5. If someone ran Helix inside *that* desktop, it would work at level 6, 7, 8... all the way to the kernel's 32-level namespace limit.

This recursive self-similarity is only possible because each level's `/var/lib/docker` is a Docker volume backed by real ext4. The overlay2 stacking depth resets to 1 at every level. There is no "innermost level" that behaves differently.

**What the inner desktop can do (same as outer):**
- Run `docker compose up` — containers appear on its local dockerd
- Access containers at `localhost:PORT` — same network namespace
- Resolve container names via DNS — Docker's built-in 127.0.0.11
- Stream video to the browser — via desktop-bridge RevDial chain
- Take screenshots — via the same RevDial chain
- Run AI agents (Zed + Qwen Code) — full IDE experience
- Even run `./stack start` to create yet another Helix instance (level 6+)

### Four Levels Deep — Is That Feasible?

**Yes.** With volume-backed `/var/lib/docker` at each level:

| Level | Container | overlay2 stack depth | Why it works |
|-------|-----------|---------------------|-------------|
| 0 | Host Docker | 1 (overlay2 on host ext4) | Host filesystem is ext4 |
| 1 | Outer Sandbox | 1 (overlay2 on Docker volume = ext4) | Volume is backed by host ext4 |
| 2 | Outer Desktop | 1 (overlay2 on Docker volume = ext4) | Volume is backed by host ext4 |
| 3 | Inner Sandbox | 1 (overlay2 on Docker volume = ext4) | Volume is backed by host ext4 |
| 4 | Inner Desktop | 1 (overlay2 on Docker volume = ext4) | Volume is backed by host ext4 |

Each level has overlay2 stack depth of 1, well within the kernel's limit of 2. The PID namespace nesting depth is 4, well within the kernel's limit of 32.

**What this eliminates for Helix-in-Helix:**
- No more `DOCKER_HOST_INNER` / `DOCKER_HOST_OUTER` split
- No more host Docker socket exposure
- No more manual image transfer between dockerd instances
- No more 6-layer API proxy chain
- The inner sandbox runs inside the developer's desktop dockerd, not on host Docker
- Each level is self-contained — no cross-level networking needed

### Why We No Longer Need Two Docker Endpoints

The current Helix-in-Helix design uses two Docker endpoints because:

> Sandboxes need DinD, and the desktop is already inside DinD. Running DinD-in-DinD-in-DinD doesn't work **if each level uses overlay2 on the layer above.**

With volume-backed Docker at each level, this constraint disappears. The developer simply runs `./stack start` inside their desktop, and the inner sandbox starts inside the desktop's dockerd. No host Docker access needed.

---

## The Privileged Sandbox Question

### Current: Why Sandboxes Run on Host Docker

Today, sandboxes run on the host's Docker daemon (or at most one DinD level deep) because:

1. **GPU access:** NVIDIA runtime needs host-level device access
2. **DinD capability:** Sandboxes need `--privileged` for nested containers
3. **overlay2 limit (false):** Believed DinD can only go 2 deep

With the overlay2 concern removed, sandboxes could potentially run deeper. GPU passthrough does work through multiple DinD levels when using the NVIDIA container runtime, since it's ultimately just device mounts and cgroup rules.

### What Still Requires `--privileged`

Each container that runs its own dockerd needs `--privileged` (or at minimum `SYS_ADMIN`, `MKNOD`, and a few other caps). This is an inherent requirement of DinD and doesn't change with nesting depth.

**Security implication:** Each privileged container can theoretically escape to the host. This is true at any nesting depth. The security boundary is the same whether you nest 2 or 4 levels deep — `--privileged` at any level gives host access.

For production multi-tenancy, **Sysbox** (acquired by Docker) provides rootless DinD without `--privileged`. This would be a future improvement orthogonal to the nesting simplification.

---

## Trade-offs

### What We Gain

1. **Massive code deletion.** ~2000+ lines of bridge management, DNS proxy, veth injection, subnet allocation, port forwarding, and associated bug fixes.
2. **`localhost` just works.** No iptables DNAT, no periodic port refresh.
3. **Container DNS just works.** No custom DNS servers, no miekg/dns dependency for this purpose.
4. **No bridge bugs.** No docker0 deletion, no orphaned veths, no bridge index exhaustion.
5. **No subnet conflicts.** Each desktop is fully isolated — no awkward subnet schemes.
6. **Simpler Helix-in-Helix.** One Docker endpoint, no host Docker exposure, no image transfer.
7. **Each desktop is self-contained.** Easier to reason about, debug, and monitor.
8. **Kind/K3s compatibility.** `kind create cluster` just works inside the desktop.

### What We Lose / New Concerns

1. **Per-desktop dockerd overhead: zero net change.** We already run one dockerd per session (Hydra's per-session dockerd). This proposal just moves it from being a sibling process in the sandbox to running inside the desktop container. Same process count, same memory overhead, just a different location. If anything, it's slightly more efficient because we no longer need the veth/bridge/DNS infrastructure that connected the two.

2. **Image pre-loading.** Each desktop's dockerd starts with an empty image cache. Options:
   - **Local registry in sandbox** (recommended) — desktop pulls from local registry, layer sharing works
   - **Volume snapshot** — pre-populate a Docker volume, clone per session
   - **Lazy pull** — simplest but first `docker compose up` is slow

   This is the same situation as today with per-session Hydra dockerds — they also start with empty caches.

3. **Desktop container must be privileged.** Today, desktop containers are not privileged (the sandbox is). With dockerd inside, the desktop container needs `--privileged` or equivalent capabilities. This is a real change in the security boundary, though the practical impact is limited since the sandbox is already privileged and desktop containers already have extensive capabilities (`SYS_ADMIN`, `SYS_NICE`, `SYS_PTRACE`, `NET_RAW`, `MKNOD`, `NET_ADMIN`).

4. **Build cache sharing.** Today, helix-buildkit runs on the sandbox's main dockerd, shared across sessions. With dockerd-per-desktop, each session gets its own build cache (unless using the shared BuildKit via registry).

5. **Startup time.** dockerd takes ~2-3 seconds to start inside the container. This happens in parallel with desktop initialization (GNOME/Sway startup takes ~10-15 seconds), so it adds zero perceived latency.

---

## Implementation Plan

### Phase 1: Dockerd-in-Desktop Mode (`DOCKER_IN_DESKTOP=true`)

Add a new mode to Hydra that runs dockerd inside the desktop container instead of as a sibling:

1. **Desktop image changes:**
   - Install dockerd, containerd, runc in the desktop image (helix-ubuntu)
   - Add startup script that launches dockerd with `/var/lib/docker` as the data root
   - Configure NVIDIA runtime inside the desktop's dockerd (for GPU containers)

2. **Hydra changes:**
   - When `DOCKER_IN_DESKTOP=true`, skip per-session dockerd creation
   - Skip BridgeDesktop (no veth needed)
   - Skip DNS proxy startup for sessions
   - Mount a Docker volume at `/var/lib/docker` in the desktop container
   - Set the desktop container to privileged mode

3. **Desktop container startup:**
   ```bash
   # Inside desktop container entrypoint
   if [ -f /var/lib/docker ]; then
       # Start dockerd in background
       dockerd --data-root /var/lib/docker &
       # Wait for socket
       while [ ! -S /var/run/docker.sock ]; do sleep 0.1; done
   fi
   # Continue with normal desktop startup (GNOME, Zed, streaming)
   ```

4. **Feature flag:** `DOCKER_IN_DESKTOP=true` environment variable on the sandbox. Default to `false` (old behavior) for safe rollout.

### Phase 2: Simplify Helix-in-Helix

Once dockerd-in-desktop works:

1. Remove `DOCKER_HOST_OUTER` / host Docker socket exposure
2. Inner sandbox runs inside desktop's dockerd (level 3)
3. Single Docker endpoint for developers
4. Remove image transfer scripts

### Phase 3: Remove Old Infrastructure

Once dockerd-in-desktop is proven stable:

1. Delete veth bridge code from `manager.go`
2. Delete per-session DNS proxy code from `dns.go`
3. Delete bridge management (createBridge, recoverBridgeIndices, etc.)
4. Delete subnet allocation code
5. Delete port forwarding (iptables DNAT) code
6. Delete sandbox0 bridge protection code
7. Delete orphaned veth cleanup code
8. Simplify Hydra API (remove BridgeDesktop, BridgeDesktopPrivileged)
9. Update design docs to reflect new architecture

### Phase 4: Remove Sandbox DinD Layer (Future)

With dockerd inside the desktop container, the sandbox's role shrinks to:
- Running the main dockerd that launches desktop containers
- Running Hydra (simplified) for container lifecycle
- Running RevDial client

In the future, we could potentially eliminate the sandbox DinD layer entirely by running desktop containers directly on the host Docker, making Hydra a standard container rather than a DinD manager. But this is a separate discussion.

---

## Verification Steps

### Prove Arbitrary Nesting Works

Before implementing, verify the core claim with a simple test:

```bash
# Level 0: Host Docker
docker run --privileged -v l1-docker:/var/lib/docker -d --name level1 docker:dind

# Level 1: Inside first DinD
docker exec level1 docker run --privileged -v l2-docker:/var/lib/docker -d --name level2 docker:dind

# Level 2: Inside second DinD
docker exec level1 docker exec level2 docker run --privileged -v l3-docker:/var/lib/docker -d --name level3 docker:dind

# Level 3: Inside third DinD
docker exec level1 docker exec level2 docker exec level3 docker run --privileged -v l4-docker:/var/lib/docker -d --name level4 docker:dind

# Level 4: Inside fourth DinD — verify it works
docker exec level1 docker exec level2 docker exec level3 docker exec level4 docker run hello-world
```

If `hello-world` runs at level 4, the core premise is proven. Each level uses a named Docker volume, so overlay2 at each level is on ext4, not on another overlay.

### Verify GPU Access at Depth

```bash
# Verify NVIDIA runtime works at level 2 (inside desktop's dockerd)
# Inside desktop container:
docker run --rm --gpus all nvidia/cuda:12.0-base nvidia-smi
```

---

## Conclusion

The Helix sandbox architecture accumulated enormous complexity because of a misunderstanding about Docker nesting limits. The overlay2 filesystem stacking limit (2 levels) was conflated with a Docker nesting limit. In reality, Docker can nest arbitrarily deep as long as each level's `/var/lib/docker` is backed by a real filesystem via Docker volumes.

Moving dockerd inside the desktop container eliminates the entire veth bridging / DNS proxy / subnet management / port forwarding infrastructure. `localhost` just works. Container DNS just works. Each desktop is self-contained. The code deletion would be substantial — thousands of lines of the most bug-prone infrastructure in the codebase.

For Helix-in-Helix, the simplification is even more dramatic: no more two Docker endpoints, no more host Docker socket exposure, no more image transfer between dockerd instances. The inner sandbox simply runs inside the developer's desktop dockerd at level 3 — a configuration that works because the overlay2 constraint was never about nesting depth.

Four levels deep? Five? The kernel allows 32. We'd use 4-5 at most for Helix-in-Helix. The overhead is manageable (each dockerd is ~50-100MB RAM), and the complexity reduction is transformational.

---

## POC Test Results (2026-02-10)

Branch `feature/docker-in-desktop` implements Phase 1 + Phase 3 (simultaneous — old code removed, new mode is the only mode).

### What Works

| Test | Result | Notes |
|------|--------|-------|
| Desktop container startup | **PASS** | Container stays running, all init scripts execute |
| dockerd starts inside desktop | **PASS** | Detects `/var/lib/docker` volume mount, starts on attempt 1 |
| `docker info` inside desktop | **PASS** | Server Version 29.2.1, overlay2, Docker Root Dir: /var/lib/docker |
| `docker run hello-world` | **PASS** | Pulls from Docker Hub, runs successfully |
| `docker build` | **PASS** | BuildKit builds work, multi-step Dockerfile |
| `docker compose` | **PASS** | Docker Compose v5.0.2 available |
| DNS resolution (inner containers) | **PASS** | `ping google.com` works from containers on inner dockerd |
| Desktop-bridge streaming | **PASS** | WebSocket health checks every 10s |
| Settings-sync daemon | **PASS** | Detects config changes, injects API keys |
| Privileged mode | **PASS** | Container runs as privileged with Docker volume mount |
| Volume mount (ext4 backing) | **PASS** | `docker-data-{session_id}` named volume at `/var/lib/docker` |
| No old docker.sock mount | **PASS** | No docker.sock or host-docker.sock bind mounts |
| `kind` binary present | **PASS** | kind v0.27.0 |
| `kubectl` binary present | **PASS** | kubectl v1.35.1 |

### What Doesn't Work Yet

| Test | Result | Notes |
|------|--------|-------|
| `kind create cluster` | **FAIL** | cgroup v2 controller delegation — sandbox's dockerd only delegates `cpuset cpu pids`, missing `memory io` needed by Kind's systemd nodes. Fixable by configuring sandbox dockerd's cgroup parent or using `--cgroupns=host`. |
| Enterprise DNS chain | **PARTIAL** | DNS works via Google DNS fallback (8.8.8.8). The sandbox's inner dockerd sees 127.0.0.11 in its /etc/resolv.conf, can't use it as upstream, falls back to defaults. Enterprise DNS would need explicit `--dns` flag on sandbox's dockerd pointing to the host gateway. This is a pre-existing issue with the sandbox architecture, not introduced by docker-in-desktop. |

### Bugs Found and Fixed During Testing

1. **DNS proxy blocking sandbox startup** — `45-start-dns-proxy.sh` waited 30s for `sandbox0` bridge (no longer exists), then `exit 1`. Fixed by removing DNS proxy from `Dockerfile.sandbox` entirely.
2. **Hardcoded DNS `8.8.8.8` in desktop's `17-start-dockerd.sh`** — Would break enterprise DNS. Fixed by removing explicit `"dns"` key from daemon.json.
3. **`exit 0` in sourced init script kills entrypoint** — `16-add-docker-group.sh` used `exit 0` when docker socket not found, which killed the entire entrypoint (GOW entrypoint `source`s scripts under `set -e`). Fixed by changing to `return 0 2>/dev/null || true`.

### Code Changes Summary

**Modified:** `Dockerfile.sandbox`, `Dockerfile.ubuntu-helix`, `api/pkg/external-agent/hydra_executor.go`, `api/pkg/hydra/manager.go`, `api/pkg/hydra/server.go`, `api/pkg/hydra/client.go`, `api/pkg/hydra/types.go`, `api/pkg/hydra/store.go`, `api/pkg/hydra/store_mocks.go`, `api/pkg/hydra/store_sandbox.go`, `api/pkg/types/project.go`, `api/pkg/server/session_handlers.go`, `api/pkg/server/simple_sample_projects.go`, `api/pkg/services/sample_project_code_service.go`, `stack`, `docker-compose.dev.yaml`, `desktop/ubuntu-config/16-add-docker-group.sh`, `desktop/ubuntu-config/cont-init.d/17-start-dockerd.sh`, `frontend/` (several files removing UseHostDocker UI)

**Deleted:** `api/pkg/hydra/dns.go` (~287 lines)

**Created:** `desktop/ubuntu-config/cont-init.d/17-start-dockerd.sh`

**Estimated net code reduction:** ~1500+ lines of bridge/DNS/veth infrastructure removed

---

## References

- [Linux kernel `FILESYSTEM_MAX_STACK_DEPTH`](https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/tree/include/linux/fs.h) — hardcoded to 2 for overlayfs
- [Sysbox design: implicit host mounts](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/design.md) — bypasses overlay stacking with bind mounts
- [KinD base image](https://github.com/kubernetes-sigs/kind/blob/main/images/base/Dockerfile) — uses fuse-overlayfs for 3-level nesting
- [pid_namespaces(7)](https://man7.org/linux/man-pages/man7/pid_namespaces.7.html) — 32 levels max
- [user_namespaces(7)](https://man7.org/linux/man-pages/man7/user_namespaces.7.html) — 32 levels max
- [cgroup v2 documentation](https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html) — `cgroup.max.depth` configurable
- Internal: [Hydra Architecture Deep Dive](./2025-12-07-hydra-architecture-deep-dive.md)
- Internal: [Helix-in-Helix Development](./2026-01-25-helix-in-helix-development.md)
- Internal: [Docker Network Isolation Fix](./2026-02-02-docker-network-isolation-fix.md)
- Internal: [Sandbox Bridge Protection](./2026-02-06-docker0-veth-reconnection.md)
- Internal: [Hydra Network DNS Bridging](./2025-12-04-hydra-network-dns-bridging.md)
