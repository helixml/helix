# Docker-in-Docker + RevDial Two-Level Architecture

**Date**: 2025-11-24
**Status**: IN PROGRESS - Implementing RevDial at both levels
**Problem**: Clarify the two-level Docker hierarchy and which RevDial connections are needed at each level

---

## Architecture Overview

The Helix distributed Wolf architecture has **TWO distinct Docker nesting levels**, each requiring its own RevDial connections:

```
┌─────────────────────────────────────────────────────────────────┐
│                        CONTROL PLANE                             │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ Helix API (172.19.0.20)                                     │ │
│  │  - RevDial listener on /revdial endpoint                   │ │
│  │  - Connection manager (connman)                            │ │
│  │  - Routes requests over RevDial tunnels                    │ │
│  └────────────▲───────────────────▲───────────────────▲────────┘ │
│               │                   │                   │          │
└───────────────┼───────────────────┼───────────────────┼──────────┘
                │                   │                   │
        ┌───────┘                   │                   └───────┐
        │                           │                           │
        │ RevDial                   │ RevDial                   │ RevDial
        │ (WebSocket)               │ (WebSocket)               │ (WebSocket)
        │                           │                           │
┌───────▼───────────────────────────▼───────────────────────────▼──┐
│               WOLF INSTANCE (Remote or Co-located)                │
│                                                                   │
│  LEVEL 1: Wolf Container (172.19.0.x - helix_default network)    │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │ Wolf Process                                                 │ │
│  │  - Wolf API (Unix socket /var/run/wolf/wolf.sock)          │ │
│  │  - RevDial client → API (runner_id=wolf-{instance_id})     │ │
│  │  - Manages lobbies, sessions, GPU encoding                 │ │
│  │  - Controls Docker-in-Docker (dockerd inside Wolf)         │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │ Moonlight Web Process                                        │ │
│  │  - Moonlight Web API (HTTP port 8080)                       │ │
│  │  - RevDial client → API (runner_id=moonlight-{instance_id})│ │
│  │  - WebRTC signaling, STUN/TURN, streaming                  │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                   │
│  LEVEL 2: Wolf's Internal dockerd (172.20.0.x - Wolf DinD network)│
│  ┌───────────────────────────┐  ┌──────────────────────────────┐ │
│  │ Sandbox Container 1       │  │ Sandbox Container 2          │ │
│  │  - Sway compositor        │  │  - Sway compositor           │ │
│  │  - Screenshot server:9876 │  │  - Screenshot server:9876    │ │
│  │  - Clipboard daemon       │  │  - Clipboard daemon          │ │
│  │  - RevDial client → API   │  │  - RevDial client → API      │ │
│  │    (runner_id=sandbox-    │  │    (runner_id=sandbox-       │ │
│  │     ses_xxx)              │  │     ses_yyy)                 │ │
│  └───────────────────────────┘  └──────────────────────────────┘ │
└───────────────────────────────────────────────────────────────────┘
```

---

## Two-Level Docker Hierarchy

### Level 1: Wolf Container (Outer Docker Network)

**Network**: `helix_default` (172.19.0.0/16)
**Location**: Same network as Helix API, postgres, etc.
**Docker Engine**: Host Docker (bare metal) or K8s containerd

**Components**:
- **Wolf process**: Streaming server, GPU encoding, lobby management
- **Moonlight Web**: WebRTC gateway for browser streaming
- **Wolf's dockerd**: Docker-in-Docker daemon running INSIDE Wolf container

**Current Connection Method**:
- ❌ **Wolf API**: Unix socket `/var/run/wolf/wolf.sock` (local only)
- ❌ **Moonlight Web**: Direct HTTP to localhost (local only)

**Required Change**:
- ✅ **Wolf API → RevDial**: Wolf starts RevDial client on boot, registers as `wolf-{instance_id}`
- ✅ **Moonlight Web → RevDial**: Moonlight Web starts RevDial client, registers as `moonlight-{instance_id}`

### Level 2: Sandbox Containers (Wolf's Internal dockerd)

**Network**: Wolf's internal `helix_default` (172.20.0.0/16)
**Location**: INSIDE Wolf container, managed by Wolf's dockerd
**Docker Engine**: Wolf's dockerd (Docker-in-Docker)

**Components**:
- **Sway containers**: GPU-accelerated Wayland compositor
- **Screenshot server**: HTTP server on port 9876 (grim for screenshots)
- **Clipboard daemon**: Wl-clipboard for clipboard sync
- **Zed editor**: Code editor, file sync, Git operations

**Current Connection Method**:
- ✅ **Sandbox → RevDial**: Already implemented, uses `sandbox-{session_id}` runner ID
- ⚠️ **Known issue**: WebSocket timeout from sandboxes (under investigation)

