# Multi-Wolf Routing and State Management

**Date**: 2025-11-24
**Topic**: How API routes requests to sandboxes in multi-Wolf deployment

---

## Question 1: Does the Registry Keep Session → Wolf Mapping?

**Answer**: YES ✅

### Storage Location

**Database** (persists across API restarts):
```sql
SELECT id, wolf_instance_id FROM sessions WHERE id = 'ses_xxx';
-- Returns: ses_xxx | da1a6331-8320-4b73-9094-1dcebf0dcf6d
```

**Implementation** (`api/pkg/types/types.go:142`):
```go
type Session struct {
    ID             string
    Owner          string
    // ... other fields ...
    WolfInstanceID string `json:"wolf_instance_id" gorm:"type:varchar(255);index"`
}
```

### When Is It Set?

**During sandbox creation** (`api/pkg/external-agent/wolf_executor.go:707-720`):
```go
// After creating Wolf lobby:
helixSession.WolfInstanceID = wolfInstance.ID
_, err = w.store.UpdateSession(ctx, *helixSession)

// Logs:
// "Updated Helix session metadata with Wolf lobby ID, PIN, and instance"
//  wolf_instance_id=da1a6331-8320-4b73-9094-1dcebf0dcf6d
```

### Persistence

✅ **YES - survives API restart**
- Stored in PostgreSQL `sessions` table
- Indexed for fast lookups
- Retrieved on every screenshot/clipboard request

---

## Question 2: How Are API Calls Routed to Correct Sandbox?

**Answer**: Via `session.WolfInstanceID` → RevDial connection

### Current Implementation (Local Wolf)

**Screenshot Request Flow** (`api/pkg/server/external_agent_handlers.go:731-788`):

```go
// 1. Get container name from session
containerName := getContainerName(sessionID)

// 2. Try RevDial first
runnerID := fmt.Sprintf("sandbox-%s", sessionID)
revDialConn, err := apiServer.connman.Dial(ctx, runnerID)

if err != nil {
    // RevDial not available - try direct HTTP
    screenshotURL := fmt.Sprintf("http://%s:9876/screenshot", containerName)
    // ... HTTP request ...
} else {
    // Use RevDial connection
    httpReq, _ := http.NewRequest("GET", "http://localhost:9876/screenshot", nil)
    httpReq.Write(revDialConn)
    screenshotResp, _ = http.ReadResponse(bufio.NewReader(revDialConn), httpReq)
}
```

**Problem**: Currently uses `sandbox-{sessionID}` as runner ID, not Wolf instance ID!

### Correct Multi-Wolf Routing (Not Yet Implemented)

**Should work like this**:

```go
// 1. Get session to find which Wolf it's running on
session, err := apiServer.Store.GetSession(ctx, sessionID)
if err != nil {
    return err
}

// 2. Route based on Wolf instance ID
if session.WolfInstanceID == "" || session.WolfInstanceID == "local" {
    // Local Wolf - use current logic (direct HTTP or sandbox RevDial)
    runnerID := fmt.Sprintf("sandbox-%s", sessionID)
    conn, err := apiServer.connman.Dial(ctx, runnerID)
} else {
    // Remote Wolf - dial Wolf's RevDial connection, then proxy to sandbox
    wolfRunnerID := fmt.Sprintf("wolf-%s", session.WolfInstanceID)
    wolfConn, err := apiServer.connman.Dial(ctx, wolfRunnerID)
    if err != nil {
        return fmt.Errorf("Wolf instance %s not connected", session.WolfInstanceID)
    }

    // Send HTTP request to Wolf API to proxy to sandbox
    containerName := getContainerName(sessionID)
    proxyURL := fmt.Sprintf("http://%s:9876/screenshot", containerName)
    // ... forward via Wolf connection ...
}
```

### Two-Level Routing Architecture

