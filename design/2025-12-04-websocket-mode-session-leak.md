# WebSocket Mode Session Leak Investigation

**Date:** 2025-12-04
**Status:** FIXED

## Problem Statement

When switching streaming modes in the browser:
- **WebRTC → close**: Cleans up properly (threads return to baseline)
- **WebSocket → close**: LEAKS threads (4 threads per close)

Additionally, the Moonlight state dashboard has regressed:
- Shows "Blank" and "Select Agent" apps with "0 clients"
- Shows "No Moonlight clients" even when connected

## Root Cause Analysis

### Why WebRTC Works But WebSocket Doesn't

**WebRTC Mode:**
- Disconnection is detected via WebRTC peer state change (ICE/DTLS layer)
- `on_peer_connection_state_change()` triggers `stop()`
- Cleanup runs reliably even if WebSocket closes uncleanly

**WebSocket-Only Mode:**
- Disconnection relies SOLELY on detecting the WebSocket close
- When browser closes uncleanly (no Close frame), `stream.recv()` hangs indefinitely
- The frame-forwarding task detects the broken connection (send fails), but the main input loop doesn't know
- Cleanup never runs → session leaks

### The Hang Scenario

1. Browser closes tab or network breaks (no clean WebSocket Close frame)
2. Frame-forwarding task tries to send video frame → fails with "Closed"
3. Frame-forwarding task exits
4. Main input loop is still blocking on `stream.recv()`
5. Without a proper Close frame, `stream.recv()` waits for TCP keepalive timeout (~2 hours!)
6. Cleanup never runs → Wolf session orphaned → test pattern producers leak

### Evidence from Logs

```
19:01:13 [WARN] [WsStream]: Failed to send audio frame: Closed
19:01:50 [INFO] [WsStream]: Init:  ← NEW session starts 37 seconds later!
```

Notice: NO cleanup messages between the frame send failure and new session. No "[WsStream]: Sending Stop" or "[WebSocket-Only]: Received stop signal".

## Solution

Use `tokio::select!` to listen for EITHER:
1. WebSocket messages (input from browser)
2. Shutdown signal from the frame-forwarding task (connection broken)

When the frame-forwarding task detects a send failure:
1. Signals shutdown via oneshot channel
2. Main loop receives signal and breaks
3. Cleanup runs immediately

### Code Change

In `ws_stream_handler()` (web-server/src/api/stream.rs):

```rust
// Create shutdown channel
let (shutdown_tx, mut shutdown_rx) = oneshot::channel::<()>();

// In frame-forwarding task, signal on exit:
info!("[WsStream]: Frame forwarder exiting, signaling shutdown");
let _ = shutdown_tx.send(());

// In main loop, use select!:
loop {
    select! {
        _ = &mut shutdown_rx => {
            info!("[WsStream]: Received shutdown signal from frame forwarder");
            break;
        }
        msg = stream.recv() => {
            // handle input...
        }
    }
}
```

## Previous Fixes Applied (This Session)

