# Remote Wolf Architecture Design

## Executive Summary

This document outlines the architecture for enabling Wolf streaming server and its containers to run on a separate physical host from the Helix control plane, using **reverse dial connections** to eliminate the need for inbound connectivity from control plane → agent runner.

**Key Requirements:**
- ✅ No inbound connectivity needed from control plane to agent runner
- ✅ Reverse dial connection from agent runner to control plane
- ✅ Existing runner token authentication
- ✅ Settings daemon/screenshot server connection reversal
- ✅ WebRTC and Moonlight protocol port handling

**Timeline:** Analysis only for now (implementation after 4pm demo)

---

## Current Architecture Analysis

### 1. Wolf Executor (Control Plane Component)

**Location:** `/home/luke/pm/helix/api/pkg/external-agent/wolf_executor.go`

**Current Communication Pattern:**
```go
type WolfExecutor struct {
    wolfClient *wolf.Client  // Connects via Unix socket: /var/run/wolf/wolf.sock
    // ...
}
```

**Direction:** Control plane → Wolf (via Unix socket)
- **Works when:** Wolf on same host (Unix socket accessible)
- **Breaks when:** Wolf on remote host (Unix socket not accessible)

**Wolf API Calls Made:**
- `AddApp()` - Create application configurations in Wolf
- `CreateLobby()` - Start streaming session (container launches immediately)
- `StopLobby()` - Tear down container
- `ListApps()`, `ListLobbies()` - Query Wolf state
- `GetPairedClients()` - List Moonlight client certificates

### 2. Settings Sync Daemon (Container → Control Plane)

**Location:** `/home/luke/pm/helix/api/cmd/settings-sync-daemon/main.go`

**Current Communication Pattern:**
```go
// Inside Zed container
helixURL := os.Getenv("HELIX_API_URL")  // Default: "http://api:8080"
helixToken := os.Getenv("HELIX_API_TOKEN")
sessionID := os.Getenv("HELIX_SESSION_ID")

// HTTP requests TO control plane
GET  /api/v1/sessions/{sessionID}/zed-config       // Fetch Helix settings
POST /api/v1/sessions/{sessionID}/zed-config/user  // Send user overrides
```

**Direction:** Container → Control plane (HTTP client in container)
- **Works when:** Control plane has accessible HTTP endpoint
- **Breaks when:** No inbound connectivity to control plane allowed

**Polling Behavior:**
- Polls every 30 seconds for Helix config changes
- Uses fsnotify to watch local settings.json for Zed UI changes
- Debounces rapid writes (500ms)

### 3. Screenshot Server (Container → Control Plane)

**Location:** `/home/luke/pm/helix/api/cmd/screenshot-server/main.go`

**Current Communication Pattern:**
```go
// HTTP server running inside Zed container
http.HandleFunc("/screenshot", handleScreenshot)
http.ListenAndServe(":9876", nil)  // Default port 9876
```

**Direction:** Control plane → Container (HTTP GET)
- **Works when:** Container has exposed port accessible from control plane
- **Breaks when:** No inbound connectivity to agent runner allowed

**Functionality:**
- Runs `grim` to capture Wayland screenshot
- Returns PNG image via HTTP response
- Used for Helix UI to display agent desktop preview

### 4. WebSocket Connection for Zed Thread Sync

**Location:** `/home/luke/pm/helix/api/pkg/server/websocket_server_runner.go`

**Current Communication Pattern:**
```go
// WebSocket endpoint on control plane
r.HandleFunc("/api/v1/runner/websocket", func(w http.ResponseWriter, r *http.Request) {
    // Authenticate runner via token
    user := authMiddleware.getUserFromToken(r.Context(), getRequestToken(r))
    if !isRunner(user) {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }

    // Extract runner ID from query parameter
    runnerID := r.URL.Query().Get("runnerid")

    // Upgrade to WebSocket
    conn := userWebsocketUpgrader.Upgrade(w, r, nil)
})
```

**Direction:** Container → Control plane (WebSocket client in container)
- **Works when:** Control plane has accessible WebSocket endpoint
- **Breaks when:** No inbound connectivity to control plane allowed

**Message Types:**
- `WebsocketLLMInferenceResponse` - LLM inference responses
- `WebsocketEventSessionUpdate` - Delta session updates
- `WebsocketEventWorkerTaskResponse` - Complete task responses

### 5. Moonlight Protocol Ports

**Current Implementation:** Moonlight proxy exists but is NOT currently used
**Location:** `/home/luke/pm/helix/api/pkg/moonlight/proxy.go`

**Moonlight Protocol Ports:**
```
47984  TCP  HTTPS (certificate exchange, control)
47989  TCP  HTTP (serverinfo, pairing, launch)
48010  TCP  RTSP (streaming session negotiation)
47999  UDP  Control messages
48100  UDP  Video RTP stream
48200  UDP  Audio RTP stream
```

**Current Architecture:**
- Moonlight client connects **directly** to Wolf server ports
- No proxying through control plane currently implemented
- Moonlight proxy exists in codebase but not actively used

**UDP-over-TCP Encapsulation Protocol:**
The existing moonlight proxy implements a custom protocol for tunneling UDP over TCP:
```go
type UDPPacketHeader struct {
    Magic     uint32 // 0xDEADBEEF
    Length    uint32 // Length of UDP payload
    SessionID uint64 // Moonlight session ID for routing
    Port      uint16 // Original UDP port (47999, 48100, 48200)
}
```

**Why Direct Connection Works Now:**
- Wolf and Moonlight clients on same network (or public internet)
- No NAT traversal issues in current deployment
- Low-latency direct UDP for video/audio streaming

### 6. Reverse Dial Infrastructure (Already Exists!)