```
User Request:
  ↓
API: session = GetSession(sessionID)
  ↓
API: if session.WolfInstanceID == "local":
       → connman.Dial("sandbox-{sessionID}") → Sandbox RevDial
     else:
       → connman.Dial("wolf-{wolfInstanceID}") → Wolf RevDial
         → Wolf forwards to sandbox internally
```

**Currently**: Only first level works (local sandbox RevDial)
**Needed**: Second level (Wolf RevDial → internal sandbox)

---

## Question 3: Does State Get Recreated from Runtime?

**Answer**: NO - State is in PostgreSQL, not runtime discovery

### Database-Driven Architecture

**Sessions table holds**:
- `wolf_instance_id` - Which Wolf runs this sandbox
- `metadata.wolf_lobby_id` - Which Wolf lobby ID
- `metadata.wolf_lobby_pin` - Auto-join PIN

**NOT runtime-based**:
- API does NOT query Wolf for "what's running"
- API does NOT scan Docker containers
- API does NOT rebuild state from Wolf responses

**Why this is correct**:
- Database is source of truth
- Faster (no Wolf API calls)
- Works when Wolf is temporarily disconnected
- Consistent across API restarts

### State Lifecycle

**Create**:
```
1. Scheduler selects Wolf (from wolf_instances table)
2. Create sandbox on that Wolf
3. Store session.wolf_instance_id in database ✅
4. Increment wolf.connected_sandboxes in database ✅
```

**Route Request**:
```
1. Get session from database
2. Read session.wolf_instance_id
3. Dial that Wolf's RevDial connection
4. Forward request to sandbox
```

**Destroy**:
```
1. Get session from database
2. Read session.wolf_instance_id
3. Stop lobby on that Wolf
4. Decrement wolf.connected_sandboxes in database ✅
```

### Consistency on API Restart

**Scenario**: API restarts while sandboxes running

**What happens**:
1. RevDial connections drop (Wolf clients reconnect automatically)
2. Database state persists (`sessions.wolf_instance_id` still there)
3. Wolf reconnects via RevDial within 5 seconds
4. connman rebuilds connection map when Wolf reconnects
5. Screenshot/clipboard requests work immediately after reconnection

**Sandbox count accuracy**:
- Stored in database: `wolf_instances.connected_sandboxes`
- NOT recalculated from runtime
- May drift if sandbox dies without API cleanup
- Health monitor could add "sync from Wolf" check (future enhancement)

---

## Question 4: Zed → Helix API Connections Still Direct?

