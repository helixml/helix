# Production External Agents Not Starting - Investigation

**Date**: 2025-10-20
**Issue**: External agents show "Ready" but containers never start, stuck at "Loading desktop..."

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         User's Browser                                   │
│  - Connects to https://code.helix.ml                                    │
│  - Clicks "Start External Agent"                                        │
│  - Opens stream view → tries to connect to moonlight-web               │
└────────────────────┬────────────────────────────────────────────────────┘
                     │ HTTPS (via Caddy)
                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│                      Helix API Container                                 │
│  1. Receives external agent creation request                            │
│  2. Calls Wolf API via Unix socket: POST /api/v1/apps/add              │
│  3. Waits for Wolf to accept app                                       │
│  4. Connects to moonlight-web WebSocket (kickoff session)              │
│     - URL: ws://moonlight-web:8080/api/host/stream                     │
│     - Sends AuthenticateAndInit with:                                   │
│       * credentials: $MOONLIGHT_CREDENTIALS                             │
│       * session_id: "agent-{sessionID}-kickoff"                         │
│       * mode: "keepalive"                                               │
│       * app_id: {wolfAppID}                                             │
│  5. Waits 10 seconds then disconnects                                  │
│  6. Returns "Ready" to browser                                         │
└────────────────────┬────────────────────────────────────────────────────┘
                     │ Unix socket (/var/run/wolf/wolf.sock)
                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│                      Wolf Container                                      │
│  1. Receives POST /api/v1/apps/add via Unix socket                     │
│  2. Stores app definition in memory (in-memory, lost on restart)       │
│  3. Selects video encoders (H264/HEVC/AV1)                             │
│  4. Returns success to API                                             │
│  5. WAITS for Moonlight client to connect via HTTP API                 │
│  6. When client connects → creates Docker container                    │
│     - Only starts container when Moonlight protocol connection made    │
└────────────────────┬────────────────────────────────────────────────────┘
                     │ HTTP (port 47989) - Moonlight protocol
                     ↓
┌─────────────────────────────────────────────────────────────────────────┐
│                   Moonlight-Web Container                                │
│  1. Receives WebSocket connection from API (kickoff)                   │
│  2. Creates session with mode=Keepalive                                │
│  3. SHOULD connect to Wolf via HTTP to start streaming                 │
│     - Query: http://wolf:47989/serverinfo                              │
│     - Then: Start Moonlight protocol handshake                         │
│     - This triggers Wolf to create container                           │
│  4. Return stream URL to browser                                       │
│                                                                         │
│  LATER: When browser connects for real streaming                       │
│  5. Browser connects via WebRTC                                        │
│  6. Moonlight-web resumes existing session (same client cert)          │
│  7. Stream frames to browser                                           │
└─────────────────────────────────────────────────────────────────────────┘

```

## The Problem

**Symptom**:
- API successfully creates Wolf app
- moonlight-web creates "Keepalive" session
- API reports "Kickoff complete"
- But Wolf NEVER creates the Docker container
- Container doesn't exist (DNS lookup fails)

**Expected flow**:
1. API → Wolf: Add app (✅ works)
2. API → moonlight-web: Create keepalive session (✅ works)
3. moonlight-web → Wolf: Connect via Moonlight protocol (❌ **FAILS**)
4. Wolf: Start Docker container when client connects (❌ never happens)

## Evidence

### What Works in Dev
```bash
# Dev environment
docker compose -f docker-compose.dev.yaml ps
# Wolf running ✅
# moonlight-web running ✅
# External agents work perfectly ✅
```

### What Fails in Prod
```bash
# Prod logs (ssh root@code.helix.ml)
API:  "Wolf app created successfully" ✅
      "Moonlight-web keepalive session established successfully" ✅
      "External agent session created, ready for WebSocket" ✅

Wolf: POST /api/v1/apps/add received ✅
      Selects encoders ✅
      NO container creation ❌

moonlight-web: "Creating new session agent-{id}-kickoff with mode Keepalive" ✅
               NO further activity ❌
               NO connection to Wolf ❌

