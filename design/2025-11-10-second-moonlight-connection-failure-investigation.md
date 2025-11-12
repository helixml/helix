# Second Moonlight Connection Failure Investigation

**Date:** 2025-11-10
**Status:** IN PROGRESS
**Severity:** HIGH - Blocks concurrent lobby streaming

## Problem Statement

When opening two spec task detail windows with Moonlight streams:
- **First connection:** Works perfectly, streams lobby successfully
- **Second connection:** Fails with `PeerDisconnect` error after auto-join

**Reproducibility:** 100% - Happens every time by simply clicking two spec tasks

## System State When Failure Occurs

### Working Connection (First Session)
```
Moonlight Client: helix-agent-ses_01k9g7cx800pd7p3m5sw9r2301-...-9cf3c572...
Wolf Session: 7388522614789140160
Lobby: 06a79a49... (Agent 2301)
Status: ✅ Streaming successfully
Wolf UI Container: Wolf-UI_7388522614789140160 (Running)
```

### Failed Connection (Second Session)
```
Moonlight Client: helix-agent-ses_01k9g9004pysc0sr4jhd37xdct-...-9c56b4af...
Wolf Session: 16143635639681485080 (created then stopped)
Lobby: c631c0d2... (Agent xdct)
Status: ❌ Failed with PeerDisconnect
Wolf UI Container: Wolf-UI_16143635639681485080 (Stopped after 7 seconds)
```

## Timeline of Second Connection Failure

```
T=0s (17:13:28)   - Moonlight session created in moonlight-web
T=4s (17:13:32)   - Wolf creates Wolf UI session 16143635639681485080
                  - Wolf UI container starts
                  - ENET client connects (172.19.0.14:31964)
                  - GStreamer pipelines start
                  - WebRTC negotiation begins

T=5s (17:13:33)   - Auto-join triggered from frontend
                  - Helix API calls Wolf JoinLobby API
                  - Wolf responds: SUCCESS ✅
                  - Interpipe switch: Wolf UI → Lobby c631c0d2
                  - Switch completes successfully ✅

T=9s (17:13:37)   - ENET client disconnects (172.19.0.14:31964) ❌
                  - Wolf fires PauseStreamEvent
                  - Lobby logic: "Moonlight stream over, leaving lobby"
                  - Client switched back: Lobby → Wolf UI
                  - WebRTC peer state: failed

T=11s (17:13:39)  - Moonlight-web streamer sends PeerDisconnect to browser
                  - Browser displays error
                  - Moonlight session cleaned up

T=12s (17:13:40)  - Wolf stops Wolf UI container
                  - Session fully terminated
```

## Key Observations

### 1. Auto-Join API Succeeds
```
[AUTO-JOIN] ✅ Successfully called JoinLobby API
Wolf log: [LOBBY] Session 16143635639681485080 joining lobby c631c0d2...
Wolf log: [HANG_DEBUG] Switch complete for session 16143635639681485080
```

**The join operation completes successfully from Wolf's API perspective.**

### 2. Interpipe Switch Completes
```
Wolf log: Switching interpipesrc listen-to: interpipesrc_16143635639681485080_video
Wolf log: Switch complete for session 16143635639681485080
```

**The GStreamer interpipe switch succeeds - no format or caps errors.**

### 3. Lobby Container is Healthy
```
$ docker ps --filter name=zed-external-01k9g9004pysc0sr4jhd37xdct
aa47abfe803c   helix-sway:latest   Up 23 minutes
```

**The agent lobby container is running and producing video/audio to interpipesink.**

### 4. But Client Never Appears in Lobby
```
Wolf memory API: lobby c631c0d2 client_count = 0
Dashboard: Lobby "Agent xdct" - 0 clients
```

**Despite JoinLobby returning success, Wolf reports 0 clients in the lobby!**

### 5. ENET Disconnect After 4 Seconds
```
17:13:33 - JoinLobby completes
17:13:37 - ENET disconnected client (exactly 4 seconds later)
```

**ENET (UDP control stream) disconnects precisely 4 seconds after joining.**