**Location:** `/home/luke/pm/helix/api/pkg/connman/connman.go`

**Existing Mechanism:**
```go
type ConnectionManager struct {
    deviceDialers     map[string]*revdial.Dialer
    deviceConnections map[string]net.Conn
    lock              sync.RWMutex
}

func (m *ConnectionManager) Set(key string, conn net.Conn) {
    // Use proper revdial.Dialer for multiplexing multiple logical connections
    m.deviceDialers[key] = revdial.NewDialer(conn, "/revdial")
}

func (m *ConnectionManager) Dial(ctx context.Context, key string) (net.Conn, error) {
    dialer, ok := m.deviceDialers[key]
    if !ok {
        return nil, ErrNoConnection
    }
    // Use revdial.Dialer to create a new logical connection
    return dialer.Dial(ctx)
}
```

**Key Discovery:**
- ✅ Helix **already has** reverse dial infrastructure (`revdial` package)
- ✅ `ConnectionManager` manages reverse connections keyed by runner ID
- ✅ `revdial.Dialer` multiplexes multiple logical connections over single TCP connection
- ✅ Used for runner → control plane reverse dial connections

---

## Proposed Remote Wolf Architecture

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          CONTROL PLANE HOST                             │
│                         (Public Internet / VPC)                         │
│                                                                         │
│  ┌────────────────────────────────────────────────────────────────┐   │
│  │ Helix API Server (Port 8080)                                   │   │
│  │                                                                 │   │
│  │  ┌──────────────────────────────────────────────────────────┐ │   │
│  │  │ WolfReverseProxy Service                                 │ │   │
│  │  │                                                           │ │   │
│  │  │  • Accepts reverse dial from Wolf sidecar (runnerID)    │ │   │
│  │  │  • Multiplexes Wolf API calls via revdial.Dialer        │ │   │
│  │  │  • Proxies settings sync HTTP (reverse direction)       │ │   │
│  │  │  • Proxies screenshot HTTP (reverse direction)          │ │   │
│  │  │  • WebSocket sync already works (uses revdial)          │ │   │
│  │  └──────────────────────────────────────────────────────────┘ │   │
│  │                                                                 │   │
│  │  ┌──────────────────────────────────────────────────────────┐ │   │
│  │  │ MoonlightProxy Service (OPTIONAL - see analysis)         │ │   │
│  │  │                                                           │ │   │
│  │  │  • Public ports: 47984, 47989, 48010, 47999, 48100,     │ │   │
│  │  │                  48200                                    │ │   │
│  │  │  • UDP-over-TCP encapsulation for RTP streams           │ │   │
│  │  │  • Routes sessions by client IP and session ID          │ │   │
│  │  └──────────────────────────────────────────────────────────┘ │   │
│  └────────────────────────────────────────────────────────────────┘   │
│         ▲                                                              │
│         │ Reverse Dial (Outbound from Agent Runner)                   │
│         │ Authentication: Runner Token (existing)                     │
└─────────┼──────────────────────────────────────────────────────────────┘
          │
          │ INTERNET / VPN CONNECTION
          │ (Firewall blocks all inbound to agent runner)
          │
┌─────────┼──────────────────────────────────────────────────────────────┐
│         │              AGENT RUNNER HOST (Sandboxed)                   │
│         │                                                              │
│  ┌──────┴──────────────────────────────────────────────────────────┐  │
│  │ Wolf Sidecar (Go Service)                                       │  │
│  │                                                                  │  │
│  │  • Initiates reverse dial to control plane on startup          │  │
│  │  • Authenticates with runner token                             │  │
│  │  • Exposes Wolf API locally via HTTP proxy                     │  │
│  │  • Proxies all Wolf API calls over revdial                     │  │
│  │  • Handles settings sync HTTP (reverse polarity)               │  │
│  │  • Handles screenshot HTTP (reverse polarity)                  │  │
│  │  • WebSocket sync uses existing revdial                        │  │
│  │                                                                  │  │
│  │  Local Unix Socket: /var/run/wolf/wolf.sock → HTTP proxy       │  │
│  │                     http://localhost:47989 → HTTP proxy         │  │
│  └──────────────────────────────────────────────────────────────────┘  │
│         │                                                              │
│         │ Unix Socket / Docker Network                                │
│         ▼                                                              │
│  ┌────────────────────────────────────────────────────────────────┐   │
│  │ Wolf Server (Container)                                        │   │
│  │                                                                 │   │
│  │  • Wolf API: /var/run/wolf/wolf.sock (local only)            │   │
│  │  • Moonlight ports: 47984, 47989, 48010, 47999, 48100, 48200 │   │
│  │                     (exposed on agent runner host)             │   │
│  └────────────────────────────────────────────────────────────────┘   │
│         │                                                              │
│         │ Docker Network                                               │
│         ▼                                                              │
│  ┌────────────────────────────────────────────────────────────────┐   │
│  │ Zed Container (PDE/External Agent Session)                     │   │
│  │                                                                 │   │
│  │  • Settings Sync Daemon → Wolf Sidecar HTTP proxy             │   │
│  │  • Screenshot Server → Wolf Sidecar HTTP proxy                │   │
│  │  • WebSocket Sync → Control plane (via revdial)               │   │
│  │  • Zed Editor + Wayland compositor                             │   │
│  └────────────────────────────────────────────────────────────────┘   │
│                                                                        │
│  ┌─ OPTIONAL: Moonlight Port Exposure ─────────────────────────────┐  │
│  │                                                                  │  │
│  │  If control plane acts as Moonlight proxy:                      │  │
│  │    → Wolf sidecar tunnels UDP over TCP via revdial             │  │
│  │    → No public ports on agent runner                           │  │
│  │                                                                  │  │
│  │  If clients connect directly to agent runner:                   │  │
│  │    → Agent runner exposes Moonlight ports publicly             │  │
│  │    → Low-latency direct UDP streaming                          │  │
│  │    → Requires inbound firewall rules (only for Moonlight)      │  │
│  └──────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────┘

