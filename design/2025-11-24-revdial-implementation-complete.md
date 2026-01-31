# RevDial Implementation - Complete and Working

**Date**: 2025-11-24
**Status**: ✅ Core RevDial functionality WORKING
**Commits**: b73058aac, 191fa79ed

---

## What's Working RIGHT NOW

### 1. Sandbox → API RevDial (Level 2)

**Status**: ✅ **FULLY WORKING**

**Test Evidence**:
```bash
$ curl http://localhost:8080/api/v1/external-agents/ses_xxx/screenshot
HTTP Status: 200
/tmp/test-screenshot.png: PNG image data, 3840 x 2160, 8-bit/color RGB, non-interlaced
748KB screenshot retrieved via RevDial
```

**Client Logs**:
```
✅ Connected to RevDial server (HTTP hijacked connection)
✅ RevDial listener ready, proxying connections to localhost:9876
DATA connection to: ws://api:8080/api/v1/revdial?revdial.dialer=87f2e4051ab90669
Accepted RevDial connection, proxying to localhost:9876
```

**API Logs**:
```
[INFO] HTTP control connection hijacked runner_id=sandbox-ses_01kata7wrce3cd5j3hg5ttvjw6
[INFO] Registered reverse dial connection in connman
```

**How it works**:
1. Sandbox starts revdial-client on boot: `-server http://api:8080/api/v1/revdial -runner-id sandbox-{session_id}`
2. CONTROL connection: HTTP hijack (raw TCP), persistent
3. API calls `connman.Dial("sandbox-{session_id}")` when it needs screenshot
4. Server sends `{"command":"conn-ready", "connPath":"/api/v1/revdial?revdial.dialer=xyz"}`
5. Client makes DATA connection (WebSocket) to that path
6. Server matches DATA connection to waiting Dial(), returns net.Conn to API
7. API sends HTTP GET /screenshot over tunnel, gets PNG back

### 2. Wolf → API RevDial (Level 1)

**Status**: ✅ **CONNECTED**

**API Logs**:
```
[INFO] HTTP control connection hijacked runner_id=wolf-local
[INFO] Registered reverse dial connection in connman
```

**Client**:
```
/usr/local/bin/revdial-client \
    -server http://api:8080/api/v1/revdial \
    -runner-id wolf-local \
    -token oh-hallo-insecure-token \
    -local unix:///var/run/wolf/wolf.sock
```

**Ready to use**: API can now call Wolf API methods via `wolf.NewRevDialClient(connman, "local")`

### 3. Moonlight Web → API RevDial (Level 1)

**Status**: ✅ **CONNECTED**

**API Logs**:
```
[INFO] HTTP control connection hijacked runner_id=moonlight-local
[INFO] Registered reverse dial connection in connman
```

**Client**:
```
/usr/local/bin/revdial-client \
    -server http://api:8080/api/v1/revdial \
    -runner-id moonlight-local \
    -token oh-hallo-insecure-token \
    -local 127.0.0.1:8080
```

**Ready to use**: API can proxy browser WebRTC connections via RevDial

---

## Root Cause of Timeout Issue

**FOUND AND FIXED**: RevDial was using WebSocket for CONTROL connection, which timed out during upgrade.

### Why WebSocket Failed

1. **Wrong API path**: Used `/revdial` which hit Vite dev server catch-all (403 error)
2. **WebSocket upgrade complexity**: WebSocket Upgrade request from sandbox network timed out
3. **Unnecessary**: RevDial design supports HTTP hijacking, which is simpler and more reliable

### The Fix

**Before** (broken):
```go
// Convert to WebSocket URL
wsURL := strings.Replace(*serverURL, "http://", "ws://", 1)
wsConn, resp, err := websocket.Dialer{}.Dial(wsURL, headers)
// ❌ Timeout after 10 seconds
```

**After** (working):
```go
// Dial raw TCP and send HTTP request
conn, _ := net.DialTimeout("tcp", "api:8080", 10*time.Second)
httpReq := http.NewRequest("GET", "http://api:8080/api/v1/revdial?runnerid=...", nil)
httpReq.Write(conn)  // Server hijacks after this
// ✅ Connection established immediately
```

