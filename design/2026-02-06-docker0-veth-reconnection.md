# Docker0 Bridge Restoration and Veth Reconnection

**Date:** 2026-02-06
**Status:** In Progress
**Branch:** fix/docker0-veth-reconnect
**PR:** #1593

## Problem

When starting a per-session Hydra dockerd with its own bridge (e.g., `hydra1`), Docker's startup sometimes deletes or disrupts the main sandbox dockerd's `docker0` bridge. This orphans veth interfaces from containers attached to docker0 (e.g., `helix-buildkit`, `buildx` builders), breaking their network connectivity.

The issue was discovered while implementing shared BuildKit cache across sandboxes (see `design/2026-01-25-shared-buildkit-cache.md`).

### Symptoms

1. `helix-buildkit` container loses network connectivity after starting a session
2. Docker builds inside desktop containers fail with "shared BuildKit container not found"
3. `ip link show docker0` returns "Device does not exist"
4. Orphaned veth pairs visible: `ip link | grep veth` shows interfaces with `master` but the bridge is gone

### Root Cause (Hypothesis)

When multiple Docker daemons run on the same host, they share global iptables chains (DOCKER, DOCKER-ISOLATION-STAGE-1, DOCKER-ISOLATION-STAGE-2). Session dockerd startup manipulates these chains, which can have side effects on the main dockerd's docker0 bridge.

Docker 28+ introduced `--ip-forward-no-drop` flag to prevent setting FORWARD policy to DROP, but testing showed this breaks internet connectivity in containers (reason unknown).

## Architecture

### Full Networking Diagram (Helix-in-Helix)

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│ HOST MACHINE                                                                         │
│                                                                                      │
│  Host Docker Daemon                                                                  │
│  └── helix_default network (172.18.0.0/16)                                          │
│       ├── api (172.18.0.14) ◄──────────────────────────┐                            │
│       ├── postgres, keycloak, frontend, etc.           │                            │
│       └── sandbox-nvidia (172.18.0.15) ◄───────────────┼── RevDial WebSocket        │
│                     │                                  │                            │
└─────────────────────┼──────────────────────────────────┼────────────────────────────┘
                      │                                  │
                      ▼                                  │
┌─────────────────────────────────────────────────────────────────────────────────────┐
│ SANDBOX CONTAINER (sandbox-nvidia)                     │                            │
│                                                        │                            │
│  eth0 ────────────────────────────────────────────────►│ (172.18.0.15)              │
│    │                                                   │                            │
│    │  Sandbox's Internal Dockerd                       │                            │
│    │  └── docker0 bridge (10.213.0.0/24) ◄─────────────┼── THIS IS WHAT GETS DELETED│
│    │       │                                           │                            │
│    │       ├── helix-buildkit (10.213.0.2)             │                            │
│    │       ├── buildx builders (10.213.0.3+)           │                            │
│    │       └── ubuntu-external-xxx (10.213.0.5) ───────┘                            │
│    │             │                                                                  │
│    │             │ eth0: 10.213.0.5 (on docker0)                                    │
│    │             │ eth1: 10.200.1.254 (on hydra1) ──────┐                           │
│    │                                                    │                           │
│    │  Per-Session Hydra Dockerd                         │                           │
│    │  └── hydra1 bridge (10.200.1.0/24) ◄───────────────┘                           │
│    │       │                                                                        │
│    │       └── User's containers via docker-compose                                 │
│    │            └── 10.112.0.0/12 range (per-session networks)                      │
│    │                                                                                │
│    │  Hydra Daemon                                                                  │
│    │  └── Manages per-session dockerds + bridges                                    │
│    │                                                                                │
└────┼────────────────────────────────────────────────────────────────────────────────┘
     │
     │  HELIX-IN-HELIX CASE
     │  (User runs ./stack start inside desktop)
     ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│ DESKTOP CONTAINER (ubuntu-external-xxx)                                             │