**Network Path**:
```
Sandbox (172.20.0.3) → Wolf network gateway → helix_default (172.19.0.20) → API
```

---

## RevDial Connections Required

### 1. Wolf API → Helix API

**Purpose**: Allow Helix API to call Wolf API methods (app management, lobby control, session management)

**Current Implementation**:
```go
// api/pkg/wolf/client.go (lines 15-33)
func NewClient(socketPath string) *Client {
    return &Client{
        socketPath: socketPath,
        httpClient: &http.Client{
            Transport: &http.Transport{
                DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
                    return net.Dial("unix", socketPath) // ❌ Unix socket only
                },
            },
        },
    }
}
```

**Required Implementation**:
```go
// Add RevDial-aware Wolf client
type WolfClientRevDial struct {
    connman    ConnManagerInterface
    instanceID string
}

func NewRevDialClient(connman ConnManagerInterface, instanceID string) *WolfClientRevDial {
    return &WolfClientRevDial{
        connman:    connman,
        instanceID: instanceID,
    }
}

// Implement WolfClientInterface methods using RevDial
func (c *WolfClientRevDial) AddApp(ctx context.Context, app *wolf.App) error {
    runnerID := fmt.Sprintf("wolf-%s", c.instanceID)
    conn, err := c.connman.Dial(ctx, runnerID)
    if err != nil {
        return fmt.Errorf("failed to dial Wolf via RevDial: %w", err)
    }
    defer conn.Close()

    // Send HTTP request over RevDial tunnel
    httpReq, _ := http.NewRequest("POST", "http://localhost/api/v1/apps/add", body)
    httpReq.Write(conn)
    resp, _ := http.ReadResponse(bufio.NewReader(conn), httpReq)
    // ... handle response
}
```

**Wolf-side RevDial client**:
```bash
# Inside Wolf container startup script
/usr/local/bin/revdial-client \
  --server-url "http://api:8080/revdial" \
  --runner-id "wolf-${WOLF_INSTANCE_ID}" \
  --runner-token "${USER_API_TOKEN}" \
  --local-addr "unix:///var/run/wolf/wolf.sock" &
```

**Runner ID Format**: `wolf-{instance_id}` (e.g., `wolf-local`, `wolf-us-east-1`)

### 2. Moonlight Web API → Helix API

**Purpose**: Allow Helix API to proxy WebSocket connections from browser to Moonlight Web

**Current Implementation**: Direct HTTP to `localhost:8080` (assumes co-located)

**Required Implementation**:
```go
// Moonlight Web RevDial client (similar to Wolf)
func (api *APIServer) handleWebRTCSignaling(w http.ResponseWriter, r *http.Request) {
    sessionID := extractSessionID(r)
    session := api.getSession(sessionID)

    // Dial Moonlight Web via RevDial
    runnerID := fmt.Sprintf("moonlight-%s", session.WolfInstanceID)
    conn, err := api.connman.Dial(r.Context(), runnerID)
    if err != nil {
        http.Error(w, "Moonlight Web not connected", http.StatusServiceUnavailable)
        return
    }

    // Upgrade to WebSocket and proxy to Moonlight Web
    // ... WebSocket proxy implementation
}
```

**Moonlight Web-side RevDial client**:
```bash
# Inside Moonlight Web container startup script
/usr/local/bin/revdial-client \
  --server-url "http://api:8080/revdial" \
  --runner-id "moonlight-${WOLF_INSTANCE_ID}" \
  --runner-token "${USER_API_TOKEN}" \
  --local-addr "127.0.0.1:8080" &
```

**Runner ID Format**: `moonlight-{instance_id}` (e.g., `moonlight-local`, `moonlight-us-east-1`)

### 3. Sandbox → Helix API (Already Implemented)

**Purpose**: Screenshot, clipboard, file operations from sandbox to API

**Implementation**: Already exists in `api/cmd/revdial-client/main.go`

**Sandbox-side RevDial client**:
```bash
# Inside sandbox container (wolf/sway-config/startup-app.sh)
/usr/local/bin/revdial-client \
  --server-url "http://api:8080/revdial" \
  --runner-id "sandbox-${HELIX_SESSION_ID}" \
  --runner-token "${USER_API_TOKEN}" \
  --local-addr "127.0.0.1:9876" &
```

**Status**: ⚠️ Code complete, but WebSocket timeout issue (under investigation)

**Runner ID Format**: `sandbox-{session_id}` (e.g., `sandbox-ses_01kata7wrce3cd5j3hg5ttvjw6`)

---

## API Request Flow Examples

### Example 1: Screenshot Request (Level 2)