## Root Cause Hypothesis

### Hypothesis 1: ENET Control Stream Breaking During Switch
**Evidence:**
- ENET connects successfully to Wolf UI
- After interpipe switch to lobby, ENET disconnects 4 seconds later
- No explicit error, just disconnect event

**Possible Causes:**
1. **Control messages not routed correctly after switch** - The lobby's control handler might not be processing ENET packets
2. **Keepalive timeout** - If ENET expects control messages every X seconds and they stop after switch, it disconnects
3. **UDP port routing** - The lobby uses different ENET ports/sockets than Wolf UI

### Hypothesis 2: WebRTC Media Stream Starvation
**Evidence:**
- WebRTC peer state: connecting → connected → disconnected → failed
- Happens after interpipe switch
- No media errors logged, but peer connection dies

**Possible Causes:**
1. **No media flowing from lobby** - interpipesrc listening to lobby's interpipesink but no data
2. **Format mismatch** - Lobby produces different caps than Wolf UI expected
3. **Buffer pool issue** - The "Failed to acquire buffer from pool: -2" might be related

### Hypothesis 3: Race Condition in Concurrent Session Setup
**Evidence:**
- First session works
- Second session fails during same timing window
- GPU buffer error seen: "Failed to acquire buffer from pool: -2"

**Possible Causes:**
1. **Shared CUDA context conflict** - Two sessions trying to allocate from same GPU context
2. **GStreamer pipeline interference** - Concurrent pipeline operations racing
3. **ENET port conflict** - Both sessions trying to use same UDP ports

## What We've Ruled Out

### ✅ Lobby Container Issues
- Container is running and healthy
- WebSocket to Helix API connected
- Receiving and processing messages correctly
- No crashes or errors in container logs

### ✅ Moonlight Certificate Issues
- Certificate generated and cached successfully
- No SSL/TLS errors
- Auto-pairing with Wolf succeeds

### ✅ Wolf API Failures
- JoinLobby API returns success
- No errors in Wolf API logs during join
- Lobby exists and is accessible

### ✅ Basic Concurrent Streaming Support
- First session streams successfully
- Dashboard shows proper state for first session
- GPU has capacity (1 of 5 lobbies used)

## Open Questions

### Q1: Why Does ENET Disconnect After Exactly 4 Seconds?
- Is there a 4-second timeout somewhere?
- Is this the Moonlight protocol's control stream keepalive interval?
- Does the lobby forward ENET packets correctly?

### Q2: Why Does Wolf Report client_count=0 Despite Successful Join?
- JoinLobby API returns success
- But lobby's `connected_sessions` vector is empty
- Is there a race between join completion and client registration?

### Q3: What's the "Failed to acquire buffer from pool: -2" Error?
- Happens during session shutdowns
- Error code -2 = GST_FLOW_NOT_NEGOTIATED
- Related to CUDA buffer pool
- Could concurrent sessions be exhausting the pool?

### Q4: Does Buffer Pool Config (0, 0) Cause Issues?
```rust
buffer_pool.configure(
    &cuda_caps,
    stream_handle,
    buffer_size,
    0,  // min_buffers
    0,  // max_buffers
)
```

This looks suspicious but first session works, so it might be intentional (fallback to direct allocation).

## BREAKTHROUGH FINDING

### Wolf Treats Second Connection as RESUME (It Shouldn't!)

**Critical Log:**
```
17:43:20.586 | [HTTPS] Received resume event from an unregistered session, ip: 172.19.0.14
```

**What This Means:**
- Wolf interprets the second client's connection as a RESUME attempt
- But there's no existing session to resume ("unregistered")
- Wolf rejects the connection
- Moonlight-web streamer calls `/cancel` and gives up

**Expected Behavior:**
- Second client should create a NEW Wolf UI container instance
- Should be treated as fresh connection, not resume

### Why Is It Being Treated as Resume?

**First client flow:**
```
17:41:30 - Certificate generated for client_unique_id ...7a9bfe63...
17:41:30 - Calls /serverinfo (accepted)
17:41:30 - Calls /launch (creates Wolf UI container)
17:41:30 - Streams successfully ✅
```