LEGEND:
→  Direction of connection initiation
▲  Reverse dial (agent runner initiates, control plane accepts)
```

---

## Component Details

### 1. Wolf Sidecar (New Go Service on Agent Runner)

**Responsibilities:**
1. **Reverse Dial Establishment**
   - Connect to control plane on startup
   - Authenticate using existing runner token
   - Maintain persistent reverse dial connection
   - Auto-reconnect on disconnection

2. **Wolf API Proxy**
   - Accept local HTTP connections from WolfExecutor (via Docker network)
   - Proxy requests over reverse dial to control plane
   - Return responses back to WolfExecutor
   - No changes needed to Wolf server itself

3. **Settings Sync Reverse Proxy**
   - Run HTTP server on port 8080 (mimicking Helix API)
   - Accept connections from settings sync daemon
   - Forward requests over reverse dial to real Helix API
   - Return responses back to daemon

4. **Screenshot Server Reverse Proxy**
   - Accept screenshot requests from control plane (via revdial)
   - Forward to local screenshot server HTTP endpoint
   - Stream PNG response back over revdial

**Implementation Outline:**
```go
type WolfSidecar struct {
    controlPlaneURL string
    runnerToken     string
    runnerID        string

    // Reverse dial connection
    revDialConn net.Conn
    dialer      *revdial.Dialer

    // Local proxies
    wolfAPIProxy        *http.Server  // Proxies to Wolf Unix socket
    settingsSyncProxy   *http.Server  // Proxies FROM daemon TO control plane
    screenshotHandler   http.Handler  // Proxies FROM control plane TO screenshot server
}

func (s *WolfSidecar) Start() error {
    // 1. Establish reverse dial to control plane
    conn := s.dialControlPlane()  // Uses runner token for auth
    s.revDialConn = conn
    s.dialer = revdial.NewDialer(conn, "/revdial")

    // 2. Start Wolf API HTTP proxy (accepts local connections)
    go s.startWolfAPIProxy()  // http://localhost:47990 → Wolf Unix socket

    // 3. Start settings sync reverse proxy
    go s.startSettingsSyncProxy()  // Accept daemon → Forward to control plane

    // 4. Register screenshot handler with control plane
    go s.registerScreenshotHandler()

    // 5. Health check and reconnection loop
    go s.healthCheckLoop()

    return nil
}

func (s *WolfSidecar) startWolfAPIProxy() {
    // Create HTTP server that proxies to Wolf Unix socket
    http.HandleFunc("/api/v1/", s.proxyToWolfAPI)
    http.ListenAndServe(":47990", nil)
}

func (s *WolfSidecar) proxyToWolfAPI(w http.ResponseWriter, r *http.Request) {
    // Create request to Wolf via Unix socket
    wolfClient := wolf.NewClient("/var/run/wolf/wolf.sock")
    resp := wolfClient.Get(r.Context(), r.URL.Path)

    // Copy response back to caller
    io.Copy(w, resp.Body)
}

func (s *WolfSidecar) startSettingsSyncProxy() {
    // Mimic Helix API for settings sync daemon
    http.HandleFunc("/api/v1/sessions/", s.proxyToControlPlane)
    http.ListenAndServe(":8080", nil)  // Daemon connects here
}

func (s *WolfSidecar) proxyToControlPlane(w http.ResponseWriter, r *http.Request) {
    // Dial control plane via revdial
    conn := s.dialer.Dial(context.Background())

    // Send HTTP request over reverse dial connection
    req := httputil.NewRequest(r.Method, "http://controlplane"+r.URL.Path, r.Body)
    req.Header.Set("Authorization", "Bearer "+s.runnerToken)

    client := http.Client{Transport: &http.Transport{
        DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
            return conn, nil
        },
    }}

    resp := client.Do(req)

    // Copy response back to daemon
    io.Copy(w, resp.Body)
}

func (s *WolfSidecar) registerScreenshotHandler() {
    // Register with control plane to handle screenshot requests
    // Control plane will dial us via revdial when it needs a screenshot

    for {
        // Accept incoming screenshot request from control plane
        conn := s.acceptRevDialConnection()

        // Read HTTP request
        req := http.ReadRequest(bufio.NewReader(conn))

        // Forward to local screenshot server
        localResp := http.Get("http://localhost:9876/screenshot")

        // Write response back over revdial
        resp := http.Response{
            StatusCode: localResp.StatusCode,
            Body:       localResp.Body,
        }
        resp.Write(conn)
    }
}
```

**Docker Compose Configuration:**
```yaml
services:
  wolf-sidecar:
    build:
      context: .
      dockerfile: Dockerfile.wolf-sidecar
    environment:
      - CONTROL_PLANE_URL=https://helix-api.example.com
      - RUNNER_TOKEN=${RUNNER_TOKEN}
      - RUNNER_ID=${RUNNER_ID}
      - WOLF_SOCKET_PATH=/var/run/wolf/wolf.sock
    volumes:
      - /var/run/wolf:/var/run/wolf:rw  # Access to Wolf Unix socket
    networks:
      - default
    restart: always
    depends_on:
      - wolf
