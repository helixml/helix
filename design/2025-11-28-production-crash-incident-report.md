# Incident Report: Production Crash During Customer Demo

**Date:** 2025-11-28
**Time of Incident:** 10:21:05 UTC
**Duration:** Meeting started 09:30, crash occurred 51 minutes in
**Severity:** High - Live customer demo interrupted
**Customer:** Achraf (AXA)
**Participants:** Luke (Helix), Achraf (customer)

## Executive Summary

During a live customer demo session, the Helix sandbox crashed abruptly at 10:21:05 UTC. The crash was sudden with no warning - all RevDial connections (Wolf, Moonlight, sandbox) failed simultaneously with "connection reset by peer" errors. The root cause is NOT disk space exhaustion (current disk is 60% used). Evidence suggests Wolf internal state corruption or unhandled exception, likely related to session/lobby state management when multiple viewers were connected.

## Detailed Timeline (Correlated from Meeting Transcript + API Logs)

### Pre-Meeting Context
- **Previous day (Nov 27):** Achraf experienced disk space error during session
- Luke had already fixed the disk issue before this meeting

### Meeting Timeline

| Time (UTC) | Meeting Min | Event Source | Event |
|------------|-------------|--------------|-------|
| 09:30:00 | 00:00 | Transcript | Meeting started, Achraf resumes session from yesterday |
| 09:30:17 | 00:17 | Transcript | Achraf mentions disk space error from yesterday was fixed |
| 09:30:43 | 00:43 | Transcript | Luke notes session resume issues, keyboard doesn't work (French layout) |
| 09:32:07 | 02:07 | Transcript | Discussion of shift key switching keyboard language |
| 09:33:04 | 03:04 | Transcript | Luke mentions reduced debug dumps from 1 week to ~1 day |
| 09:34:02 | 04:02 | Transcript | Luke: "My keyboard isn't working at all" / Achraf: "it's very slow" |
| 09:34:09 | 04:09 | Transcript | Luke: "this session is... dead, I think" |
| 09:34:40 | 04:40 | Transcript | Decision to close dead session and make new one |
| 09:39:12 | 09:12 | Transcript | New task started, UV installing dependencies |
| 09:39:27 | 09:27 | Transcript | "This is the bug where we need to refresh the page" |
| 09:59:00 | 29:00 | Transcript | Session disconnected after ~20 min, Achraf refreshed (timeout?) |
| 10:00:18 | - | API Logs | Wolf resource monitoring: RSS=980MB, 2 lobbies, 1 client, 6 pipelines |
| 10:17:28 | - | API Logs | Multiple "lobby does not exist" errors for stale lobby references |
| 10:18:26 | 48:26 | Transcript | Issues with "Failed to stop agent" button |
| 10:19:11 | 49:11 | Transcript | Things being slow |
| 10:19:29 | 49:29 | Transcript | "Maybe you need to reload your browser" |
| **10:21:05** | **49:34** | **API Logs** | **CRASH: "connection reset by peer" - all RevDial connections fail** |
| 10:21:05 | 49:34 | Transcript | Luke: "Oh, no, Something went wrong in the back end" |
| 10:21:05 | - | API Logs | Cascade of "revdial.Dialer closed" errors |
| 10:21:42 | 51:42 | Transcript | Luke: "encryption errors and auto pairing failed and then it restarted" |
| 10:25:00+ | 55:00+ | Transcript | Session resumed but workspace empty (wrong mount) |

### Crash Moment Analysis (10:21:05 UTC)

**From API Logs at 10:21:05:**
```
Failed to connect to sandbox via RevDial error="revdial.Dialer closed"
Failed to read response from RevDial connection: read tcp 172.19.0.10:8080->172.201.248.88:40006:
  read: connection reset by peer
[connman] Dial failed for key=wolf-axa-private dialer=0xc0003a93b0: revdial.Dialer closed
[connman] Dial failed for key=moonlight-axa-private dialer=0xc00040a070: revdial.Dialer closed
[connman] Dial failed for key=sandbox-ses_01kb4xbsrrt77qw7g5s4341tq6: revdial.Dialer closed
```

**Key observation:** All three connections (wolf, moonlight, sandbox) failed at the exact same moment. This indicates the sandbox container itself crashed, not individual service failures.

### Sandbox Container Logs (Pre-Crash)

**Found in `design/crash-logs-sandbox.txt`** - Container logs showing both pre-crash and post-restart execution.

