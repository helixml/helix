# Session Review: Moonlight Observability & Stress Testing

**Date:** 2025-11-08
**Focus:** Lobbies mode (de facto standard)

## Changes Made

### 1. Moonlight-web Enhancements (~/pm/moonlight-web-stream/)

**File:** `moonlight-web/web-server/src/api/mod.rs`

**Added `/api/admin/status` endpoint (line 493):**
- Returns all cached client certificates
- Returns all active sessions with detailed state:
  - `session_id`, `client_unique_id`, `mode`
  - `has_websocket` (WebSocket connected?)
  - `streamer_pid`, `streamer_alive` (process health)
- Detects zombie sessions (dead process not cleaned up)

**Registration:** Added to api_service (lines 544-545)

**Build status:** ✅ Built successfully, container rebuilt

### 2. Helix API Changes

**File:** `api/pkg/server/moonlight_proxy.go`

**Added getMoonlightStatus handler (line 367):**
- Proxies to `http://moonlight-web:8080/api/admin/status`
- Adds Helix authentication
- Forwards full response to frontend

**File:** `api/pkg/server/server.go` (line 851)

**Registered route:**
```go
authRouter.HandleFunc("/moonlight/status", apiServer.getMoonlightStatus).Methods(http.MethodGet)
```

**Build status:** ✅ Running (verified with API restart)

### 3. Frontend Dashboard

**New component:** `frontend/src/components/admin/MoonlightMonitor.tsx`

**Features:**
- 5 health metric cards:
  - Total Clients (cached certificates)
  - Total Sessions (streamer processes)
  - Healthy (WS + process alive)
  - Orphaned (no WS, process alive)
  - Zombie (process dead) - RED ALERT
- Health alerts for issues
- **Client-grouped view:**
  - Each client shows their sessions
  - Connection mode (CREATE expected, others flagged)
  - WebSocket state (ACTIVE/IDLE)
  - Wolf state (RUNNING/PAUSED/ABSENT)
  - Streamer process (PID + ALIVE/DEAD)
  - Helix session ID correlation

**Integration:** `frontend/src/pages/Dashboard.tsx` (line 635)
- Added below AgentSandboxes component
- Location: Dashboard → Agent Sandboxes tab

**Build status:** ✅ Should hot reload

### 4. Moonlight-web Logging (for debugging)

**File:** `frontend/src/lib/moonlight-web-ts/stream/index.ts`

**Enhanced logging:**
- Stores `clientUniqueId` in Stream class (line 51)
- Logs WebSocket lifecycle with client ID prefix
- Logs RTCPeerConnection state changes with client ID
- Logs ICE connection state with client ID
- Logs AuthenticateAndInit message details

**Purpose:** Debug multiple concurrent connection issues

### 5. Stress Test Suite

**File:** `api/pkg/server/moonlight_stress_test.go` (NEW)

**6 test scenarios:**
1. **Health Check** - Basic connectivity + state sanity
2. **Rapid Connect/Disconnect** - 10 cycles checking for leaks
3. **Concurrent Multi-Session** - 5 simultaneous, 30s stability
4. **Service Restart** - Behavior during Wolf/moonlight restarts
5. **Browser Tab Simulation** - Multiple tabs, same lobby
6. **Memory Leak Detection** - 5 cycles, check accumulation
7. **Concurrent Disconnects** - Mass disconnect cleanup

**README:** `api/pkg/server/MOONLIGHT_STRESS_TEST_README.md`
**Design doc:** `design/2025-11-08-moonlight-streaming-stress-test-suite.md`

### 6. SpecTask Detail Windows

**File:** `frontend/src/components/tasks/SpecTaskDetailDialog.tsx`

**Features:**
- Tileable floating windows (7 positions)
- Drag to snap with blue preview
- Multiple windows open simultaneously (array state)
- Two tabs:
  - **Active Session:** Moonlight viewer + message input
  - **Details:** Task info + action buttons
- Polls task every 2s to detect spec_session_id
- Auto-switches to Active Session tab when planning starts
- Shows debug info: Session ID + Moonlight Client ID