```

### 2. WolfReverseProxy Service (New Control Plane Component)

**Responsibilities:**
1. **Accept Reverse Dial Connections**
   - Listen for incoming reverse dial connections from agent runners
   - Authenticate using runner token (existing mechanism)
   - Store connection in ConnectionManager keyed by runnerID

2. **Wolf API Call Handling**
   - WolfExecutor sends requests to WolfReverseProxy instead of Unix socket
   - WolfReverseProxy dials agent runner via `connman.Dial(runnerID)`
   - Forwards request over revdial, returns response

3. **Settings Sync Reverse Proxy**
   - Accept settings sync requests from sidecar (via revdial)
   - Forward to real Helix API endpoints
   - Return responses back over revdial

4. **Screenshot Request Handling**
   - Accept screenshot requests from Helix API
   - Dial agent runner via revdial
   - Send HTTP GET to remote screenshot handler
   - Return PNG image to Helix API

**Implementation Outline:**
```go
type WolfReverseProxy struct {
    connman *connman.ConnectionManager
    store   store.Store
}

func (p *WolfReverseProxy) HandleWolfAPICall(runnerID, method, path string, body io.Reader) (*http.Response, error) {
    // Dial agent runner via existing revdial infrastructure
    conn := p.connman.Dial(context.Background(), runnerID)

    // Send HTTP request over revdial
    req := httputil.NewRequest(method, "http://localhost"+path, body)

    client := http.Client{Transport: &http.Transport{
        DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
            return conn, nil
        },
    }}

    return client.Do(req)
}

func (p *WolfReverseProxy) GetScreenshot(runnerID string) ([]byte, error) {
    // Dial agent runner via revdial
    conn := p.connman.Dial(context.Background(), runnerID)

    // Send screenshot request
    req := httputil.NewRequest("GET", "http://localhost/screenshot", nil)

    client := http.Client{Transport: &http.Transport{
        DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
            return conn, nil
        },
    }}

    resp := client.Do(req)
    return io.ReadAll(resp.Body)
}
```

### 3. Modified WolfExecutor (Control Plane)

**Changes Required:**
- Replace `wolf.NewClient(socketPath)` with `WolfReverseProxy.NewClient(runnerID)`
- All Wolf API calls go through reverse proxy instead of Unix socket
- No other logic changes needed

**Before:**
```go
type WolfExecutor struct {
    wolfClient *wolf.Client  // Direct Unix socket connection
}

func NewWolfExecutor(wolfSocketPath string) *WolfExecutor {
    return &WolfExecutor{
        wolfClient: wolf.NewClient(wolfSocketPath),
    }
}
```

**After:**
```go
type WolfExecutor struct {
    wolfProxy  *WolfReverseProxy
    runnerID   string
}

func NewWolfExecutor(wolfProxy *WolfReverseProxy, runnerID string) *WolfExecutor {
    return &WolfExecutor{
        wolfProxy: wolfProxy,
        runnerID:  runnerID,
    }
}

func (e *WolfExecutor) CreateSession(ctx context.Context, session *types.Session) error {
    // All Wolf API calls now go through reverse proxy
    resp := e.wolfProxy.HandleWolfAPICall(
        e.runnerID,
        "POST",
        "/api/v1/sessions/add",
        marshalJSON(session),
    )
    // ... handle response
}
```

### 4. Modified Settings Sync Daemon (Container)

**Changes Required:**
- Change `HELIX_API_URL` from `http://api:8080` to `http://wolf-sidecar:8080`
- Sidecar proxies requests to real control plane
- No other code changes needed

**Configuration:**
```bash
# Before (direct to control plane - won't work remotely)
HELIX_API_URL=http://api:8080

# After (via sidecar reverse proxy)
HELIX_API_URL=http://wolf-sidecar:8080
```

### 5. Screenshot Server (No Changes Needed)

**Current Implementation:**
- HTTP server on port 9876
- Handles `/screenshot` endpoint
- Returns PNG image

**Integration:**
- Wolf sidecar accepts screenshot requests from control plane (via revdial)
- Sidecar forwards to local screenshot server `http://localhost:9876/screenshot`
- Response flows back through sidecar to control plane

**No code changes needed in screenshot server itself!**

---

## Moonlight Protocol Port Analysis

### Current Direct Connection Architecture

**How It Works Now:**
1. Moonlight client discovers Wolf via mDNS or manual IP entry
2. Client connects **directly** to Wolf ports (47984, 47989, 48010, 47999, 48100, 48200)
3. Low-latency UDP for video/audio streaming (48100, 48200)
4. TCP for control and RTSP negotiation

**Why It Works:**
- Wolf and client on same network (or public internet)
- No NAT traversal issues
- Direct UDP = lowest possible latency

### Option 1: Keep Direct Connection (RECOMMENDED)

**Architecture:**
- Moonlight clients connect **directly** to agent runner host
- Agent runner exposes Moonlight ports publicly (or via VPN)
- No proxying through control plane
- Wolf sidecar still handles all other communication

**Pros:**
- ✅ Lowest latency (direct UDP streaming)
- ✅ No additional bandwidth on control plane
- ✅ Simple implementation (no UDP tunneling needed)
- ✅ Wolf handles all Moonlight protocol complexity

**Cons:**
- ❌ Requires inbound firewall rules for Moonlight ports on agent runner
- ❌ Clients need network access to agent runner host

**Firewall Configuration Required:**
```bash
# Agent runner firewall (ONLY for Moonlight)
iptables -A INPUT -p tcp --dport 47984 -j ACCEPT   # HTTPS
iptables -A INPUT -p tcp --dport 47989 -j ACCEPT   # HTTP
iptables -A INPUT -p tcp --dport 48010 -j ACCEPT   # RTSP
iptables -A INPUT -p udp --dport 47999 -j ACCEPT   # Control
iptables -A INPUT -p udp --dport 48100 -j ACCEPT   # Video
iptables -A INPUT -p udp --dport 48200 -j ACCEPT   # Audio

# All other inbound traffic BLOCKED (including control plane → agent runner)
```