│                                                                                      │
│  eth0 (10.213.0.5) ──► docker0 ──► sandbox eth0 ──► host ──► internet              │
│  eth1 (10.200.1.254) ──► hydra1 ──► per-session dockerd                            │
│                                                                                      │
│  When user runs: DOCKER_HOST=unix:///var/run/docker.sock ./stack start              │
│  └── Containers created on per-session Hydra dockerd                                │
│       └── helix_default (inner): 10.112.0.0/20                                      │
│            ├── api (inner): 10.112.0.2                                              │
│            ├── sandbox (inner): 10.112.0.3 ◄── Can use host Docker via             │
│            │                                   /var/run/host-docker.sock            │
│            └── etc.                                                                 │
│                                                                                      │
│  Desktop reaches inner containers via:                                              │
│    curl http://10.112.0.2:8080 (via eth1 → hydra1 → per-session dockerd)           │
│                                                                                      │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

### Network Path Summary

| From | To | Path |
|------|-----|------|
| Desktop → Internet | `eth0 → docker0 → sandbox eth0 → host → internet` |
| Desktop → API (control plane) | `eth0 → docker0 → sandbox eth0 → helix_default → api` |
| Desktop → User containers (docker-compose) | `eth1 → hydra1 → per-session dockerd networks` |
| Desktop → helix-buildkit | `eth0 → docker0 → helix-buildkit` |

### Simplified View

```
Sandbox Container (sandbox-nvidia)
├── Main Dockerd (sandbox's native dockerd)
│   └── docker0 bridge (10.213.0.0/24)
│       ├── helix-buildkit container
│       ├── helix-desktop container
│       └── buildx builders
│
└── Per-Session Hydra Dockerds
    ├── Session A: hydra1 bridge (10.200.1.0/24)
    ├── Session B: hydra2 bridge (10.200.2.0/24)
    └── Session C: hydra3 bridge (10.200.3.0/24)
```

The desktop container runs on the main dockerd (docker0), while user's `docker compose` projects run on per-session Hydra dockerds. The desktop is bridged to the Hydra network via a veth pair for connectivity.

## Practical Implications

When a new session starts and docker0 gets deleted (before restoration), the following containers on the sandbox's main dockerd temporarily lose network connectivity:

1. **helix-buildkit** - The shared BuildKit container for caching Docker builds
2. **buildx builders** - Docker buildx builder containers
3. **Desktop container** (ubuntu-external-*, sway-external-*) - The container streaming video to the user

For the desktop container specifically, during the brief outage window (~100ms):

| Interface | Status | Impact |
|-----------|--------|--------|
| eth0 (docker0) | **BROKEN** | Internet access lost, API connectivity lost |
| eth1 (hydra bridge) | Still works | Can still reach per-session dockerd containers |

**Practical impact:**
- **RevDial connection to the API could drop momentarily** when a new session starts
- Video streaming may glitch briefly
- The restoration happens immediately after detection, so it should reconnect automatically
- Users may see a brief freeze in the video stream if timing is unlucky

**What does NOT break:**
- Containers on per-session Hydra dockerds (they use hydra bridges, not docker0)
- Communication between desktop and user's docker-compose projects (via eth1)

## Solution: Reactive Restoration

Instead of preventing docker0 deletion (which proved problematic), we detect when it happens and restore it:

### 1. Detect docker0 Deletion

Check docker0 status before and after session dockerd startup:

```go
docker0ExistsBefore := m.checkDocker0Exists()
// ... start session dockerd ...
docker0ExistsAfter := m.checkDocker0Exists()

if docker0ExistsBefore && !docker0ExistsAfter {
    log.Warn().Msg("docker0 was deleted by session dockerd startup, restoring it")
    m.EnsureDocker0Bridge()
}
```

### 2. Restore docker0 Bridge

If docker0 was deleted, recreate it with the correct subnet:

```go
func (m *Manager) EnsureDocker0Bridge() error {
    // Check if docker0 exists
    if m.checkDocker0Exists() {
        m.reconnectOrphanedVeths()  // Still reconnect any orphaned veths
        return nil
    }

    // Recreate docker0 with correct IP
    m.runCommand("ip", "link", "add", "docker0", "type", "bridge")
    m.runCommand("ip", "addr", "add", "10.213.0.1/24", "dev", "docker0")
    m.runCommand("ip", "link", "set", "docker0", "up")

    // Reconnect orphaned veths
    m.reconnectOrphanedVeths()
    return nil
}
```

### 3. Reconnect Orphaned Veths

Find veth interfaces that lost their bridge master and reconnect them to docker0:

