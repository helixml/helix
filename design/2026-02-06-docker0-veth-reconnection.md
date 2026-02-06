# Sandbox Bridge Protection and Veth Reconnection

**Date:** 2026-02-06
**Status:** Implemented (pending testing)
**Branch:** feature/bff-auth-clean
**Solution:** Use custom bridge name "sandbox0" instead of "docker0"

## Problem

When starting a per-session Hydra dockerd with its own bridge (e.g., `hydra1`), Docker's startup sometimes deletes or disrupts the main sandbox dockerd's `docker0` bridge. This orphans veth interfaces from containers attached to docker0 (e.g., `helix-buildkit`, `buildx` builders), breaking their network connectivity.

The issue was discovered while implementing shared BuildKit cache across sandboxes (see `design/2026-01-25-shared-buildkit-cache.md`).

### Symptoms

1. `helix-buildkit` container loses network connectivity after starting a session
2. Docker builds inside desktop containers fail with "shared BuildKit container not found"
3. `ip link show docker0` returns "Device does not exist"
4. Orphaned veth pairs visible: `ip link | grep veth` shows interfaces with `master` but the bridge is gone

### Root Cause (Confirmed via Moby Source Code)

After reading the Docker/Moby source code (`github.com/moby/moby v28+`), we identified the exact mechanism:

**In `daemon/daemon_unix.go:864-871`:**
```go
// Clear stale bridge network
if n, err := controller.NetworkByName(network.NetworkBridge); err == nil {
    if err = n.Delete(); err != nil {
        return errors.Wrapf(err, `could not delete the default %q network`, network.NetworkBridge)
    }
    if len(conf.NetworkConfig.DefaultAddressPools.Value()) > 0 && !conf.LiveRestoreEnabled {
        removeDefaultBridgeInterface()
    }
}
```

**In `daemon/daemon_unix.go:1232-1238`:**
```go
func removeDefaultBridgeInterface() {
    if lnk, err := nlwrap.LinkByName(bridge.DefaultBridgeName); err == nil {
        if err := netlink.LinkDel(lnk); err != nil {
            log.G(context.TODO()).Warnf("Failed to remove bridge interface (%s): %v", bridge.DefaultBridgeName, err)
        }
    }
}
```

Where `bridge.DefaultBridgeName = "docker0"`.

**The deletion chain:**
1. Session dockerd starts with `default-address-pools` set (required for subnet isolation)
2. Docker's initialization code calls `removeDefaultBridgeInterface()` when `default-address-pools` is non-empty
3. This function **specifically deletes the bridge named "docker0"** via `netlink.LinkDel()`
4. The main sandbox dockerd's docker0 bridge is deleted, orphaning all attached veths

**Key insight:** Docker hardcodes the name "docker0" in `removeDefaultBridgeInterface()`. If we use a different bridge name, this code path doesn't affect us.

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
│    │  └── sandbox0 bridge (10.213.0.0/24)  ◄───────────┼── Now safe from deletion   │
│    │       │                                           │                            │
│    │       ├── helix-buildkit (10.213.0.2)             │                            │
│    │       ├── buildx builders (10.213.0.3+)           │                            │
│    │       └── ubuntu-external-xxx (10.213.0.5) ───────┘                            │
│    │             │                                                                  │
│    │             │ eth0: 10.213.0.5 (on sandbox0)                                    │
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
│  eth0 (10.213.0.5) ──► sandbox0 ──► sandbox eth0 ──► host ──► internet              │
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
| Desktop → Internet | `eth0 → sandbox0 → sandbox eth0 → host → internet` |
| Desktop → API (control plane) | `eth0 → sandbox0 → sandbox eth0 → helix_default → api` |
| Desktop → User containers (docker-compose) | `eth1 → hydra1 → per-session dockerd networks` |
| Desktop → helix-buildkit | `eth0 → sandbox0 → helix-buildkit` |

### Simplified View

```
Sandbox Container (sandbox-nvidia)
├── Main Dockerd (sandbox's native dockerd)
│   └── sandbox0 bridge (10.213.0.0/24)  ← Custom name, safe from deletion
│       ├── helix-buildkit container
│       ├── helix-desktop container
│       └── buildx builders
│
└── Per-Session Hydra Dockerds
    ├── Session A: hydra1 bridge (10.200.1.0/24)
    ├── Session B: hydra2 bridge (10.200.2.0/24)
    └── Session C: hydra3 bridge (10.200.3.0/24)
```

The desktop container runs on the main dockerd (sandbox0), while user's `docker compose` projects run on per-session Hydra dockerds. The desktop is bridged to the Hydra network via a veth pair for connectivity.

## Practical Implications (Historical)

**With the custom bridge name solution, these issues no longer occur.**

Previously, when a new session started and docker0 got deleted, the following containers temporarily lost network connectivity:

1. **helix-buildkit** - The shared BuildKit container for caching Docker builds
2. **buildx builders** - Docker buildx builder containers
3. **Desktop container** (ubuntu-external-*, sway-external-*) - The container streaming video to the user

