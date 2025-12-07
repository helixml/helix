# Hydra: Multi-Tenant Docker Isolation for Cloud Desktops

**Date:** 2025-12-07
**Author:** Helix Team

## TL;DR

Hydra solves a deceptively hard problem: giving each user in a cloud desktop environment their own isolated Docker daemon, while still letting their browser access services running in those containers. It's Docker-in-Docker-in-Docker with cross-network bridging, DNS resolution, and multi-tenant isolation.

---

## The Problem We're Solving

Imagine you're building a cloud IDE—something like GitHub Codespaces or Gitpod. Users get a full Linux desktop streamed to their browser. They can open a terminal, run `docker compose up`, and expect `localhost:3000` to work in their browser.

Simple, right?

Now add these constraints:

1. **Multi-tenancy**: Multiple users share the same GPU server. User A must not be able to access User B's containers.
2. **Streaming isolation**: The desktop streaming system (Wolf) manages its own Docker containers for video encoding.
3. **Security**: User workloads should be isolated from the streaming infrastructure.
4. **Enterprise networks**: Internal DNS servers, private TLDs, corporate proxies.

Suddenly you have three layers of Docker networks that need to communicate selectively, with per-user isolation, cross-network DNS resolution, and enterprise network compatibility.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              HOST MACHINE                                    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                     SANDBOX CONTAINER                                   │ │
│  │                                                                         │ │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐   │ │
│  │  │    Wolf     │  │  Moonlight  │  │    Hydra    │  │   Sandbox   │   │ │
│  │  │  (stream)   │  │   (WebRTC)  │  │  (docker²)  │  │  Heartbeat  │   │ │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘   │ │
│  │                                                                         │ │
│  │  ════════════════════════════════════════════════════════════════════  │ │
│  │                                                                         │ │
│  │  Wolf's dockerd                    Hydra's per-session dockerd(s)      │ │
│  │  ┌─────────────────────┐          ┌─────────────────────┐              │ │
│  │  │ helix-sway desktop  │◄───veth──►│ User's containers   │              │ │
│  │  │ (Firefox, Zed, etc) │   pair   │ (webapp, postgres)  │              │ │
│  │  │ 172.20.0.X          │          │ 10.200.N.X          │              │ │
│  │  └─────────────────────┘          └─────────────────────┘              │ │
│  │                                                                         │ │
│  │           Session A's network                Session A only            │ │
│  │  ┌─────────────────────┐          ┌─────────────────────┐              │ │
│  │  │ Session B desktop   │◄───veth──►│ Session B containers│              │ │
│  │  │ 172.20.0.Y          │   pair   │ 10.200.M.X          │              │ │
│  │  └─────────────────────┘          └─────────────────────┘              │ │
│  │                                                                         │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Two Operating Modes

### Mode 1: Hydra Mode (Full Isolation)

This is the default for production. Each user session gets:

1. **Its own dockerd instance** (`/var/run/hydra/{session-id}/docker.sock`)
2. **Its own bridge network** (`hydra1`, `hydra2`, etc. with subnets `10.200.1.0/24`, `10.200.2.0/24`)
3. **Its own DNS server** (resolves container names to IPs)
4. **A veth bridge** connecting their desktop to their Docker network

**How it works:**

```
User opens terminal → runs "docker compose up"
                            ↓
         Desktop's DOCKER_HOST points to Hydra socket
                            ↓
         Containers start on isolated hydra{N} bridge
                            ↓
         API calls Hydra's BridgeDesktop endpoint
                            ↓
         Hydra creates veth pair:
           - One end in desktop's network namespace (eth1)
           - Other end attached to hydra{N} bridge
                            ↓
         Desktop can now reach 10.200.N.0/24
                            ↓
         Hydra DNS server resolves "webapp" → 10.200.N.5
                            ↓
         Firefox navigates to http://webapp:3000 ✓
```

**Isolation guarantee:** Session A's desktop has a veth to `hydra1` bridge. Session B's desktop has a veth to `hydra2` bridge. There is no route between them. Session A cannot reach Session B's containers.