```go
func (m *Manager) reconnectOrphanedVeths() {
    // List all veth interfaces
    output, _ := exec.Command("ip", "-o", "link", "show", "type", "veth").Output()

    for _, line := range strings.Split(string(output), "\n") {
        // Skip hydra veths (vethb-*) - they belong to session bridges
        if strings.Contains(line, "vethb-") {
            continue
        }

        // Check if veth has a master
        if !strings.Contains(line, "master") {
            // Orphaned veth - reconnect to docker0
            vethName := extractVethName(line)
            m.runCommand("ip", "link", "set", vethName, "master", "docker0")
        }
    }
}
```

## Approaches Tested

### Approach 1: `--iptables=false` with Manual NAT (Reverted)

Added `--iptables=false` to session dockerd to prevent it from touching global iptables state, then manually set up NAT rules:

```go
cmd := exec.Command("dockerd",
    "--host=unix://"+socketPath,
    "--iptables=false",  // Don't touch global iptables
    "--bridge="+bridgeName,
    // ...
)

// Manually add NAT rules
m.runCommand("iptables", "-t", "nat", "-A", "POSTROUTING",
    "-s", bridgeSubnet, "!", "-o", bridgeName, "-j", "MASQUERADE")
```

**Result:** More isolated but complex. Essentially reimplements part of Docker's networking stack. Reverted in favor of simpler reactive approach.

### Approach 2: `--ip-forward-no-drop` (Docker 28+)

Used `--ip-forward-no-drop` flag which prevents Docker from setting FORWARD policy to DROP:

```go
cmd := exec.Command("dockerd",
    "--host=unix://"+socketPath,
    "--ip-forward-no-drop",  // Don't touch FORWARD policy
    "--bridge="+bridgeName,
    // ...
)
```

**Result:** docker0 was preserved (not deleted), but internet connectivity broke in containers. Cause unknown - possibly Docker 29 implementation issue. Reverted.

### Approach 3: Reactive Restoration (Current)

Let Docker manage its own networking, but detect and fix docker0 deletion when it happens:

```go
cmd := exec.Command("dockerd",
    "--host=unix://"+socketPath,
    "--bridge="+bridgeName,
    // No special flags - Docker manages its own iptables
)

// After dockerd starts, check and restore if needed
if docker0ExistsBefore && !docker0ExistsAfter {
    m.EnsureDocker0Bridge()
}
```

**Result:** Simpler, works with Docker's default networking. Fixes the symptom rather than the root cause, but is more robust.

## Testing

1. Start a session and check logs for docker0 status:
   ```bash
   docker compose logs sandbox-nvidia | grep docker0
   ```

2. Verify helix-buildkit connectivity after session start:
   ```bash
   docker compose exec sandbox-nvidia docker exec helix-buildkit ping -c1 8.8.8.8
   ```

3. Verify internet works from desktop container:
   ```bash
   docker compose exec sandbox-nvidia docker exec ubuntu-external-XXX curl -I https://google.com
   ```

## Bug Fix: eth0 Attached to docker0

During testing, we discovered that `reconnectOrphanedVeths()` was incorrectly attaching the sandbox's `eth0` interface to docker0. This broke internet connectivity because:

1. Inside the sandbox container, `ip link show type veth` shows eth0 as a veth (it's one end of a veth pair to the host)
2. eth0 has no master bridge (which is correct - it's the container's external interface)
3. The function saw it as an "orphaned veth" and attached it to docker0
4. This broke routing - traffic to the internet was routed through docker0 instead of the host

**Fix:** Added explicit check to skip interfaces named `eth*` or `lo`:

```go
// Skip container interfaces like eth0, eth1, lo - these are NOT orphaned veths
if strings.HasPrefix(vethName, "eth") || vethName == "lo" {
    continue
}
```

## Open Questions

1. **Why does session dockerd sometimes delete docker0?** The exact mechanism is unclear. It may be related to iptables chain cleanup or bridge management.

2. **Why does `--ip-forward-no-drop` break internet?** Docker 28+ added this flag specifically for multi-daemon scenarios, but it doesn't work as expected in Docker 29.

3. **Is there a proper prevention approach?** The reactive fix works but is defensive. A proper prevention approach would be cleaner.

## Related

- `design/2026-01-25-shared-buildkit-cache.md` - BuildKit cache feature that discovered this issue
- `design/2026-02-02-docker-network-isolation-fix.md` - Docker network subnet isolation (separate issue)
- `desktop/docker-shim/docker.go` - Docker shim that injects BuildKit cache flags