**Second client flow:**
```
17:42:50 - Certificate generated for client_unique_id ...f22dc2af...
17:43:20 - Calls /serverinfo (Wolf: "unregistered session, resume event")
17:43:20 - Calls /cancel (aborts, no /launch)
17:43:20 - IPC closes, connection fails ❌
```

**Key Questions:**
1. How does Wolf determine if a connection is RESUME vs NEW?
2. Is Wolf using the TLS certificate to identify resume attempts?
3. Is there a global Wolf limit on concurrent NEW sessions?
4. Did we accidentally enable resume mode somewhere?

### Apps Mode RESUME Logic (Removed from Lobbies)

From commit `fe7a5cdd5` (Oct 15):
```javascript
// OLD Apps mode approach:
client_unique_id: `helix-agent-${sessionId}` // SAME for kickoff and browser
// Comment: "SAME client ID as browser → enables RESUME"
```

This was for Apps mode where:
- Kickoff creates session with mode=keepalive
- Browser reconnects with mode=create
- **Same certificate** → Wolf treats as RESUME → reuses existing app

**In Lobbies mode we changed this:**
```javascript
client_unique_id: `helix-agent-${sessionId}${lobbyIdPart}-${componentInstanceId}`
```

Each component gets unique ID → unique certificate → should be NEW connection.

**But Wolf still sees it as RESUME!**

### Hypothesis: Wolf Uses Certificate Fingerprint, Not client_unique_id

Wolf's Moonlight protocol might:
1. Hash the client TLS certificate
2. Check if that cert has an existing session
3. If yes → RESUME
4. If no → NEW

But there might be a bug where:
- Wolf checks if **ANY** session from moonlight-web exists
- Sees the first client's session
- Treats ALL subsequent moonlight-web connections as RESUME attempts

Or:
- Moonlight-web is using the same certificate for all clients despite different client_unique_id
- Certificate caching is broken

## ROOT CAUSE IDENTIFIED ✅

### Certificate Subject Collision

**Problem:** All moonlight-web generated certificates had identical subject/issuer:
```
CN=example.com, O=Example Corp, L=San Francisco, ST=CA, C=US
```

**Impact:** Wolf's `X509_V_FLAG_PARTIAL_CHAIN` verification matched all certificates as the same client based on subject/issuer, despite different RSA keys.

**Evidence:**
```cpp
// Wolf's get_client_via_ssl uses find_if - returns FIRST match
auto search_result = std::find_if(
    paired_clients->begin(),
    paired_clients->end(),
    [&client_cert](const immer::box<PairedClient> &pair_client) {
        auto verification_error = x509::verification_error(paired_cert, client_cert);
        return !verification_error;  // Returns true on FIRST matching cert
    });
```

With `X509_V_FLAG_PARTIAL_CHAIN`, certificates with same subject chain match even if keys differ.

**Result:**
- Client 1 pairs → Wolf stores cert with CN=example.com
- Client 2 pairs → Wolf stores cert with CN=example.com (different key)
- Client 2 connects → Wolf loops, finds Client 1's cert matches → Returns Client 1
- Client 1 already has session → Wolf rejects as "unregistered resume attempt"

## SOLUTION IMPLEMENTED ✅

### Fix: Unique Certificate Common Names

**File:** `/home/luke/pm/moonlight-web-stream/moonlight-common/src/pair.rs`

**Change:**
```rust
// OLD (lines 149-153):
name.append_entry_by_text("CN", "example.com")?;

// NEW:
#[cfg(feature = "network")]
let unique_cn = format!("moonlight-{}", Uuid::new_v4());
#[cfg(not(feature = "network"))]
let unique_cn = "example.com".to_string();

name.append_entry_by_text("CN", &unique_cn)?;
```

**Result:** Each certificate now has unique CN like:
```
CN=moonlight-f47ac10b-58cc-4372-a567-0e02b2c3d479, O=Moonlight Client, ...
```