Result: Container zed-external-{id} does not exist ❌
```

## Differences Found

### 1. Wolf Config Version (FIXED)
- **Dev**: `config_version = 6`, `apps = []`
- **Prod (before fix)**: `config_version = 5`, static apps defined
- **Status**: ✅ Fixed by copying dev config to prod + updating install.sh

### 2. Logging Levels
- **Dev**:
  - Wolf: `WOLF_LOG_LEVEL=DEBUG`
  - moonlight-web: `RUST_LOG=moonlight_common=trace,moonlight_web=trace`
- **Prod (before fix)**:
  - Wolf: `WOLF_LOG_LEVEL=ERROR` → changed to DEBUG
  - moonlight-web: `RUST_LOG=moonlight_common=info,moonlight_web=info` → changed to trace
- **Status**: ✅ Fixed

### 3. Environment Variables
- **Dev**: `WOLF_MODE=apps` set in .env
- **Prod**: Not set (defaults to "apps")
- **Status**: ✅ Added to prod .env

### 4. moonlight-web Connection to Wolf

**The Core Issue**: moonlight-web creates a Keepalive session but never actually
connects to Wolf's Moonlight HTTP API (port 47989) to trigger container startup.

**Trace evidence**:
```
moonlight-web: "Creating new session agent-{id}-kickoff with mode Keepalive"
moonlight-web: (nothing more - no connection attempt)
Wolf: (waiting forever for client connection)
```

**Expected behavior in Keepalive mode**:
- moonlight-web should connect to `http://wolf:47989/serverinfo`
- Perform Moonlight protocol handshake
- This triggers Wolf to start the container
- Then maintain headless stream (no WebRTC peer)

**Actual behavior**:
- moonlight-web creates session object in memory
- Never initiates Moonlight protocol connection to Wolf
- Wolf never sees a client → never starts container

## Questions to Investigate

### Is moonlight-web trying to connect but failing?
```bash
# Check moonlight-web trace logs
docker compose logs moonlight-web --since 5m | grep -E "wolf|47989|connecting|serverinfo"
```

**Result**: One line shows `starting new connection: http://wolf:47989/` but no follow-up

### Is Wolf's HTTP API reachable?
```bash
# From moonlight-web container
curl http://wolf:47989/serverinfo
```

**Result**: Can't test (no curl in container), but Wolf IS listening on 47989

### Does Keepalive mode actually work?
moonlight-web might not properly implement Keepalive mode to trigger Wolf connection.

## Configuration Comparison

### moonlight-web Environment

**Dev**:
```yaml
environment:
  - RUST_LOG=moonlight_common=trace,moonlight_web=trace
  - MOONLIGHT_INTERNAL_PAIRING_PIN=${MOONLIGHT_INTERNAL_PAIRING_PIN:-}
```

**Prod**:
```yaml
environment:
  - RUST_LOG=moonlight_common=info,moonlight_web=info  # Changed to trace
  - MOONLIGHT_INTERNAL_PAIRING_PIN=${MOONLIGHT_INTERNAL_PAIRING_PIN:-}
```

### moonlight-web config.json

**Both Dev and Prod** (after install.sh):
```json
{
  "bind_address": "0.0.0.0:8080",
  "credentials": "{random}",  // Matches MOONLIGHT_CREDENTIALS env var
  "webrtc_ice_servers": [...]
}
```

**Note**: No Wolf hostname configured - moonlight-web must auto-discover or default

### Wolf Environment

**Dev**:
```yaml
environment:
  - WOLF_LOG_LEVEL=DEBUG
  - WOLF_SOCKET_PATH=/var/run/wolf/wolf.sock
  - WOLF_INTERNAL_IP=172.19.0.50
```

**Prod**:
```yaml
environment:
  - WOLF_LOG_LEVEL=DEBUG  # Fixed
  - WOLF_SOCKET_PATH=/var/run/wolf/wolf.sock
  - WOLF_INTERNAL_IP=172.19.0.50
```

## Network Configuration

Both dev and prod use same network:
- Network name: `helix_default`
- Subnet: `172.19.0.0/16`
- Wolf IP: `172.19.0.50`
- Services can reach each other by service name (wolf, moonlight-web, api)

## TURN Server

**Dev**: `TURN_PUBLIC_IP=api` (local relay)
**Prod**: `TURN_PUBLIC_IP=code.helix.ml` (public hostname)

This shouldn't affect kickoff (no WebRTC in Keepalive mode).

## Next Steps