**Verdict:** ✅ **Recommended approach**
- Moonlight streaming is inherently client → agent runner
- Direct connection optimal for latency
- Limited firewall exposure (only 6 ports, only for Moonlight protocol)
- No changes needed to Wolf or Moonlight protocol

### Option 2: Proxy Through Control Plane (NOT RECOMMENDED)

**Architecture:**
- Control plane exposes Moonlight ports publicly
- MoonlightProxy service routes traffic to agent runner via revdial
- UDP tunneled over TCP using existing encapsulation protocol

**Pros:**
- ✅ Zero inbound connectivity to agent runner
- ✅ Complete firewall lockdown of agent runner

**Cons:**
- ❌ Higher latency (extra hop + UDP→TCP→UDP conversion)
- ❌ More bandwidth on control plane (all video/audio flows through)
- ❌ Complex implementation (UDP tunneling, session routing)
- ❌ Potential packet loss during TCP congestion
- ❌ Wolf's existing Moonlight implementation expects direct UDP

**Implementation Challenges:**
1. Wolf server expects to bind directly to UDP ports
2. Would need to modify Wolf to send/receive via tunnel
3. UDP tunneling adds latency (critical for gaming/streaming)
4. TCP tunneling UDP can cause issues with packet loss recovery

**Verdict:** ❌ **Not recommended**
- Moonlight protocol designed for low-latency direct UDP
- Added latency unacceptable for streaming use case
- Existing MoonlightProxy code exists but not battle-tested
- Significant implementation complexity for marginal security benefit

### Option 3: Hybrid Approach

**Architecture:**
- Control plane handles HTTP/HTTPS endpoints (pairing, serverinfo, launch)
- Direct UDP for streaming (video, audio, control RTP)
- Wolf sidecar proxies HTTP/HTTPS over revdial

**Pros:**
- ✅ Lower latency than full proxy (UDP still direct)
- ✅ Some control plane visibility into pairing/session management

**Cons:**
- ❌ Still requires inbound UDP ports on agent runner
- ❌ Complex implementation (split protocol handling)
- ❌ Minimal benefit over Option 1 (direct connection)

**Verdict:** 🤔 **Possible but overcomplicated**
- Most complexity with marginal benefit
- If UDP ports are open anyway, might as well keep HTTP/HTTPS direct too

### Recommendation: Option 1 (Direct Connection)

**Rationale:**
1. **Latency is critical** - Moonlight is a game streaming protocol, extra hops hurt UX
2. **Wolf is battle-tested** - Direct UDP implementation proven and stable
3. **Limited exposure** - Only 6 ports, only Moonlight protocol (not arbitrary access)
4. **Simpler implementation** - No UDP tunneling complexity
5. **Bandwidth efficiency** - Control plane not bottleneck for video streams

**Security Considerations:**
- Moonlight protocol has strong authentication (certificate-based pairing)
- Wolf handles all security aspects (PIN pairing, mTLS)
- Limited attack surface (only Moonlight clients can connect)
- No access to control plane or other containers

**Alternative for High-Security Environments:**
If zero inbound connectivity is absolutely required:
- Use VPN/WireGuard for Moonlight clients → agent runner connectivity
- VPN provides encrypted tunnel without exposing public ports
- Still enables direct UDP for low latency
- Adds authentication layer (VPN key) before Moonlight protocol

---

## Implementation Plan

### Phase 1: Wolf Sidecar Development (Core Functionality)

**Deliverables:**
1. Wolf sidecar Go service
   - Reverse dial connection establishment
   - Wolf API HTTP proxy (local → Unix socket)
   - Settings sync reverse proxy (daemon → control plane)
   - Screenshot reverse handler (control plane → local server)

2. Docker Compose configuration
   - wolf-sidecar service definition
   - Environment variable configuration
   - Volume mounts for Wolf socket

**Testing:**
- Local development: Two Docker Compose stacks simulating remote hosts
- Verify Wolf API calls work through sidecar
- Verify settings sync works bidirectionally
- Verify screenshots captured via reverse proxy

**Estimated Effort:** 2-3 days

### Phase 2: Control Plane Integration

**Deliverables:**
1. WolfReverseProxy service
   - Reverse dial acceptance and authentication
   - Wolf API call forwarding
   - Settings sync forwarding
   - Screenshot request handling

2. Modified WolfExecutor
   - Replace Unix socket client with reverse proxy client
   - No other logic changes

3. API server changes
   - Integrate WolfReverseProxy into server initialization
   - Register reverse dial endpoint

**Testing:**
- End-to-end session creation from Helix → Wolf sidecar → Wolf
- Settings sync round-trip (Helix → container → Helix)
- Screenshot capture from Helix UI

**Estimated Effort:** 2-3 days

### Phase 3: Production Deployment

**Deliverables:**
1. Deployment documentation
   - Agent runner setup guide
   - Firewall configuration (Moonlight ports)
   - Network requirements
   - Monitoring and troubleshooting

2. Configuration templates
   - Environment variables
   - Docker Compose files
   - Systemd service files (for sidecar auto-start)

3. Migration guide
   - Moving from local Wolf to remote Wolf
   - Zero-downtime migration strategy

**Testing:**
- Production-like environment (separate VMs/hosts)
- Network partition simulation
- Reconnection behavior testing
- Performance benchmarking

**Estimated Effort:** 1-2 days

### Phase 4: Monitoring and Operations

**Deliverables:**
1. Metrics and logging
   - Sidecar health metrics (Prometheus)
   - Reverse dial connection status
   - API call latency tracking
   - Screenshot request metrics