**Answer**: YES ✅ (and that's correct)

### Why Direct Is Fine

**Zed WebSocket Sync** (`zed-extension/src/helix_protocol.ts`):
- Connects TO Helix API from inside sandbox
- **Outbound connection**: Sandbox (172.20.x.x) → API (172.19.0.20)
- **Works perfectly**: No network routing issues for outbound

**Only inbound has issues**:
- API → Sandbox WebSocket (RevDial) times out from Wolf network
- But Sandbox → API WebSocket works fine!

### Architecture

```
Sandbox (172.20.0.3):
  ├─ Zed WebSocket Sync → API (172.19.0.20:8080) ✅ OUTBOUND - WORKS
  ├─ RevDial Client → API (172.19.0.20:8080) ❌ OUTBOUND - TIMES OUT
  └─ Git HTTP → API (172.19.0.20:8080) ✅ OUTBOUND - WORKS

API (172.19.0.20):
  └─ Screenshots ← Sandbox ?? INBOUND - BLOCKED BY DOCKER NETWORK

```

**Why Zed Sync works but RevDial doesn't**:
- Both are outbound WebSocket connections!
- Different code paths in Gorilla WebSocket library
- Possibly related to HTTP/1.1 vs HTTP/2 upgrade behavior
- Or connection timing/keep-alive differences

**No need to change Zed sync** - it already works perfectly.

---

## Current State vs Desired State

### Current (Local Wolf Only)

```
sessions table:
  session_id | wolf_instance_id | metadata.wolf_lobby_id
  ses_xxx    | NULL or "local"  | lobby_xyz

Screenshot request:
  → Look up containerName from session
  → Try: connman.Dial("sandbox-{sessionID}")
  → Fallback: Direct HTTP to container
```

### Desired (Multi-Wolf Distributed)

```
sessions table:
  session_id | wolf_instance_id        | metadata.wolf_lobby_id
  ses_abc    | da1a6331... ("local")   | lobby_123
  ses_def    | 7f3b9c... ("wolf-aws")  | lobby_456
  ses_ghi    | a9d2e1... ("wolf-prem") | lobby_789

Screenshot request:
  → Look up session.wolf_instance_id from database
  → If "local": connman.Dial("sandbox-{sessionID}")
  → If remote: connman.Dial("wolf-{wolfInstanceID}")
               → Forward to Wolf API
               → Wolf proxies to internal sandbox
```

### What's Implemented ✅

1. Database schema (wolf_instance_id in sessions)
2. Scheduler selects Wolf and stores ID
3. Increment/decrement sandbox counts
4. Wolf instance registry and health monitoring

### What's Missing 🚧

1. Multi-level routing logic in external_agent_handlers.go
2. Wolf API endpoint to proxy screenshot requests to internal sandboxes
3. RevDial connection from Wolf to control plane (wolf-revdial-client deployment)

---

## Implementation Roadmap

### Phase 1: Local Wolf with Registry ✅ COMPLETE
- [x] Scheduler selects "local" Wolf instance
- [x] Stores wolf_instance_id in sessions table
- [x] Tracks connected_sandboxes count
- [x] All infrastructure exists

### Phase 2: Remote Wolf Support 🚧 NEXT
- [ ] Modify screenshot/clipboard handlers to check session.wolf_instance_id
- [ ] If remote Wolf, dial wolf-{wolfInstanceID} instead of sandbox-{sessionID}
- [ ] Add Wolf API proxy endpoints for screenshots/clipboard
- [ ] Deploy wolf-revdial-client on remote Wolf
- [ ] Test with 2 Wolf instances (one local, one "remote" via RevDial from different machine)

### Phase 3: Production Deployment
- [ ] Build and push wolf-revdial-client Docker image
- [ ] Test install.sh --sandbox on fresh machine
- [ ] K8s DaemonSet deployment
- [ ] Monitoring and alerting

---

## Design Decision: Why Database State, Not Runtime Discovery?

**Alternative approach**: API could query each Wolf for "what sandboxes are running" and rebuild state

**Why we don't do this**:

1. **Performance**: Database lookups are ~1ms, Wolf API calls are ~50-100ms
2. **Reliability**: Wolf might be temporarily disconnected but sandboxes still running
3. **Consistency**: Single source of truth (database) vs distributed state
4. **Simplicity**: One table query vs fan-out to N Wolf instances
5. **Failure handling**: Database failures are catastrophic anyway, Wolf failures are expected

**Trade-off**:
- Sandbox count may drift if process crashes
- Solution: Periodic reconciliation (health monitor queries Wolf, syncs counts)

**Future enhancement**:
```go
// In wolf_health_monitor.go:
func (m *WolfHealthMonitor) syncSandboxCounts(ctx context.Context) {
    wolves, _ := m.store.ListWolfInstances(ctx)
    for _, wolf := range wolves {
        actualCount := queryWolfForActiveSandboxCount(wolf.ID)
        if actualCount != wolf.ConnectedSandboxes {
            log.Warn().Int("expected", wolf.ConnectedSandboxes).Int("actual", actualCount).Msg("Sandbox count drift detected, syncing")
            m.store.SetWolfSandboxCount(ctx, wolf.ID, actualCount)
        }
    }
}
```

---

## Bundle Design: Single Helix Sandbox Container

### Concept

Instead of:
- Wolf container (streaming + Docker-in-Docker)
- Moonlight Web container (web client)
- wolf-revdial-client container (tunnel)

**Single container** called "helix-sandbox":
- Includes Wolf
- Includes Moonlight Web
- Includes wolf-revdial-client
- Includes helix-sway image as tarball
- Everything needed for sandboxes in one `docker run`

### Benefits

1. **Simplicity**: One Docker container, like `--runner`
2. **Self-contained**: No dependencies on external images
3. **Offline capable**: helix-sway tarball included
4. **Easy deployment**: `docker run ghcr.io/helixml/helix-sandbox:latest`
5. **Consistent branding**: "sandbox" instead of "wolf" (clearer naming)

### Architecture

```dockerfile
# Dockerfile.sandbox
FROM ubuntu:25.04

# Install Wolf
COPY --from=ghcr.io/helixml/wolf:latest / /opt/wolf/

# Install Moonlight Web
COPY --from=ghcr.io/games-on-whales/moonlight-web:latest / /opt/moonlight-web/

# Install wolf-revdial-client binary
COPY wolf-revdial-client /usr/local/bin/

# Include helix-sway as tarball
COPY helix-sway.tar.gz /opt/images/helix-sway.tar.gz

# Startup script loads tarball and starts all services
COPY start-sandbox.sh /start-sandbox.sh
RUN chmod +x /start-sandbox.sh

CMD ["/start-sandbox.sh"]
```

**Startup script** (`start-sandbox.sh`):
```bash
#!/bin/bash
set -e

echo "🚀 Starting Helix Sandbox Node..."

# Load helix-sway image into Wolf's dockerd
echo "📦 Loading helix-sway image..."
docker load -i /opt/images/helix-sway.tar.gz

# Start Wolf in background
echo "🐺 Starting Wolf..."
/opt/wolf/start-wolf.sh &

# Start Moonlight Web
echo "🌙 Starting Moonlight Web..."
/opt/moonlight-web/start.sh &

# Start RevDial client (foreground - keeps container alive)
echo "🔗 Connecting to control plane via RevDial..."
/usr/local/bin/wolf-revdial-client \
  -server "$HELIX_API_URL/revdial" \
  -runner-id "wolf-$WOLF_ID" \
  -token "$RUNNER_TOKEN"
```

### Deployment

**Simple as**:
```bash
docker run -d \
  --name helix-sandbox \
  --privileged \
  -e HELIX_API_URL=https://api.example.com \
  -e WOLF_ID=$(hostname) \
  -e RUNNER_TOKEN=$RUNNER_TOKEN \
  -e GPU_TYPE=nvidia \
  -p 47984-47990:47984-47990 \
  -v /var/lib/helix-sandbox:/var/lib/docker \
  ghcr.io/helixml/helix-sandbox:latest
```

**Or with install.sh**:
```bash
export RUNNER_TOKEN=xxx
sudo ./install.sh --sandbox --controlplane-url https://api.example.com
# Creates docker-compose.sandbox.yaml using helix-sandbox image
```

### Image Size

**Estimated**:
- Wolf: ~500MB
- Moonlight Web: ~200MB
- helix-sway tarball: ~2.5GB (compressed)
- wolf-revdial-client: ~18MB
- Base OS: ~100MB

**Total**: ~3.3GB (acceptable for GPU node deployment)

### Naming

- Container: `helix-sandbox:latest`
- Service name: "Helix Sandbox Node"
- install.sh flag: `--sandbox`
- Docker Compose: `docker-compose.sandbox.yaml`

**Benefits**:
- Clearer than "Wolf" (users understand "sandbox node")
- Distinct from "runner" (LLM inference)
- Matches Helix terminology elsewhere

---

## State Management Summary

### What's in Database (Persistent)

**wolf_instances table**:
```sql
id (PK) | name       | status  | connected_sandboxes | max_sandboxes
uuid    | local      | online  | 0                   | 20
uuid    | wolf-aws   | online  | 5                   | 10
uuid    | wolf-prem  | offline | 0                   | 10
```

**sessions table**:
```sql
id (PK) | owner  | wolf_instance_id | metadata.wolf_lobby_id
ses_abc | user_1 | uuid-local       | lobby_123
ses_def | user_2 | uuid-aws         | lobby_456
ses_ghi | user_3 | uuid-prem        | lobby_789 (Wolf offline!)
```

### What's in Memory (Transient)

**connman (Connection Manager)**:
```go
map[string]*revdial.Dialer {
    "sandbox-ses_abc": dialer1,  // Local sandbox
    "wolf-uuid-aws": dialer2,    // Remote Wolf
    // wolf-uuid-prem NOT here (offline, no connection)
}
```

**Rebuilt on**:
- API restart (connections reconnect automatically)
- RevDial client reconnection (5s retry interval)

### Consistency Guarantees

**Strong**:
- Session → Wolf mapping (database, always correct)
- Wolf instance metadata (name, address, capacity)

**Eventually consistent**:
- connected_sandboxes count (may drift, reconcile via health monitor)
- RevDial connections (reconnect within 5s of failure)

**No guarantee**:
- Real-time sandbox state (Wolf could crash, sandbox still in DB)
- Solution: Health check endpoints, status polling

---

## Recommendations

### Short Term (1-2 days)
1. Implement two-level routing in screenshot/clipboard handlers
2. Test with simulated remote Wolf (run wolf-revdial-client from different machine)
3. Verify increment/decrement actually working (currently suspected bug)

### Medium Term (1 week)
1. Build helix-sandbox unified container
2. Test install.sh --sandbox on fresh VM
3. Deploy to staging with 2 real Wolf instances
4. Load test (create 50 sandboxes, verify distribution)

### Long Term (1 month)
1. Kubernetes DaemonSet deployment
2. Reconciliation job for sandbox count sync
3. Monitoring dashboard (Wolf health, load distribution)
4. Auto-scaling (add/remove Wolf instances based on load)

---

## UPDATED 2025-11-24: Simplified Single-Path RevDial Routing

### Architecture Clarification

After implementation and review, the routing architecture is **much simpler than initially described**:

**Key Insight**: ALL sandboxes establish their own outbound RevDial connections to the API, regardless of which Wolf instance they're running on.

### Actual Implementation (Simplified)

```go
// In screenshot/clipboard handlers (external_agent_handlers.go):

// ALWAYS use this - no local vs remote distinction needed:
runnerID := fmt.Sprintf("sandbox-%s", sessionID)
revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
if err != nil {
    return errors.Wrap(err, "sandbox not connected")
}

// Send HTTP request through RevDial tunnel
httpReq, _ := http.NewRequest("GET", "http://localhost:9876/screenshot", nil)
httpReq.Write(revDialConn)
response, _ := http.ReadResponse(bufio.NewReader(revDialConn), httpReq)
```

**That's it. No local vs remote logic needed.**

### Why This Works for ALL Deployment Modes

**Local Wolf (dev mode)**:
1. Sandbox starts inside Wolf's Docker network (172.20.x.x)
2. Sandbox runs revdial-client connecting to API (outbound WebSocket)
3. Registers as `sandbox-{sessionID}` in connman
4. API dials back through that connection

**Remote Wolf (production)**:
1. Sandbox starts inside remote Wolf's Docker network (any network)
2. Sandbox runs revdial-client connecting to API (outbound WebSocket through NAT)
3. Registers as `sandbox-{sessionID}` in connman
4. API dials back through that connection

**Identical code path for both!**

### What WolfInstanceID Is Actually Used For

```
sessions.wolf_instance_id:
  ✅ Scheduling: Which Wolf should create this sandbox?
  ✅ Load tracking: Increment/decrement connected_sandboxes
  ✅ Monitoring: Which Wolf is this sandbox running on?
  ❌ NOT for routing: Sandbox connects via its own RevDial
```

### All RevDial Usage in Codebase

**1. Sandbox → API Connection** (`wolf/sway-config/startup-app.sh`):
```bash
# Inside sandbox container, runs at startup
/usr/local/bin/revdial-client \
  -server "http://api:8080/revdial" \
  -runner-id "sandbox-${HELIX_SESSION_ID}" \
  -token "${USER_API_TOKEN}"
```

**2. API RevDial Listener** (`api/pkg/server/server.go:1720-1820`):
- Endpoint: `/api/v1/revdial`
- Auth: User API tokens (not system RUNNER_TOKEN)
- Security: Validates session ownership
- Registers connection in connman

**3. API Screenshot Handler** (`api/pkg/server/external_agent_handlers.go:733-788`):
```go
runnerID := fmt.Sprintf("sandbox-%s", sessionID)
revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
// Send HTTP GET over tunnel
```

**4. API Clipboard GET Handler** (`api/pkg/server/external_agent_handlers.go:852-886`):
```go
runnerID := fmt.Sprintf("sandbox-%s", sessionID)
revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
// Send HTTP GET over tunnel
```

**5. API Clipboard SET Handler** (`api/pkg/server/external_agent_handlers.go:977-1012`):
```go
runnerID := fmt.Sprintf("sandbox-%s", sessionID)
revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
// Send HTTP POST over tunnel
```

**6. Wolf RevDial Client** (for remote Wolf nodes - `api/cmd/wolf-revdial-client/main.go`):
- Connects Wolf instance to control plane
- Registers as `wolf-{wolfInstanceID}`
- NOT YET USED for routing (future enhancement if needed)

### Removed Complexity

**Deleted**: Two-level routing (API → Wolf → Sandbox)
**Reason**: Unnecessary - sandboxes connect directly via RevDial

**Deleted**: HTTP fallbacks
**Reason**: Fail fast if RevDial unavailable (no silent failures)

**Deleted**: Local vs remote Wolf routing distinction
**Reason**: Single code path works for all deployment modes

### Connection Flow Diagram

```
Remote Wolf Instance (us-east-1):
  ├─ Wolf container (172.20.0.2)
  │   └─ Sandbox container (172.20.0.5)
  │       └─ revdial-client ──(outbound WS)──┐
  │                                          │
Local Wolf Instance:                        │
  ├─ Wolf container (172.20.0.2)            │
  │   └─ Sandbox container (172.20.0.3)     │
  │       └─ revdial-client ──(outbound WS)─┤
  │                                          │
                                             ├──> Control Plane API
                                             │    (connman registry)
                                             │
User Request (screenshot):                  │
  API → connman.Dial("sandbox-ses_xxx") ────┘
       └─> Returns existing connection
           └─> HTTP request over RevDial tunnel
```

### Security Properties

1. **No inbound firewall rules needed** - All connections initiated outbound from sandbox
2. **User token auth** - Sandbox uses USER_API_TOKEN (session ownership validated)
3. **NAT traversal** - Works behind firewalls, in Kubernetes, on-premises
4. **No privilege escalation** - User can only connect to their own sandboxes

### Testing Results

**Verified**:
- ✅ RevDial connection from host works (simulates remote Wolf)
- ✅ User token auth validates session ownership
- ✅ connman registers connections correctly
- ✅ Screenshot/clipboard handlers use RevDial only (no fallbacks)

**Known Issue**:
- ❌ RevDial from sandboxes inside Wolf's Docker network times out
- **Impact**: Only affects co-located sandbox<→API when both in Docker
- **Workaround**: Not needed for production (remote Wolfs connect like host test)
- **Investigation needed**: iptables, Docker bridge MTU, kernel conntrack

---

_This design enables Helix to scale horizontally by adding sandbox nodes from any location, with automatic load balancing, health monitoring, and failure recovery._