1. ✅ Wolf config version 6 (fixed)
2. ✅ Trace logging enabled (fixed)
3. ✅ WOLF_MODE=apps set (fixed)
4. ❓ **Why doesn't moonlight-web connect to Wolf in Keepalive mode?**

### Hypothesis: moonlight-web Keepalive Mode Bug

The `mode: "keepalive"` parameter might not trigger Wolf connection in the production
moonlight-web version. In dev, we're using a locally built image (`helix-moonlight-web:helix-fixed`)
vs prod using the registry image.

**Test needed**: Compare moonlight-web image versions and behavior.

### Alternative: Wolf Auto-Start

If Keepalive mode doesn't trigger container startup, we need a different approach:
- Wolf needs to auto-start containers when apps are added
- Or use a different trigger mechanism
- Or verify moonlight-web Keepalive implementation

## Complete Architecture and Message Flow

### Components and Their Roles

```
┌──────────────────────────────────────────────────────────────────────┐
│                    1. User Browser                                    │
│  - Navigates to https://code.helix.ml                                │
│  - Clicks "Start External Agent" button                              │
│  - Sends: POST /api/v1/sessions (with app config)                   │
└─────────────────────────┬────────────────────────────────────────────┘
                          │ HTTPS
                          ↓
┌──────────────────────────────────────────────────────────────────────┐
│                    2. Helix API Container                             │
│  Step 2a: Create Wolf App                                            │
│    - Generates wolf_app_id (numeric hash of user+session)           │
│    - Creates workspace directory                                     │
│    - Calls: WolfClient.AddApp() via Unix socket                     │
│      → POST http://localhost/api/v1/apps/add                        │
│      → Body: {"id": "547432281", "title": "Agent 607q", ...}        │
│    - Wolf responds: {"success": true}                               │
│    - API logs: "Wolf app created successfully"                      │
│                                                                       │
│  Step 2b: Create Kickoff Session (Trigger Container Start)          │
│    - Connects WebSocket: ws://moonlight-web:8080/api/host/stream   │
│    - Sends AuthenticateAndInit message:                             │
│      {                                                               │
│        "AuthenticateAndInit": {                                      │
│          "credentials": "$MOONLIGHT_CREDENTIALS",                    │
│          "session_id": "agent-{sessionID}-kickoff",                 │
│          "mode": "keepalive",                                        │
│          "client_unique_id": "helix-agent-{sessionID}",            │
│          "host_id": 0,                                              │
│          "app_id": 547432281,  // The Wolf app ID                  │
│          "bitrate": 20000,                                          │
│          "fps": 60, "width": 1920, "height": 1080,                 │
│          ...                                                         │
│        }                                                             │
│      }                                                               │
│    - Waits 10 seconds                                               │
│    - Closes WebSocket                                               │
│    - API logs: "Kickoff complete"                                   │
│    - Returns "Ready" to browser                                     │
└─────────────────────────┬────────────────────────────────────────────┘
                          │ Unix Socket
                          ↓
┌──────────────────────────────────────────────────────────────────────┐
│                    3. Wolf Container                                  │
│  Step 3a: Receive AddApp Request                                     │
│    - Unix socket listener receives POST /api/v1/apps/add            │
│    - Parses app JSON                                                 │
│    - Stores app in memory: apps_map[547432281] = app_config        │
│    - Selects encoders (H264/HEVC/AV1)                               │
│    - Logs: "Using H264 encoder: nvcodec"                            │
│    - Returns: {"success": true}                                      │
│    - App now in memory, waiting for Moonlight client                │
│                                                                       │
│  Step 3b: Wait for Moonlight Client Connection                      │
│    - HTTP server listening on port 47989                            │
│    - Waits for: GET /serverinfo?uniqueid=X&uuid=Y                   │
│    - When received:                                                  │
│      * Verifies client is paired (has certificate)                  │
│      * Returns server info + app list                               │
│    - Then waits for: POST /launch?rikey=...&rikeyid=...            │
│    - When received:                                                  │
│      * CREATES DOCKER CONTAINER for app_id                          │
│      * Starts Wayland compositor                                     │
│      * Starts GStreamer pipelines                                    │
│      * Begins streaming video/audio                                  │
│                                                                       │
│  CRITICAL: Container only created when Moonlight protocol starts!  │
│  If no client connects via HTTP:47989, app sits idle forever       │
└─────────────────────────┬────────────────────────────────────────────┘
                          │ HTTP (Moonlight Protocol)
                          │ Port 47989
                          ↓
┌──────────────────────────────────────────────────────────────────────┐
│                  4. Moonlight-Web Container                           │
│  Step 4a: Receive Kickoff WebSocket Connection                      │
│    - WebSocket endpoint: /api/host/stream                           │
│    - Receives AuthenticateAndInit message from API                  │
│    - Validates credentials: $MOONLIGHT_CREDENTIALS                  │
│    - Logs: "Creating new session agent-{id}-kickoff with mode      │
│             Keepalive"                                               │
│    - Creates StreamSession object:                                   │
│      * session_id: "agent-{sessionID}-kickoff"                      │
│      * mode: SessionMode::Keepalive                                  │
│      * app_id: 547432281                                            │
│      * host_id: 0                                                    │
│                                                                       │
│  Step 4b: Start Moonlight Stream (CRITICAL!)                        │
│    - Code path (streamer/src/main.rs:429):                          │
│      if keepalive_mode {                                             │
│        info!("[Keepalive]: Starting Moonlight stream once");        │
│        spawn(async {                                                 │
│          this_clone.start_stream().await  // THIS MUST EXECUTE!    │
│        });                                                           │
│      }                                                               │
│                                                                       │
│    - start_stream() should:                                         │
│      1. Query Wolf: GET http://wolf:47989/serverinfo               │
│      2. Get app list                                                │
│      3. Call: POST http://wolf:47989/launch?appid=547432281        │
│      4. Perform Moonlight protocol handshake                        │
│      5. Start receiving video/audio streams from Wolf               │
│                                                                       │
│  THIS IS WHERE IT FAILS IN PROD!                                    │
│  - Session created ✅                                                │
│  - start_stream() never called or fails silently ❌                 │
│  - No "[Keepalive]: Starting Moonlight stream" log ❌              │
│  - Wolf never receives /serverinfo or /launch ❌                    │
│  - Container never created ❌                                        │
└──────────────────────────────────────────────────────────────────────┘
```

