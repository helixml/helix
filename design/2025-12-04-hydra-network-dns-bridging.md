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