1. **WebRTC mode: call cancel() before drop()** - Fixed WebRTC cleanup order
2. **Added PauseStreamEvent handlers to test pattern producers** - Test pattern producers now stop on both StopStreamEvent AND PauseStreamEvent
3. **Purple loading screen** - Changed from chroma-zone-plate to solid purple (#a100c7)
4. **Canvas clearing on disconnect** - Clears canvas on disconnect to prevent stale frames persisting when switching modes

## Files Modified

- `moonlight-web/web-server/src/api/stream.rs` - Added shutdown signal + select!
- `wolf/config.toml.template` - Changed loading screen to purple
- `wolf/src/moonlight-server/streaming/streaming.cpp` - Added PauseStreamEvent handlers
- `helix/frontend/src/components/external-agent/MoonlightStreamViewer.tsx` - Added canvas.clearRect() on disconnect

## Verification

After fix deployment:
1. Start WebSocket-only stream
2. Close browser tab (unclean close)
3. Check logs for: "[WsStream]: Received shutdown signal from frame forwarder"
4. Check logs for: "[WsStream]: Sending Stop to streamer for clean shutdown"
5. Verify thread count returns to baseline (6 threads)

## Secondary Issue: Black Screen After Mode Switch

### Symptom
After session leak fix was deployed:
1. First session works
2. Switching modes (WebSocket → WebRTC) or reconnecting = black screen
3. All future sessions show blank screens

### Attempted Fixes (All Failed)

#### Attempt 1: CUDA memory format mismatch theory (WRONG)
**Theory:** Test pattern outputs system RAM, lobby outputs CUDA → format mismatch during switch.

**Fix tried:** Added `cudaupload` to test pattern source:
```toml
source = '''videotestsrc ... ! cudaupload ! video/x-raw(memory:CUDAMemory)'''
```

**Result:** Broke streaming entirely. Even first session failed with InternalServerError.

**Why it failed:**
- `cudaupload` is NVIDIA-specific (wouldn't work on AMD/Intel anyway)
- Theory was based on GStreamer errors seen during switch, but may have been a symptom not root cause

#### Attempt 2: Timing delay before auto-join (DIDN'T HELP)
**Theory:** Auto-join happens too fast (36ms after consumer starts), before pipeline is ready.

**Fix tried:** Added 500ms delay in `MoonlightStreamViewer.tsx`:
```typescript
const timer = setTimeout(doAutoJoin, 500);
```

**Result:** Still black screen on mode switch. Delay alone doesn't fix it.

### Current Status: NEEDS BISECTION

The black screen issue appeared after the session leak fix, but the exact cause is unclear.

**Changes made in session leak fix:**
1. `stream.rs`: Added `tokio::select!` with shutdown signal from frame forwarder
2. `streaming.cpp`: Added `PauseStreamEvent` handlers to test pattern producers
3. `config.toml.template`: Changed test pattern to purple solid color
4. `MoonlightStreamViewer.tsx`: Added canvas clearing on disconnect

**One of these changes (or their interaction) broke mode switching.**

**Next steps:**
1. Revert to last known working state (before session leak fix)
2. Apply changes one at a time to isolate which one breaks mode switching
3. The PauseStreamEvent handlers are most suspicious - they change when test pattern producers stop

### Key Question
Why did mode switching work before with thread leaks, but breaks now with proper cleanup?

**Hypothesis:** The old behavior (test pattern never stopping) was actually important for the interpipe switch to work. When we made cleanup work properly, we also stopped something that shouldn't have been stopped.

## Bisection Findings (2025-12-05)

### Critical Discovery: Race Condition on Low-Latency Connections

User tested from two browsers:
- **Localhost (via RDP)**: Blank screen RELIABLY on WebSocket mode
- **WiFi over SSH tunnel**: Works RELIABLY (higher latency)

**Conclusion:** The race condition is timing-related. On fast connections (localhost), the interpipe switch happens before the consumer pipeline is ready to receive frames.

### Attempted Timing Fixes (Did Not Help)

1. **Frontend 500ms delay before auto-join:**
   ```typescript
   // In MoonlightStreamViewer.tsx
   await new Promise(resolve => setTimeout(resolve, 500));
   ```

2. **Backend polling loop after JoinLobby:**
   ```go
   // In external_agent_handlers.go autoJoinWolfLobby()
   maxWait := 2 * time.Second
   pollInterval := 100 * time.Millisecond
   // Poll Wolf sessions until session.AppID changes from test pattern to lobby
   ```

Both fixes are currently uncommitted on `main` branch.

### Current Hypothesis: Test Pattern Producer Issue

The test pattern producer (`videotestsrc pattern=solid-color`) may be causing the issue:
- Test pattern outputs system RAM frames
- Lobby producer outputs GPU memory frames
- Format mismatch during switch could cause black screen on fast connections

**Test:** Switch from "Blank" (test pattern) to "Select Agent" (Wolf-UI Docker container) to see if real compositor eliminates the race condition.

### Uncommitted Changes to Preserve

**Helix (main branch):**
1. `frontend/src/components/external-agent/MoonlightStreamViewer.tsx` - 500ms delay before auto-join
2. `api/pkg/server/external_agent_handlers.go` - Polling loop after JoinLobby

These changes can be recovered from git stash if needed.

## CRITICAL FINDING: WebSocket vs WebRTC Cleanup Difference (2025-12-05)

### The Problem
- **WebSocket-only mode**: Closing connections STILL leaks interpipesrc sessions and threads
- **WebRTC mode**: Closing connections cleans up correctly
- **Reproduction**: Simply open WebSocket connection, close browser tab. Even refresh doesn't clean up.

**This rules out Wolf changes (test pattern producers, PauseStreamEvent handlers) as the cause.**
The issue is in moonlight-web-stream's WebSocket cleanup path.

### Code Comparison Summary

**WebRTC Mode (`StreamConnection::stop()` in main.rs:880-952):**
1. Idempotency check via `stopped` mutex
2. Sends `PeerDisconnect` IPC message
3. Sends `ConnectionTerminated` via general_channel
4. Calls `host.cancel().await` (HTTP request to Wolf)
5. Drops stream in `spawn_blocking`
6. Sends `StreamerIpcMessage::Stop`
7. Notifies `terminate` waiters

**WebSocket-Only Mode (`run_websocket_only_mode` cleanup in main.rs:1243-1272):**
1. Calls `host.cancel().await`
2. Drops stream in `spawn_blocking`
3. Sends `StreamerIpcMessage::Stop`

**Key Differences:**
- No idempotency check in WebSocket mode
- No `PeerDisconnect` / `ConnectionTerminated` messages
- No `terminate.notify_waiters()` call

### Hypotheses for Cleanup Failure

1. **H1: IPC Stop Not Reaching Streamer**
   - Web-server sends `ServerIpcMessage::Stop` but streamer never receives it
   - Could be due to IPC buffer issues or timing
   - Test: Add logging to confirm Stop is received

2. **H2: `host.cancel()` Hanging or Timing Out**
   - HTTP request to Wolf takes >15 seconds
   - Web-server kills streamer before cancel completes
   - Test: Add timing logs around `host.cancel()`

3. **H3: Race Between WebSocket Close and Cleanup**
   - When WebSocket closes, frame forwarder task exits
   - Shutdown signal race with main loop detection
   - Test: Add sequence logging

4. **H4: ENET Connection Already Dead**
   - In WebSocket mode, ENET might time out before `host.cancel()` is called
   - Wolf might ignore cancel for dead sessions
   - Test: Compare Wolf logs for WebRTC vs WebSocket close

5. **H5: Missing Termination Signals**
   - WebRTC sends `PeerDisconnect` and `ConnectionTerminated` before cleanup
   - These might trigger additional cleanup in Wolf
   - Test: Add these messages to WebSocket cleanup

6. **H6: No Idempotency = Double Cleanup Issues**
   - WebSocket mode might call cleanup twice
   - Second call might interfere with first
   - Test: Add idempotency check like WebRTC mode

7. **H7: Stream Drop Before Cancel Completes**
   - `spawn_blocking` for drop might race with `host.cancel()`
   - Stream drops → ENET disconnect → Wolf fires `PauseStreamEvent` before `StopStreamEvent`
   - Test: Add barrier between cancel completion and stream drop

8. **H8: Frame Forwarder Task Keeps Resources Alive**
   - Frame forwarder holds clones of resources
   - When WebSocket closes, forwarder might not exit cleanly
   - Test: Add explicit cleanup of frame forwarder

### Most Likely Cause

**Hypothesis H7 (Stream Drop Before Cancel)** seems most likely because:
- Wolf cleanup depends on `StopStreamEvent` (not `PauseStreamEvent`)
- ENET disconnect fires `PauseStreamEvent`
- If stream drops before cancel HTTP completes, sequence is wrong:
  1. `host.cancel()` starts (HTTP in flight)
  2. `spawn_blocking(drop(stream))` starts immediately (not waiting for cancel!)
  3. Stream drops → ENET disconnects → `PauseStreamEvent` fires
  4. Cancel HTTP completes → `StopStreamEvent` fires (too late?)

**The fix**: Ensure `host.cancel()` fully completes BEFORE calling `spawn_blocking(drop(stream))`.

Current code already awaits cancel, but there might be a subtle issue with how Rust handles the async boundary.

## Dashboard Regression

Still investigating. May be related to session tracking changes in lobbies mode.
