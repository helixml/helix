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

### Root Cause
**CUDA memory format mismatch during interpipe source switch**

The test pattern producer in `config.toml.template` produced system RAM frames:
```
videotestsrc ... ! video/x-raw, format=NV12  # System memory
```

But the consumer pipeline expects CUDA memory (from `video_params_zero_copy`):
```
interpipesrc ... ! cudaupload ! video/x-raw(memory:CUDAMemory)
```

**Timeline of failure:**
1. Session starts → test pattern producer creates `{session_id}_video` interpipesink (system RAM)
2. Consumer creates interpipesrc with `cudaupload` in pipeline
3. Wolf fires SwitchStreamProducerEvent to switch to lobby producer (CUDA memory)
4. `cudaupload` fails: "Failed to map input buffer" / "Failed to copy CUDA -> CUDA"
5. Pipeline crashes with "Internal data stream error"
6. All subsequent sessions inherit corrupted GStreamer state

### Fix
Updated test pattern to produce CUDA memory frames:
```toml
source = '''videotestsrc pattern=solid-color foreground-color=4288938183 is-live=true !
video/x-raw, width={width}, height={height}, framerate={fps}/1, format=NV12 !
cudaupload ! video/x-raw(memory:CUDAMemory)'''
```

Now both test pattern and lobby producer output CUDA memory → no format mismatch during switch.

### Why This Wasn't Caught Before
The session leak fix caused more frequent test pattern → lobby switches (cleanup works now), exposing the latent format mismatch bug that was always present but rarely triggered.

## Dashboard Regression

Still investigating. May be related to session tracking changes in lobbies mode.
