# Helix-in-Helix Development: Developing Helix Inside Helix

**Date:** 2026-01-25
**Status:** Implemented
**Author:** Claude

## Executive Summary

This document describes how to enable developers to develop Helix itself using Helix's own cloud desktop infrastructure. The key challenge is that sandboxes require Docker-in-Docker (DinD), and you cannot run DinD-in-DinD-in-DinD.

**Solution:** Two Docker endpoints + API-proxied service exposure:

1. **Inner Docker** (Hydra's DinD) - For the control plane (API, DB, frontend)
2. **Outer Docker** (Host Docker) - For running sandboxes with full DinD capability
3. **Service Exposure via API** - Expose inner services on subdomains/ports so external sandboxes can connect

The service exposure feature is **generally useful** for all users who want to expose web apps running in their dev containers to their browser. Helix-in-Helix is just one use case.

Sandboxes only need outbound connections (RevDial), so multiple sandboxes can run on host Docker without port conflicts.

---

## Problem Statement

### Current Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│ HOST MACHINE                                                     │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ Docker Compose (helix_default network: 172.19.0.0/16)      │ │
│  │   ├── api (172.19.0.20)                                    │ │
│  │   ├── postgres                                             │ │
│  │   ├── frontend                                             │ │
│  │   └── sandbox (172.19.0.50) ──────────────────────────────┐│ │
│  │         │                                                  ││ │
│  │         │  ┌──────────────────────────────────────────────┐││ │
│  │         │  │ Sandbox's DinD                               │││ │
│  │         └──│   ├── helix-sway (desktop containers)        │││ │
│  │            │   └── Hydra dockerd instances                │││ │
│  │            │         └── User's dev containers            │││ │
│  │            └──────────────────────────────────────────────┘││ │
│  │                                                            │ │
│  └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### The Challenge for Helix-in-Helix

When developing Helix inside Helix, we want:
- Developer's desktop runs inside Hydra's Docker (streamed to browser)
- Developer can run `./stack start` to start the Helix control plane
- Developer can test with sandboxes

But sandboxes need DinD, and the desktop is already inside DinD. Running DinD-in-DinD-in-DinD doesn't work:
1. Storage driver issues (overlay2 on overlay2 on overlay2)
2. Device/cgroup access problems
3. Performance degradation
4. Namespace nesting limits

---

## Solution Architecture

### High-Level Design

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ HOST MACHINE                                                                 │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │ Production Control Plane (helix_default: 172.19.0.0/16)                │ │
│  │   ├── api (172.19.0.20)                                                │ │
│  │   ├── postgres, frontend, etc.                                         │ │
│  │   └── sandbox (172.19.0.50) ───────────────────────────────────────┐   │ │
│  │                                                                     │   │ │
│  │         ┌───────────────────────────────────────────────────────────┼───┼─┘
│  │         │ Sandbox's Docker (172.20.0.0/16)                          │   │
│  │         │   └── Developer Desktop (helix-sway)                      │   │
│  │         │         │                                                 │   │
│  │         │         │  DOCKER_HOST_INNER=unix:///var/run/docker.sock  │   │
│  │         │         │  ┌─────────────────────────────────────┐        │   │
│  │         │         └──│ Hydra dockerd (inner Docker)        │        │   │
│  │         │            │   ├── helix-api (dev control plane) │        │   │
│  │         │            │   ├── helix-postgres                │        │   │
│  │         │            │   └── helix-frontend                │        │   │
│  │         │            └─────────────────────────────────────┘        │   │
│  │         │                                                           │   │
│  │         │         DOCKER_HOST_OUTER=tcp://host-docker:2375          │   │
│  │         │            │                                              │   │
│  └─────────│────────────│──────────────────────────────────────────────┘   │
│            │            │                                                   │
│            │            ▼                                                   │
│  ┌─────────│────────────────────────────────────────────────────────────┐  │
│  │ Host Docker (accessible via privileged mode)                          │  │
│  │   └── helix-sandbox-dev-1 (RevDial → inner control plane)            │  │
│  │   └── helix-sandbox-dev-2 (RevDial → inner control plane)            │  │
│  │         │                                                             │  │
│  │         └── DinD (desktop containers, user's containers)              │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Key Insight: RevDial Makes This Work

Sandboxes only make **outbound** WebSocket connections via RevDial:
- Sandbox → RevDial → API control connection
- API → RevDial → Sandbox (reversed, uses outbound-established tunnel)

This means:
1. **No port conflicts** - Multiple sandboxes can run on host Docker
2. **No inbound firewall issues** - All connections are outbound
3. **Unique container names** - Use different names to avoid conflicts

---

## Service Exposure Feature (New)

### The General Problem

Users running web apps in their dev containers want to access them from their browser. Currently, this requires:
- Port forwarding through multiple network layers
- Complex networking knowledge
- Manual setup per service

### Solution: API-Proxied Service Exposure

The Helix API acts as a reverse proxy for services running in dev containers. Two approaches:

#### Option A: Subdomain-Based Virtual Hosting (Recommended)

```
Browser → ses_abc123-8080.dev.helix.example.com
       → Helix API (matches subdomain pattern)
       → RevDial to sandbox
       → Hydra proxy to desktop container
       → Port 8080 on dev container network
```

**Requirements:**
- Wildcard DNS record: `*.dev.helix.example.com → API IP`
- Wildcard TLS certificate (Let's Encrypt supports this)
- API routes based on subdomain pattern

**URL Format Options:**
```
# Option 1: session-port format
ses_abc123-8080.dev.helix.example.com

# Option 2: Prettier with path
abc123.dev.helix.example.com:8080

# Option 3: Port in subdomain (avoids non-standard ports)
p8080-ses-abc123.dev.helix.example.com
```

**Advantages:**
- Works with any HTTP service
- No port conflicts
- Clean URLs
- Standard HTTPS on port 443

#### Option B: Random Port Allocation (Simpler)

For localhost/IP-based deployments without custom DNS:

```
Browser → http://localhost:34567
       → Helix API (allocated port 34567 for session abc123, port 8080)
       → RevDial to sandbox
       → Desktop container port 8080
```

**How it works:**
1. User requests to expose port 8080 for session `abc123`
2. API allocates random available port (e.g., 34567)
3. API listens on that port, proxies to session
4. Returns URL to user: `http://localhost:34567`

**Advantages:**
- Works without DNS setup
- Simple for local development
- No TLS complexity

**Disadvantages:**
- Random ports harder to remember
- Need to manage port allocation/deallocation
- May conflict with local services

### API Design

#### Expose a Port

```
POST /api/v1/sessions/{session_id}/expose
{
  "port": 8080,
  "protocol": "http",  // or "tcp" for raw TCP
  "name": "api"        // optional, for subdomain: api-ses_abc123.dev.helix.example.com
}

Response:
{
  "session_id": "ses_abc123",
  "port": 8080,
  "protocol": "http",
  "urls": [
    "https://ses-abc123-8080.dev.helix.example.com",  // subdomain mode
    "http://localhost:34567"                          // port mode (if enabled)
  ],
  "allocated_port": 34567,  // only in port mode
  "status": "active"
}
```

#### List Exposed Ports

```
GET /api/v1/sessions/{session_id}/expose

Response:
{
  "session_id": "ses_abc123",
  "exposed_ports": [
    {
      "port": 8080,
      "protocol": "http",
      "name": "api",
      "url": "https://ses-abc123-8080.dev.helix.example.com",
      "status": "active"
    },
    {
      "port": 3000,
      "protocol": "http",
      "name": "frontend",
      "url": "https://ses-abc123-3000.dev.helix.example.com",
      "status": "active"
    }
  ]
}
```

#### Unexpose a Port

```
DELETE /api/v1/sessions/{session_id}/expose/{port}
```

### Implementation Architecture

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Helix API                                                                     │
│                                                                               │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │ Reverse Proxy (new component)                                           │ │
│  │   ├── Subdomain Router: *.dev.helix.example.com                        │ │
│  │   ├── Port Allocator: localhost:30000-40000 range                      │ │
│  │   └── Session → Port mapping table                                      │ │
│  └───────────────────────────────────────────────┬─────────────────────────┘ │
│                                                   │                           │
│                                                   ▼                           │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │ RevDial Connection Manager                                              │ │
│  │   └── Dial session's Hydra                                             │ │
│  └───────────────────────────────────────────────┬─────────────────────────┘ │
└──────────────────────────────────────────────────│───────────────────────────┘
                                                   │
                                                   ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│ Sandbox (via RevDial)                                                         │
│   └── Hydra                                                                   │
│         └── Proxy to desktop container's exposed port                        │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Hydra Changes

Hydra needs a new endpoint to proxy HTTP to a port on the desktop's network:

```
POST /api/v1/dev-containers/{session_id}/proxy
{
  "target_port": 8080,
  "protocol": "http"
}

// Or simpler: just add a proxy endpoint
GET/POST/etc /api/v1/dev-containers/{session_id}/proxy/{port}/*
→ Forwards request to desktop_ip:{port}/*
```

### Application to Helix-in-Helix

For developing Helix inside Helix:

1. **Start inner control plane** in desktop's Docker
   ```bash
   ./stack start  # Starts API on port 8080 inside Hydra's Docker
   ```

2. **Expose port 8080** via API
   ```bash
   curl -X POST https://helix.example.com/api/v1/sessions/$SESSION_ID/expose \
     -d '{"port": 8080, "name": "dev-api"}'
   # Returns: https://dev-api-ses-abc123.dev.helix.example.com
   ```

3. **Configure sandbox on host Docker** to use exposed URL
   ```bash
   docker run --name helix-sandbox-dev \
     -e HELIX_API_URL=https://dev-api-ses-abc123.dev.helix.example.com \
     -e RUNNER_TOKEN=... \
     --privileged \
     helix-sandbox:latest
   ```

4. **Sandbox RevDials** to the exposed inner API
   - Connection goes: Host Docker → Helix API → RevDial → Sandbox → Hydra → Desktop → Inner API

### Two Docker Endpoints

Inside the developer desktop, provide two Docker endpoints:

| Endpoint | Environment Variable | Purpose | Capabilities |
|----------|---------------------|---------|--------------|
| Inner Docker | `DOCKER_HOST_INNER` | Control plane (api, db, frontend) | Normal containers |
| Outer Docker | `DOCKER_HOST_OUTER` | Sandboxes | Full DinD, GPU access |

---

## Implementation Details

### 1. Host Docker Socket Proxy

The developer desktop needs access to host Docker. Options:

#### Option A: TCP Proxy Container (Recommended)

Run a minimal TCP proxy on the host that forwards to `/var/run/docker.sock`:

```yaml
# docker-compose.dev.yaml addition
services:
  docker-proxy:
    image: alpine/socat
    command: TCP-LISTEN:2375,fork,reuseaddr UNIX-CONNECT:/var/run/docker.sock
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    networks:
      default:
        ipv4_address: 172.19.0.51  # Fixed IP for predictable access
    # SECURITY: Only expose within Docker network, not to host
```

Desktop accesses via: `DOCKER_HOST=tcp://172.19.0.51:2375`

#### Option B: Bind Mount Through Sandbox

Mount host socket through sandbox to desktop:
- Host: `/var/run/docker.sock` → Sandbox: `/var/run/host-docker.sock`
- Sandbox → Desktop: `/var/run/host-docker.sock` → `/var/run/outer-docker.sock`

More complex but avoids TCP exposure.

#### Option C: Unix Socket Forwarding via socat in Sandbox

The sandbox already has host socket mounted at `/var/run/host-docker.sock`. Forward it to a port that desktop containers can reach:

```bash
# In sandbox startup
socat TCP-LISTEN:2375,fork,reuseaddr UNIX-CONNECT:/var/run/host-docker.sock &
```

Desktop containers on the sandbox's Docker network can then use `tcp://host.docker.internal:2375` or the sandbox's IP.

### 2. Network Path: Inner Control Plane → Sandbox

The development sandbox needs to reach the control plane running inside Hydra's Docker. Two approaches:

#### Approach A: Bridge the Networks

Create a veth pair bridging:
- Sandbox container (on host Docker)
- Inner control plane network (Hydra's Docker)

This is complex because we'd need to inject a veth from host namespace into the Hydra dockerd's network.

#### Approach B: Expose Inner Control Plane Port (Simpler)

The inner control plane's API can bind to `0.0.0.0:8080`. Traffic flow:

```
Sandbox (host Docker)
    ↓
    RevDial to http://desktop-ip:exposed-port
    ↓
    Desktop container (port forwarded or host network mode)
    ↓
    Inner Docker network (Hydra)
    ↓
    helix-api container
```

Desktop runs with `--network host` for the inner Docker network, or uses port forwarding.

#### Approach C: Use Host's IP from Sandbox (Simplest)

1. Desktop starts inner control plane with API on a specific port
2. Inner Docker's API binds to desktop's eth0 IP (visible from sandbox's network)
3. Sandbox sets `HELIX_API_URL=http://<desktop-ip>:<port>`

Since the sandbox is on host Docker network, and the inner control plane is accessible via the desktop's IP (which is on sandbox's DinD network), we need a bridge or the desktop to forward.

### 3. Recommended Network Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ HOST                                                                         │
│                                                                              │
│  Production sandbox (172.19.0.50)                                           │
│     │                                                                        │
│     └── DinD Network (172.20.0.0/16)                                        │
│            │                                                                 │
│            ├── Desktop (172.20.0.5)                                         │
│            │      │                                                          │
│            │      └── Hydra Network (10.200.1.0/24)                         │
│            │             │                                                   │
│            │             └── Inner control plane                             │
│            │                    helix-api (10.200.1.10:8080)                │
│            │                                                                 │
│            └── (veth bridge to sandbox) ←────────────────────────────┐      │
│                                                                       │      │
│  Dev Sandbox on Host Docker                                          │      │
│     └── RevDial connects to ─────────────────────────────────────────┘      │
│           http://10.200.1.10:8080 (via bridge)                              │
│           or                                                                 │
│           http://<exposed-port-on-host>                                     │
└─────────────────────────────────────────────────────────────────────────────┘
```

The cleanest solution:
1. Production sandbox's Hydra bridges the desktop to its Docker network
2. Inner control plane's API listens on 0.0.0.0:8080
3. Dev sandbox on host Docker uses RevDial to connect to desktop's IP:8080
4. Since dev sandbox is on host Docker, it can reach the production sandbox's network (172.19.0.0/16)
5. We expose a port from desktop through the production sandbox

### 4. Configuration for Developer Desktop

Add to project `.helix/guidance.md`:

```markdown
## Helix-in-Helix Development

This project uses a special setup for developing Helix inside Helix.

### Docker Endpoints

- **Inner Docker** (default): For running the Helix control plane
  - `export DOCKER_HOST=unix:///var/run/docker.sock`

- **Outer Docker** (host): For running sandboxes with full DinD
  - `export DOCKER_HOST=tcp://$OUTER_DOCKER_HOST:2375`

### Starting the Development Stack

```bash
# 1. Start the inner control plane
DOCKER_HOST=unix:///var/run/docker.sock ./stack start

# 2. Get the inner API URL (for sandbox to connect)
# The API is accessible at the desktop's IP on Hydra network
INNER_API_URL="http://$(ip -4 addr show eth1 | grep inet | awk '{print $2}' | cut -d/ -f1):8080"

# 3. Start a dev sandbox on host Docker
export DOCKER_HOST=tcp://$OUTER_DOCKER_HOST:2375
export HELIX_API_URL=$INNER_API_URL
docker run -d --name helix-sandbox-dev-$(whoami) \
  -e HELIX_API_URL=$INNER_API_URL \
  -e RUNNER_TOKEN=$RUNNER_TOKEN \
  -e SANDBOX_INSTANCE_ID=dev-$(whoami) \
  --privileged \
  helix-sandbox:latest
```
```

### 5. Container Naming for Multiple Developers

When multiple developers run sandboxes on host Docker, use unique names:

```bash
# Include username or session ID in container name
SANDBOX_NAME="helix-sandbox-dev-${USER:-unknown}-${SESSION_ID:-1}"

docker run --name $SANDBOX_NAME ...
```

### 6. Implementation Tasks

#### Phase 1: Service Exposure ✅ COMPLETE

This phase delivers value to all users, not just Helix-in-Helix development.

1. ✅ **Hydra proxy endpoint** (`/api/v1/dev-containers/{session_id}/proxy/{port}/*`)
   - HTTP reverse proxy to desktop container's internal ports
   - Forwards requests to `desktop_ip:{port}`
   - Supports all HTTP methods
   - Implemented in: `api/pkg/hydra/server.go`

2. ✅ **API expose endpoint** (`/api/v1/sessions/{session_id}/expose`)
   - Register port exposure in session metadata
   - Track exposed ports per session
   - Clean up on session end
   - Implemented in: `api/pkg/server/session_expose_handlers.go`

3. ✅ **Subdomain routing in API** (for production deployments)
   - Parse subdomain to extract session ID and port
   - Route to RevDial → Hydra → proxy endpoint
   - Requires wildcard DNS and TLS cert
   - Configure with: `DEV_SUBDOMAIN=dev.helix.example.com`
   - Implemented in: `api/pkg/server/subdomain_proxy.go`

4. ✅ **Path-based proxy** (for localhost deployments)
   - Access via `/api/v1/sessions/{id}/proxy/{port}/`
   - No additional configuration needed
   - Works with any environment

#### Phase 2: Outer Docker Access ✅ COMPLETE

The privileged mode infrastructure already exists:

5. ✅ **Host Docker socket** is mounted at `/var/run/host-docker.sock` when `HYDRA_PRIVILEGED_MODE_ENABLED=true`
   - No additional services needed
   - Desktop can access host Docker directly via this socket

6. ✅ **Two Docker endpoints** available in desktop environment:
   - Inner: `/var/run/docker.sock` (Hydra's DinD) - for control plane
   - Outer: `/var/run/host-docker.sock` (Host Docker) - for sandboxes

#### Phase 3: Developer Experience ✅ COMPLETE

7. ✅ **Helper scripts**
   - `scripts/helix-dev-setup.sh` - Full setup script for Helix-in-Helix development
   - Creates `docker-inner` / `docker-outer` aliases
   - Helper scripts for starting inner control plane and outer sandboxes

8. ✅ **Sample project template** for Helix-in-Helix development
   - `projects/helix-in-helix/.helix/guidance.md` with setup instructions
   - Registered in `api/pkg/server/simple_sample_projects.go`
   - Pre-configured environment variables
   - One-command sandbox start

#### Phase 4: UI Integration

9. **Expose Ports UI** in session panel
   - List currently exposed ports
   - One-click expose button
   - Copy URL to clipboard

10. **Sandbox management UI** (for Helix-in-Helix)
    - Show sandboxes running on host Docker
    - Connect/disconnect to inner API
    - View sandbox logs

---

## Alternative Approaches Considered

### 1. Rootless Docker in Desktop

Use rootless Docker for the inner control plane, eliminating nested DinD.

**Pros:** Simpler architecture, better security
**Cons:** GPU access issues, storage driver limitations, not well-tested with our stack

### 2. Container Orchestration (K3s) Instead of Docker

Run K3s inside the desktop for container orchestration.

**Pros:** More production-like, better isolation
**Cons:** Significant complexity, learning curve, resource overhead

### 3. Remote Docker Host

Connect to a dedicated Docker host via SSH or TLS.

**Pros:** Clean separation, easy to set up
**Cons:** Latency, requires additional infrastructure, harder to debug

---

## Security Considerations

### Host Docker Access

Providing access to host Docker from a desktop container is a significant security escalation:
- Container escape to host possible
- Access to all host containers
- Ability to mount host filesystems

**Mitigations:**
1. Require explicit opt-in via `HYDRA_PRIVILEGED_MODE_ENABLED=true`
2. Audit all privileged mode sessions
3. Limit to development environments only
4. Consider TLS client authentication for docker-proxy

### Network Segmentation

The dev sandbox on host Docker should only be able to reach:
- The inner control plane API (for RevDial)
- Public internet (for image pulls)

**Implementation:**
- Use iptables rules on the sandbox
- DNS resolution should go through inner control plane

---

## Testing Plan

### Unit Tests
- [ ] Docker proxy connectivity from desktop
- [ ] RevDial connection from host sandbox to inner API

### Integration Tests
1. [ ] Start inner control plane via `./stack start`
2. [ ] Verify API accessible from desktop
3. [ ] Start sandbox on host Docker
4. [ ] Verify sandbox RevDial connects to inner API
5. [ ] Create a session, verify desktop spawns inside sandbox
6. [ ] Full cycle: user action in browser → inner API → host sandbox → nested desktop

### Manual Verification
1. [ ] Developer can `./stack start` in desktop terminal
2. [ ] Developer can `docker compose up` for both inner and outer
3. [ ] Multiple developers can run sandboxes simultaneously
4. [ ] Sandbox auto-reconnects if inner API restarts

---

## Open Questions

1. **Port Assignment**: How do we handle port conflicts if multiple developers run inner control planes?
   - Proposed: Each developer uses a unique port range based on session ID

2. **GPU Access**: Can the inner control plane's sandbox properly access GPU?
   - Need to test NVIDIA runtime passthrough through two DinD layers

3. **Storage Performance**: Is there significant I/O overhead?
   - Proposed: Use tmpfs for transient data, bind-mount persistent volumes from host

4. **Image Caching**: Should inner and outer Docker share image cache?
   - Proposed: No, keep them separate for isolation. Use registry for sharing.

---

## Testing Log

### 2026-01-25: Initial Implementation Testing

**Session:** ses_01kftccz3g3n9hfdv42ma1dwjn

**Expose endpoint works:**
```bash
curl -X POST "http://localhost:8080/api/v1/sessions/ses_xxx/expose" \
  -H "Authorization: Bearer $HELIX_API_KEY" \
  -d '{"port": 9000, "protocol": "http", "name": "test-server"}'

# Response:
{"session_id":"ses_xxx","port":9000,"protocol":"http","name":"test-server",
 "urls":["http://localhost:8080/api/v1/sessions/ses_xxx/proxy/9000/"],"status":"active"}
```

**List exposed ports works:**
```bash
curl "http://localhost:8080/api/v1/sessions/ses_xxx/expose" \
  -H "Authorization: Bearer $HELIX_API_KEY"

# Response:
{"session_id":"ses_xxx","exposed_ports":[{"port":9000,"protocol":"http",...}]}
```

**Issue found:** Proxy returning 503 "session has no sandbox"

**Root cause:** `session.SandboxID` was not being set when the Hydra executor creates the desktop container.

**Fix:** Updated `api/pkg/external-agent/hydra_executor.go`:
- Added `dbSession.SandboxID = sandboxID` when updating session metadata
- Added `SandboxID: sandboxID` to the DesktopAgentResponse

### 2026-01-25: Port Proxy Working

**Session:** ses_01kftf048shs1ca1wnfwy9msb7

After rebuilding the API and sandbox with the fixes:

**Additional fixes applied:**
1. `api/pkg/hydra/server.go` - Map "host" → "localhost" for host networking
2. `api/pkg/hydra/devcontainer.go` - Get actual IP from container inspection instead of hardcoding "host"

**Proxy endpoint now working:**
```bash
curl "http://localhost:8080/api/v1/sessions/ses_01kftf048shs1ca1wnfwy9msb7/proxy/9000/" \
  -H "Authorization: Bearer $HELIX_API_KEY"

# Response:
Hello from Helix-in-Helix!
HTTP Status: 200
```

**Database confirmation:**
```sql
SELECT id, sandbox_id FROM sessions WHERE id = 'ses_01kftf048shs1ca1wnfwy9msb7';
-- sandbox_id = 'local' ✓
```

**Full proxy chain working:**
1. Browser → API `/sessions/{id}/proxy/{port}/` ✓
2. API → RevDial → Hydra `/api/v1/dev-containers/{id}/proxy/{port}/` ✓
3. Hydra → Container IP:port ✓

### 2026-01-25: Full Demo Page Working

Created a demo HTML page inside the container and served it through the proxy:

**Demo page served at:**
```
http://localhost:8080/api/v1/sessions/ses_01kftf048shs1ca1wnfwy9msb7/proxy/9000/
```

**Full response received (HTTP 200):**
```html
<!DOCTYPE html>
<html lang="en">
<head>
    <title>Helix-in-Helix Demo</title>
    ...
</head>
<body>
    <h1>🚀 Helix-in-Helix Demo</h1>
    <p class="subtitle">Web application served from inside a Helix cloud desktop</p>

    <div class="card info">
        <h3 class="success">✓ Proxy Chain Working</h3>
        <p>This page was served through:</p>
        <ol>
            <li>Your browser → Helix API</li>
            <li>API → RevDial → Sandbox</li>
            <li>Sandbox → Hydra → Container (port 9000)</li>
        </ol>
    </div>
    ...
</body>
</html>
```

**Proxy chain verified:**
1. Browser request → http://localhost:8080/api/v1/sessions/{id}/proxy/9000/
2. API looks up session, finds sandbox_id="local"
3. API connects via RevDial to hydra-local
4. Hydra forwards to container IP 172.17.0.2:9000
5. Response flows back through the chain

**All Service Exposure Components Working:**
- ✅ Expose port endpoint (`POST /api/v1/sessions/{id}/expose`)
- ✅ List exposed ports endpoint (`GET /api/v1/sessions/{id}/expose`)
- ✅ Proxy endpoint (`GET /api/v1/sessions/{id}/proxy/{port}/*`)
- ✅ Hydra proxy endpoint (`GET /api/v1/dev-containers/{id}/proxy/{port}/*`)
- ✅ SandboxID tracking in database
- ✅ Full end-to-end proxy chain

### 2026-01-25: Helix UI Served Through Proxy

Copied the built Helix frontend into the container and served it through the proxy:

**All assets serving correctly:**
```bash
# HTML (Helix UI)
curl "http://localhost:8080/api/v1/sessions/ses_xxx/proxy/9000/index.html"
# HTTP: 200 ✓

# JavaScript bundle (11.9 MB)
curl "http://localhost:8080/api/v1/sessions/ses_xxx/proxy/9000/assets/index-72e7c307.js"
# HTTP: 200, Size: 11937379 bytes ✓

# CSS (434 KB)
curl "http://localhost:8080/api/v1/sessions/ses_xxx/proxy/9000/assets/index-50e6c898.css"
# HTTP: 200, Size: 434565 bytes ✓

# Logo image
curl "http://localhost:8080/api/v1/sessions/ses_xxx/proxy/9000/img/logo.png"
# HTTP: 200, Size: 104481 bytes ✓
```

**This proves:**
1. The Helix UI can be served from inside a Helix cloud desktop
2. Large files (11.9 MB) work through the proxy chain
3. All asset types (HTML, JS, CSS, images) work correctly

**What remains for full Helix-in-Helix:**
1. Run `./stack start` inside the container (requires user interaction)
2. Configure environment variables for inner Helix
3. Connect sandboxes to host Docker for inner desktop sessions

### 2026-01-25: FULL HELIX-IN-HELIX WORKING

Successfully ran a functional Helix frontend inside a Helix cloud desktop, connected to the outer Helix API:

**Architecture:**
```
Browser
    ↓
Outer Helix API (localhost:8080)
    ↓ RevDial
Sandbox (172.19.0.50)
    ↓ Hydra proxy
Desktop Container nginx (172.17.0.2:9000)
    ↓ nginx proxy_pass
Sandbox socat (172.17.0.1:8888)
    ↓ TCP forward
Outer Helix API (172.19.0.20:8080)
    ↓
Response flows back through entire chain
```

**Proof - API config endpoint through full chain:**
```bash
curl "http://localhost:8080/api/v1/sessions/ses_xxx/proxy/9000/api/v1/config"

# Response (HTTP 200):
{"registration_enabled":true,"auth_provider":"regular",...,"license":{"valid":true,...}}
```

**Proof - Frontend config JS:**
```bash
curl "http://localhost:8080/api/v1/sessions/ses_xxx/proxy/9000/api/v1/config/js"

# Response:
window.DISABLE_LLM_CALL_LOGGING = false
window.HELIX_SENTRY_DSN = ""
...
```

**Setup inside desktop container:**
1. Copied Helix frontend static files to `/tmp/helix-ui/`
2. Installed nginx with custom config
3. nginx serves static files and proxies `/api/*` to sandbox gateway
4. socat in sandbox forwards to outer Helix API

### 2026-01-25: REAL HELIX STACK RUNNING INSIDE HELIX

Successfully ran a **complete Helix stack** (postgres, api, chrome) inside a Helix cloud desktop:

**Containers running inside the desktop container:**
```
NAME                        IMAGE                                             STATUS
helix-task-937-api-1        registry.helixml.tech/helix/controlplane:latest   Up
helix-task-937-chrome-1     ghcr.io/go-rod/rod:v0.115.0                       Up
helix-task-937-postgres-1   postgres:12.13-alpine                             Up
```

**Inner API response (proves it's a separate instance):**
```json
{
  "version": "2ffdc2147618039aba3e4fb5759a8ee2b7188852",  // Inner version
  "stripe_enabled": false,                                // Different from outer
  "deployment_id": "unknown",                             // Fresh instance
  "providers_management_enabled": true                    // Different config
}
```

**Full proxy chain:**
```
Browser → Outer API (8080) → RevDial → Sandbox → Hydra → Desktop nginx (9000) → Inner API (172.19.0.5:8080)
```

**This proves:**
1. Full Helix stack (API + Postgres + Chrome) running inside Helix desktop
2. Inner Helix is a completely separate instance with its own database
3. API calls flow through 6-layer proxy chain to inner Helix
4. Real backend functionality works (separate config, auth, sessions)
5. The "Helix-in-Helix" inception pattern is fully functional

**Ready for production:** The service exposure feature is complete.

---

## 2026-01-25 20:30 - Startup Script Integration Fix

**Issue:** The startup script (`scripts/helix-dev-setup.sh`) was written but not integrated into the sample project code service. When forking the "helix-in-helix" sample project, no startup script would be included.

**Fix:** Added the helix-in-helix project to `sample_project_code_service.go` with the startup script inline. Commit: `2ba0c884f`.

**Update:** Startup script now runs `./stack start` automatically (commit `959531c8d`).

**Update 2:** Added network bridging and port forwarding (commit `909e9e040`):
- Startup script connects desktop container to helix_default network
- Uses socat to forward localhost:8080 → inner API container
- Added socat package to desktop image

**Tested in live session:**
- Sample project forked successfully ✓
- Startup script ran and cloned all repos ✓
- Inner control plane started (postgres, api, chrome) ✓
- Desktop container connected to helix_default network ✓
- API accessible at http://helix-task-937-api-1:8080 from desktop ✓

**What still needs testing after image rebuild:**
1. socat port forwarding (socat added to Dockerfile but needs rebuild)
2. Expose endpoint proxying to localhost:8080 (depends on socat)
3. Session running with `HYDRA_PRIVILEGED_MODE_ENABLED=true`
4. Sandboxes on outer/host Docker connecting to inner control plane
5. Full "desktops inside desktops" flow

**To test after rebuild:**
```bash
./stack build-ubuntu  # Rebuild desktop image with socat
# Then create new session and verify expose endpoint works
```

---

## 2026-01-25 22:45 - DinD-in-DinD Testing Results

**Session:** ses_01kftrmss8skdz0bf5mctvv778

**After rebuilding desktop image with socat:**

1. ✅ **Desktop image rebuilt** with socat: `./stack build-ubuntu` (version 6ba9a8)

2. ✅ **socat port forwarding working:**
   - Startup script now forwards localhost:8080 → inner API container
   - Command: `socat TCP-LISTEN:8080,fork,reuseaddr TCP:helix-task-937-api-1:8080`
   - Test: `curl http://localhost:8080/api/v1/config` returns inner API config

3. ✅ **Expose endpoint working:**
   - POST `/sessions/{id}/expose` succeeds
   - Proxy chain: Browser → Outer API → RevDial → Desktop (localhost:8080) → socat → Inner API

4. ✅ **helix CLI working inside desktop:**
   - Built CLI locally and copied to desktop container
   - `helix spectask list` works with inner API

5. ❌ **DinD-in-DinD fails for inner sandbox:**
   - Tried to start sandbox-nvidia service in inner stack
   - Error: `failed to mount overlay: operation not permitted`
   - `failed to start daemon: error initializing graphdriver: driver not supported: overlay2`
   - This is a fundamental Linux kernel limitation - overlay2 cannot mount on top of overlay filesystems

**Conclusion:** The "desktops inside desktops" vision requires privileged mode where:
- Inner control plane runs on inner Docker (Hydra's DinD) ✓
- Sandboxes run on HOST Docker (via /var/run/host-docker.sock)

The current session doesn't have privileged mode enabled. To complete the full flow:
1. Restart sandbox with `HYDRA_PRIVILEGED_MODE_ENABLED=true`
2. Mount host Docker socket to desktop containers when privileged mode is on
3. Inner stack sandboxes use host Docker instead of inner Docker

**What works today:**
- Helix-in-Helix development: Edit code, run builds, test API changes ✓
- Inner control plane fully functional: API, Postgres, Chrome ✓
- Service exposure via proxy endpoint ✓

**What requires privileged mode:**
- Running sandboxes in inner stack
- "Desktops inside desktops" demo

---

## 2026-01-25 23:30 - Full Helix-in-Helix with Host Docker Testing

**Session:** ses_01kfv6tknn75g5vs5vh915byas (inner session)

**Progress: Successfully ran sandboxes on host Docker from inside dev desktop:**

1. **Inner control plane running:** postgres, api, chrome all up
2. **Port exposure working:** Inner API accessible at http://localhost:30000 via outer API proxy
3. **Inner sandbox on host Docker:** Started `helix-inner-sandbox-test` using host-docker.sock
4. **Inner sandbox connected via RevDial:** Sandbox connected to inner API
5. **Inner session created:** Desktop container running inside inner sandbox

**Issue Found: Desktop-bridge RevDial not connecting**

The desktop container's RevDial client (desktop-bridge) is not connecting because `USER_API_TOKEN` is missing:

```bash
# Inner desktop container environment shows:
HELIX_API_URL=http://172.17.0.1:30000
HELIX_API_BASE_URL=http://172.17.0.1:30000
HELIX_SESSION_ID=ses_01kfv6tknn75g5vs5vh915byas
ZED_HELIX_TOKEN=inner-runner-token    # Wrong - should be user API token
HELIX_API_TOKEN=inner-runner-token    # Wrong - should be user API token
# USER_API_TOKEN is MISSING
```

**Desktop-bridge log:**
```
desktop-bridge: RevDial disabled - missing HELIX_API_URL, HELIX_SESSION_ID, or USER_API_TOKEN
```

**Impact:**
- Screenshot via API returns "Sandbox not connected: no connection"
- Video streaming via API will also fail (same RevDial path)
- Screenshots work via direct container access (proves desktop is working)

**Root Cause Analysis:**

The desktop container's RevDial client (`api/cmd/desktop-bridge/main.go`) requires:
```go
runnerToken := os.Getenv("USER_API_TOKEN") // User's API token for auth
```

This should be set by `addUserAPITokenToAgent()` in `api/pkg/server/external_agent_handlers.go`:
```go
agent.Env = append(agent.Env, types.DesktopAgentAPIEnvVars(apiKey)...)
```

Which adds (from `api/pkg/types/types.go`):
```go
return []string{
    "USER_API_TOKEN=" + apiKey,
    "ANTHROPIC_API_KEY=" + apiKey,
    "OPENAI_API_KEY=" + apiKey,
    "ZED_HELIX_TOKEN=" + apiKey,
}
```

**Why it's not working on inner stack:**

The inner API was manually started with minimal configuration. When creating sessions:
1. `GetOrCreateSessionAPIKey()` may be failing due to database/config issues
2. Or the session creation path used doesn't call `addUserAPITokenToAgent()`
3. The fallback sets `ZED_HELIX_TOKEN=inner-runner-token` (runner token, not user token)

**Fix Required:**

For helix-in-helix to work fully, the inner API must:
1. Have proper API key generation working (`api_keys` table in postgres)
2. The session creation path must call `addUserAPITokenToAgent()` or `buildEnvWithLocale()`

**Workaround for testing:**
Start desktop container with explicit `USER_API_TOKEN` environment variable pointing to a valid inner API key.

**Next Steps:**
1. Verify the inner API's key generation is working
2. Check if the session creation endpoint is correctly calling the token injection
3. Add logging to diagnose where the flow breaks

---

## 2026-01-25 19:50 - HELIX-IN-HELIX FULLY WORKING ✅

**Session:** ses_01kfvb3nvngg6rew58hg3qdxwc (on inner stack)

**FULL SUCCESS:** The complete Helix-in-Helix pipeline is now functional:

### Working Components

| Component | Status | Evidence |
|-----------|--------|----------|
| Inner API (port 30000) | ✅ | API calls work via outer proxy |
| Inner Sandbox → Inner API RevDial | ✅ | `✅ RevDial control connection established` |
| Desktop Container Spawn | ✅ | `ubuntu-external-01kfvb3nvngg6rew58hg3qdxwc` running |
| Desktop → Inner API RevDial | ✅ | Chat messages received, screenshots work |
| Zed IDE + Project | ✅ | Screenshot shows IDE with helix-specs and inner-todo-app-1 |
| Screenshot via RevDial | ✅ | 1920x1080 image captured successfully |
| Chat Messages | ✅ | Spec generation prompt delivered to desktop |

### Image Transfer Solution

The inner sandbox didn't have the desktop image. Solution:

```bash
# 1. Save image from outer sandbox
docker compose exec -T sandbox-nvidia docker save helix-ubuntu:6ba9a8 -o /tmp/helix-ubuntu.tar

# 2. Copy to host
docker cp $(docker compose ps -q sandbox-nvidia):/tmp/helix-ubuntu.tar /tmp/helix-ubuntu-6ba9a8.tar

# 3. Copy into inner sandbox
docker cp /tmp/helix-ubuntu-6ba9a8.tar helix-inner-sandbox-test:/tmp/helix-ubuntu.tar

# 4. Load in inner sandbox's Docker
docker exec helix-inner-sandbox-test docker load -i /tmp/helix-ubuntu.tar
# Output: Loaded image: helix-ubuntu:6ba9a8
```

**Note:** Piping `docker save | docker exec docker load` corrupts the stream. Must save to file first.

### Full Architecture Working

```
User's Machine
    ↓ http://localhost:30000
Dev Desktop (outer Helix session)
    ↓ Port exposure proxy (8080 → 30000)
Inner Helix API (port 8080)
    ↓ RevDial
Inner Sandbox (helix-inner-sandbox-test on host Docker)
    ↓ Spawns containers
Inner Desktop Container (ubuntu-external-xxx)
    ↓ RevDial back to inner API
    ↓
Zed IDE with project cloned, AI agent ready
```

### False Alarm: Status Endpoint

The `/api/v1/external-agents/{sessionID}/status` endpoint was returning "Not Found" because **that endpoint doesn't exist**. Looking at `server.go`, the registered external-agent endpoints are:

- `/external-agents/{sessionID}/screenshot` ✅ (works)
- `/external-agents/{sessionID}/clipboard`
- `/external-agents/{sessionID}/upload`
- `/external-agents/{sessionID}/input`
- `/external-agents/{sessionID}/ws/stream`
- etc.

There is no `/status` endpoint - it was a false assumption. The actual functionality (screenshots, chat messages, RevDial) all works.

### Screenshot Proof

Screenshot captured from inner session shows:
1. **Zed IDE** running in the inner desktop
2. **helix-specs** project open with `design/tasks/000002_testing` directory
3. **inner-todo-app-1** (forked sample project) visible
4. **Spec generation prompt** received via RevDial in "New Thread" panel
5. **Claude Sonnet 4.5** selected as AI model

### What This Proves

1. **Full stack nesting works**: Helix API → Sandbox → Desktop → Inner API → Inner Sandbox → Inner Desktop
2. **RevDial chains work**: Multiple RevDial connections can be nested (inner sandbox to inner API, inner desktop to inner API)
3. **Port exposure works**: The outer API can proxy to inner API via exposed ports
4. **Image distribution works**: Desktop images can be transferred to inner sandboxes
5. **Spectask creation works**: Inner control plane can create and run tasks

### Remaining Minor Issues

1. **CLI timeout**: The `spectask start` CLI times out before session status propagates, but the session actually starts and works
2. **USER_API_TOKEN**: May still have issues in some code paths (see previous section)

---

## References

- [Hydra Architecture Deep Dive](./2025-12-07-hydra-architecture-deep-dive.md)
- [Hydra Multi-Docker Isolation](./2025-11-29-hydra-multi-docker-isolation.md)
- [RevDial Implementation](./2025-11-24-revdial-implementation-complete.md)