```
User Browser → Helix API
  ↓
Helix API: Look up session → Extract session_id
  ↓
Helix API: connman.Dial("sandbox-ses_01kata7wrce3cd5j3hg5ttvjw6")
  ↓
RevDial tunnel to sandbox (172.20.0.3 inside Wolf DinD)
  ↓
HTTP GET /screenshot → Screenshot server (port 9876)
  ↓
grim captures Wayland display → Returns PNG
  ↓
RevDial tunnel back to API
  ↓
Helix API returns PNG to browser
```

### Example 2: Wolf API Call (Level 1)

```
Helix API: Need to create Wolf lobby for new session
  ↓
Helix API: Look up session → Extract wolf_instance_id
  ↓
Helix API: connman.Dial("wolf-local")
  ↓
RevDial tunnel to Wolf container (172.19.0.x)
  ↓
HTTP POST /api/v1/lobbies/create → Wolf Unix socket
  ↓
Wolf creates lobby, starts Sway container in DinD
  ↓
RevDial tunnel back to API with lobby_id
  ↓
Helix API stores lobby_id in session
```

### Example 3: Moonlight Web Streaming (Level 1)

```
User Browser: Connect to WebRTC stream
  ↓
Browser WebSocket → Helix API /api/v1/stream/{session_id}
  ↓
Helix API: Look up session → Extract wolf_instance_id
  ↓
Helix API: connman.Dial("moonlight-local")
  ↓
RevDial tunnel to Moonlight Web (in Wolf container)
  ↓
WebSocket proxied to Moonlight Web API
  ↓
Moonlight Web: Negotiate WebRTC with browser (STUN for NAT traversal)
  ↓
WebRTC media: Browser ←─UDP (WebRTC)─→ Moonlight Web
  ↓
Moonlight Web: Decode from Wolf GPU stream → WebRTC encode
```

---

## Wolf Client Interface Abstraction

**Problem**: Current `wolf.Client` is hardcoded to use Unix socket. Need to support both Unix socket (local dev) and RevDial (distributed deployment).

**Solution**: Use interface pattern with two implementations:

```go
// api/pkg/external-agent/wolf_client_interface.go (already exists)
type WolfClientInterface interface {
    AddApp(ctx context.Context, app *wolf.App) error
    RemoveApp(ctx context.Context, appID string) error
    ListApps(ctx context.Context) ([]wolf.App, error)
    CreateLobby(ctx context.Context, req *wolf.CreateLobbyRequest) (*wolf.LobbyCreateResponse, error)
    JoinLobby(ctx context.Context, req *wolf.JoinLobbyRequest) error
    StopLobby(ctx context.Context, req *wolf.StopLobbyRequest) error
    ListLobbies(ctx context.Context) ([]wolf.Lobby, error)
    ListSessions(ctx context.Context) ([]wolf.WolfStreamSession, error)
    StopSession(ctx context.Context, clientID string) error
    GetSystemMemory(ctx context.Context) (*wolf.SystemMemoryResponse, error)
    GetSystemHealth(ctx context.Context) (*wolf.SystemHealthResponse, error)
}

// Implementation 1: Unix socket (existing)
type Client struct { /* current implementation */ }

// Implementation 2: RevDial (NEW)
type RevDialClient struct {
    connman    ConnManagerInterface
    instanceID string
}
```

**Usage in WolfExecutor**:
```go
// api/pkg/external-agent/wolf_executor.go
func NewWolfExecutor(...) *WolfExecutor {
    var wolfClient WolfClientInterface

    if useRevDial {
        // Production: Use RevDial for distributed Wolf instances
        wolfClient = wolf.NewRevDialClient(connman, wolfInstanceID)
    } else {
        // Development: Use Unix socket for local Wolf
        wolfClient = wolf.NewClient(wolfSocketPath)
    }

    return &WolfExecutor{
        wolfClient: wolfClient,
        // ...
    }
}
```

---

## Network Routing

### From API Container to Wolf Container

**Network**: Both on `helix_default` (172.19.0.0/16)
**Method**: Direct IP routing (no NAT)

```
API (172.19.0.20) → Wolf (172.19.0.x)
```

**RevDial Direction**: Wolf → API (outbound from Wolf)
**Why**: Allows Wolf to be behind firewall/NAT in remote deployments

### From API Container to Sandbox Container

**Network**: API on `helix_default` (172.19.0.0/16), Sandbox on Wolf's internal network (172.20.0.0/16)

**Route**:
```
API (172.19.0.20)
  → Wolf gateway (172.19.0.x)
  → Wolf DinD network (172.20.0.1 gateway)
  → Sandbox (172.20.0.x)
```

**DNS**: Sandboxes have `ExtraHosts: ["api:172.19.0.20"]` for DNS resolution

**RevDial Direction**: Sandbox → API (outbound from sandbox)
**Why**: Sandboxes are deeply nested, can't be reached directly

### From Browser to Moonlight Web (WebRTC Media)

