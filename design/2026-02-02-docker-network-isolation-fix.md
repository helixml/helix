# Docker Network Isolation Fix: Preventing Subnet Conflicts

**Date:** 2026-02-02
**Status:** OUTDATED - The per-session sibling dockerd architecture this fixed has been removed. Docker-in-desktop mode eliminates subnet conflicts by design (see `2026-02-10-arbitrary-dind-nesting-simplification.md`).
**Author:** Claude (with Luke)

## Executive Summary

Docker-in-Docker network subnet conflicts caused streaming sessions to fail with RevDial connection timeouts. The root cause was twofold:

1. **Desktop containers received the wrong Docker socket** - They connected to the sandbox's main dockerd instead of their per-session Hydra dockerd
2. **No subnet isolation** - Docker Compose projects running inside desktop containers created networks with the same subnet as the outer control plane

## Solution Implemented

### Security Fix: Always Use Per-Session Hydra Dockerd

Each desktop container now ALWAYS gets its own isolated dockerd instance. This is a security requirement to prevent:

1. Users breaking each other's containers (`docker stop/rm`)
2. Users reading secrets from other containers (`docker exec/logs`)
3. Network conflicts from docker-compose with conflicting subnets
4. Container escape to the sandbox's control plane dockerd

**Code change:** `api/pkg/external-agent/hydra_executor.go` - Removed the `if agent.UseHydraDocker` condition, making per-session dockerd mandatory.

### Configuration Cleanup: Remove Hardcoded IPs

Removed all hardcoded IP addresses from `docker-compose.dev.yaml`:

- Removed static IP assignments for API and sandbox services
- Removed hardcoded subnet (Docker uses default-address-pools from daemon.json)
- Changed runner API_HOST from gateway IP to service name (`api:8080`)
- Made network name configurable via `HELIX_NETWORK_NAME` environment variable

This document describes the architecture and root cause analysis.

---

## Architecture Overview