**Impact:**
- Wolf can properly differentiate between clients
- Multiple concurrent sessions now possible
- Each client gets their own Wolf UI container instance
- Certificates won't match each other during verification

**Status:** ✅ Moonlight-web rebuilt with fix
**Status:** ✅ Wolf rebuilt with public key verification fix

## Test Results After Certificate Fix

### Test 1: With Auto-Join Enabled ✅ PARTIAL SUCCESS

**Timeline (Session 14530388960276128580):**
```
21:04:05 - Wolf creates session ✅ NEW! (was failing before)
21:04:06 - Session joins lobby 630dd72c ✅ NEW!
21:04:06.364 - Interpipe switch completes ✅
21:04:18.949 - ENET disconnects ❌ (12 seconds after join)
21:04:18.949 - PauseStreamEvent fires
21:04:18.949 - Session leaves lobby
21:04:19 - Wolf UI container stopped
```

**Progress:**
- ✅ No more "unregistered session" error
- ✅ Second client successfully connects to Wolf
- ✅ Second client creates Wolf UI session
- ✅ Auto-join successfully switches to lobby
- ❌ ENET (UDP control stream) disconnects after 12 seconds

### Test 2: Earlier Test Without Auto-Join (Before Cert Fix)

**Result:** Connection failed even earlier - never reached /launch
**Error:** "Received resume event from an unregistered session"
**Conclusion:** Proved interpipe switching wasn't the root cause

## Current Issue: ENET Disconnect After Lobby Join

### Symptom
ENET control stream (UDP port 47999) disconnects 12 seconds after interpipe switch, triggering PauseStreamEvent which kicks client out of lobby.

### Evidence
```
21:04:06.364 - Interpipe switch complete
... 12 seconds of silence (no ENET logs) ...
21:04:18.949 - [ENET] disconnected client: 172.19.0.14:62130
```

**No control messages during those 12 seconds.**

### Why ENET Disconnects - Hypotheses

#### Hypothesis 1: ENET Keepalive Timeout
- ENET expects periodic control messages (mouse/keyboard)
- After ~12 seconds of inactivity, times out and disconnects
- User isn't moving mouse/typing during connection

**Test:** Move mouse/type during connection to keep ENET alive
**Likelihood:** Medium

#### Hypothesis 2: Lobby Not Forwarding Control Messages
- When switching from Wolf UI → Lobby, control routing breaks
- Browser sends control messages but lobby doesn't process them
- ENET sees no traffic, times out

**Code Check:** lobbies.cpp lines 236-238 switch mouse/keyboard to lobby's Wayland
**Likelihood:** High

#### Hypothesis 3: WebRTC Media Stream Failure
- After interpipe switch, video/audio stops flowing
- WebRTC peer connection times out due to no media
- Triggers disconnect cascade

**Evidence:** ICE state: checking → connected → disconnected → failed
**Likelihood:** High

## ACTUAL ROOT CAUSE IDENTIFIED ✅

### WebRTC Port Exhaustion - Only 10 Ports for All Connections

**Location:** `/home/luke/pm/helix/moonlight-web-config/config.json:24-27`

**The Bottleneck:**
```json
"webrtc_port_range": {
  "min": 40000,
  "max": 40010
}
```

Only **10 UDP ports** allocated for ALL WebRTC connections!

**What Happens:**
1. **First Session:** Binds to ports 40000-40001 (video + audio RTP) ✅
2. **Second Session:** Tries to bind → All ports in use → ICE fails ❌

**WebRTC-Internals Evidence (Second Session):**
```
21:52:08 - ICE gathering starts
21:52:08 - ICE gathering completes
21:52:08 - ICE candidates exchanged (host, srflx, relay)
21:52:23 - iceconnectionstatechange: "disconnected" (15s timeout)
21:52:23 - connectionstatechange: "failed"
```

**ICE never reaches "connected" state** - classic port exhaustion symptom.

### Why Moonlight Stream Works But WebRTC Fails

**Two separate connections:**
1. **Moonlight Protocol (Wolf ↔ Streamer):** Uses UDP port 47999 (ENET control) + dynamic RTP ports ✅
2. **WebRTC (Streamer ↔ Browser):** Uses ports 40000-40010 (configured range) ❌