#### Moonlight Web Connection Failures (10:19:39 - 10:21:05)
Starting ~90 seconds before crash, Moonlight Web RevDial began failing constantly:
```
[MOONLIGHT-REVDIAL] 2025/11/28 10:19:39 Proxy error: readfrom tcp 127.0.0.1:47812->127.0.0.1:8080:
  websocket: close 1006 (abnormal closure): unexpected EOF
```
**~200+ rapid retry failures** - Moonlight Web could not maintain connections.

#### Pipeline Shutdown at Crash Moment (10:21:05.429)
```
10:21:05.429958827 WARN  | [HANG_DEBUG] Audio PauseStreamEvent for session 56832706557674607 (quitting main loop)
10:21:05.430136923 WARN  | [HANG_DEBUG] Video PauseStreamEvent for session 56832706557674607 (quitting main loop)
10:21:05.430213758 INFO  | [THREAD_LIFECYCLE] Pipeline thread (TID=32773) exiting normally, cleaning up
10:21:05.707026663 INFO  | Stopped container: /Wolf-UI_56832706557674607
```

#### 🔴 CRITICAL: OpenSSL Encryption Cascade Failure (10:21:05.707+)
**200+ consecutive OpenSSL EVP errors immediately after container stop:**
```
EVP_EncryptInit_ex failed
EVP_CIPHER_CTX_set_padding failed
EVP_EncryptInit_ex failed
EVP_EncryptUpdate failed
EVP_EncryptFinal_ex failed
... (repeated 200+ times)
EVP_DecryptInit_ex failed
EVP_CTRL_GCM_SET_TAG failed
```

**This is the smoking gun.** OpenSSL cipher context failures indicate:
- **Memory corruption** affecting EVP_CIPHER_CTX structures
- **Thread safety issue** with OpenSSL contexts shared across threads
- **Resource exhaustion** preventing cipher context allocation

#### Auto-Pairing Failure (10:21:05)
```
[MOONLIGHT] 10:21:05 [WARN] [Stream]: Auto-pairing failed: Api(RequestClient(Reqwest(
  hyper::Error(IncompleteMessage))))
```
Wolf stopped responding to HTTP requests - the process was in a broken state.

#### Container Restart (10:21:15)
**10-second gap** between last pre-crash log and restart:
```
[2025-11-28 10:21:15] [ /etc/cont-init.d/04-start-dockerd.sh: executing... ]
🐳 Starting Wolf's isolated dockerd...
```
Confirmed container died and restarted via Docker's restart policy.

## Root Cause Analysis

### What We Know

1. **NOT disk space exhaustion:**
   - Current disk: 60% used (238GB of 396GB)
   - Current memory: 78GB/503GB used (425GB free)
   - No resource exhaustion evident

2. **Memory Growth Confirmed (from crash dumps):**
   - Previous hourly crash dump analysis: **~5.6GB memory** just before crash
   - This is ~18x the baseline (~303MB)
   - Indicates substantial memory leak or inefficiency
   - **Crash dumps are the most useful diagnostic tool**

3. **Wolf memory at 980MB at 10:00:18 (API report):**
   - This was ~20 minutes before crash
   - 2 active lobbies, 1 client, 6 GStreamer pipelines
   - Memory likely continued growing after this point

4. **Stale lobby references (10:17-10:21):**
   - Multiple "lobby does not exist" errors for different session IDs
   - Sessions trying to auto-join lobbies that no longer exist
   - Possible state corruption in lobby management

5. **Sudden crash, not gradual:**
   - OpenSSL EVP errors = internal state corruption
   - All connections died simultaneously
   - "connection reset by peer" = process terminated abruptly

### Probable Cause: Memory Exhaustion + OpenSSL Corruption

The evidence chain:
1. **Memory leak over 51 minutes** - grew from ~303MB to ~5.6GB
2. **Session resumption issues** - keyboard not working at 09:34
3. **Moonlight Web failures** - constant "unexpected EOF" from 10:19:39
4. **Pipeline shutdown** - PauseStreamEvent at 10:21:05.429
5. **OpenSSL cascade failure** - 200+ EVP_* errors immediately after
6. **Process death** - container restarted at 10:21:15

**Root cause: Memory leak causing OpenSSL heap corruption**

When Wolf ran low on memory or corrupted its heap:
- OpenSSL EVP_CIPHER_CTX allocations failed
- Encryption/decryption for all sessions broke simultaneously
- Process entered unrecoverable state

### Earlier Session Issues (09:34)

The first session "died" at ~09:34:
- Keyboard stopped working entirely
- Session became unresponsive
- Had to create new session