## Detailed Message Flow (Successful Case)

### Phase 1: App Registration
```
API → Wolf (Unix socket):
  POST /api/v1/apps/add
  {
    "id": "547432281",
    "title": "Agent 607q",
    "runner": {
      "type": "docker",
      "image": "registry.helixml.tech/helix/zed-agent:2.4.0-rc15",
      "name": "zed-external-01k80s1eyvmmqsqa53ke6w607q",
      ...
    }
  }

Wolf → API:
  HTTP 200 OK
  {"success": true}
```

### Phase 2: Kickoff Session (Trigger Container Start)
```
API → moonlight-web (WebSocket):
  WS CONNECT ws://moonlight-web:8080/api/host/stream

API → moonlight-web:
  {
    "AuthenticateAndInit": {
      "credentials": "UtoL7ZKr2m7WhJW",
      "session_id": "agent-ses_01k80s1eyvmmqsqa53ke6w607q-kickoff",
      "mode": "keepalive",
      "app_id": 547432281,  // Tells moonlight-web which Wolf app
      "host_id": 0,          // Local Wolf instance
      ...
    }
  }

moonlight-web receives message:
  - Creates StreamSession in memory
  - Spawns streamer subprocess
  - **SHOULD call start_stream()** ← THIS IS THE FAILURE POINT
```

### Phase 3: Moonlight Protocol Handshake (Container Creation Trigger)
```
moonlight-web → Wolf (HTTP):
  GET http://wolf:47989/serverinfo?uniqueid=helix-agent-{sessionID}&uuid={uuid}

Wolf → moonlight-web:
  HTTP 200 OK
  <XML with server details, app list>

moonlight-web → Wolf:
  POST http://wolf:47989/launch?appid=547432281&rikey=...&rikeyid=...

Wolf receives /launch:
  - Creates Docker container for app 547432281  ← CONTAINER STARTS HERE
  - Starts Wayland compositor inside container
  - Starts GStreamer video producer pipeline
  - Returns RTSP URL

moonlight-web → Wolf:
  Moonlight RTP/RTSP streaming begins
  - Video frames: UDP port 48100
  - Audio frames: UDP port 48200
  - Control: UDP port 47999
```

## Validation Plan: Dev vs Prod

### Test 1: Wolf App Registration
**Goal**: Verify Wolf receives and stores app