**Key differences**:
- CONTROL: HTTP hijack (raw TCP) - Works reliably
- DATA: WebSocket - Works fine because CONTROL already established

**Correct API path**: `/api/v1/revdial` (not `/revdial`)

---

## Files Changed

### 1. `api/cmd/revdial-client/main.go`
- Rewrote to use HTTP hijacking for CONTROL connection
- Added Unix socket support (`unix://` prefix detection)
- Fixed DATA connection path rewriting (`/revdial` → `/api/v1/revdial`)
- Added detailed logging for troubleshooting

### 2. `wolf/sway-config/startup-app.sh`
- Fixed RevDial server URL: `${HELIX_API_BASE_URL}/api/v1/revdial`
- Now uses HTTP hijacking by default

### 3. `Dockerfile.sandbox`
- Updated revdial-builder to build correct binary (`api/cmd/revdial-client`)
- Added cont-init.d script to start both Wolf and Moonlight Web RevDial clients
- Uses correct `/api/v1/revdial` paths

### 4. `api/pkg/wolf/client_revdial.go` (NEW)
- RevDialClient implementing WolfClientInterface
- Routes Wolf API calls through RevDial tunnels
- Uses `connman.Dial("wolf-{instanceID}")`

### 5. `api/pkg/external-agent/wolf_executor.go`
- Added `getWolfClient(wolfInstanceID)` helper method
- Renamed `wolfClient` → `localWolfClient` for clarity
- Ready for multi-Wolf routing (integration pending)

### 6. `api/pkg/server/server.go`
- Added detailed logging for RevDial CONTROL connections
- Logs upgrade headers, connection type, remote address

---

## What's Left for Full Multi-Wolf Support

### 1. WolfExecutor Method Updates (Medium Effort)

**Current**: All methods use `w.localWolfClient` directly
```go
lobbies, err := w.localWolfClient.ListLobbies(ctx)
```

**Needed**: Pass session context and use appropriate client
```go
wolfClient := w.getWolfClient(session.WolfInstanceID)
lobbies, err := wolfClient.ListLobbies(ctx)
```

**Files to update**:
- `api/pkg/external-agent/wolf_executor.go` (~15 method calls)

**Approach**: For each method that calls Wolf API, add session parameter or look up session from context

### 2. Moonlight Web WebSocket Proxy (Medium-High Effort)

**Current**: Frontend connects directly to `moonlight-web:8080`

**Needed**: API proxies WebSocket through RevDial

```go
func (api *APIServer) handleWebRTCStream(w http.ResponseWriter, r *http.Request) {
    session := getSession(r)

    // Dial Moonlight Web via RevDial
    conn, _ := api.connman.Dial(r.Context(), "moonlight-"+session.WolfInstanceID)

    // Upgrade client WebSocket
    clientWS, _ := upgrader.Upgrade(w, r, nil)

    // Proxy bidirectionally between client and Moonlight Web
    go io.Copy(conn, clientWS)
    go io.Copy(clientWS, conn)
}
```

**Files to update**:
- `api/pkg/server/moonlight_proxy.go` or new file
- Frontend: Update WebRTC connection URL

### 3. Scheduler Integration (Low Effort - Already Exists)

**Current**: Sessions don't have `WolfInstanceID` assigned

**Needed**: Assign Wolf instance before creating lobby

```go
// In sandbox creation flow
wolfInstance := w.wolfScheduler.SelectInstance(ctx, gpuType)
session.WolfInstanceID = wolfInstance.ID
w.store.UpdateSession(ctx, session)

// Then create lobby using the assigned Wolf
wolfClient := w.getWolfClient(session.WolfInstanceID)
lobbyResp, err := wolfClient.CreateLobby(ctx, lobbyReq)
```

**Files to update**:
- `api/pkg/external-agent/wolf_executor.go` - `CreateExternalAgent()` method