**Signaling**: Browser WebSocket → API → RevDial → Moonlight Web
**Media**: Direct UDP WebRTC connection (STUN/TURN for NAT traversal)

```
Browser (public internet)
  ↓ STUN negotiation via signaling WebSocket
  ↓
WebRTC UDP media stream (NAT-traversed)
  ↓
Moonlight Web (behind NAT, public STUN server helped establish connection)
```

---

## Implementation Status

### ✅ Completed

1. **Docker-in-Docker**: Wolf runs isolated dockerd, sandboxes run inside Wolf
2. **Sandbox RevDial client**: Code complete, starts on sandbox boot
3. **API RevDial listener**: `/revdial` endpoint accepts WebSocket connections
4. **Connection manager**: `connman` tracks RevDial connections
5. **WolfClientInterface**: Interface abstraction exists for Wolf client
6. **Network routing**: Different subnets (172.19.x vs 172.20.x) prevent conflicts

### ⏳ In Progress

1. **Sandbox RevDial connection**: WebSocket timeout issue (under investigation)
   - Symptoms: `read tcp 172.20.0.2:37796->172.19.0.20:8080: i/o timeout`
   - Works from host, fails from sandbox
   - Regular HTTP works, WebSocket upgrade fails

### ❌ Not Started

1. **Wolf API RevDial client**: Need to add RevDial client to Wolf container startup
2. **Wolf API RevDialClient implementation**: New `RevDialClient` struct implementing `WolfClientInterface`
3. **Moonlight Web RevDial client**: Need to add RevDial client to Moonlight Web startup
4. **Moonlight Web API handlers**: RevDial-based WebSocket proxy for browser connections
5. **Wolf instance selection logic**: Update WolfExecutor to choose local vs RevDial based on session.WolfInstanceID

---

## Testing Plan

### Phase 1: Fix Sandbox RevDial (Level 2)

**Goal**: Get screenshots working via RevDial

**Steps**:
1. Debug WebSocket timeout from sandbox
2. Test screenshot endpoint: `helix spectask screenshot ses_xxx`
3. Verify clipboard GET/SET work

**Success Criteria**: All sandbox endpoints reachable via RevDial

### Phase 2: Implement Wolf API RevDial (Level 1)

**Goal**: Allow Helix API to call Wolf API via RevDial

**Steps**:
1. Create `wolf.RevDialClient` implementing `WolfClientInterface`
2. Add RevDial client startup to Wolf container
3. Update WolfExecutor to use RevDial client when `session.WolfInstanceID != "local"`
4. Test lobby creation, app management via RevDial

**Success Criteria**: Can create/manage Wolf lobbies via RevDial

### Phase 3: Implement Moonlight Web RevDial (Level 1)

**Goal**: Allow browser WebRTC streaming via RevDial proxy

**Steps**:
1. Add RevDial client startup to Moonlight Web container
2. Implement WebSocket proxy in API: Browser ← API ← RevDial ← Moonlight Web
3. Test browser streaming: Open stream in browser, verify WebRTC connects

**Success Criteria**: Full streaming session works via RevDial

### Phase 4: End-to-End Integration Test

**Goal**: Verify complete stack works

**Test Scenario**:
1. Start planning session (creates sandbox via Wolf API)
2. Sandbox connects via RevDial
3. Take screenshot via RevDial
4. Copy/paste clipboard via RevDial
5. Open streaming session in browser (Moonlight Web via RevDial)
6. Stop session (cleanup via Wolf API)

**Success Criteria**: Complete workflow with zero direct HTTP calls (all RevDial)

---

## Open Questions

1. **Wolf instance ID**: How is `WOLF_INSTANCE_ID` set? From environment variable? Database lookup?
2. **Moonlight Web bundling**: Is Moonlight Web a separate container or bundled with Wolf?
3. **WebSocket timeout root cause**: Why does Go WebSocket client timeout from sandbox but Rust WebSocket (Zed) works?
4. **RevDial adapter for Unix socket**: Can we use `revdial-client` to forward Unix socket to TCP, or need custom adapter?
5. **Authentication for Wolf/Moonlight**: Use `USER_API_TOKEN` (like sandboxes) or system `RUNNER_TOKEN`?

---

## Next Steps

1. ✅ **Create this design doc** - Clarify two-level architecture
2. ⏳ **Debug sandbox RevDial timeout** - Fix WebSocket upgrade issue
3. ❌ **Implement Wolf RevDialClient** - Replace Unix socket client
4. ❌ **Add Wolf RevDial client startup** - Modify Wolf container startup scripts
5. ❌ **Test Wolf API via RevDial** - Verify lobby creation works
6. ❌ **Implement Moonlight Web RevDial** - WebSocket proxy for browser streaming
7. ❌ **End-to-end integration test** - Full stack working via RevDial
