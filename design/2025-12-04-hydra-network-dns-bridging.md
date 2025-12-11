# Hydra Network and DNS Bridging: Desktop to Dev Container Communication

**Date:** 2025-12-04
**Status:** Design - APPROVED APPROACH
**Author:** Claude

## Problem Statement

When using Hydra for Docker isolation, the **desktop container** (where the user's browser and Zed run) cannot access services running in the user's **dev containers** (started via `docker compose up` in the desktop terminal).

### Concrete Example

1. User starts a SpecTask session
2. Desktop container (helix-sway with Firefox, Zed) runs on **Wolf's DinD dockerd**
3. User opens terminal in desktop and runs `docker compose up` for a web app on port 3000
4. The web app container runs on **Hydra's isolated dockerd** (sibling to Wolf's dockerd)
5. User opens Firefox and navigates to `http://my-webapp:3000`
6. **FAILS** because:
   - Firefox is on Wolf's network (172.20.0.X)
   - my-webapp is on Hydra's network (10.200.Y.Z)
   - No DNS resolution for `my-webapp`
   - No network routing between the two networks

### Design Constraints

1. **Tenant isolation:** Multiple sessions can share a sandbox. Session A must NOT be able to reach Session B's dev containers.
2. **Wolf simplicity:** Wolf should only manage one dockerd. We don't want Wolf provisioning containers into multiple Docker instances.
3. **Security:** Keeping desktop containers on a separate dockerd from user workloads provides better isolation.

## Correct Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│ HOST MACHINE (Docker - helix_default 172.19.0.0/16)                             │
│                                                                                  │
│   ┌────────────────────────┐                                                    │
│   │ API Container          │                                                    │
│   │ 172.19.0.20            │                                                    │
│   └────────────────────────┘                                                    │
│                                                                                  │
│   ┌─────────────────────────────────────────────────────────────────────────┐   │
│   │ SANDBOX CONTAINER (172.19.0.50)                                          │   │
│   │                                                                          │   │
│   │   Processes running directly in sandbox:                                 │   │
│   │   ┌──────────┐  ┌───────────────┐  ┌──────────────────────┐             │   │
│   │   │ Wolf     │  │ Moonlight Web │  │ Hydra Daemon         │             │   │
│   │   │ (stream) │  │ (WebRTC)      │  │ (multi-docker mgmt)  │             │   │
│   │   └──────────┘  └───────────────┘  └──────────────────────┘             │   │
│   │                                                                          │   │
│   │   ═══════════════════════════════════════════════════════════════════   │   │
│   │                                                                          │   │
│   │   Wolf's dockerd                      Hydra's isolated dockerd          │   │
│   │   /var/run/docker.sock                /var/run/hydra/{id}/docker.sock   │   │
│   │   Network: helix_default              Network: hydra{N} bridge          │   │
│   │   Subnet: 172.20.0.0/16               Subnet: 10.200.{N}.0/24           │   │
│   │                                                                          │   │
│   │   ┌───────────────────────┐           ┌───────────────────────────┐     │   │
│   │   │ Desktop Container     │           │ User's Dev Containers     │     │   │
│   │   │ (helix-sway)          │           │                           │     │   │
│   │   │                       │           │ ┌─────────────────────┐   │     │   │
│   │   │ - Firefox browser ────┼───────?───┼→│ my-webapp:3000      │   │     │   │
│   │   │ - Zed editor          │   Can't   │ └─────────────────────┘   │     │   │
│   │   │ - Terminal            │   reach!  │                           │     │   │
│   │   │                       │           │ ┌─────────────────────┐   │     │   │
│   │   │ 172.20.0.X           │           │ │ postgres:5432       │   │     │   │
│   │   └───────────────────────┘           │ └─────────────────────┘   │     │   │
│   │                                       │                           │     │   │
│   │                                       │ 10.200.{N}.X              │     │   │
│   │                                       └───────────────────────────┘     │   │
│   │                                                                          │   │
│   └─────────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Key Insight: Two Sibling dockerd Instances

Inside the sandbox container, there are **two separate Docker daemons** running:

1. **Wolf's primary dockerd** (`/var/run/docker.sock`)
   - Subnet: 172.20.0.0/16 (helix_default network created in `04-start-dockerd.sh`)
   - Runs: Desktop containers (helix-sway) spawned by Wolf for streaming

2. **Hydra's per-session dockerd** (`/var/run/hydra/{scope}/docker.sock`)
   - Subnet: 10.200.{N}.0/24 (unique per session, managed in `manager.go`)
   - Runs: User's development containers (started via Docker CLI in desktop)

The desktop container gets Hydra's socket mounted at `/var/run/docker.sock`, so when the user runs `docker compose up`, containers start on **Hydra's** dockerd, not Wolf's.

## What Needs to Work

From the desktop container (on 172.20.0.X), users need to:

1. **DNS Resolution**: `http://my-webapp:3000` should resolve to the container's IP (10.200.N.Y)
2. **Network Routing**: Packets should be able to reach 10.200.N.0/24 from 172.20.0.0/16

## Approved Solution: API-Orchestrated Veth Bridging

### Why This Approach

We considered several alternatives:

1. **Desktop on Hydra's dockerd** - Would make networking trivial, but Wolf only manages one dockerd and we don't want to change that. Also provides better security isolation keeping desktop separate from user workloads.