The Moonlight connection succeeds (H264 frames reach streamer), but WebRTC connection fails (video never reaches browser).

### Why First Session Works

**First session gets lucky:**
- Port 40000 available → binds successfully
- Port 40001 available → binds successfully
- ICE connects, WebRTC established ✅

**Second session unlucky:**
- Ports 40000-40001 already in use by first session
- Tries remaining ports 40002-40010
- NAT/firewall issues with remaining ports
- ICE fails → WebRTC fails ❌

### Why This Wasn't Obvious Earlier

**Misleading evidence:**
- Certificate issues masked the problem initially
- "AlreadyStreaming" errors were from certificate collisions, not singleton
- Moonlight logs showing H264 frames suggested everything working
- 12-second timing suggested ENET keepalive timeout
- Design doc incorrectly blamed global singleton (streamers are separate processes)

## SOLUTION IMPLEMENTED ✅

### Part 1: Expand WebRTC Port Range

**Files Updated:**
1. `/home/luke/pm/helix/moonlight-web-config/config.json`
2. `/home/luke/pm/helix/moonlight-web-config/config.json.template`

```json
// Before (10 ports - supports ~3 concurrent sessions max)
"webrtc_port_range": {
  "min": 40000,
  "max": 40010
}

// After (100 ports - supports ~30 concurrent sessions)
"webrtc_port_range": {
  "min": 40000,
  "max": 40100
}
```

**Port calculation:**
- 1-2 ports for RTP media (video/audio)
- 1 port for RTCP (control)
- ~3 ports total per session
- 100 ports ÷ 3 = ~30 concurrent sessions ✅

### Testing Results ✅

**Fix verified working:** Port range expansion (10 → 100 ports) completely resolved the issue!

**What we learned:**
- Port exhaustion prevented second WebRTC session from binding
- TURN relay fallback was handling NAT traversal automatically (no `webrtc_nat_1to1` needed!)
- Docker port mapping `40000-40100:40000-40100/udp` + TURN server was sufficient

**Deployment:**
```bash
docker compose -f docker-compose.dev.yaml restart moonlight-web
```

**Tested:** Multiple concurrent sessions now work successfully!

## Deep Dive Analysis - Actual Code Review

### Architecture Confirmed:
- **Web-server** (parent process) spawns **streamer** child processes (separate address space)
- Each streamer communicates via stdin/stdout IPC
- **No shared global state** between streamers (original singleton theory was wrong)
- Wolf supports up to 20 concurrent ENET peers (not a bottleneck)

### Cascade When Second Session Fails:
```
WebRTC peer enters Failed state
→ streamer's on_peer_connection_state_change() triggers stop()
→ stop() sends PeerDisconnect via IPC to web-server
→ stop() drops Moonlight stream
→ stop() calls host.cancel() to terminate Wolf session
→ Streamer exits
→ Browser displays error
```

### Working Hypotheses (Ranked by Likelihood)

#### Hypothesis 1: WebRTC Peer Connection Never Fully Establishes ⭐⭐⭐
**Evidence:**
- Second streamer successfully receives H264 frames (Moonlight works)
- After 12 seconds, connection fails
- 12 seconds is typical WebRTC connection timeout

**Possible Causes:**
1. **ICE negotiation fails** - Second client can't establish P2P connection
2. **STUN/NAT conflict** - Both clients behind same NAT, ICE candidates collide
3. **Port allocation conflict** - WebRTC UDP ports already in use by first session

**Test:** Check browser console for second tab - look for:
- ICE state stuck in "checking" or "failed"
- No remote video track received
- "Failed to set remote offer" errors

#### Hypothesis 2: ENET Control Stream Not Being Sent by Second Client ⭐⭐
**Evidence:**
- Wolf logs show ENET disconnect after 12 seconds
- No "ENET received" logs during those 12 seconds
- Moonlight stream working (H264 frames) but control stream dead