### Network Layers

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ LAYER 0: Host Machine                                                        │
│                                                                              │
│  docker0: 172.17.0.0/16 (host Docker default bridge)                        │
│  helix_default: 172.19.0.0/16 (Helix control plane)                         │
│    ├── api: 172.19.0.20                                                      │
│    ├── postgres, keycloak, frontend, etc.                                   │
│    └── sandbox-nvidia: 172.19.0.50                                          │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ LAYER 1: Sandbox Container (sandbox-nvidia)                                  │
│                                                                              │
│  External interface: eth0 = 172.19.0.50 (on helix_default)                  │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │ Sandbox's Main Dockerd (Wolf's dockerd)                                 │ │
│  │   docker0: 172.17.0.0/16                                                │ │
│  │   (Runs desktop containers: ubuntu-external-*, helix-sway-*)            │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │ Per-Session Hydra Dockerds (when enabled)                               │ │
│  │   Session A: 10.200.1.0/24 (hydra1 bridge)                              │ │
│  │   Session B: 10.200.2.0/24 (hydra2 bridge)                              │ │
│  │   Session C: 10.200.3.0/24 (hydra3 bridge)                              │ │
│  │   ...                                                                    │ │
│  │   (These run user's docker-compose projects)                            │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │ Host Docker Socket (privileged mode only)                               │ │
│  │   /var/run/host-docker.sock → Host's /var/run/docker.sock              │ │
│  │   (For Helix-in-Helix: run sandboxes on host Docker)                   │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Docker Socket Hierarchy

| Socket Location (in sandbox) | Controls | Purpose |
|------------------------------|----------|---------|
| `/var/run/docker.sock` | Sandbox's main dockerd | Desktop containers (Wolf) |
| `/var/run/hydra/active/{session}/docker.sock` | Per-session Hydra dockerd | User's docker-compose projects |
| `/var/run/host-docker.sock` | Host machine's dockerd | Privileged mode (Helix-in-Helix sandboxes) |

---

## Root Cause Analysis

### Problem 1: Wrong Docker Socket Mounted

**Expected behavior:** Desktop containers should have their per-session Hydra dockerd socket mounted at `/var/run/docker.sock`.

**Actual behavior:** Desktop containers receive the sandbox's main dockerd socket (`/var/run/docker.sock`) regardless of Hydra isolation settings.

**Code path:**

1. `hydra_executor.go:294-315` - Creates isolated Hydra dockerd, sets `req.DockerSocket`
2. `hydra_executor.go:908-912` - **BUG**: Always mounts `/var/run/docker.sock` as source, ignoring `req.DockerSocket`

```go
// Line 908-912 in hydra_executor.go
mounts = append(mounts, hydra.MountConfig{
    Source:      "/var/run/docker.sock",        // BUG: Should use req.DockerSocket if set
    Destination: "/var/run/docker.sock",
    ReadOnly:    false,
})
```

### Problem 2: Docker Network Subnet Conflicts

When docker-compose runs inside a desktop container connected to the wrong dockerd:

1. User runs `./stack start` inside desktop
2. Docker Compose creates `helix_default` network
3. Docker assigns the default 172.x.x.x range (e.g., 172.19.0.0/16)
4. This conflicts with the outer `helix_default` on the same subnet
5. Routing table shows two routes for 172.19.0.0/16:
   - `eth0` (outer helix_default - correct route to API)
   - `br-xxx` (inner helix_default - wrong route)
6. RevDial connections to 172.19.0.20 (API) fail with timeout

**Evidence from sandbox routing table:**
```
172.19.0.0/16 dev eth0 proto kernel scope link src 172.19.0.50
172.19.0.0/16 dev br-4ae016078c46 proto kernel scope link src 172.19.0.1
```

---

## The Fix

### Fix 1: Use Correct Docker Socket Mount

**File:** `api/pkg/external-agent/hydra_executor.go`

When Hydra creates an isolated dockerd, use that socket for the mount instead of the main one:

```go
// Docker socket mount - use isolated socket if available
dockerSocketSource := "/var/run/docker.sock"
if req.DockerSocket != "" {
    dockerSocketSource = req.DockerSocket
}
mounts = append(mounts, hydra.MountConfig{
    Source:      dockerSocketSource,
    Destination: "/var/run/docker.sock",
    ReadOnly:    false,
})
```

### Fix 2: Configure Sandbox Dockerd Subnets

**File:** `sandbox/04-start-dockerd.sh`

Configure the sandbox's main dockerd to use an awkward 10.x range no human would choose:

```json
{
  "default-address-pools": [
    {"base": "10.213.0.0/16", "size": 24}
  ]
}
```

This ensures any user-created networks on the sandbox's main dockerd use 10.213.x.x instead of 172.x.x.x. We chose 213 (3×71) because it's awkward and contains 13 (unlucky).

**Note:** We avoid 100.64.0.0/10 (CGNAT range) because Tailscale uses it for mesh VPN addresses.

### Fix 3: Configure Per-Session Hydra Dockerd Subnets

**File:** `api/pkg/hydra/manager.go`

Each per-session Hydra dockerd should get a unique subnet range from an awkward 10.x range:

```go
// Allocation scheme using /20 blocks (16 /24 networks per session):
// - Range: 10.112.0.0/12 (10.112.0.0 - 10.127.255.255) for per-session dockerds
// - Sandbox's main dockerd uses 10.213.0.0/16 (separate range)
// - Each session gets unique /20 block
//
// Formula: 10.(112 + (N-1)/16).((N-1)%16 * 16).0/20
secondOctet := 112 + (bridgeIndex-1)/16
thirdOctet := ((bridgeIndex - 1) % 16) * 16
sessionPoolBase := fmt.Sprintf("10.%d.%d.0/20", secondOctet, thirdOctet)
```

### Fix 4: Bridge Desktop to Hydra Network

**File:** `api/pkg/external-agent/hydra_executor.go`

After creating the desktop container, call `BridgeDesktop` to create a veth pair connecting the desktop (on sandbox's docker0) to the per-session Hydra dockerd network. This enables the desktop to reach containers started via `docker compose` on the per-session dockerd.

```go
// Bridge desktop container to per-session Hydra dockerd network
bridgeReq := &hydra.BridgeDesktopRequest{
    SessionID:          agent.SessionID,
    DesktopContainerID: resp.ContainerID,
}
bridgeResp, err := hydraClient.BridgeDesktop(ctx, bridgeReq)
if err != nil {
    log.Warn().Err(err).Msg("Failed to bridge desktop to Hydra network")
} else {
    log.Info().
        Str("desktop_ip", bridgeResp.DesktopIP).
        Str("gateway", bridgeResp.Gateway).
        Msg("Desktop bridged to Hydra network")
}
```

**What BridgeDesktop does:**
1. Creates a veth pair: `veth-{session}-h` (host end) and `veth-{session}-c` (container end)
2. Attaches host end to the Hydra bridge (e.g., `hydra3`)
3. Injects container end into the desktop container's network namespace
4. Assigns IP address from the Hydra subnet (e.g., `10.200.3.254/24`)
5. Adds route for the Hydra bridge network (`10.200.x.0/24`) via eth1
6. Adds route for per-session dockerd networks (`10.112.0.0/12`) via the Hydra gateway
7. Configures DNS to use Hydra's DNS server (resolves container names)
8. Sets up localhost forwarding for exposed Docker ports (refreshed every 10 seconds)

**Result:** Desktop container gets a second interface (`eth1`) with:
- Connectivity to containers on the per-session dockerd (via 10.112.0.0/12 route)
- DNS resolution for container names (via Hydra DNS on 10.200.x.1:53)
- Localhost port forwarding for `docker run -p` exposed ports

### Fix 5: Bind dns-proxy to Specific Interface

**File:** `sandbox/05-start-dns-proxy.sh` (renamed from `sandbox/03-start-dns-proxy.sh`)

The dns-proxy was binding to `0.0.0.0:53`, which blocked Hydra's per-session DNS servers from binding to `10.200.X.1:53`. This prevented container name resolution.

**Changes:**
1. Renamed script from `03-start-dns-proxy.sh` to `05-start-dns-proxy.sh` (runs AFTER dockerd)
2. Changed bind address from `0.0.0.0:53` to `10.213.0.1:53` (sandbox docker0 gateway)
3. Updated Dockerfile.sandbox to use `45-start-dns-proxy.sh` instead of `35-`

**Result:** Two DNS servers run simultaneously:
- `dns-proxy` on `10.213.0.1:53` - forwards to Docker DNS for enterprise DNS resolution
- Hydra DNS on `10.200.X.1:53` - resolves container names on per-session dockerd

Desktop containers can now use `curl http://my-container:80` to reach containers by name.


### Network Allocation Summary

| Component | Subnet Range | Purpose |
|-----------|--------------|---------|
| Host docker0 | 172.17.0.0/16 | Host Docker default bridge |
| helix_default (outer) | 172.19.0.0/16 | Helix control plane |
| Sandbox docker0 | 10.213.0.0/24 | Sandbox default bridge (desktop containers) |
| Sandbox user networks | 10.213.0.0/16 | Networks created on sandbox's main dockerd |
| Hydra bridges | 10.200.X.0/24 | Per-session Docker bridges (veth connections) |
| Per-session dockerd networks | 10.112.0.0/12 | User docker-compose projects in isolated dockerds |

**Key insight:** By using awkward 10.x values (112, 213) that humans wouldn't naturally choose, we avoid conflicts with:
- The outer 172.19.0.0/16 network (Helix control plane)
- Common home/corporate networks (10.0.x.x, 10.1.x.x, 10.10.x.x, 10.100.x.x)
- Tailscale's CGNAT range (100.64.0.0/10)

---

## Fix 6: Helix-in-Helix Docker Socket Isolation

**File:** `api/pkg/external-agent/hydra_executor.go`

When `UseHostDocker` is true (helix-in-helix mode), we must NOT pass it to `CreateDockerInstance()`. Otherwise, Hydra returns the host docker socket for BOTH mounts, breaking isolation:

**Before (broken):**
- `/var/run/docker.sock` → host Docker (wrong!)
- `/var/run/host-docker.sock` → host Docker

**After (fixed):**
- `/var/run/docker.sock` → per-session Hydra dockerd (for inner control plane)
- `/var/run/host-docker.sock` → host Docker (for inner sandbox, only when UseHostDocker=true)

This allows helix-in-helix to work correctly:
1. Inner control plane (`./stack start`) runs on isolated per-session dockerd
2. Inner sandbox uses host Docker (via `DOCKER_HOST=unix:///var/run/host-docker.sock`)
3. No DinD-in-DinD-in-DinD issues

---

## Testing Plan

1. **Build and deploy changes:**
   - Rebuild API with hydra_executor.go fix
   - Restart sandbox with new daemon.json

2. **Verify Docker socket:**
   ```bash
   # Inside desktop container
   docker info --format '{{.Name}}'
   # Should show unique per-session ID, not sandbox's ID
   ```

3. **Verify network isolation:**
   ```bash
   # Start docker-compose project inside desktop
   docker compose up -d
   docker network ls --format '{{.Name}}: {{.Driver}}'
   # Networks should use 10.64+ range, not 172.x.x.x
   ```

4. **Verify RevDial connectivity:**
   ```bash
   # From desktop container
   curl http://172.19.0.20:8080/api/v1/config
   # Should succeed (no routing conflict)
   ```

---

## References

- [Helix-in-Helix Development](./2026-01-25-helix-in-helix-development.md)
- [Hydra Architecture Deep Dive](./2025-12-07-hydra-architecture-deep-dive.md)