**Integration:** `frontend/src/pages/SpecTasksPage.tsx`
- Opens new window per task (no reuse)
- Filters duplicates (can't open same task twice)

**Kanban card:** Click opens detail window, removed screenshot link

## Potential Issues to Verify

### 1. API Endpoint Authentication
**Test:** Can frontend actually call `/api/v1/moonlight/status`?
- Route registered on `authRouter` (requires auth middleware)
- Frontend uses `api.get()` with token
- **Status:** Should work, but user reports "data undefined" error

**Action needed:** Check browser console for actual error

### 2. Moonlight-web Session State After Restart
**Observation:** Currently shows 4 clients, 0 sessions
- This is expected after restart (certs persist, sessions cleared)
- Dashboard should show "No active sessions (certificate cached for resume)"

**Status:** Working as designed

### 3. Multiple Concurrent Streams
**Original issue:** Second stream gets "PeerDisconnect"
**Root cause:** Not fully identified yet
**Current status:** User reports "after stopping/starting everything, multiple sessions are now working"

**Concern:** "I'm suspicious that the system is not reliable after multiple stops/starts"

**Monitoring now available:**
- Dashboard shows all Moonlight sessions
- Dashboard shows Wolf state per session
- Can detect zombie/orphaned sessions
- Stress tests will reproduce issues

### 4. Stress Test Execution
**Potential issue:** Tests need ADMIN_TOKEN
**Setup required:**
```bash
export ADMIN_TOKEN=$(curl -s http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"luke.marsden@gmail.com","password":"'${ADMIN_USER_PASSWORD}'"}' \
  | jq -r '.token')
```

**Status:** Documented in README, should work

### 5. Frontend TypeScript Compilation
**File:** `MoonlightMonitor.tsx`
- Added new fields to interface
- Added null checks for data
- Added Wolf state fetching with useEffect

**Potential issue:** Missing Tooltip import
**Status:** Already added (line 14)

## Lobbies Mode Architecture (Confirmed)

**Wolf side:**
- One Wolf UI App (shared)
- Multiple Wolf lobbies (one per external agent session)
- Each lobby → separate Sway container

**Moonlight-web side:**
- Each browser tab → unique `client_unique_id`
- Each connection → fresh CREATE mode session
- Each session → separate streamer child process
- Wolf lobby ID passed to moonlight-web
- Moonlight connects to that specific lobby in Wolf UI app

**Expected behavior:**
- Multiple browser tabs can stream same lobby concurrently? OR
- Only one client can stream a lobby at a time?

**Current dashboard will show:**
- If CREATE mode (✓ normal)
- If JOIN/KEEPALIVE/PEER mode (⚠️ unexpected in lobbies)
- Each client's sessions
- Wolf state per session

## Tests to Run

1. **Basic connectivity:**
   ```bash
   curl -s http://localhost:8080/api/v1/moonlight/status -H "Authorization: Bearer <token>"
   ```

2. **Dashboard:**
   - Navigate to Dashboard → Agent Sandboxes
   - Verify Moonlight section loads
   - Open task detail windows
   - Verify client IDs appear in dashboard

3. **Stress tests:**
   ```bash
   cd api/pkg/server
   go test -run TestMoonlightHealthCheck -v
   ```

## Known Issues to Address

1. **Frontend error "data undefined"**
   - Need to see browser console
   - May be authentication issue
   - Added null checks as fallback

2. **Wolf restart clears all apps**
   - Lobbies disappear
   - Moonlight sessions orphaned
   - Need graceful handling or auto-recreation

3. **Certificate cache grows unbounded**
   - Every browser tab adds cert
   - Never cleaned up
   - Long-running instances accumulate hundreds
   - Need expiry policy (e.g., prune certs unused for 7 days)

4. **SCTP "chunk too short" warnings**
   - Indicates WebRTC data corruption
   - May cause connection instability
   - Need to monitor frequency

## Next Steps

1. Verify frontend dashboard loads without errors
2. Run health check stress test to baseline
3. Use dashboard to identify stuck/zombie sessions
4. Fix any issues found
5. Run full stress test suite
6. Add certificate cache cleanup policy