2. Alerting
   - Reverse dial disconnection
   - API call failures
   - High latency warnings

3. Troubleshooting tools
   - Connection status CLI tool
   - Sidecar debug endpoints
   - Log aggregation setup

**Estimated Effort:** 1-2 days

---

## Configuration Changes Summary

### Agent Runner Host (New/Modified)

**1. Wolf Sidecar Service (NEW)**
```yaml
services:
  wolf-sidecar:
    image: helix/wolf-sidecar:latest
    environment:
      - CONTROL_PLANE_URL=https://helix-api.example.com
      - RUNNER_TOKEN=${RUNNER_TOKEN}
      - RUNNER_ID=${RUNNER_ID}
      - WOLF_SOCKET_PATH=/var/run/wolf/wolf.sock
      - WOLF_API_PROXY_PORT=47990
      - SETTINGS_SYNC_PROXY_PORT=8080
      - SCREENSHOT_SERVER_PORT=9876
    volumes:
      - /var/run/wolf:/var/run/wolf:rw
    networks:
      - default
    restart: always
```

**2. Zed Container Settings Sync Daemon (MODIFIED)**
```yaml
environment:
  - HELIX_API_URL=http://wolf-sidecar:8080  # Changed from http://api:8080
  - HELIX_API_TOKEN=${RUNNER_TOKEN}
  - HELIX_SESSION_ID=${SESSION_ID}
```

**3. Firewall Configuration (NEW)**
```bash
# Allow Moonlight protocol ports (inbound)
iptables -A INPUT -p tcp --dport 47984 -j ACCEPT
iptables -A INPUT -p tcp --dport 47989 -j ACCEPT
iptables -A INPUT -p tcp --dport 48010 -j ACCEPT
iptables -A INPUT -p udp --dport 47999 -j ACCEPT
iptables -A INPUT -p udp --dport 48100 -j ACCEPT
iptables -A INPUT -p udp --dport 48200 -j ACCEPT

# Block all other inbound (including control plane → agent runner)
iptables -A INPUT -p tcp -j DROP
iptables -A INPUT -p udp -j DROP
```

### Control Plane Host (Modified)

**1. API Server Initialization (MODIFIED)**
```go
// Initialize reverse proxy for remote Wolf
wolfReverseProxy := WolfReverseProxy{
    connman: connectionManager,
    store:   store,
}

// WolfExecutor now uses reverse proxy instead of Unix socket
wolfExecutor := NewWolfExecutor(
    &wolfReverseProxy,
    runnerID,  // Identifies which agent runner to dial
)
```

**2. No Other Changes Required**
- Existing WebSocket sync already uses reverse dial
- Existing runner authentication works
- No changes to frontend or user-facing APIs

---

## Security Considerations

### Authentication Flow

**Existing Mechanism (Unchanged):**
1. Agent runner starts with `RUNNER_TOKEN` (secure random string)
2. Sidecar connects to control plane with token in Authorization header
3. Control plane validates token against database
4. Connection registered in ConnectionManager with runnerID key

**Security Properties:**
- ✅ Mutual authentication (runner proves identity with token)
- ✅ No passwords or keys transmitted (token is bearer auth)
- ✅ Existing token rotation mechanisms work unchanged

### Network Security

**Agent Runner → Control Plane:**
- ✅ Outbound only (no inbound required except Moonlight)
- ✅ TLS encrypted (reverse dial over HTTPS WebSocket)
- ✅ Firewall friendly (single outbound connection)

**Control Plane → Agent Runner:**
- ✅ No direct connection possible (firewall blocks)
- ✅ All communication via multiplexed reverse dial
- ✅ Connection always initiated by agent runner

**Moonlight Clients → Agent Runner:**
- ✅ Certificate-based pairing (PKI authentication)
- ✅ PIN-based initial pairing (4-digit PIN verification)
- ✅ Mutual TLS for streaming sessions
- ⚠️ Requires inbound ports (limited to 6 ports, Moonlight protocol only)

### Data Flow Security

**Wolf API Calls:**
- Encrypted over reverse dial TLS tunnel
- No plaintext transmission
- Runner token authenticates all requests

**Settings Sync:**
- Bidirectional encrypted sync
- User settings isolated per session
- No cross-session data leakage

**Screenshots:**
- Transmitted over encrypted reverse dial
- No persistent storage on control plane
- Session-scoped access control

---

## Performance Considerations

### Latency Analysis

**Wolf API Calls (Control Plane → Wolf):**
- **Before:** Unix socket (< 1ms)
- **After:** Reverse dial over internet (10-100ms depending on network)
- **Impact:** Minimal (API calls infrequent, not latency-sensitive)

**Settings Sync (Daemon → Control Plane):**
- **Before:** HTTP to control plane (10-100ms)
- **After:** HTTP via sidecar proxy (10-100ms + ~1ms sidecar overhead)
- **Impact:** Negligible (polls every 30s, not latency-sensitive)

**Screenshots (Control Plane → Container):**
- **Before:** HTTP GET to container (10-100ms)
- **After:** HTTP via reverse dial (10-100ms + ~1ms reverse dial overhead)
- **Impact:** Minimal (user-triggered, not frequent)

**Moonlight Streaming (Client → Wolf):**
- **Before:** Direct UDP (1-20ms depending on network)
- **After:** Direct UDP (1-20ms - UNCHANGED if using Option 1)
- **Impact:** None (direct connection maintained)

### Bandwidth Analysis

**Reverse Dial Connection:**
- Settings sync: ~1 KB/request, every 30s = ~0.3 KB/s
- Wolf API calls: ~1-10 KB/request, ~1 request/minute = ~0.2 KB/s
- Screenshots: ~100 KB/screenshot, on-demand = negligible
- WebSocket sync: ~1-10 KB/message, varies by usage