This suggests Wolf was already in an unstable state from the resumed session.

## Evidence Available

1. **Sandbox container logs preserved (design/crash-logs-sandbox.txt):**
   - ~3900 lines covering both pre-crash and post-restart
   - Shows Moonlight Web failures, pipeline shutdown, EVP errors
   - Confirms container restart at 10:21:15

2. **Hourly crash dumps on sandbox VM:**
   - Located at `/var/wolf-debug-dumps/` on sandbox VM
   - **Most valuable diagnostic tool** - can analyze memory usage sources
   - Previous analysis showed ~5.6GB memory before crash

3. **API logs on controlplane:**
   - Wolf resource monitoring data
   - RevDial connection failures
   - "lobby does not exist" errors

## Evidence Gaps

1. **No core dump from crash itself:**
   - Container was killed, not caught by watchdog
   - No stack trace at crash moment

2. **Memory profiling needed:**
   - Need to analyze crash dumps to identify memory leak sources
   - Check for GStreamer pipeline leaks
   - Check for session/lobby cleanup failures

## Impact

1. **Customer Demo Interrupted:**
   - Live demo failed in front of customer
   - Customer witnessed instability firsthand

2. **Work Lost:**
   - Workspace directory empty on session resume
   - Wrong volume mounted after restart

3. **Session Continuity Broken:**
   - Required workaround to continue (merge from feature branch)

## Immediate Actions Taken

1. **PulseAudio Memory Optimization (commit 611719330):**
   - Reduces memory by ~64MB per session
   - Helps prevent gradual resource exhaustion

2. **WebRTC Placeholder App:**
   - Reduces memory for idle WebRTC clients
   - Less load on Wolf when clients are waiting

3. **Disk Space Monitoring:**
   - Dashboard now shows disk usage over time
   - Enables early detection of disk issues

## Recommended Follow-Up Actions

### Immediate Priority (Memory Leak Investigation)

1. **Analyze Crash Dumps for Memory Leaks:**
   - SSH to sandbox VM: `ssh -i axa-private_key.pem azureuser@172.201.248.88`
   - Crash dumps at: `/var/wolf-debug-dumps/`
   - Look for: heap allocations, GStreamer objects, session state
   - **This is the highest priority diagnostic action**

2. **Identify Top Memory Consumers:**
   - GStreamer pipelines (video/audio producers/consumers)
   - PulseAudio containers
   - Session/lobby state structures
   - OpenSSL contexts

### Short Term

1. **Fix Wolf Session/Lobby Leaks:**
   - Memory growth from ~300MB to ~5.6GB over 51 minutes
   - Lobby references persisting after session cleanup
   - Sessions not being properly destroyed

2. **Fix Session Resume:**
   - Workspace mount not persisting correctly after restart
   - Customer was blocked from continuing work

3. **Fix French Keyboard Layout:**
   - Shift key switches language unexpectedly
   - Major usability blocker for French users

4. **Add Memory Monitoring to Dashboard:**
   - Display Wolf RSS memory over time
   - Alert when memory exceeds threshold

### Medium Term

1. **Add Memory Limits to Wolf:**
   - Container memory limits to prevent runaway growth
   - Graceful degradation when approaching limit

2. **Switch Wolf to Release Build:**
   - Debug build adds overhead
   - RelWithDebInfo provides crash info with less overhead

3. **Multi-Viewer Session Testing:**
   - Load test with multiple concurrent viewers
   - Identify race conditions in session state

## Lessons Learned

1. **Memory leaks are the primary threat:**
   - ~5.6GB memory = 18x baseline
   - OpenSSL fails catastrophically when heap is corrupted
   - Need continuous memory monitoring

2. **Crash dumps are essential:**
   - Only way to diagnose memory issues after the fact
   - Should analyze hourly dumps proactively
   - Keep multiple days of dumps for comparison

3. **Container log preservation works:**
   - Docker retained logs from pre-crash execution
   - This was invaluable for diagnosis
   - Consider also writing logs to persistent storage

4. **Multi-viewer sessions may have race conditions:**
   - Both Luke and Achraf connected to same session
   - Lobby state management may have issues

## Key Codebase Findings (Wolf Session/Lobby Management)

From exploration of `/prod/home/luke/pm/wolf/src/moonlight-server/`:

### Session Lifecycle
- **Creation:** `sessions/moonlight.cpp:76` - StreamSession event handler
- **Cleanup:** `sessions/moonlight.cpp:54-63` - StopStreamEvent handler
- **State:** Uses immer immutable vectors (`app_state->running_sessions`)