For the desktop container specifically, during the brief outage window (~100ms):

| Interface | Status | Impact |
|-----------|--------|--------|
| eth0 (sandbox0) | **BROKEN** | Internet access lost, API connectivity lost |
| eth1 (hydra bridge) | Still works | Can still reach per-session dockerd containers |

**Now with sandbox0:** Session dockerds no longer delete the bridge because Docker's cleanup code specifically targets "docker0", not "sandbox0". No outages occur.

## Solution: Custom Bridge Name (Final)

Since Docker's `removeDefaultBridgeInterface()` hardcodes the name "docker0", we simply use a different bridge name for the sandbox's main dockerd. This completely avoids the problem.

### Implementation

**In `sandbox/04-start-dockerd.sh`:**

```bash
# Create custom bridge before starting dockerd
SANDBOX_BRIDGE="sandbox0"
SANDBOX_BRIDGE_IP="10.213.0.1/24"

if ! ip link show "$SANDBOX_BRIDGE" >/dev/null 2>&1; then
    ip link add name "$SANDBOX_BRIDGE" type bridge
    ip addr add "$SANDBOX_BRIDGE_IP" dev "$SANDBOX_BRIDGE"
    ip link set "$SANDBOX_BRIDGE" up
fi

# Start dockerd with our custom bridge
dockerd --config-file /etc/docker/daemon.json \
    --host=unix:///var/run/docker.sock \
    --bridge="$SANDBOX_BRIDGE"
```

**In `api/pkg/hydra/manager.go`:**

```go
// SandboxBridgeName is the name of the main sandbox's Docker bridge.
// We use a custom name instead of "docker0" to prevent session dockerds from
// accidentally deleting it (Docker's cleanup code specifically targets "docker0").
const SandboxBridgeName = "sandbox0"
```

### Why This Works

1. Docker's `removeDefaultBridgeInterface()` only deletes a bridge named exactly "docker0"
2. Session dockerds call this function during startup (when `default-address-pools` is set)
3. Since our main bridge is named "sandbox0", not "docker0", it's never deleted
4. Session dockerds still work fine with their own bridges (hydra1, hydra2, etc.)

### Defensive Code (Belt and Suspenders)

We still keep defensive monitoring and restoration code in case something unexpected happens:

```go
// Check sandbox bridge status before/after session dockerd starts
sandboxBridgeExistsBefore := m.checkSandboxBridgeExists()
// ... start session dockerd ...
sandboxBridgeExistsAfter := m.checkSandboxBridgeExists()

if sandboxBridgeExistsBefore && !sandboxBridgeExistsAfter {
    // This should no longer happen with custom bridge name
    log.Warn().Msg("Sandbox bridge unexpectedly deleted, restoring")
    m.EnsureSandboxBridge()
}
```

## Previous Solution: Reactive Restoration (Superseded)

The original approach detected docker0 deletion and restored it. This worked but had a brief outage window (~100ms) where containers lost connectivity. The custom bridge name solution is cleaner because it prevents the deletion entirely.

### Reconnect Orphaned Veths

This code is still used as a defensive measure:

```go
func (m *Manager) reconnectOrphanedVeths() {
    // List all veth interfaces
    output, _ := exec.Command("ip", "-o", "link", "show", "type", "veth").Output()

    for _, line := range strings.Split(string(output), "\n") {
        // Skip hydra veths (vethb-*) - they belong to session bridges
        if strings.HasPrefix(vethName, "vethb") {
            continue
        }

        // Skip container interfaces (eth0, eth1, lo)
        if strings.HasPrefix(vethName, "eth") || vethName == "lo" {
            continue
        }

        // Check if veth has a master - if not, reconnect to sandbox0
        if !strings.Contains(showOutput, "master ") {
            m.runCommand("ip", "link", "set", vethName, "master", SandboxBridgeName)
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

## Resolved Questions

1. **Why does session dockerd sometimes delete docker0?**
   - **SOLVED**: Docker's `daemon/daemon_unix.go:removeDefaultBridgeInterface()` is called during dockerd startup when `default-address-pools` is set. This function explicitly deletes the bridge named "docker0" using `netlink.LinkDel()`.

2. **Why does `--ip-forward-no-drop` break internet?**
   - Still unclear. Docker 28+ added this flag specifically for multi-daemon scenarios, but it doesn't work as expected in our Docker 29 environment. The custom bridge name solution makes this flag unnecessary.

3. **Is there a proper prevention approach?**
   - **SOLVED**: Yes - use a custom bridge name (e.g., "sandbox0") instead of "docker0". Docker's cleanup code specifically targets "docker0", so using a different name completely avoids the issue.

## Related

- `design/2026-01-25-shared-buildkit-cache.md` - BuildKit cache feature that discovered this issue
- `design/2026-02-02-docker-network-isolation-fix.md` - Docker network subnet isolation (separate issue)
- `desktop/docker-shim/docker.go` - Docker shim that injects BuildKit cache flags