**Dev**:
```bash
cd /home/luke/pm/helix
# After creating external agent in UI:
docker compose -f docker-compose.dev.yaml exec api curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/apps | python3 -c "import sys, json; apps=json.load(sys.stdin).get('apps', []); print(f'Apps: {len(apps)}'); [print(f\"  {a['id']}: {a['title']}\") for a in apps]"
```

**Prod**:
```bash
ssh root@code.helix.ml "docker compose -f /opt/HelixML/docker-compose.yaml exec api curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/apps | python3 -c \"import sys, json; apps=json.load(sys.stdin).get('apps', []); print(f'Apps: {len(apps)}'); [print(f'  {a[\\\"id\\\"]}: {a[\\\"title\\\"]}') for a in apps]\""
```

**Expected**: Both should show 1 app after session creation
**Actual in Prod**: 0 apps (config version issue - FIXED)

### Test 2: moonlight-web Session Creation
**Goal**: Verify session created with correct mode

**Dev**:
```bash
docker compose -f docker-compose.dev.yaml logs moonlight-web --tail 50 | grep "Creating new session"
```

**Prod**:
```bash
ssh root@code.helix.ml "docker compose -f /opt/HelixML/docker-compose.yaml logs moonlight-web --tail 50 | grep 'Creating new session'"
```

**Expected**: `Creating new session agent-{id}-kickoff with mode Keepalive`
**Actual**: ✅ Both show this

### Test 3: Keepalive Stream Start (CRITICAL)
**Goal**: Verify moonlight-web calls start_stream() for keepalive sessions

**Dev**:
```bash
docker compose -f docker-compose.dev.yaml logs moonlight-web --tail 100 | grep "\[Keepalive\]"
```

**Prod**:
```bash
ssh root@code.helix.ml "docker compose -f /opt/HelixML/docker-compose.yaml logs moonlight-web --tail 100 | grep '\[Keepalive\]'"
```

**Expected**:
```
[Keepalive]: Starting Moonlight stream once (no auto-restart)
[Keepalive]: Moonlight stream started successfully
```

**Actual in Prod**: ❌ NO LOGS - start_stream() never called or binary doesn't have code

### Test 4: Wolf Moonlight Protocol Connection
**Goal**: Verify Wolf receives /serverinfo and /launch requests

**Dev**:
```bash
docker compose -f docker-compose.dev.yaml logs wolf --tail 200 | grep -E "GET.*serverinfo|POST.*launch|Creating.*session.*for app"
```

**Prod**:
```bash
ssh root@code.helix.ml "docker compose -f /opt/HelixML/docker-compose.yaml logs wolf --tail 200 | grep -E 'GET.*serverinfo|POST.*launch|Creating.*session.*for app'"
```

**Expected**: Wolf should log HTTP requests from moonlight-web
**Actual in Prod**: ❌ No requests - moonlight-web never connects

### Test 5: Container Creation
**Goal**: Verify Docker container exists

**Dev**:
```bash
docker ps | grep zed-external
```

**Prod**:
```bash
ssh root@code.helix.ml "docker ps | grep zed-external"
```

**Expected**: Container running
**Actual in Prod**: ❌ No container exists

## Root Cause Analysis

### Confirmed Working in Dev
1. ✅ Wolf app registration works
2. ✅ moonlight-web creates Keepalive session
3. ✅ `[Keepalive]: Starting Moonlight stream` appears in logs
4. ✅ moonlight-web connects to Wolf via HTTP:47989
5. ✅ Wolf receives /serverinfo and /launch
6. ✅ Container created and starts
7. ✅ Desktop loads

### Broken in Prod
1. ✅ Wolf app registration works (after config version 6 fix)
2. ✅ moonlight-web creates Keepalive session
3. ❌ **NO `[Keepalive]: Starting Moonlight stream` log**
4. ❌ moonlight-web NEVER connects to Wolf
5. ❌ Wolf receives no /serverinfo or /launch
6. ❌ Container never created
7. ❌ Browser stuck at "Loading desktop..."

### The Breakpoint

**Issue**: moonlight-web creates session but `start_stream()` never executes (line 429-442 in streamer/src/main.rs)

**Possible causes**:
1. **Binary version mismatch**: Prod binary doesn't have keepalive start_stream code
2. **Silent failure**: Code exists but fails before reaching info!() log
3. **Streamer not spawned**: Session created but streamer subprocess never starts
4. **Race condition**: Streamer starts but exits before calling start_stream()