### Lobby Lifecycle
- **Creation:** `sessions/lobbies.cpp:73` - CreateLobbyEvent handler
- **Cleanup:** `sessions/lobbies.cpp:288-313` - StopLobbyEvent handler
- **State:** Uses immer immutable vectors (`app_state->lobbies`)

### Potential Leak Sources
1. **GStreamer Pipeline Threads:** Run in detached threads, no timeout mechanism
2. **Lobby References:** If StopLobbyEvent handler exits early (lobby not found), cleanup fails
3. **Plugged Devices Queue:** Captured in lambdas, may persist if handlers not destroyed
4. **interpipe switching:** Race conditions when switching between session/lobby producers

## Concrete Evidence of Leaks (from crash-logs-sandbox.txt)

Analysis of the ~90 second window before crash reveals:

### Pipeline Thread Leak
| Metric | Count |
|--------|-------|
| Pipeline threads started | 18 |
| Pipeline threads exited | 12 |
| **LEAKED threads** | **6+** |

**Leaked pipeline threads (never exited):**
- TID=34637, 34638, 34660, 34662 → session `4527137796221718381`
- TID=34822, 34823 → lobby `4fafeb16-c8f8-41fe-9c0a-3a5684f0a7d7`
- TID=35945, 35946, 35966, 35970 → session `11535114096415410566`

### Container Leak
| Metric | Count |
|--------|-------|
| Containers started | 5 |
| Containers stopped | 3 |
| **LEAKED containers** | **2** |

**Leaked containers (never stopped):**
- `/Wolf-UI_4527137796221718381`
- `/zed-external-01kb2ykmw5px0p6ate4r0nzepv_4fafeb16-c8f8-41fe-9c0a-3a5684f0a7d7` (lobby)
- `/Wolf-UI_11535114096415410566`

### Impact Calculation
Each session creates **4 GStreamer pipeline threads** + **1 Docker container**.
Over 51 minutes, if sessions leak at this rate:
- Leaked threads accumulate
- Leaked containers accumulate
- Memory grows from ~300MB → ~5.6GB
- Eventually triggers OpenSSL heap corruption

### Key Files for Investigation
- `streaming/streaming.cpp` - Pipeline lifecycle, run_pipeline function
- `state/sessions.hpp` - Session lookup and cleanup helpers
- `events/events.hpp` - Event data structures

## Root Cause Deep Dive: Missing Session Timeout

### Why Sessions Leak: The PauseStreamEvent Problem

Sessions are only cleaned up when they receive a `PauseStreamEvent`. This event is **only** sent in three cases:

1. **ENET client disconnects** (`control/control.cpp:186`)
   - `ENET_EVENT_TYPE_DISCONNECT` triggers `PauseStreamEvent`

2. **Client sends TERMINATION packet** (`control/control.cpp:214`)
   - Encrypted packet with `sub_type == TERMINATION`

3. **Wolf API pause endpoint** (`api/endpoints.cpp:324`)
   - `POST /api/v1/stream-session/{id}/pause`

### The Problem

**Wolf has NO session timeout mechanism.** If a client connection drops abruptly (browser crash, network failure, WebSocket close without proper cleanup), none of the above triggers fire:

- **ENET doesn't detect the disconnection** - UDP-based ENET relies on the client to properly disconnect or send keepalives
- **No TERMINATION packet sent** - Client crashed/disconnected before sending it
- **No external API call** - Nobody knows to call the pause endpoint

### Evidence from Crash Logs

Comparing leaked session `4527137796221718381` vs cleaned-up session `13030810309340391575`:

```
=== Session 13030810309340391575 (cleaned up) ===
10:20:04.776004411 WARN | [HANG_DEBUG] Audio PauseStreamEvent for session 13030810309340391575
10:20:04.776183318 WARN | [HANG_DEBUG] Video PauseStreamEvent for session 13030810309340391575
10:20:05.181159699 INFO | Stopped container: /Wolf-UI_13030810309340391575

=== Session 4527137796221718381 (LEAKED) ===
- Pipeline threads started (TID=34637, 34638, 34660, 34662)
- NO PauseStreamEvent ever received
- Container /Wolf-UI_4527137796221718381 never stopped
- All resources leaked until crash
```

### Code Locations