2. **Sandbox-level bridging** - Would break tenant isolation (Session A could reach Session B's containers).

3. **Host network mode** - No isolation, port conflicts, security concerns.

The approved approach: **API orchestrates per-session veth injection via Hydra**.

### Architecture

```
SANDBOX CONTAINER
│
├── Wolf's dockerd (/var/run/docker.sock)
│   │   Network: helix_default (172.20.0.0/16)
│   │
│   ├── Desktop A (Session A) ─── eth0: 172.20.0.X
│   │                          └── eth1: 10.200.1.254 ←─┐
│   │                                                    │ veth pair
│   ├── Desktop B (Session B) ─── eth0: 172.20.0.Y      │
│   │                          └── eth1: 10.200.2.254   │
│   │                                                    │
├── Hydra dockerd for Session A                          │
│   │   Network: hydra1 (10.200.1.0/24)                 │
│   │   Bridge: hydra1 ←─────────────────────────────────┘
│   │
│   ├── webapp (10.200.1.2)
│   └── postgres (10.200.1.3)
│
└── Hydra dockerd for Session B
    │   Network: hydra2 (10.200.2.0/24)
    │
    ├── my-app (10.200.2.2)
    └── redis (10.200.2.3)
```

**Key points:**
- Each desktop gets a second interface (eth1) on its specific Hydra network
- Desktop A can reach 10.200.1.0/24 but NOT 10.200.2.0/24
- Perfect tenant isolation maintained

### Orchestration Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                         HELIX API                                    │
│                    (wolf_executor.go)                                │
└─────────────────────────────────────────────────────────────────────┘
                │                    │                    │
                ▼                    ▼                    ▼
         ┌──────────┐         ┌──────────┐         ┌──────────┐
         │  STEP 1  │         │  STEP 2  │         │  STEP 3  │
         └──────────┘         └──────────┘         └──────────┘
              │                    │                    │
              ▼                    ▼                    ▼
    ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
    │     HYDRA       │  │      WOLF       │  │     HYDRA       │
    │                 │  │                 │  │                 │
    │ CreateDockerD   │  │ CreateDesktop   │  │ BridgeDesktop   │
    │                 │  │                 │  │                 │
    │ Returns:        │  │ Returns:        │  │ Creates veth    │
    │ - bridge name   │  │ - container ID  │  │ Injects into    │
    │ - subnet        │  │                 │  │ desktop netns   │
    │ - gateway IP    │  │                 │  │ Configures DNS  │
    └─────────────────┘  └─────────────────┘  └─────────────────┘
```

### Implementation Details

#### Step 1: Hydra CreateDockerInstance (existing, needs extension)

Already returns docker instance info. Need to also return:

```go
type DockerInstance struct {
    ID          string
    SocketPath  string
    BridgeName  string   // e.g., "hydra3"
    Subnet      string   // e.g., "10.200.3.0/24"
    Gateway     string   // e.g., "10.200.3.1"
}
```

#### Step 2: Wolf CreateDesktop (existing)

No changes needed. Returns container ID as it does now.

#### Step 3: Hydra BridgeDesktop (NEW)

New Hydra API endpoint:

```go
// POST /api/v1/bridge-desktop
type BridgeDesktopRequest struct {
    SessionID          string `json:"session_id"`           // Which Hydra dockerd
    DesktopContainerID string `json:"desktop_container_id"` // Container on Wolf's dockerd
}

type BridgeDesktopResponse struct {
    DesktopIP   string `json:"desktop_ip"`   // IP assigned (e.g., "10.200.3.254")
    Gateway     string `json:"gateway"`      // Gateway/DNS (e.g., "10.200.3.1")
    Interface   string `json:"interface"`    // Interface name (e.g., "eth1")
}
```

**Hydra BridgeDesktop implementation:**

```go
func (h *Hydra) BridgeDesktop(req *BridgeDesktopRequest) (*BridgeDesktopResponse, error) {
    // 1. Get the Hydra instance for this session
    instance := h.instances[req.SessionID]
    bridgeName := instance.BridgeName  // e.g., "hydra3"
    subnet := instance.Subnet          // e.g., "10.200.3.0/24"

    // 2. Get desktop container's PID from Wolf's dockerd
    wolfDockerClient := docker.NewClient("/var/run/docker.sock")
    containerInfo, _ := wolfDockerClient.ContainerInspect(req.DesktopContainerID)
    desktopPID := containerInfo.State.Pid

    // 3. Generate unique veth names and desktop IP
    vethDesktop := fmt.Sprintf("veth-d-%s", req.SessionID[:8])
    vethBridge := fmt.Sprintf("veth-b-%s", req.SessionID[:8])
    desktopIP := calculateDesktopIP(subnet)  // e.g., "10.200.3.254"

    // 4. Create veth pair
    exec.Command("ip", "link", "add", vethDesktop, "type", "veth", "peer", "name", vethBridge).Run()

    // 5. Attach bridge-side veth to Hydra's bridge
    exec.Command("ip", "link", "set", vethBridge, "master", bridgeName).Run()
    exec.Command("ip", "link", "set", vethBridge, "up").Run()

    // 6. Move desktop-side veth into container's network namespace
    exec.Command("ip", "link", "set", vethDesktop, "netns", strconv.Itoa(desktopPID)).Run()

    // 7. Configure the interface inside the container
    nsenter := func(args ...string) {
        cmd := append([]string{"-t", strconv.Itoa(desktopPID), "-n"}, args...)
        exec.Command("nsenter", cmd...).Run()
    }
    nsenter("ip", "link", "set", vethDesktop, "name", "eth1")
    nsenter("ip", "addr", "add", desktopIP+"/24", "dev", "eth1")
    nsenter("ip", "link", "set", "eth1", "up")

    // 8. Configure DNS (add Hydra's DNS server to resolv.conf)
    // Option A: Add to /etc/resolv.conf
    // Option B: Run dnsmasq on the bridge gateway

    return &BridgeDesktopResponse{
        DesktopIP: desktopIP,
        Gateway:   instance.Gateway,
        Interface: "eth1",
    }, nil
}
```

#### DNS Resolution

**Option A: dnsmasq on each Hydra bridge**

Hydra runs dnsmasq listening on the gateway IP (e.g., 10.200.3.1):

```bash
# When Hydra creates dockerd, also start dnsmasq
dnsmasq \
    --interface=hydra3 \
    --listen-address=10.200.3.1 \
    --bind-interfaces \
    --no-resolv \
    --server=8.8.8.8 \
    --server=8.8.4.4
```

Then configure dnsmasq to resolve Docker container names by querying the Hydra dockerd's API.

**Option B: Docker's embedded DNS**

Configure the desktop's eth1 to use Docker's embedded DNS at 127.0.0.11 (but this is tricky since it's in the Hydra dockerd's context, not the desktop's).

**Option C: Custom DNS proxy in Hydra**

Hydra runs a small DNS server that:
1. Listens on each bridge gateway
2. For queries, inspects Hydra's dockerd containers
3. Returns container IPs for matching names
4. Forwards other queries upstream

This is the cleanest option as it integrates directly with Hydra's container knowledge.

### API Integration (wolf_executor.go)

```go
func (e *WolfExecutor) createSessionWithNetwork(ctx context.Context, session *Session) error {
    // Step 1: Create Hydra dockerd
    hydraInstance, err := e.hydraClient.CreateDockerInstance(ctx, &hydra.CreateRequest{
        Scope: session.ID,
    })
    if err != nil {
        return fmt.Errorf("failed to create Hydra dockerd: %w", err)
    }

    // Step 2: Create desktop container via Wolf
    desktopContainerID, err := e.createSwayWolfApp(ctx, session, hydraInstance)
    if err != nil {
        return fmt.Errorf("failed to create desktop: %w", err)
    }

    // Step 3: Bridge desktop to Hydra network
    bridgeResp, err := e.hydraClient.BridgeDesktop(ctx, &hydra.BridgeDesktopRequest{
        SessionID:          session.ID,
        DesktopContainerID: desktopContainerID,
    })
    if err != nil {
        return fmt.Errorf("failed to bridge desktop to Hydra network: %w", err)
    }

    log.Info().
        Str("session_id", session.ID).
        Str("desktop_ip", bridgeResp.DesktopIP).
        Str("gateway", bridgeResp.Gateway).
        Msg("Desktop bridged to Hydra network")

    return nil
}
```

### Lifecycle Management

#### Desktop Container Restart

If Wolf restarts the desktop container:
1. Old container's netns is destroyed, taking veth end with it
2. Bridge-side veth becomes orphaned
3. API must call `BridgeDesktop` again after Wolf recreates container

**Solution:** Add cleanup in Hydra:
```go
func (h *Hydra) CleanupOrphanedVeths(sessionID string) {
    vethBridge := fmt.Sprintf("veth-b-%s", sessionID[:8])
    exec.Command("ip", "link", "del", vethBridge).Run()
}
```

Call this before `BridgeDesktop` to ensure clean state.

#### Session End

When session ends:
1. API calls Wolf to stop desktop → container destroyed, veth end gone
2. API calls Hydra to destroy dockerd → bridge destroyed, other veth end gone
3. Automatic cleanup, no orphans

#### Hydra Dockerd Restart

Edge case: if Hydra's dockerd for a session crashes and restarts:
1. Bridge is recreated with same name
2. Need to re-attach the veth to the new bridge
3. This is rare - for now, document as known limitation

### Testing Plan

1. **Unit tests for Hydra BridgeDesktop:**
   - Mock container inspect
   - Verify correct ip commands generated

2. **Integration test:**
   ```bash
   # From desktop container
   ping 10.200.3.2  # Dev container IP
   curl http://webapp:3000  # DNS resolution
   docker ps  # See dev containers
   ```

3. **Isolation test:**
   ```bash
   # From desktop A
   ping 10.200.2.2  # Session B's container - should FAIL
   ```

### Files to Modify

1. **api/pkg/hydra/types.go** - Add BridgeDesktopRequest/Response
2. **api/pkg/hydra/server.go** - Add BridgeDesktop endpoint
3. **api/pkg/hydra/manager.go** - Implement veth injection logic
4. **api/pkg/hydra/dns.go** (new) - DNS proxy for container resolution
5. **api/pkg/external-agent/wolf_executor.go** - Orchestrate the 3-step flow

### Open Questions (Resolved)

1. ~~Desktop on Hydra's dockerd?~~ No - Wolf needs single dockerd, better security separation.
2. ~~Sandbox-level bridging?~~ No - breaks tenant isolation.
3. ~~Userspace proxy?~~ No - user wants proper L3/L4 with DNS.

### Decisions (Finalized)

1. **DNS implementation:** Custom Go DNS server using `miekg/dns` library
   - Queries Hydra's dockerd API for container name → IP resolution
   - Listens on bridge gateway IP (e.g., 10.200.3.1:53)
   - Forwards non-container queries to upstream DNS (8.8.8.8)
   - Single server can handle multiple bridges

2. **IP allocation:** Static .254 for desktop (e.g., 10.200.3.254)
   - Simple, predictable
   - One desktop per Hydra network anyway

3. **IPv6:** Not needed, IPv4 only

### Future Enhancements

#### Desktop-to-Desktop Isolation

**Current state:** All desktop containers run on Wolf's shared `helix_default` network (172.20.0.0/16). Containers on the same Docker bridge can reach each other by default.

**Risk:** A malicious tenant could port scan or attempt to connect to other desktop containers.

**Mitigation (future):** Create a separate network per desktop in Wolf's dockerd:
- Desktop A on `desktop-a-net` (172.20.1.0/24)
- Desktop B on `desktop-b-net` (172.20.2.0/24)
- No route between them, perfect isolation

**Implementation:** When Wolf creates a desktop, also create an isolated network for it. The veth to Hydra provides the only external connectivity.

**Priority:** Medium - depends on threat model and whether desktops expose any listening services.

## Implementation Status (2025-12-07)

The core implementation is complete. Files modified:

1. **api/pkg/hydra/types.go** - Added BridgeDesktopRequest/Response, extended DockerInstance with bridge state
2. **api/pkg/hydra/server.go** - Added `/api/v1/bridge-desktop` endpoint
3. **api/pkg/hydra/manager.go** - Implemented BridgeDesktop veth injection with self-healing
4. **api/pkg/hydra/client.go** - Added BridgeDesktop client methods for both Client and RevDialClient
5. **api/pkg/hydra/dns.go** (new) - Custom DNS server using miekg/dns
6. **api/pkg/external-agent/wolf_executor.go** - Calls BridgeDesktop after lobby creation

### Robustness Features Implemented

#### 1. Retry Logic for Container Readiness
Wolf starts containers asynchronously. BridgeDesktop now retries up to 10 times with exponential backoff (500ms, 1s, 1.5s, ...) waiting for the container to be running.

#### 2. Duplicate Bridge Prevention
Bridge state is tracked in DockerInstance:
- `DesktopBridged` - whether desktop is currently bridged
- `DesktopContainerID` - container ID/name that was bridged
- `DesktopPID` - PID of bridged container (for detecting restart)
- `VethBridgeName` - bridge-side veth name for cleanup

If BridgeDesktop is called with the same container and same PID, it returns cached response.

#### 3. Container Restart Detection (Self-Healing)
If container PID changes (indicating Wolf recreated the container), BridgeDesktop:
1. Detects PID mismatch
2. Cleans up orphaned veth from old container
3. Creates new veth pair for new container
4. Updates bridge state

#### 4. Orphaned Veth Cleanup
Before creating new veth pair, cleanupOrphanedVeth() checks if veth already exists and deletes it. Handles crashes and unexpected restarts.

#### 5. DNS Server Lifecycle
DNS servers are started when Hydra dockerd starts and stopped when it stops, via startDNSForInstance() and stopDNSForInstance() hooks.

### Limits and Capacity

#### Bridge Index Range
- Range: 1-254 (254 max concurrent sessions per sandbox)
- Subnet: 10.200.{N}.0/24 per session
- When sessions end, bridge indices are returned to the pool for reuse
- This is unlikely to be a bottleneck - 254 concurrent sessions per sandbox is far more than expected

#### Resource Cleanup on Session End
When a session ends:
1. Desktop container destroyed → veth end automatically destroyed with network namespace
2. Hydra dockerd stopped → bridge destroyed → remaining veth orphaned
3. Orphaned veth cleaned up on next session start (if same index reused) or sandbox restart

### Known Limitations

1. **Hydra Restart:** If Hydra process restarts, in-memory bridge state is lost. Bridge indices will be recovered from existing interfaces, but desktop bridging state is lost. Next BridgeDesktop call will detect PID mismatch and re-bridge.

2. **Bridge Index Exhaustion:** If 254+ concurrent sessions exist in one sandbox, new sessions will fail. This is extremely unlikely given expected load.

### Critical Fixes Applied (2025-12-07)

During implementation review, several issues were identified and fixed:

#### 1. Bridge Creation (Fixed)
**Problem:** Dockerd was configured with `--bip` only, which sets the IP on docker0, not a custom bridge. Multiple dockerd instances would conflict on docker0.

**Fix:** Added explicit bridge creation with `ip link add hydra{N} type bridge` before starting dockerd, then pass `--bridge=hydra{N}` to dockerd. Each Hydra instance now has its own isolated bridge.

#### 2. DNS Server Initialization (Fixed)
**Problem:** DNS server code existed but was never instantiated. Container name resolution would fail.

**Fix:** `NewDNSServer()` is now called in `NewServer()` and passed to the manager via `SetDNSServer()`. DNS servers are started for each dockerd instance.

#### 3. Enterprise DNS Passthrough (Added)
**Problem:** DNS was hardcoded to Google DNS (8.8.8.8), breaking enterprise deployments with internal DNS servers.

**Fix:** DNS configuration is now inherited from sandbox's `/etc/resolv.conf`. Both the Hydra DNS proxy and the dockerd daemon.json use the sandbox's nameservers. Enterprise internal DNS (intranet TLDs, internal services) now works correctly.

#### 4. Privileged Mode Routing (Fixed)
**Problem:** In privileged mode, assigning `172.17.255.254/16` auto-creates a direct route, causing the subsequent gateway route to fail.

**Fix:** Changed to `/32` prefix for IP assignment, then explicitly add point-to-point route to sandbox gateway, then add subnet route via gateway.

#### 5. Veth Naming Collisions (Fixed)
**Problem:** Veth names used truncated 8-char session IDs, risking collisions. Linux interface names are limited to 15 characters.

**Fix:** Normal mode uses bridge index (`vethd-h{N}`, `vethb-h{N}`) which is guaranteed unique per instance. Privileged mode uses container PID (`vethd-p{PID}`, `veths-p{PID}`) which is unique at runtime.

#### 6. Iptables Cleanup (Fixed)
**Problem:** MASQUERADE rules in privileged mode accumulated without cleanup.

**Fix:** Before adding a new rule, delete any existing rule with the same parameters (idempotent). This prevents rule accumulation across session restarts.

---

## Port Forwarding: Making localhost:PORT Work

### Problem

When users run `docker run -p 8080:8080` in their terminal, they expect `localhost:8080` to work in Firefox. However:

1. The dev container runs on Hydra's dockerd
2. Docker binds port 8080 on the Hydra bridge gateway (10.200.N.1)
3. The desktop's localhost (127.0.0.1) is a different network namespace
4. `localhost:8080` fails in the desktop

### Solution: Port Forwarding via iptables NAT

When bridging the desktop, we also set up iptables rules to redirect localhost traffic to the Hydra gateway:

```bash
# Inside desktop container's network namespace
# Redirect localhost:8080 to Hydra gateway 10.200.3.1:8080
nsenter -t $DESKTOP_PID -n iptables -t nat -A OUTPUT \
    -p tcp -d 127.0.0.1 --dport 8080 \
    -j DNAT --to-destination 10.200.3.1:8080
```

### Design

#### Dynamic Port Detection
Poll Hydra's dockerd for exposed ports:

```go
func (m *Manager) GetExposedPorts(sessionID string) ([]PortMapping, error) {
    client := docker.NewClientWithOpts(docker.WithHost("unix://" + inst.SocketPath))
    containers, _ := client.ContainerList(ctx, container.ListOptions{})

    var ports []PortMapping
    for _, c := range containers {
        for _, p := range c.Ports {
            if p.PublicPort != 0 {
                ports = append(ports, PortMapping{
                    HostPort:      p.PublicPort,
                    ContainerPort: p.PrivatePort,
                    Protocol:      p.Type, // "tcp" or "udp"
                })
            }
        }
    }
    return ports, nil
}
```

#### Port Forwarding Setup
Add NAT rules for each exposed port:

```go
func (m *Manager) setupPortForwarding(containerPID int, gateway string, ports []PortMapping) error {
    for _, p := range ports {
        // Redirect localhost:port -> gateway:port
        err := m.runNsenter(containerPID,
            "iptables", "-t", "nat", "-A", "OUTPUT",
            "-p", p.Protocol, "-d", "127.0.0.1", "--dport", fmt.Sprintf("%d", p.HostPort),
            "-j", "DNAT", "--to-destination", fmt.Sprintf("%s:%d", gateway, p.HostPort))
        if err != nil {
            log.Warn().Err(err).Int("port", p.HostPort).Msg("Failed to setup port forward")
        }
    }
    return nil
}
```

#### Continuous Port Sync
Ports can change as users start/stop containers. Options:

**Option A: One-time Setup (Simplest)**
- Set up port forwarding at bridge time
- Users must re-connect if they start containers with new exposed ports
- Simple but limited UX

**Option B: Periodic Polling**
- Hydra polls exposed ports every 5-10 seconds
- Adds/removes iptables rules as ports change
- Better UX but adds complexity and potential for stale rules

**Option C: Docker Event Watcher**
- Hydra watches Docker events for container start/stop
- Updates iptables rules in real-time
- Best UX, moderate complexity

**Recommended:** Start with Option A (one-time) for MVP. Users can trigger re-bridge by reconnecting. Add Option C as enhancement.

#### Integration with BridgeDesktop

```go
func (m *Manager) BridgeDesktop(ctx context.Context, req *BridgeDesktopRequest) (*BridgeDesktopResponse, error) {
    // ... existing veth bridging code ...

    // 10. Setup port forwarding for exposed ports
    ports, err := m.GetExposedPorts(req.SessionID)
    if err != nil {
        log.Warn().Err(err).Msg("Failed to get exposed ports (port forwarding unavailable)")
    } else if len(ports) > 0 {
        if err := m.setupPortForwarding(containerPID, gateway, ports); err != nil {
            log.Warn().Err(err).Msg("Failed to setup port forwarding")
        }
        log.Info().
            Interface("ports", ports).
            Msg("Port forwarding configured for exposed ports")
    }

    return &BridgeDesktopResponse{...}, nil
}
```

### User Experience

After bridging:

```bash
# In desktop terminal
docker run -p 8080:80 nginx

# In desktop Firefox
# All of these work:
http://localhost:8080      # ✓ via iptables NAT
http://10.200.3.1:8080     # ✓ direct gateway access
http://nginx:80            # ✓ via DNS resolution (container on default bridge)
```

### Limitations

1. **Port conflicts:** If two containers expose same port, only one works
2. **UDP support:** iptables NAT for UDP is less reliable
3. **One-time setup:** New exposed ports after bridge require manual re-bridge

### Future: RebridgeDesktop Endpoint

Add endpoint to refresh port forwarding without full re-bridge:

```go
// POST /api/v1/rebridge-desktop
// Clears old iptables rules, re-scans exposed ports, sets up new rules
```

This allows users to click "Refresh Ports" in UI when they start new containers.

---

## Component Restart Resilience

### Design Principles

1. **Source of Truth:** State should be recoverable from authoritative sources:
   - Network interfaces (bridge indices, veths)
   - Docker containers (PIDs, exposed ports)
   - Database (session metadata)

2. **Idempotency:** All operations should be safe to retry.

3. **Self-Healing:** Components should detect stale state and automatically recover.

4. **No Cross-Component State:** Each component manages its own state. The API orchestrates but doesn't cache sandbox state.

### Restart Scenarios

#### 1. Entire Sandbox Restarts

**What happens:**
- All Docker daemons (Wolf's and Hydra's) stop
- All containers destroyed
- All network interfaces (bridges, veths) destroyed
- Wolf and Hydra processes restart fresh

**Recovery:**
- Clean slate - no recovery needed
- Hydra starts with empty instance map
- When user reconnects, API calls CreateDockerInstance (if Hydra mode enabled)
- API calls Wolf CreateLobby for new desktop
- API calls Hydra BridgeDesktop to create new veth

**Implementation:** No special code needed. Fresh start.

#### 2. Wolf Restarts (Without Hydra or Sandbox)

**What happens:**
- Wolf's dockerd restarts (or Wolf process restarts)
- Desktop containers destroyed
- Wolf-side veth ends destroyed with container network namespaces
- Hydra's bridges and dockerd instances still running
- Orphaned veth-b-* interfaces on Hydra bridges

**Recovery:**
- API detects Wolf is back (RevDial reconnect or health check)
- User reconnects to stream → API calls Wolf CreateLobby
- Wolf creates new desktop container (new PID)
- API calls Hydra BridgeDesktop
- Hydra detects `DesktopPID` mismatch (or container not found)
- Hydra cleans up orphaned veth-b-* interface
- Hydra creates new veth pair for new container
- Works automatically via existing self-healing logic

**Implementation:** Already handled by `BridgeDesktop` PID mismatch detection.

#### 3. Hydra Restarts (Without Wolf or Sandbox)

**What happens:**
- Hydra process restarts
- In-memory state lost (instances map, bridge state)
- But network interfaces still exist (bridges, dockerd processes)
- DNS servers stopped

**Recovery:**
1. **On startup:** Hydra scans for existing bridges:
   ```go
   func (m *Manager) recoverFromRestart() {
       // Find existing hydra* bridges
       output, _ := exec.Command("ip", "link", "show", "type", "bridge").Output()
       // Parse "hydraN" bridges, mark indices as used
       // Don't restore full instance state - let API re-create on demand
   }
   ```

2. **Bridge index recovery:** Mark used indices to prevent collisions
3. **Instance recovery:** When API calls CreateDockerInstance:
   - If bridge already exists, reuse it
   - If dockerd already running, reuse socket
   - Update in-memory state

4. **DNS servers:** Restart DNS for recovered instances

**Implementation needed:** Add `recoverFromRestart()` to Manager.

#### 4. Helix API Restarts

**What happens:**
- API server restarts
- In-memory session state lost
- RevDial connections dropped and re-established
- Database state preserved

**Recovery:**
- Sessions stored in database with `WolfInstanceID`, `WolfLobbyID`
- On reconnect to sandbox, API can query Hydra for instance state
- For active sessions, check if desktop still running in Wolf
- If desktop running and bridge intact, session continues
- If desktop or bridge missing, session needs re-creation

**Key insight:** API shouldn't cache sandbox state. Always query sandbox.

**Implementation:**
```go
// In session resume flow
func (e *WolfExecutor) resumeSession(ctx context.Context, session *Session) error {
    // Check if lobby still exists
    lobbyInfo, err := wolfClient.GetLobby(session.WolfLobbyID)
    if err != nil {
        // Lobby gone, need to re-create
        return e.StartZedAgent(ctx, session)
    }

    // Check if bridge still intact (if Hydra mode)
    if e.hydraEnabled {
        hydraStatus, err := hydraClient.GetInstanceStatus(session.ID)
        if err != nil || !hydraStatus.DesktopBridged {
            // Need to re-bridge
            _, err = hydraClient.BridgeDesktop(ctx, &BridgeDesktopRequest{
                SessionID:          session.ID,
                DesktopContainerID: lobbyInfo.ContainerName,
            })
        }
    }

    return nil
}
```

#### 5. Control Plane Disconnects from Sandbox

**What happens:**
- Network partition or temporary outage
- RevDial connections timeout
- API can't reach Wolf or Hydra
- Sandbox continues running independently

**On Reconnect:**
- RevDial connections re-established
- API queries current state from Wolf and Hydra
- Compares with database session records
- Reconciles any differences

**Implementation:**
- Wolf heartbeats already exist
- Add Hydra heartbeat with instance summary
- On reconnect, compare heartbeat state with expected state
- Re-bridge any sessions that were bridged but now aren't

#### 6. Partial Upgrades (API first, then Sandbox)

**What happens:**
- User upgrades API and restarts it
- New API code, old sandbox code
- Later, user upgrades sandbox

**Compatibility:**
- BridgeDesktop API is additive (new endpoint)
- Old sandbox without BridgeDesktop: API catches error, logs warning
- New API with old sandbox: graceful degradation, no bridging

**Implementation:**
```go
// In wolf_executor.go BridgeDesktop call
bridgeResp, err := hydraClient.BridgeDesktop(ctx, req)
if err != nil {
    // Could be old Hydra without endpoint, or actual error
    log.Warn().Err(err).Msg("BridgeDesktop failed - running without network bridging")
    // Continue without bridging - session works, just no dev container access
    return
}
```

#### 7. Partial Upgrades (Sandbox first, then API)

**What happens:**
- User upgrades sandbox and restarts it
- New sandbox code, old API code
- Old API doesn't call BridgeDesktop

**Result:**
- New Hydra has BridgeDesktop endpoint but never called
- Sessions work, just without network bridging
- When API is upgraded and calls BridgeDesktop, everything works

### State Recovery Implementation

#### Bridge Index Recovery on Hydra Startup

```go
func (m *Manager) recoverBridgeIndices() {
    // List all bridge interfaces matching "hydra*"
    output, err := exec.Command("ip", "-o", "link", "show", "type", "bridge").Output()
    if err != nil {
        log.Warn().Err(err).Msg("Failed to list bridges for recovery")
        return
    }

    // Parse output for "hydraN" bridges
    lines := strings.Split(string(output), "\n")
    for _, line := range lines {
        // Format: "3: hydra5: <BROADCAST,MULTICAST,UP> ..."
        if idx := strings.Index(line, "hydra"); idx != -1 {
            // Extract bridge index
            var bridgeNum int
            fmt.Sscanf(line[idx:], "hydra%d", &bridgeNum)
            if bridgeNum > 0 && bridgeNum < 255 {
                m.usedBridges[uint8(bridgeNum)] = "recovered"
                log.Info().Int("bridge_index", bridgeNum).Msg("Recovered existing bridge")
            }
        }
    }
}
```

#### Instance State Query Endpoint

Add endpoint for API to query Hydra's current state:

```go
// GET /api/v1/docker-instances/status-summary
type InstancesSummary struct {
    Instances []struct {
        ScopeID        string `json:"scope_id"`
        BridgeIndex    int    `json:"bridge_index"`
        DesktopBridged bool   `json:"desktop_bridged"`
        Status         string `json:"status"`
    } `json:"instances"`
}
```

#### Periodic Health Check and Auto-Rebridge

```go
// In wolf_executor.go, run periodically for active sessions
func (e *WolfExecutor) healthCheckBridges(ctx context.Context) {
    for _, session := range e.getActiveSessions() {
        if !e.hydraEnabled {
            continue
        }

        hydraClient := e.getHydraClient(session.WolfInstanceID)
        status, err := hydraClient.GetInstanceStatus(ctx, session.ID)
        if err != nil {
            log.Warn().Err(err).Str("session_id", session.ID).Msg("Failed to get Hydra status")
            continue
        }

        // Check if desktop still running
        if session.DesktopContainerName != "" && !status.DesktopBridged {
            log.Info().Str("session_id", session.ID).Msg("Desktop not bridged, attempting re-bridge")
            _, err := hydraClient.BridgeDesktop(ctx, &BridgeDesktopRequest{
                SessionID:          session.ID,
                DesktopContainerID: session.DesktopContainerName,
            })
            if err != nil {
                log.Warn().Err(err).Msg("Auto-rebridge failed")
            }
        }
    }
}
```

### Database Schema Extension

Store bridge state in session metadata for recovery:

```go
// In types/sessions.go
type SessionMetadata struct {
    // ... existing fields ...

    // Bridge state for recovery after API restart
    HydraBridged   bool   `json:"hydra_bridged,omitempty"`
    HydraBridgeIP  string `json:"hydra_bridge_ip,omitempty"`
    HydraGateway   string `json:"hydra_gateway,omitempty"`
}
```

After successful BridgeDesktop, update session in database:
```go
session.Metadata.HydraBridged = true
session.Metadata.HydraBridgeIP = bridgeResp.DesktopIP
session.Metadata.HydraGateway = bridgeResp.Gateway
store.UpdateSession(ctx, session)
```

### Summary of Recovery Behavior

| Scenario | Sandbox State | Recovery Action |
|----------|---------------|-----------------|
| Sandbox restart | Clean slate | Re-create everything on demand |
| Wolf restart | Desktop gone, bridges intact | BridgeDesktop detects PID change, re-bridges |
| Hydra restart | Process state lost, network intact | Recover bridge indices, re-create instances on demand |
| API restart | DB intact | Query sandbox state, reconcile with DB |
| Network partition | Both running independently | Query state on reconnect, re-bridge if needed |
| API upgrade first | Old sandbox | Graceful degradation, logs warning |
| Sandbox upgrade first | Old API | Works, bridging enabled when API upgraded |

---

## Privileged Mode (Host Docker) Networking

### Context

When `HYDRA_PRIVILEGED_MODE_ENABLED=true`, Hydra provides access to the host Docker socket instead of creating isolated per-session dockerd instances. This mode is used for:
- Helix development (developing Helix itself)
- Testing scenarios requiring host Docker access
- Advanced users who understand the security implications

### Architecture Difference

**Normal Mode (Hydra isolation):**
```
Sandbox Container
├── Wolf's dockerd (172.20.0.0/16)
│   └── Desktop container (helix-sway)
└── Hydra's per-session dockerd (10.200.N.0/24)
    └── User's dev containers
```

**Privileged Mode:**
```
Host Machine
├── Host Docker (docker0: 172.17.0.0/16)
│   └── User's dev containers (run on HOST)
│
└── Sandbox Container (container on host Docker)
    └── Wolf's dockerd (172.20.0.0/16)
        └── Desktop container (helix-sway)
```

### Networking Challenge

In privileged mode:
- Dev containers run on **host Docker** (172.17.0.0/16)
- Desktop container runs on **Wolf's dockerd inside sandbox** (172.20.0.0/16)
- These are in different network namespaces (host vs sandbox)

The sandbox container itself IS on the host Docker network, so:
- Sandbox can reach dev containers directly (172.17.0.X)
- But desktop container inside sandbox cannot (it's on Wolf's isolated network)

### Solution: Host Network Access via Sandbox Bridge

When privileged mode is enabled and desktop bridging is requested:

1. **Identify host Docker network:**
   - Host's docker0 bridge is typically at 172.17.0.1
   - Or use custom network if specified

2. **Bridge desktop to sandbox's network namespace:**
   - Create veth pair
   - One end in desktop's netns
   - Other end in sandbox's netns (not host's)

3. **Route through sandbox:**
   - Desktop gets route to 172.17.0.0/16 via sandbox
   - Sandbox forwards traffic to host Docker network
   - Works because sandbox IS on host Docker network

### Implementation

```go
func (m *Manager) BridgeDesktopPrivileged(ctx context.Context, req *BridgeDesktopRequest) (*BridgeDesktopResponse, error) {
    // In privileged mode, bridge desktop to sandbox's network namespace
    // The sandbox container already has access to host Docker network

    // 1. Get desktop container's PID from Wolf's dockerd
    containerPID, err := m.getContainerPID(req.DesktopContainerID)
    if err != nil {
        return nil, fmt.Errorf("failed to get desktop container PID: %w", err)
    }

    // 2. Get host Docker's default network gateway (typically 172.17.0.1)
    hostGateway := m.getHostDockerGateway() // Reads from /etc/resolv.conf or inspects docker0

    // 3. Create veth pair (sandbox namespace to desktop namespace)
    vethDesktop := fmt.Sprintf("veth-priv-%s", req.SessionID[:8])
    vethSandbox := fmt.Sprintf("veth-sand-%s", req.SessionID[:8])

    // Create veth in sandbox namespace
    exec.Command("ip", "link", "add", vethDesktop, "type", "veth", "peer", "name", vethSandbox).Run()

    // 4. Move one end into desktop container
    exec.Command("ip", "link", "set", vethDesktop, "netns", strconv.Itoa(containerPID)).Run()

    // 5. Configure desktop side with IP in docker0 range
    // Use high IP to avoid conflicts (e.g., 172.17.255.254)
    desktopIP := "172.17.255.254/16"
    nsenter(containerPID, "ip", "addr", "add", desktopIP, "dev", vethDesktop)
    nsenter(containerPID, "ip", "link", "set", vethDesktop, "name", "eth1")
    nsenter(containerPID, "ip", "link", "set", "eth1", "up")

    // 6. Configure sandbox side and enable forwarding
    exec.Command("ip", "addr", "add", "172.17.255.253/16", "dev", vethSandbox).Run()
    exec.Command("ip", "link", "set", vethSandbox, "up").Run()

    // 7. Enable IP forwarding in sandbox (if not already)
    exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()

    // 8. Add route in desktop for host Docker network via sandbox
    nsenter(containerPID, "ip", "route", "add", "172.17.0.0/16", "via", "172.17.255.253")

    // 9. Configure DNS to use host Docker's DNS (127.0.0.11 in container)
    // or just use 8.8.8.8 for simplicity

    return &BridgeDesktopResponse{
        DesktopIP: "172.17.255.254",
        Gateway:   hostGateway,
        Subnet:    "172.17.0.0/16",
        Interface: "eth1",
    }, nil
}
```

### Detection and Dispatch

In `BridgeDesktop`, check if this session is using privileged mode:

```go
func (m *Manager) BridgeDesktop(ctx context.Context, req *BridgeDesktopRequest) (*BridgeDesktopResponse, error) {
    // Find the instance
    inst := m.findInstance(req.SessionID)

    if inst == nil {
        // No Hydra instance = might be privileged mode
        if m.privilegedModeEnabled {
            return m.BridgeDesktopPrivileged(ctx, req)
        }
        return nil, fmt.Errorf("no Hydra Docker instance found for session %s", req.SessionID)
    }

    // Normal Hydra instance - use standard bridging
    // ... existing code ...
}
```

### Simplified Approach for MVP

For the initial implementation, privileged mode users can access dev containers via:
- Host IP address (e.g., `http://172.17.0.5:3000`)
- Container name via host Docker DNS (if DNS is configured)

The full veth bridging for privileged mode can be deferred to a future enhancement.

### Configuration

```env
# Enable privileged mode (exposes host Docker)
HYDRA_PRIVILEGED_MODE_ENABLED=true

# Optional: Specify host Docker network to bridge
HYDRA_HOST_DOCKER_NETWORK=bridge  # default: "bridge"
```

### Security Considerations

Privileged mode is inherently less secure:
- Dev containers run on host Docker (can access host resources)
- No tenant isolation (all privileged mode users share host Docker)
- Should only be enabled for trusted development scenarios

### Comparison

| Feature | Normal Mode | Privileged Mode |
|---------|-------------|-----------------|
| Dev containers | Isolated per-session dockerd | Shared host Docker |
| Tenant isolation | Yes (separate networks) | No (shared network) |
| Desktop bridging | veth to Hydra bridge | veth to sandbox netns |
| DNS resolution | Hydra DNS server | Host Docker DNS |
| Security | Strong isolation | Minimal isolation |
| Use case | Production, multi-tenant | Development, single-tenant |