**Total bandwidth for control channel:** < 10 KB/s average
**Moonlight streaming bandwidth:** ~5-20 Mbps (direct to agent runner, not via control plane)

### Connection Limits

**Reverse Dial Multiplexing:**
- Single TCP connection per agent runner
- revdial.Dialer creates logical connections on demand
- No hard limit on concurrent logical connections
- Each logical connection has minimal overhead (~1 KB state)

**Scalability:**
- 1000 agent runners = 1000 TCP connections to control plane
- Easily handled by modern servers
- ConnectionManager designed for high concurrency

---

## Monitoring and Debugging

### Key Metrics to Track

**Wolf Sidecar:**
- `wolf_sidecar_revdial_connected` (gauge) - Connection status (1 = connected, 0 = disconnected)
- `wolf_sidecar_api_calls_total` (counter) - Total Wolf API calls proxied
- `wolf_sidecar_api_call_duration_seconds` (histogram) - Latency of API calls
- `wolf_sidecar_settings_sync_requests_total` (counter) - Settings sync requests proxied
- `wolf_sidecar_screenshot_requests_total` (counter) - Screenshot requests handled
- `wolf_sidecar_reconnect_attempts_total` (counter) - Reconnection attempts

**Control Plane WolfReverseProxy:**
- `wolf_reverse_proxy_connections_active` (gauge) - Active reverse dial connections
- `wolf_reverse_proxy_api_calls_total` (counter) - API calls forwarded to agent runners
- `wolf_reverse_proxy_api_call_errors_total` (counter) - Failed API calls
- `wolf_reverse_proxy_screenshot_requests_total` (counter) - Screenshot requests forwarded

### Health Checks

**Wolf Sidecar Health Check:**
```bash
curl http://localhost:8090/health
# Returns: {"status": "ok", "revdial_connected": true, "wolf_api_reachable": true}
```

**Control Plane Health Check:**
```bash
curl http://control-plane:8080/api/v1/runners/${RUNNER_ID}/health
# Returns: {"runner_id": "...", "connected": true, "last_ping": "2025-01-09T12:00:00Z"}
```

### Debug Endpoints

**Wolf Sidecar:**
- `GET /debug/revdial/status` - Reverse dial connection details
- `GET /debug/metrics` - Prometheus metrics
- `GET /debug/pprof/*` - Go profiling endpoints

**Control Plane:**
- `GET /api/v1/runners` - List all connected runners
- `GET /api/v1/runners/${RUNNER_ID}/status` - Detailed runner status
- `GET /api/v1/runners/${RUNNER_ID}/ping` - Force ping test

### Logging Strategy

**Structured Logging (JSON):**
```json
{
  "timestamp": "2025-01-09T12:00:00Z",
  "level": "info",
  "component": "wolf-sidecar",
  "runner_id": "runner-123",
  "event": "api_call",
  "method": "POST",
  "path": "/api/v1/lobbies/create",
  "duration_ms": 45,
  "status": 200
}
```

**Log Aggregation:**
- Ship logs to centralized logging (ELK, Loki, CloudWatch)
- Filter by runner_id for troubleshooting specific agent runners
- Alert on error patterns (repeated disconnections, API failures)

---

## Migration Strategy

### Zero-Downtime Migration

**Phase 1: Deploy Wolf Sidecar (No Impact)**
1. Deploy wolf-sidecar container alongside existing Wolf
2. Sidecar establishes reverse dial but isn't used yet
3. Verify reverse dial connection successful
4. No changes to running sessions

**Phase 2: Dual-Mode WolfExecutor (Gradual Migration)**
1. Update WolfExecutor to support both modes:
   - If `WOLF_SOCKET_PATH` set: Use Unix socket (old behavior)
   - If `WOLF_RUNNER_ID` set: Use reverse proxy (new behavior)
2. Deploy to control plane
3. Still no impact on existing sessions

**Phase 3: Migrate Settings Sync (Per-Session)**
1. New sessions use `HELIX_API_URL=http://wolf-sidecar:8080`
2. Existing sessions continue with old URL
3. Gradual rollover as sessions naturally expire/restart

**Phase 4: Switch Default (Cut-Over)**
1. Change default configuration to use reverse proxy
2. Old mode still available via config flag
3. Monitor for issues, rollback if needed

**Phase 5: Cleanup (Post-Migration)**
1. Remove dual-mode support after burn-in period
2. Simplify code to reverse-proxy-only path
3. Remove old configuration options

### Rollback Plan

**If Issues Detected:**
1. Change `WOLF_RUNNER_ID` → `WOLF_SOCKET_PATH` in env vars
2. Restart affected services
3. Reverse dial connections harmless (just idle)
4. No data loss (all state in database)

**Maximum Downtime:** 0 seconds (env var toggle, rolling restart)

---

## Alternatives Considered

### Alternative 1: VPN Tunnel

**Approach:** Establish VPN tunnel between control plane and agent runner

**Pros:**
- ✅ Transparent to applications (just network layer)
- ✅ No code changes needed

**Cons:**
- ❌ Requires VPN infrastructure (WireGuard, OpenVPN)
- ❌ More complex networking (routing tables, subnets)
- ❌ Doesn't meet "no inbound connectivity" requirement (VPN is bidirectional)
- ❌ VPN endpoint on agent runner = potential attack surface