**Possible Causes:**
1. **WebRTC data channel not established** - ENET packets sent via WebRTC data channel "input"
2. **Input handler not initialized** - StreamInput not forwarding ENET packets
3. **Browser not sending control events** - Second tab not focused, no mouse/keyboard events

**Test:** Check Wolf logs for second session:
```bash
# Look for ENET received messages during 12-second window
grep "ENET.*received" wolf.log
```

#### Hypothesis 3: Wolf Session State Conflict ⭐
**Evidence:**
- First session works, second fails
- Both connect to same Wolf instance

**Possible Causes:**
1. **ENET secret collision** - Unlikely (cryptographically random)
2. **Wolf app_id conflict** - Both using same Wolf UI app ID (134906179)?
3. **Client certificate still matching** - Despite unique CN fix

**Test:** Check if both sessions use same Wolf app_id:
```
Dashboard → Check Wolf UI app ID for both sessions
```

## Next Steps - Experiments to Run

### Experiment 1: Browser Console Inspection (CRITICAL)
**Open second browser tab's console and check for:**
- WebRTC peer connection state timeline
- ICE connection state (checking → connected → failed?)
- Media track status (remote video/audio tracks added?)
- JavaScript errors during connection

**Expected if Hypothesis 1 correct:** ICE state stuck in "checking", never reaches "connected"

### Experiment 2: Check Wolf ENET Logs During Failure Window
```bash
docker compose -f docker-compose.dev.yaml logs --tail 200 wolf | grep -A5 -B5 "ENET"
```

**Look for:**
- ENET connect for second session
- Any ENET received messages during 12-second window
- ENET disconnect event

**Expected if Hypothesis 2 correct:** No "ENET received" messages between connect and disconnect

### Experiment 3: Single Client Test with Manual ENET Silence
Modify frontend to NOT send mouse/keyboard for 15 seconds after connection.

**Expected:** First client also disconnects after ~12 seconds if ENET keepalive required

### Experiment 4: Check Wolf App IDs
Query Wolf API for both sessions:
```bash
docker compose -f docker-compose.dev.yaml exec api \
  curl --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/sessions
```

**Look for:** Both sessions using same `app_id`? Different Wolf session IDs?

### Experiment 5: Disable Auto-Join AND Manual Testing
1. Disable auto-join
2. Open first tab → Connect successfully → Leave in Wolf UI
3. Open second tab → Try to connect
4. Check if second tab fails at same 12-second mark

**Expected if interpipe unrelated:** Still fails (already tested this previously)

## Relevant Code Locations

### Wolf
- `/home/luke/pm/wolf/src/moonlight-server/sessions/lobbies.cpp:353` - on_moonlight_session_over callback
- `/home/luke/pm/wolf/src/moonlight-server/control/control.cpp` - ENET disconnect triggers PauseStreamEvent
- `/home/luke/pm/wolf/src/moonlight-server/streaming/streaming.cpp` - PauseStreamEvent handlers

### gst-wayland-display
- `/home/luke/pm/gst-wayland-display/wayland-display-core/src/utils/allocator/cuda/ffi.rs:390` - Buffer pool error
- `/home/luke/pm/gst-wayland-display/wayland-display-core/src/comp/rendering.rs:94` - MappingError
- Commit: `e89d9f5d` (used by Wolf)

### Moonlight-web
- `/home/luke/pm/moonlight-web-stream/moonlight-web/streamer/src/main.rs:484` - Peer disconnect triggers stop()
- `/home/luke/pm/moonlight-web-stream/moonlight-web/streamer/src/main.rs:869` - stop() sends PeerDisconnect

### Helix
- `/home/luke/pm/helix/api/pkg/server/external_agent_handlers.go:1289` - JoinLobby API call
- `/home/luke/pm/helix/frontend/src/components/external-agent/MoonlightStreamViewer.tsx:280` - Auto-join (now disabled)

## References

- Previous session: design/2025-11-08-session-review-changes.md
- Concurrency analysis: (from earlier Explore agent analysis)
- Lobbies architecture: design/2025-11-08-moonlight-streaming-stress-test-suite.md