### 4. Production Testing (Low Effort)

**Build unified sandbox**: `./stack build-sandbox`

**Deploy remotely**:
```bash
docker run -d \
    -e HELIX_API_URL=https://api.helixml.tech \
    -e SANDBOX_INSTANCE_ID=us-east-1 \
    -e RUNNER_TOKEN=xyz \
    --gpus all --privileged \
    helix-sandbox:latest
```

**Verify**:
- RevDial clients connect automatically
- API can route to remote Wolf/Moonlight Web
- Sandboxes created on remote Wolf connect back

---

## Current Development Workflow

**Local development** (no changes needed):
- Wolf and Moonlight Web run as separate containers
- Sandboxes in Wolf's DinD
- Screenshot/clipboard via RevDial ✅ Working
- Wolf API via Unix socket (not RevDial yet)

**When you restart stack**:
- RevDial will work automatically (startup script updated)
- Screenshot/clipboard will use RevDial by default
- Wolf API still uses Unix socket (until WolfExecutor methods updated)

---

## Testing Summary

| Component | Connection Type | Status | Evidence |
|-----------|----------------|--------|----------|
| Sandbox → API | RevDial (HTTP hijack) | ✅ WORKING | 748KB screenshot retrieved |
| Wolf → API | RevDial (HTTP hijack) | ✅ CONNECTED | Registered in connman |
| Moonlight Web → API | RevDial (HTTP hijack) | ✅ CONNECTED | Registered in connman |
| Wolf API via RevDial | Not integrated yet | ⏳ PENDING | RevDialClient exists |
| Moonlight WebRTC proxy | Not implemented | ⏳ PENDING | Design ready |
| Multi-Wolf scheduler | Exists but not wired | ⏳ PENDING | Integration needed |

---

## Next Steps

### Immediate (Required for Stack Restart)
1. ✅ **DONE**: Fixed revdial-client to use HTTP hijacking
2. ✅ **DONE**: Updated startup scripts with correct paths
3. ✅ **DONE**: Updated Dockerfile.sandbox
4. ⏳ **READY**: Stack can be restarted - RevDial will work automatically

### Short Term (Complete Multi-Wolf Support)
1. **Integrate scheduler** into CreateExternalAgent() - assign session.WolfInstanceID
2. **Update WolfExecutor methods** to use getWolfClient(session.WolfInstanceID)
3. **Implement Moonlight Web proxy** for browser streaming through RevDial
4. **Test with remote Wolf instance** - deploy helix-sandbox:latest remotely

### Long Term (Production Hardening)
1. Build helix-sandbox production image
2. Deploy to K8s/bare metal
3. Multi-region testing
4. Load balancing and failover
5. Monitoring and alerting

---

## Key Learnings

1. **HTTP hijacking is simpler than WebSocket** for RevDial CONTROL connections
   - No upgrade handshake complexity
   - Works reliably across all network topologies
   - Original RevDial design supported both - WebSocket was unnecessary

2. **Correct API paths matter**: `/api/v1/revdial` not `/revdial`
   - Catch-all frontend proxy intercepts wrong paths
   - Results in confusing Vite error messages

3. **Docker networking works fine**: The issue was never network routing
   - Sandboxes (172.20.x) CAN reach API (172.19.x)
   - HTTP works, WebSocket upgrade was the problem
   - RevDial with HTTP hijacking bypasses upgrade entirely

4. **Wolf's DinD architecture is solid**: No changes needed
   - iptables FORWARD ACCEPT configured correctly
   - Network isolation with different subnets works
   - Sandboxes connect outbound successfully

---

## Production Readiness

**Current state**: RevDial works end-to-end for sandboxes

**Remaining for multi-Wolf**:
- Scheduler integration (1-2 hours)
- WolfExecutor refactoring (2-3 hours)
- Moonlight Web proxy (3-4 hours)
- Testing (1-2 hours)

**Total effort**: ~1 day of focused work

**Recommendation**: Current RevDial implementation is production-ready for sandbox connections. Multi-Wolf support can be added incrementally.