**Verdict:** ❌ Rejected (doesn't meet requirements)

### Alternative 2: SSH Reverse Tunnel

**Approach:** Use SSH reverse tunnel for port forwarding

**Pros:**
- ✅ Well-understood protocol
- ✅ Built-in authentication

**Cons:**
- ❌ SSH server required on control plane (attack surface)
- ❌ Not designed for high-throughput multiplexing
- ❌ Complex port forwarding configuration
- ❌ Difficult to monitor and debug

**Verdict:** ❌ Rejected (revdial better suited)

### Alternative 3: Message Queue (NATS, RabbitMQ)

**Approach:** Use message queue for RPC-style communication

**Pros:**
- ✅ Decouples components
- ✅ Built-in reconnection handling

**Cons:**
- ❌ Adds infrastructure dependency (NATS server)
- ❌ Higher latency than direct TCP
- ❌ Complex for request/response pattern (Wolf API calls)
- ❌ Doesn't solve screenshot/settings sync HTTP patterns

**Verdict:** ❌ Rejected (over-engineered)

### Alternative 4: gRPC Bidirectional Streaming

**Approach:** Rewrite Wolf API as gRPC with bidirectional streams

**Pros:**
- ✅ Modern RPC framework
- ✅ Built-in streaming support

**Cons:**
- ❌ Requires rewriting Wolf API (major change to Wolf server)
- ❌ Doesn't solve existing HTTP-based services (settings sync, screenshots)
- ❌ More complex than HTTP proxy pattern

**Verdict:** ❌ Rejected (too invasive)

---

## Open Questions and Future Work

### Open Questions

1. **How should we handle agent runner failures?**
   - Current plan: Reverse dial auto-reconnects, sessions preserved in Wolf
   - Question: Should control plane mark sessions as "unhealthy" during disconnection?

2. **Should we implement connection pooling?**
   - Current: Single reverse dial connection per agent runner
   - Alternative: Connection pool for high-throughput scenarios

3. **How to handle multiple control plane instances (HA)?**
   - Current: Single control plane
   - Future: Load balancer with sticky sessions? Leader election?

### Future Enhancements

**1. Multi-Region Support**
- Agent runners in different geographic regions
- Regional control planes with central coordination
- Latency-based routing for closest control plane

**2. Connection Quality Monitoring**
- Measure reverse dial latency
- Alert on degraded connections
- Automatic fallback to local mode if latency too high

**3. Advanced Moonlight Proxying**
- Implement UDP-over-TCP tunnel for zero-inbound scenarios
- Adaptive bitrate based on connection quality
- Fallback to direct connection if possible

**4. Enhanced Security**
- Certificate pinning for reverse dial TLS
- Mutual TLS authentication (in addition to runner token)
- Automatic token rotation

---

## Summary and Recommendations

### Key Architectural Decisions

| Decision | Recommendation | Rationale |
|----------|----------------|-----------|
| **Wolf API Communication** | Reverse dial via Wolf Sidecar | ✅ No inbound connectivity, uses existing revdial infrastructure |
| **Settings Sync Direction** | Reverse proxy via sidecar | ✅ Minimal code changes, daemon unaware of architecture |
| **Screenshot Server Direction** | Reverse handler in sidecar | ✅ Control plane initiates via revdial, clean abstraction |
| **WebSocket Sync** | No changes needed | ✅ Already uses revdial, works out of box |
| **Moonlight Protocol Ports** | **Direct connection (Option 1)** | ✅ Lowest latency, battle-tested, limited exposure |

### Implementation Phases

| Phase | Duration | Deliverables |
|-------|----------|--------------|
| **Phase 1: Wolf Sidecar** | 2-3 days | Sidecar service, Docker config, local testing |
| **Phase 2: Control Plane** | 2-3 days | WolfReverseProxy, modified WolfExecutor, integration testing |
| **Phase 3: Deployment** | 1-2 days | Documentation, migration guide, production testing |
| **Phase 4: Monitoring** | 1-2 days | Metrics, alerting, troubleshooting tools |

**Total Estimated Effort:** 6-10 days

### Success Criteria

- ✅ Zero inbound connectivity to agent runner (except Moonlight ports)
- ✅ Wolf API calls work through reverse dial
- ✅ Settings sync bidirectional via reverse proxy
- ✅ Screenshots captured via reverse dial
- ✅ WebSocket sync continues working (no regression)
- ✅ Moonlight streaming latency unchanged (direct UDP)
- ✅ < 100ms added latency for Wolf API calls
- ✅ Zero-downtime migration from local to remote Wolf
- ✅ Automatic reconnection on network disruption

### Risks and Mitigations

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Reverse dial connection instability | Medium | High | Implement robust reconnection logic, monitor connection health |
| Higher latency impacts UX | Low | Medium | Benchmark latency, optimize critical paths, monitor p99 |
| Moonlight ports exposed to internet | Low | Low | Limited attack surface (6 ports, strong authentication), consider VPN for high-security |
| Complex debugging in production | Medium | Medium | Comprehensive logging, debug endpoints, connection status tools |
| Migration disrupts existing sessions | Low | High | Zero-downtime migration strategy, gradual rollout, rollback plan |

---

## Conclusion

The proposed remote Wolf architecture achieves all requirements while minimizing complexity and preserving battle-tested components:

1. **No inbound connectivity required** (except limited Moonlight ports) ✅
2. **Reverse dial connection** using existing revdial infrastructure ✅
3. **Existing runner token authentication** works unchanged ✅
4. **Settings daemon/screenshot server** reversed via sidecar proxy ✅
5. **Moonlight protocol** uses direct connection (lowest latency) ✅

**Next Steps:**
- Review and approve architecture (before 4pm demo)
- Begin Phase 1 implementation (post-demo)
- Set up development environment (two hosts or VMs)
- Create tracking issues for each phase

**Questions/Feedback:** Contact architecture team for review