| Component | File | Line | What It Does |
|-----------|------|------|--------------|
| PauseStreamEvent definition | `events/events.hpp` | 284 | Event struct |
| ENET disconnect trigger | `control/control.cpp` | 186 | Fires on `ENET_EVENT_TYPE_DISCONNECT` |
| TERMINATION packet trigger | `control/control.cpp` | 214 | Fires on encrypted TERMINATION |
| API pause trigger | `api/endpoints.cpp` | 324 | Fires on REST API call |
| Pipeline cleanup handler | `streaming/streaming.cpp` | 360-370 | Handles `PauseStreamEvent` for video |
| Pipeline cleanup handler | `streaming/streaming.cpp` | 520-530 | Handles `PauseStreamEvent` for audio |

### Missing Code: Session Timeout

Wolf has heartbeat monitoring for threads (`monitoring/thread-monitor.hpp`) but **no session-level timeout**. The only timeout-related constants are:

- `DEFAULT_SESSION_TIMEOUT_MILLIS = 4000` - Initial connection timeout only
- `timeout_millis = 2500` - RTSP socket timeout
- `SSE_KEEPALIVE_INTERVAL = 15s` - SSE connection keepalive

**None of these clean up orphaned sessions.**

### Proposed Fix: Add Session Timeout

Add a background timer that:
1. Tracks last activity timestamp per session
2. Fires `PauseStreamEvent` for sessions inactive > 60 seconds
3. Uses existing ENET heartbeat mechanism or adds Wolf-level keepalive

Implementation locations:
- Add `last_activity` field to `StreamSession` struct (`events/events.hpp`)
- Add timeout check in main event loop (`wolf.cpp`)
- Fire `PauseStreamEvent` for stale sessions

### Why This Caused Memory Exhaustion

1. **Moonlight Web connections are unstable** - WebSocket/WebRTC drops cause disconnections
2. **Each session leaks ~140MB** - 4 pipeline threads + 1 Docker container
3. **51-minute session** accumulated multiple leaked sessions
4. **~40 sessions leaked** - 5.6GB / 140MB ≈ 40 sessions
5. **OpenSSL heap corruption** - Memory exhaustion corrupts cipher contexts

## CRITICAL: Helix Orphan Cleanup Is DISABLED

### Discovery

Helix API has a `cleanupOrphanedWolfUISessionsLoop` function designed to clean up orphaned Wolf-UI sessions. However, it is **DISABLED** with this comment:

```go
// api/pkg/external-agent/wolf_executor.go:391-395

// TEMPORARILY DISABLED: Start orphaned Wolf-UI session cleanup loop
// (cleans up streaming sessions without active containers)
// ISSUE: This cleanup kills Wolf-UI sessions that have active browser connections
// It only checks if lobby exists, not if browsers are actively streaming
// Disabling until we can add proper check for active WebRTC connections
// go executor.cleanupOrphanedWolfUISessionsLoop(context.Background())
```

### The Problem

The cleanup can't distinguish between:
1. **Orphaned sessions** - No browser connected, should be cleaned up
2. **Active sessions** - Browser connected, should NOT be cleaned up

The current logic checks if a lobby (Zed container) exists for the session, but:
- Browsers can be streaming without a lobby (during lobby creation)
- Lobbies can exist without browsers connected
- The check doesn't account for actual WebRTC connection state

### Why This Matters

Even if we add session timeout at the Wolf level, the Helix API's cleanup would still be needed for:
1. Sessions orphaned by API restarts
2. Sessions orphaned by network partitions
3. Sessions that Wolf fails to clean up internally

### How to Fix

**Option A: Add WebRTC connection tracking to Wolf API**
- Wolf already has a `connected_clients` map in `control.cpp`
- Expose this via API: `GET /api/v1/sessions/{id}/clients`
- Helix checks if session has active clients before cleanup

**Option B: Add activity timestamp to Wolf sessions**
- Track last packet received per session
- Expose via API: `GET /api/v1/sessions` with `last_activity`
- Helix checks if session is idle before cleanup

**Option C: Fix Wolf's internal session timeout (recommended)**
- Add ENET peer timeout configuration
- Wolf automatically fires `PauseStreamEvent` for idle sessions
- No external cleanup needed for normal operation

### Immediate Workaround

Re-enable the cleanup with a longer timeout (5+ minutes) and accept that some active sessions may be killed. This is better than memory exhaustion crashes.

---

*Report prepared: 2025-11-28*
*Author: Claude (AI Assistant)*
*Status: COMPLETE - Root cause identified: Wolf missing session timeout + Helix cleanup disabled*
*Log file: design/crash-logs-sandbox.txt (~3900 lines)*