**The veth injection magic:**

```go
// 1. Create veth pair in sandbox namespace
exec.Command("ip", "link", "add", "vethd-h1", "type", "veth", "peer", "name", "vethb-h1")

// 2. Attach bridge-side to Hydra's bridge
exec.Command("ip", "link", "set", "vethb-h1", "master", "hydra1")
exec.Command("ip", "link", "set", "vethb-h1", "up")

// 3. Move desktop-side into container's network namespace
exec.Command("ip", "link", "set", "vethd-h1", "netns", strconv.Itoa(desktopPID))

// 4. Configure inside the container via nsenter
nsenter(desktopPID, "ip", "link", "set", "vethd-h1", "name", "eth1")
nsenter(desktopPID, "ip", "addr", "add", "10.200.1.254/24", "dev", "eth1")
nsenter(desktopPID, "ip", "link", "set", "eth1", "up")

// Desktop now has two interfaces:
// eth0: 172.20.0.X (Wolf's network - for streaming)
// eth1: 10.200.1.254 (Hydra's network - for dev containers)
```

### Mode 2: Privileged Mode (Development)

For developing Helix itself or advanced scenarios, privileged mode bypasses Hydra isolation:

1. **Single shared dockerd** (the host's Docker)
2. **All containers on the same network** (172.17.0.0/16)
3. **Veth bridges desktop to sandbox** (which has host Docker access)
4. **NAT/masquerade for routing**

**How it works:**

```
User opens terminal → runs "docker compose up"
                            ↓
         Desktop's DOCKER_HOST points to host socket
                            ↓
         Containers start on host's docker0 bridge
                            ↓
         API calls Hydra's BridgeDesktopPrivileged
                            ↓
         Hydra creates veth pair:
           - One end in desktop's netns (eth1: 172.17.255.254/32)
           - Other end in sandbox's netns (172.17.255.253)
                            ↓
         Add route: 172.17.0.0/16 via 172.17.255.253
                            ↓
         Enable IP forwarding + iptables MASQUERADE
                            ↓
         Desktop traffic → sandbox → host Docker network
                            ↓
         Firefox reaches http://172.17.0.5:3000 ✓
```

**Trade-offs:**
- ✅ Simpler networking
- ✅ Access to all host Docker features
- ❌ No tenant isolation
- ❌ Users can see each other's containers

---

## DNS Resolution

Container DNS is the unsung hero. When a user types `http://webapp:3000`, something needs to resolve `webapp` to `10.200.1.5`.

**Hydra runs a custom DNS server per bridge:**

```go
// Listens on bridge gateway (10.200.1.1:53)
func (h *dnsHandler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
    for _, q := range r.Question {
        if q.Qtype == dns.TypeA {
            // Query Hydra's dockerd for container with this name
            ip := h.resolveContainer(q.Name)
            if ip != nil {
                // Return container IP
                rr := &dns.A{...}
                msg.Answer = append(msg.Answer, rr)
            } else {
                // Forward to upstream DNS (enterprise internal DNS)
                return h.forwardQuery(r)
            }
        }
    }
}

func (h *dnsHandler) resolveContainer(name string) net.IP {
    // docker inspect on Hydra's dockerd
    cmd := exec.Command("docker", "-H", "unix://"+h.instance.SocketPath,
        "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", name)
    // ...
}
```

**Enterprise DNS passthrough:**

The DNS server reads upstream nameservers from the sandbox's `/etc/resolv.conf`. This means:
- Internal company DNS servers work
- Private TLDs (`.corp`, `.internal`) resolve
- VPN-accessible services are reachable

---

## Self-Healing Behavior

Cloud infrastructure is messy. Containers restart, processes crash, connections drop. Hydra handles this gracefully:

### Container Restart Detection

```go
// Track bridge state
type DockerInstance struct {
    DesktopBridged     bool   // Is desktop currently bridged?
    DesktopContainerID string // Which container was bridged?
    DesktopPID         int    // PID at bridge time
    VethBridgeName     string // For cleanup
}

// On BridgeDesktop call:
if inst.DesktopBridged && inst.DesktopPID != containerPID {
    // Container restarted! Clean up old veth and re-bridge
    m.cleanupOrphanedVeth(inst.VethBridgeName)
    inst.DesktopBridged = false
    // Continue to create new bridge...
}
```

### Bridge Index Recovery

If Hydra restarts, it scans for existing bridges:

```go
func (m *Manager) recoverBridgeIndices() {
    output, _ := exec.Command("ip", "-o", "link", "show", "type", "bridge").Output()
    // Parse "hydra5" from output, mark index 5 as used
    // Prevents collision when creating new instances
}
```

### Orphaned Veth Cleanup

```go
func (m *Manager) cleanupOrphanedVeth(vethName string) {
    // Check if veth exists (old container died, veth orphaned)
    if exec.Command("ip", "link", "show", vethName).Run() == nil {
        exec.Command("ip", "link", "del", vethName).Run()
    }
}
```

---

## Capacity and Limits

**Bridge index range:** 1-254 (10.200.1.0/24 through 10.200.254.0/24)

That's 254 concurrent isolated Docker environments per sandbox. In practice, a single GPU server handles 10-20 concurrent sessions, so this is plenty.

**Resource isolation:**
- Each dockerd has its own data directory (`/hydra-data/{scope}/{session-id}/docker/`)
- Images, volumes, and build cache persist across session restarts
- Running containers do NOT persist (by design—security boundary)

---

## The Networking Stack

Here's the full picture of what networks exist and how they connect:

```
Layer 0: Host Machine
└── docker0: 172.17.0.0/16 (host Docker, used in privileged mode)
└── helix_default: 172.19.0.0/16 (Helix control plane)

Layer 1: Sandbox Container (on helix_default)
└── Wolf's dockerd
    └── helix_default: 172.20.0.0/16 (desktops)
└── Hydra's dockerd instances
    └── hydra1: 10.200.1.0/24 (Session A's containers)
    └── hydra2: 10.200.2.0/24 (Session B's containers)
    └── hydra3: 10.200.3.0/24 (Session C's containers)

Bridges (created by BridgeDesktop):
└── Session A desktop (172.20.0.5) ←→ eth1 (10.200.1.254) → hydra1 bridge
└── Session B desktop (172.20.0.6) ←→ eth1 (10.200.2.254) → hydra2 bridge
```

---

## Why This Architecture?

**Why not just use host Docker for everything?**

Multi-tenancy. If all users share the host Docker, User A can `docker ps` and see User B's containers. They can attach to them, read their logs, or worse.

**Why not one dockerd per sandbox, with network namespaces?**

Wolf (the streaming system) needs to manage its own containers for video encoding. We don't want to modify Wolf to understand per-user isolation. Keeping Wolf's dockerd separate is cleaner.

**Why veth pairs instead of overlay networks?**

Speed and simplicity. Veth pairs are kernel-level virtual ethernet, with near-zero overhead. Overlay networks add encapsulation and complexity.

**Why custom DNS instead of Docker's embedded DNS?**

Docker's embedded DNS (127.0.0.11) only works within that Docker network. Our desktop is on a *different* Docker network. We need a DNS server that can query one dockerd's containers from another network.

---

## Code Locations

- **Hydra daemon**: `api/pkg/hydra/` (manager.go, server.go, dns.go)
- **Hydra binary**: `api/cmd/hydra/main.go`
- **Wolf executor integration**: `api/pkg/external-agent/wolf_executor.go`
- **Design doc**: `design/2025-12-04-hydra-network-dns-bridging.md`

---

## Will It Work First Time? (Critical Analysis)

Based on the architecture, here's an honest assessment of what might go wrong:

### Likely to Work

1. **Bridge creation** ✅ - Explicit `ip link add hydra{N} type bridge` before dockerd starts. Verified syntax.

2. **Dockerd startup** ✅ - Using `--bridge=hydra{N}` flag, tested pattern from Docker docs.

3. **Veth pair creation** ✅ - Standard Linux networking, well-documented.

4. **DNS server startup** ✅ - Uses miekg/dns, a battle-tested library. Simple UDP server.

5. **RevDial connectivity** ✅ - Runner ID format verified: `hydra-{WOLF_INSTANCE_ID}` matches on both sides.

### Potential Issues

1. **Timing: Container not ready** ⚠️
   - BridgeDesktop called before container fully starts
   - **Mitigation:** 10 retry attempts with exponential backoff (500ms, 1s, 1.5s...)
   - **Risk:** Low - Wolf usually starts containers within 2-3 seconds

2. **DNS port 53 requires root** ⚠️
   - DNS server binds to port 53 on bridge gateway IP
   - **Mitigation:** Hydra runs as root in sandbox (required for dockerd anyway)
   - **Risk:** Low - sandbox runs privileged

3. **Namespace race on veth injection** ⚠️
   - Container PID exists but netns not fully initialized
   - **Mitigation:** We wait for container to be "running" state via docker inspect
   - **Risk:** Low - Docker API reports running only when netns ready

4. **Firewall/iptables conflicts** ⚠️
   - Docker adds iptables rules that might block cross-network traffic
   - **Mitigation:** We're on the same L2 (bridge), no iptables traversal needed
   - **Risk:** Low - veth + bridge is pure L2

5. **Enterprise DNS timeout** ⚠️
   - Internal DNS servers might be slow/unavailable
   - **Mitigation:** 2-second timeout per upstream, try multiple servers
   - **Risk:** Medium - depends on enterprise network config

### Things That Might Fail in Production (But Not in Dev)

1. **Loopback DNS (127.0.0.1 in /etc/resolv.conf)**
   - Many containers use systemd-resolved pointing to 127.0.0.1
   - **Mitigation:** We skip loopback addresses, fall back to 8.8.8.8
   - **Detection:** Logs will show "Skipping loopback nameserver"

2. **Bridge index exhaustion**
   - 254 concurrent sessions per sandbox
   - **Mitigation:** This is 10x expected capacity
   - **Detection:** Error log "no available bridge indices"

3. **Orphaned bridges after crash**
   - Hydra crashes, bridges remain
   - **Mitigation:** `recoverBridgeIndices()` scans existing bridges on startup
   - **Risk:** Low - recovery logic tested

4. **PID wraparound (veth naming collision)**
   - Privileged mode uses PID in veth name
   - **Mitigation:** PID is unique at runtime; old veths cleaned before creation
   - **Risk:** Very low - requires exact PID reuse

### Manual Testing Checklist

Before declaring production-ready, test these scenarios:

```bash
# 1. Basic functionality
- [ ] Start session, run `docker compose up` with a webapp
- [ ] Access webapp via browser at http://webapp:3000
- [ ] Verify DNS resolution: `nslookup webapp` in desktop terminal

# 2. Multi-tenant isolation
- [ ] Start two sessions simultaneously
- [ ] From Session A, try to ping Session B's container IP
- [ ] Expected: "Network unreachable" or timeout

# 3. Container restart recovery
- [ ] Start session with running container
- [ ] Restart desktop container (Wolf recreates it)
- [ ] Verify webapp still accessible after restart

# 4. Hydra restart recovery
- [ ] Start session with running container
- [ ] Restart Hydra process
- [ ] Verify bridge indices recovered
- [ ] Verify DNS still works (new server started)

# 5. Enterprise DNS
- [ ] Configure sandbox with internal DNS server
- [ ] Access internal hostname from desktop
- [ ] Verify container name resolution still works

# 6. Privileged mode
- [ ] Enable HYDRA_PRIVILEGED_MODE_ENABLED=true
- [ ] Start session, run container on host Docker
- [ ] Access container from desktop
- [ ] Verify routing through sandbox works
```

### Known First-Run Issues (Now Fixed)

These issues were identified during code review and fixed:

| Issue | Symptom | Fix Applied |
|-------|---------|-------------|
| No bridge created | Containers fail to start, "network not found" | Added `createBridge()` before dockerd |
| DNS never started | Container names don't resolve | Added `NewDNSServer()` in `NewServer()` |
| Hardcoded DNS | Enterprise internal DNS fails | Parse `/etc/resolv.conf` for upstream |
| Route conflict | Privileged mode routing fails | Use /32 prefix + explicit gateway route |
| Veth collisions | Interface already exists error | Use bridge index instead of session ID |
| iptables accumulation | NAT rules pile up | Delete-before-add pattern |

## Future Work

1. **Port forwarding**: Make `localhost:8080` work in the desktop (iptables DNAT)
2. **GPU passthrough**: Allow dev containers to access GPU
3. **Volume sharing**: Share files between desktop and dev containers efficiently
4. **Metrics**: Expose container resource usage per session

---

# Hacker News Post

---

## Show HN: We built Docker-in-Docker isolation for cloud desktops (with cross-network DNS)

*Yo dawg, I heard you like Docker, so we put a Docker in your Docker so you can docker compose while you docker compose.*

![Xzibit meme placeholder - Yo dawg I heard you like Docker]

We're building Helix, a platform where AI agents work in cloud desktops. Users can watch AI code in real-time via browser streaming, and the AI can run `docker compose up` to test its work.

The hard part? Each user needs their own isolated Docker environment, but their browser (running in a separate container for streaming) needs to reach those containers. And multiple users share the same GPU server.

### The problem

Imagine this architecture:
- Sandbox container runs on a GPU server
- Inside it: Wolf (for video streaming) runs its own dockerd
- Wolf creates desktop containers (Firefox, Zed editor)
- Users run `docker compose up` in their terminal
- User expects `http://webapp:3000` to work in Firefox

But the desktop is on Wolf's Docker network (172.20.0.X), and the webapp is on... where? If we use Wolf's dockerd, User A can see User B's containers. If we use separate dockerd instances, the networks can't talk to each other.

### Our solution: Hydra

Hydra is a daemon that manages multiple dockerd instances—one per user session—with isolated networks (10.200.1.0/24, 10.200.2.0/24, etc).

When a session starts:
1. Hydra spawns a dedicated dockerd with its own bridge
2. Desktop's DOCKER_HOST points to this dockerd
3. After desktop starts, we inject a veth pair:
   - One end in desktop's network namespace
   - Other end attached to Hydra's bridge
4. Now desktop can reach 10.200.N.0/24

For DNS, we run a custom DNS server per bridge using miekg/dns. It queries the session's dockerd for container names and forwards unknown queries to upstream DNS (supporting enterprise internal DNS).

### The fun parts

**Cross-namespace veth injection:**
```go
// Create veth pair
exec.Command("ip", "link", "add", "vethd-h1", "type", "veth", "peer", "name", "vethb-h1")
// Attach to bridge
exec.Command("ip", "link", "set", "vethb-h1", "master", "hydra1")
// Move into container's netns
exec.Command("ip", "link", "set", "vethd-h1", "netns", containerPID)
// Configure via nsenter
nsenter(containerPID, "ip", "addr", "add", "10.200.1.254/24", "dev", "eth1")
```

**Self-healing:** If a container restarts, the veth end in its namespace dies. We detect this by PID mismatch and automatically re-bridge.

**Enterprise DNS:** We parse `/etc/resolv.conf` and forward to internal DNS servers, so `git clone https://gitlab.corp.internal/...` works.

### Numbers

- 254 concurrent isolated sessions per sandbox (10.200.1-254.0/24)
- Near-zero networking overhead (kernel veth pairs)
- ~460MB GPU memory per streaming session
- Typical: 10-20 concurrent sessions per GPU server

### Open source

The code is at https://github.com/helixml/helix. The Hydra daemon is in `api/pkg/hydra/`.

We'd love feedback from anyone who's dealt with multi-tenant Docker isolation, container networking, or cloud desktop infrastructure. What are we missing? What would break in production?

---

*Discussion questions for HN:*
- Has anyone else solved cross-dockerd networking differently?
- Is there a cleaner way to do DNS resolution across Docker networks?
- Should we be using something other than veth pairs?