## Systematic Debugging Plan

### Step 1: Verify Binary Has Code
**Test**: Check if log string exists in binary

**Dev**:
```bash
docker run --rm --entrypoint /bin/sh helix-moonlight-web:helix-fixed -c 'grep -a "Starting Moonlight stream once" /app/streamer || echo NOT FOUND'
```

**Prod**:
```bash
ssh root@code.helix.ml "docker run --rm --entrypoint /bin/sh registry.helixml.tech/helix/moonlight-web:2.4.0-rc15 -c 'grep -a \"Starting Moonlight stream once\" /app/streamer || echo NOT FOUND'"
```

### Step 2: Check Streamer Process
**Test**: Verify streamer subprocess is spawned

**Prod after session creation**:
```bash
ssh root@code.helix.ml "docker exec helixml-moonlight-web-1 ps aux | grep streamer"
```

**Expected**: Should show running `streamer` process
**If missing**: Streamer not spawned → Check web-server logs for spawn errors

### Step 3: Verify Moonlight-Web Session Registry
**Test**: Check if session exists in memory

**Prod (no API endpoint, need to infer from logs)**:
```bash
ssh root@code.helix.ml "docker compose -f /opt/HelixML/docker-compose.yaml logs moonlight-web --since 5m | grep -E 'session.*agent.*kickoff|Removing session|Session.*persisting'"
```

**Expected**: Session should persist (not removed)

### Step 4: Check Wolf HTTP API Accessibility
**Test**: Verify Wolf's Moonlight HTTP API responds

**From API container** (simulates moonlight-web):
```bash
# Dev
docker compose -f docker-compose.dev.yaml exec api wget -q -O- http://wolf:47989/serverinfo?uniqueid=test&uuid=test | head -20

# Prod
ssh root@code.helix.ml "docker compose -f /opt/HelixML/docker-compose.yaml exec api wget -q -O- http://wolf:47989/serverinfo?uniqueid=test&uuid=test | head -20"
```

**Expected**: XML response with server info
**If fails**: Network issue or Wolf not listening

### Step 5: Manual Moonlight Connection Test
**Test**: Manually trigger what moonlight-web should do

**From API container to simulate moonlight-web**:
```bash
ssh root@code.helix.ml "docker compose -f /opt/HelixML/docker-compose.yaml exec api curl -v http://wolf:47989/applist?uniqueid=helix-test&uuid=test"
```

**Expected**: Should return app list with app 547432281
**If 404**: Apps not being stored (config issue - should be fixed now)

### Step 6: Enable Streamer Debug Logging
**Test**: Get detailed logs from streamer subprocess

**Add to moonlight-web environment**:
```yaml
environment:
  - RUST_LOG=moonlight_common=trace,moonlight_web=trace
  - RUST_BACKTRACE=1  # Enable backtraces for panics
```

**Check**: Streamer stdout/stderr should go to container logs

## Current Status (After Fixes)

**Fixes Applied**:
1. ✅ Wolf config.toml → version 6, apps = []
2. ✅ install.sh → creates config.toml for new installs
3. ✅ WOLF_MODE=apps in prod .env
4. ✅ MOONLIGHT_CREDENTIALS set and matching
5. ✅ Wolf DEBUG logging enabled
6. ✅ moonlight-web trace logging enabled
7. ✅ moonlight-web rebuilt and deployed (commit 7ef5916a8)

**Still Broken**:
- ❌ No `[Keepalive]: Starting Moonlight stream` log in prod
- ❌ Container never created
- ❌ Desktop stuck at "Loading..."

**Next Actions**:
1. Run validation plan above to identify exact failure point
2. Compare binary contents if needed (strings, objdump, etc.)
3. Check if streamer subprocess is even running
4. Verify moonlight-web → Wolf network connectivity
5. Check for silent panics/crashes in streamer

## Hypothesis

**Most likely**: The moonlight-web binary in registry doesn't have the keepalive
start_stream code (added Oct 14). The image was built Oct 15 but might be from
before that commit.

**Test**: Check git commit used for rc15 build and compare to HEAD.

**Solution if true**: Rebuild moonlight-web from current HEAD, push to registry,
deploy to prod.
