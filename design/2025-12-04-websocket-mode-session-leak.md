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

## Deep Analysis: Blank vs Select Agent (2025-12-05)

### Problem Statement
Black screen still occurs on second stream when using Blank app (test pattern) instead of Select Agent (Wolf-UI).
Despite implementing GPU-aware test pattern producer with matching memory formats, the issue persists.

### Code Path Comparison

**Select Agent (Wolf-UI) - `start_virtual_compositor = true`:**
```
moonlight.cpp:93-107 → streaming::start_video_producer()
```

Pipeline structure:
```
waylanddisplaysrc name=wolf_wayland_source render_node=/dev/dri/renderD128 !
  video/x-raw(memory:CUDAMemory), width=1920, height=1080, framerate=60/1 !
  interpipesink sync=true async=false name={session_id}_video max-buffers=5
```

Key characteristics:
- waylanddisplaysrc DIRECTLY outputs GPU memory (native output)
- Caps negotiation is implicit - waylanddisplaysrc decides format
- No explicit `format=NV12` in caps - format is negotiated dynamically
- framerate is included in output caps

**Blank App (test pattern) - `start_virtual_compositor = false`:**
```
moonlight.cpp:136-148 → streaming::start_test_pattern_producer()
```

Pipeline structure (after GPU-aware fix):
```
videotestsrc pattern=solid-color foreground-color=4288938183 is-live=true !
  video/x-raw, width=1920, height=1080, framerate=60/1, format=NV12 !
  cudaupload !
  video/x-raw(memory:CUDAMemory), format=NV12, width=1920, height=1080 !
  interpipesink sync=true async=false name={session_id}_video max-buffers=5
```

Key characteristics:
- videotestsrc outputs **CPU memory** with explicit NV12 format
- We upload to GPU via cudaupload
- **Explicit `format=NV12` on GPU output caps** (different from waylanddisplaysrc!)
- **Missing `framerate=60/1` on GPU output caps** (different from waylanddisplaysrc!)

### Critical Differences Identified

#### 1. Explicit vs Implicit Format Specification

**waylanddisplaysrc output caps:**
```
video/x-raw(memory:CUDAMemory), width=1920, height=1080, framerate=60/1
```
- No `format=NV12` - format is negotiated
- waylanddisplaysrc produces whatever format is optimal for the compositor

**test pattern output caps (after cudaupload):**
```
video/x-raw(memory:CUDAMemory), format=NV12, width=1920, height=1080
```
- Explicit `format=NV12`
- Forces NV12 regardless of what consumer wants

#### 2. Missing Framerate on Test Pattern GPU Output

waylanddisplaysrc includes `framerate=60/1` on its output caps.
Test pattern's GPU output caps have no framerate.

This could cause buffer pool negotiation differences.

#### 3. DMABuf Format List vs Single Format (AMD/Intel)

**waylanddisplaysrc (via compute_pipeline_defaults):**
```
video/x-raw(memory:DMABuf), drm-format={NV12,P010,...}
```
- List of acceptable DRM formats in curly braces

**test pattern (my fix):**
```
video/x-raw(memory:DMABuf), drm-format=NV12
```
- Single format without curly braces
- May not negotiate correctly if consumer expects format list

### Buffer Pool Hypothesis

GStreamer interpipe elements can hold onto buffer pools. When interpipesrc switches
`listen-to` from one producer to another:

1. First session starts → test pattern producer creates buffer pool with its caps
2. Consumer pipeline negotiates caps with that pool
3. Second session starts → test pattern producer tries to use same interpipesink name?

Wait - each session has a DIFFERENT interpipesink name (`{session_id}_video`).
So buffer pools shouldn't interfere between sessions...

Unless the **consumer** is the problem:

1. Session 1: Consumer creates pool based on Session 1's producer caps
2. Session 1 joins lobby: Consumer switches to lobby producer (different caps)
3. Session 2: Consumer creates pool based on Session 2's producer caps
4. Something in the encoder pipeline holds stale pool reference?

### New Hypothesis: Encoder Pipeline Re-negotiation Failure

The consumer pipeline structure (from config.toml):
```toml
[gstreamer.video.defaults.nvcodec]
video_params = '''cudaupload !
cudaconvertscale add-borders=true !
video/x-raw(memory:CUDAMemory), width={width}, height={height}, chroma-site={color_range}, format=NV12, colorimetry={color_space}, pixel-aspect-ratio=1/1'''
```

Full consumer pipeline:
```
interpipesrc name=interpipesrc_{}_video listen-to={session_id}_video ...
  ! cudaupload
  ! cudaconvertscale add-borders=true
  ! video/x-raw(memory:CUDAMemory), width=..., height=..., format=NV12, ...
  ! nvh264enc ...
  ! h264parse
  ! rtpmoonlightpay_video
  ! appsink
```

The consumer does `cudaupload` which can pass-through CUDAMemory input.
But if the incoming format doesn't match expectations, cudaconvertscale might fail.

### Trace the Actual Caps Negotiation

**waylanddisplaysrc → interpipesink:**
- Output: `video/x-raw(memory:CUDAMemory)` (format negotiated, likely NV12 or P010)
- The caps filter adds width/height/framerate constraints

**interpipesrc → cudaupload (consumer):**
- Receives CUDAMemory buffer
- cudaupload passes through (already CUDA)

**test pattern → cudaupload → interpipesink:**
- videotestsrc outputs: `video/x-raw, format=NV12` (CPU)
- cudaupload receives NV12 CPU, outputs: `video/x-raw(memory:CUDAMemory)` (GPU)
- Caps filter: `format=NV12, width=..., height=...`

**Key difference:** The test pattern FORCES explicit format=NV12 on the interpipesink,
while waylanddisplaysrc lets the format be negotiated.

### Root Cause Theory

**The explicit `format=NV12` caps filter on test pattern's interpipesink
is too restrictive.**

When the interpipesrc switches to the lobby producer (waylanddisplaysrc),
the interpipesink for the test pattern might have cached caps with explicit NV12.
When a second session connects, its consumer pipeline might fail to negotiate
because of stale caps state in the interpipe layer.

But wait - each session has its OWN interpipesink... so this doesn't explain it.

### Alternative Theory: Global State in cudaupload

`cudaupload` might maintain global CUDA context or pool state.

1. Session 1: test pattern → cudaupload creates CUDA context with NV12 pool
2. Session 1 joins lobby: consumer switches to lobby's waylanddisplaysrc (different pool)
3. Session 2: test pattern → cudaupload reuses stale CUDA context/pool
4. Pool mismatch causes black frames

This would explain why:
- First session works
- Lobby switch works
- Second session fails

### Recommended Fixes to Test

1. **Remove explicit `format=NV12` from GPU output caps:**
```cpp
gpu_upload = fmt::format("cudaupload ! "
                         "video/x-raw(memory:CUDAMemory), width={}, height={}",
                         display_mode.width, display_mode.height);
```

2. **Add framerate to GPU output caps:**
```cpp
gpu_upload = fmt::format("cudaupload ! "
                         "video/x-raw(memory:CUDAMemory), width={}, height={}, framerate={}/1",
                         display_mode.width, display_mode.height, display_mode.refreshRate);
```

3. **Match waylanddisplaysrc's exact caps format:**
```cpp
gpu_upload = fmt::format("cudaupload ! "
                         "{}, width={}, height={}, framerate={}/1",
                         buffer_caps,  // Use the exact same caps string
                         display_mode.width, display_mode.height, display_mode.refreshRate);
```

### Why Select Agent Works

Select Agent uses waylanddisplaysrc, which:
1. Runs a real Wayland compositor (Docker container)
2. Outputs GPU-native frames with negotiated format
3. Never goes through CPU memory or cudaupload
4. Every session creates fresh waylanddisplaysrc with fresh context

The test pattern shares nothing between sessions either, but the cudaupload
element might have global state that persists across sessions within the same
Wolf process.

### Action Items

1. Test with framerate added to test pattern GPU caps
2. Test with format=NV12 removed from test pattern GPU caps
3. Test with buffer_caps used directly (matching waylanddisplaysrc exactly)
4. If still failing, add GST_DEBUG logging to see actual caps negotiation
5. Consider if cudaupload has global pool state that needs explicit cleanup

### Fix Implemented (2025-12-05)

Updated `start_test_pattern_producer()` in `wolf/src/moonlight-server/streaming/streaming.cpp`:

**Before (incorrect):**
```cpp
gpu_upload = fmt::format("cudaupload ! "
                         "video/x-raw(memory:CUDAMemory), format=NV12, width={}, height={}",
                         display_mode.width, display_mode.height);
```

Output caps: `video/x-raw(memory:CUDAMemory), format=NV12, width=1920, height=1080`
- Explicit `format=NV12` (not in waylanddisplaysrc)
- Missing framerate

**After (matching waylanddisplaysrc):**
```cpp
gpu_upload = fmt::format("cudaupload ! "
                         "{}, width={}, height={}, framerate={}/1",
                         buffer_caps, display_mode.width, display_mode.height, display_mode.refreshRate);
```

Output caps: `video/x-raw(memory:CUDAMemory), width=1920, height=1080, framerate=60/1`
- Uses exact buffer_caps from Wolf's compute_pipeline_defaults()
- Includes framerate (matching waylanddisplaysrc)
- No explicit format (negotiated, like waylanddisplaysrc)

**Why this should work:**
- waylanddisplaysrc pipeline: `waylanddisplaysrc ! {buffer_caps}, width=W, height=H, framerate=F/1 ! interpipesink`
- test pattern pipeline: `videotestsrc ! ... ! cudaupload ! {buffer_caps}, width=W, height=H, framerate=F/1 ! interpipesink`

Both now have identical output caps, so interpipesrc should negotiate identically regardless of which
producer it's listening to.

## GPU Context Sharing Fix (2025-12-05)

### Why First Session Works, Subsequent Fail

The caps format fix alone wasn't sufficient. The root cause was **GPU context isolation**.

**The shared context atom lifecycle (before fix):**

```
Wolf starts → gst_context atom is EMPTY

Session 1 (test pattern):
├─ start_test_pattern_producer() runs
├─ cudaupload/vapostproc needs GPU context
├─ WITHOUT fix: No bus_sync_handler → creates Context-A internally
├─ Context-A is NOT stored in shared atom (atom still empty)
├─ Consumer pipeline (nvh264enc/vaapih264enc) receives buffers with Context-A
├─ Encoder registers resources with Context-A
└─ ✅ WORKS - everything uses Context-A

Session 1 joins lobby:
├─ waylanddisplaysrc starts (this HAS bus_sync_handler)
├─ Creates Context-B, STORES it in shared atom
├─ interpipesrc switches listen-to from test_pattern to lobby
├─ Encoder now receives Context-B buffers
└─ ✅ WORKS - encoder re-negotiates with new source

Session 1 ends, Session 2 starts (test pattern):
├─ start_test_pattern_producer() runs again
├─ WITHOUT fix: No bus_sync_handler → creates Context-C
├─ Context-C ≠ Context-B (shared atom still has Context-B)
├─ Consumer pipeline starts, may reference shared Context-B for encoder init
├─ Receives buffers with Context-C
└─ ❌ RESOURCE_REGISTER_FAILED
   Context mismatch: encoder has Context-B, buffers have Context-C
```

**The key insight:** The first test pattern "wins" because nothing is in the shared atom yet -
everything happens in its own isolated context. But once a lobby (waylanddisplaysrc) runs and
populates the shared atom, subsequent test patterns create *different* contexts, causing the mismatch.

### This Applies to ALL GPU Types (Not Just NVIDIA)

The `gst_video_context` abstraction handles multiple GPU types:

| GPU Vendor | Context Type | Upload Element | Encoder |
|------------|--------------|----------------|---------|
| NVIDIA | CUDA context | `cudaupload` | `nvh264enc` |
| AMD | VA-API context | `vapostproc` | `vaapih264enc` |
| Intel | VA-API context | `vapostproc` | `vaapih264enc` |

All GPU encoders require buffers from a consistent context. The same context-sharing bug would
manifest on AMD/Intel as VA-API context mismatches, causing similar encoder registration failures.

### The Fix

Added `video_context` parameter and `bus_sync_handler` to `start_test_pattern_producer()`:

**Wolf commit:** `c1637c6` on stable branch

**streaming.cpp changes:**
```cpp
// CRITICAL: Set up GPU context sharing so cudaupload/vapostproc uses the same
// context as waylanddisplaysrc. Without this, the encoder receives buffers from
// a different context when interpipesrc switches, causing RESOURCE_REGISTER_FAILED.
std::shared_ptr<NeedContextData> ctx_data_ptr =
    std::make_shared<NeedContextData>(NeedContextData{
        .device_path = render_node,
        .gst_context = video_context
    });

run_pipeline(pipeline, [=](auto pipeline, auto loop) {
    auto bus = gst_pipeline_get_bus(GST_PIPELINE(pipeline.get()));
    gst_bus_set_sync_handler(bus, bus_sync_handler, ctx_data_ptr.get(), nullptr);
    gst_object_unref(bus);
    // ...
});
```

**moonlight.cpp changes:**
```cpp
// Pass gst_context to share GPU context with waylanddisplaysrc
std::thread([session, gst_context = app_state->gst_context]() {
    streaming::start_test_pattern_producer(
        std::to_string(session->session_id),
        session->app->video_producer_source.value(),
        session->app->video_producer_buffer_caps,
        session->app->render_node,
        {.width = session->display_mode.width,
         .height = session->display_mode.height,
         .refreshRate = session->display_mode.refreshRate},
        gst_context,  // <-- NEW: shared context
        session->event_bus);
}).detach();
```

**With the fix (all sessions work):**
```
Session 1 (test pattern):
├─ bus_sync_handler installed
├─ Checks atom → empty → creates Context-A, STORES in atom
└─ ✅ WORKS

Session 1 joins lobby:
├─ waylanddisplaysrc checks atom → Context-A exists → REUSES it
└─ ✅ WORKS - same context

Session 2 (test pattern):
├─ bus_sync_handler checks atom → Context-A exists → REUSES it
└─ ✅ WORKS - same context as everything else
```

All pipelines now share one GPU context via the `immer::atom<gst_video_context::gst_context_ptr>`.

### Threading Safety

The context sharing is thread-safe because:
- `immer::atom` provides atomic `load()`/`store()` operations
- `bus_sync_handler` runs synchronously on the GStreamer bus thread
- Same proven pattern used by `waylanddisplaysrc` for years

## Docker BuildKit Cache Staleness Fix (2025-12-05)

### Problem

When `wolf:helix-fixed` is rebuilt with new source, `./stack build-sandbox` may not pick up
the new Wolf binary due to Docker BuildKit caching:

1. `wolf:helix-fixed` is rebuilt → new image ID (e.g., `sha256:abc123`)
2. `Dockerfile.sandbox` has `FROM wolf:helix-fixed AS wolf-upstream`
3. BuildKit cache for that FROM layer still references OLD image ID
4. Sandbox build uses cached layer with OLD wolf binary

### Solution

Pass wolf/moonlight-web image IDs as build args to bust the cache:

**Dockerfile.sandbox (top of file):**
```dockerfile
ARG WOLF_IMAGE_ID
ARG MOONLIGHT_IMAGE_ID
```

**stack `build-sandbox` function:**
```bash
local WOLF_IMAGE_ID=$(docker images wolf:helix-fixed -q)
local MOONLIGHT_IMAGE_ID=$(docker images helix-moonlight-web:helix-fixed -q)
docker build -f Dockerfile.sandbox \
  --build-arg WOLF_IMAGE_ID="${WOLF_IMAGE_ID}" \
  --build-arg MOONLIGHT_IMAGE_ID="${MOONLIGHT_IMAGE_ID}" \
  ...
```

When the upstream images are rebuilt, their image IDs change, the build args change,
and BuildKit invalidates the cached layers.

### Long-term Fix

Merge wolf.Dockerfile into Dockerfile.sandbox using multi-context builds:
```bash
docker build --build-context wolf-src=../wolf -f Dockerfile.sandbox .
```
This would make wolf source changes directly bust the cache.

## Black Screen on Second Session - Continuing Investigation (2025-12-05)

### Current State

Despite implementing the GPU context sharing fix, black screen still occurs on second session.
The fix is deployed and verified via Docker cache busting, but something else is wrong.

### Detailed Pipeline Architecture

Wolf's streaming system uses three types of GStreamer pipelines:

#### 1. Producer Pipelines (One Per Session/Lobby)

**Test Pattern Producer** (for apps with `start_virtual_compositor = false`):
```
videotestsrc pattern=solid-color foreground-color=0xa100c7ff is-live=true
  → video/x-raw, width={width}, height={height}, framerate={fps}/1, format=NV12
  → cudaupload
  → video/x-raw(memory:CUDAMemory), width={width}, height={height}, framerate={fps}/1
  → interpipesink name="{session_id}_video" sync=true async=false max-buffers=5
```
- Created by `start_test_pattern_producer()` in `streaming.cpp:204-294`
- Called from `moonlight.cpp:142-153` when session starts
- Runs in its own thread with dedicated GMainLoop
- Has `bus_sync_handler` for CUDA context sharing

**Lobby/Wayland Producer** (for apps with `start_virtual_compositor = true`):
```
waylanddisplaysrc name=wolf_wayland_source render_node=/dev/dri/renderD128
  → video/x-raw(memory:CUDAMemory), width={width}, height={height}, framerate={fps}/1
  → interpipesink name="{lobby_id}_video" sync=true async=false max-buffers=5
```
- Created by `start_video_producer()` in `streaming.cpp:99-150`
- Called from `moonlight.cpp:97-107` when wayland compositor is needed
- Also runs in its own thread with GMainLoop
- Has `bus_sync_handler` for CUDA context sharing

**Key Point:** Both producers output to `interpipesink` with a unique name. The consumer
pipeline dynamically switches which producer it reads from via `interpipesrc listen-to`.

#### 2. Consumer Pipeline (One Per Streaming Session)

```
interpipesrc name="interpipesrc_{session_id}_video" listen-to="{current_producer}_video"
  → cudaupload                      ← May pass-through if already CUDA memory
  → cudaconvertscale add-borders=true
  → video/x-raw(memory:CUDAMemory), width={width}, height={height}, format=NV12
  → nvh264enc                       ← THIS IS WHERE NV_ENC_ERR_RESOURCE_REGISTER_FAILED HAPPENS
  → h264parse
  → rtpmoonlightpay_video
  → appsink name="wolf_udp_sink"
```
- Created by `start_streaming_video()` in `streaming.cpp:415-608`
- Called when client starts video streaming (after RTP ping)
- The `listen-to` property is dynamically switched when joining a lobby

**Key Point:** The consumer pipeline's `nvh264enc` is the element that fails with
`NV_ENC_ERR_RESOURCE_REGISTER_FAILED (0x17)` when it receives buffers from a different
CUDA context than it was initialized with.

#### 3. The PAUSED vs PLAYING Question

When logs show:
```
[HANG_DEBUG] Video SwitchStreamProducerEvents: session 2 switching to lobby_video, pipeline state: PAUSED
```

The "pipeline state: PAUSED" refers to the **Consumer Pipeline** (the one with nvh264enc).
This is checked at `streaming.cpp:554` in the `switch_producer_handler`.

**GStreamer Pipeline States:**
1. `NULL` - Pipeline created but not initialized
2. `READY` - All elements allocated, ready to preroll
3. `PAUSED` - Data flows, clocks paused (prerolling/buffering)
4. `PLAYING` - Data flows with clock running

**Session 1 (works):**
- Consumer pipeline starts, reaches PAUSED, then PLAYING
- Switch to lobby happens when consumer is PLAYING
- nvh264enc has fully initialized with correct CUDA context
- Switch succeeds

**Session 2 (fails):**
- Consumer pipeline starts, in PAUSED state
- Switch to lobby happens while consumer is still PAUSED
- nvh264enc may not have fully bound to CUDA context yet
- Buffer from lobby arrives with different context reference → FAIL

### The Real Problem

The PAUSED/PLAYING timing is a symptom, not the root cause. The real problem is:

**Why does Session 2's test pattern have a different CUDA context than Session 1's lobby?**

Both should be using the SAME context from the shared `immer::atom<gst_context_ptr>`.

### Debug Logging Added

Added `[CUDA_CONTEXT_DEBUG]` logging to `need_context_handler()`:
```cpp
logs::log(logs::warning, "[CUDA_CONTEXT_DEBUG] need_context_handler called for type={}, device={}",
          context_type ? context_type : "unknown", ctx_data->device_path);

if (auto gst_context = ctx_data->gst_context->load().get()) {
    logs::log(logs::warning, "[CUDA_CONTEXT_DEBUG] Context already in atom (addr={}), passing to pipeline",
              static_cast<void*>(gst_context));
    gst_video_context::set_context(gst_context, msg);
} else if (auto video_context = gst_video_context::need_context_for_device(ctx_data->device_path, msg)) {
    logs::log(logs::warning, "[CUDA_CONTEXT_DEBUG] Created NEW context (addr={}), storing in atom",
              static_cast<void*>(video_context.get()));
    ctx_data->gst_context->store(video_context);
}
```

**Expected log output:**
```
# Session 1 test pattern starts
[CUDA_CONTEXT_DEBUG] need_context_handler called for type=gst.cuda.context, device=/dev/dri/renderD128
[CUDA_CONTEXT_DEBUG] Context already in atom (addr=0x5555abc), passing to pipeline
# OR
[CUDA_CONTEXT_DEBUG] Created NEW context (addr=0x5555abc), storing in atom

# Session 1 lobby starts
[CUDA_CONTEXT_DEBUG] need_context_handler called for type=gst.cuda.context, device=/dev/dri/renderD128
[CUDA_CONTEXT_DEBUG] Context already in atom (addr=0x5555abc), passing to pipeline

# Session 2 test pattern starts - SHOULD show same context addr!
[CUDA_CONTEXT_DEBUG] need_context_handler called for type=gst.cuda.context, device=/dev/dri/renderD128
[CUDA_CONTEXT_DEBUG] Context already in atom (addr=0x5555abc), passing to pipeline
```

If Session 2 shows "Created NEW context" with a DIFFERENT address, that confirms
the atom isn't being shared properly between pipelines.

### Possible Causes to Investigate

1. **Atom not shared**: Are all pipelines receiving the same `gst_context` atom pointer?
2. **Context destroyed**: Is the context being cleaned up when lobby stops?
3. **Race condition**: Is context creation happening before atom update completes?
4. **Wrong context type**: Is `need_context_for_device()` creating different context types?

### Next Steps

1. ~~Rebuild sandbox with debug logging~~
2. ~~Test two sessions~~
3. ~~Analyze `[CUDA_CONTEXT_DEBUG]` output to see actual context flow~~
4. ~~Identify where context diverges between sessions~~

## CUDA Context Sharing Works - NVENC Session is the Problem (2025-12-05)

### Debug Logging Results

Added `[CUDA_CONTEXT_DEBUG]` logging to `need_context_handler()` and tested two sessions:

```
# Session 1 test pattern starts
[CUDA_CONTEXT_DEBUG] need_context_handler called for type=gst.cuda.context, device=/dev/dri/renderD128
[CUDA_CONTEXT_DEBUG] Created NEW context (addr=0x74a840caf4b0), storing in atom

# Session 1 consumer pipeline
[CUDA_CONTEXT_DEBUG] need_context_handler called for type=gst.cuda.context, device=/dev/dri/renderD128
[CUDA_CONTEXT_DEBUG] Context already in atom (addr=0x74a840caf4b0), passing to pipeline

# Session 2 test pattern starts
[CUDA_CONTEXT_DEBUG] need_context_handler called for type=gst.cuda.context, device=/dev/dri/renderD128
[CUDA_CONTEXT_DEBUG] Context already in atom (addr=0x74a840caf4b0), passing to pipeline

# Session 2 consumer pipeline
[CUDA_CONTEXT_DEBUG] need_context_handler called for type=gst.cuda.context, device=/dev/dri/renderD128
[CUDA_CONTEXT_DEBUG] Context already in atom (addr=0x74a840caf4b0), passing to pipeline

# BUT Session 2's encoder STILL fails:
NvEnc API call failed: 0x17, NV_ENC_ERR_RESOURCE_REGISTER_FAILED ()
Failed to get resource, status NV_ENC_ERR_RESOURCE_REGISTER_FAILED (23)
Failed to upload frame
```

### Key Finding: CUDA Context IS Shared Correctly

**All pipelines use the SAME CUDA context (addr=0x74a840caf4b0)**:
- Session 1 test pattern: Created NEW context, stored in atom
- Session 1 consumer: Context already in atom, passed to pipeline
- Session 2 test pattern: Context already in atom, passed to pipeline
- Session 2 consumer: Context already in atom, passed to pipeline

**Yet `nvh264enc3` still fails with `NV_ENC_ERR_RESOURCE_REGISTER_FAILED`!**

This proves the CUDA context atom sharing is working correctly. The problem is NOT the CUDA context.

### Root Cause: NVENC Session Resource Registration

The timeline from logs:

1. **Lobby producer** (`f3c24a88...`) is created once, runs continuously
2. **Session 1** starts, switches to lobby at 12:17:25
3. **Session 1** leaves lobby at 12:17:27 (encoder destroyed)
4. **Session 2** starts with NEW nvh264enc instance at 12:17:28
5. **Session 2** switches to SAME lobby at 12:17:28.909
6. **nvh264enc3 fails** immediately when first lobby buffer arrives

**The problem:** NVENC has a per-encoder-session resource registration model:

1. Each `nvh264enc` instance creates its own NVENC encoding session
2. NVENC registers input CUDA buffers with that specific session via `nvEncRegisterResource`
3. When Session 1's encoder is destroyed, the CUDA buffers from the lobby may still have orphaned registrations
4. Session 2's NEW encoder instance tries to register the SAME buffers (from same lobby producer)
5. NVENC fails because those buffers have stale registrations from the dead session

### Why Test Pattern Works But Lobby Fails

| Producer Type | Buffer Ownership | Why It Works/Fails |
|--------------|------------------|-------------------|
| Test pattern | Per-session (unique interpipesink name) | Each session gets fresh buffers |
| Lobby | Shared (single waylanddisplaysrc) | Buffers were created/registered for previous encoder |

The test pattern creates a NEW producer per session (`{session_id}_video`), so each encoder gets fresh buffers.
The lobby uses a SINGLE shared producer (`{lobby_id}_video`), so buffers persist across encoder lifetimes.

### Potential Solutions (Not Yet Implemented)

1. **Force buffer pool renegotiation on switch**: When interpipesrc switches `listen-to`, trigger a caps renegotiation that forces new buffer allocation
2. **Per-session lobby producers**: Each session gets its own waylanddisplaysrc for the lobby (defeats multi-user purpose)
3. **Use nvh264enc extern-cuda-bufferpool**: Pass a shared buffer pool that handles registration properly
4. **Pipeline flush on switch**: Send FLUSH_START/FLUSH_STOP to clear cached buffers
5. **Fix in GStreamer nvh264enc**: Ensure proper unregistration of resources on encoder stop

### Current Workaround

Use **Select Agent (Wolf-UI)** app instead of **Blank** test pattern:
- Wolf-UI runs a real Wayland compositor in a container
- Each session spawns its own waylanddisplaysrc
- No buffer sharing issues because each session has independent producers

This is a workaround, not a fix. The test pattern approach would be more efficient for the "loading screen" use case.

## Why Select Agent Works But Blank Doesn't (2025-12-05)

### Revised Understanding: Buffer Pool Initialization

The earlier NVENC buffer registration theory is only part of the story. The key insight is:

**The consumer pipeline's `nvh264enc` initializes its buffer pool based on the FIRST source it reads from.**

### The Switch Problem

When interpipesrc switches from the initial app to the lobby, the encoder may not handle the transition cleanly:

```
Blank (test pattern):                    Select Agent (waylanddisplaysrc):

videotestsrc                             waylanddisplaysrc (Wolf-UI)
    ↓                                        ↓
cudaupload ← (creates buffers)           (creates buffers directly)
    ↓                                        ↓
interpipesink                            interpipesink
    ↓                                        ↓
    ╔═══════════════════════════════════════════════════╗
    ║  interpipesrc (in consumer)                        ║
    ║     ↓                                              ║
    ║  nvh264enc ← (initializes pool from FIRST source) ║
    ╚═══════════════════════════════════════════════════╝
    ↓                                        ↓
Lobby switch happens                     Lobby switch happens
    ↓                                        ↓
waylanddisplaysrc (lobby)               waylanddisplaysrc (lobby)
```

**With Blank:**
- nvh264enc initializes with buffers from `cudaupload`
- Then switches to buffers from `waylanddisplaysrc` (lobby)
- **Mismatched buffer pool characteristics** → NVENC registration issues

**With Select Agent:**
- nvh264enc initializes with buffers from `waylanddisplaysrc` (Wolf-UI)
- Then switches to buffers from `waylanddisplaysrc` (lobby)
- **Same buffer pool characteristics** → works fine

The issue isn't that buffers are "shared" - it's that `cudaupload` creates buffers with different internal properties than `waylanddisplaysrc`, and nvh264enc can't handle the transition cleanly.

### Why Buffer Pools Differ

Even though we matched the VIDEO CAPS (resolution, framerate, memory type), the underlying buffer pools may differ in:

1. **Buffer pool allocator** - cudaupload vs waylanddisplaysrc use different allocators
2. **Internal buffer flags** - CUDA memory allocation parameters
3. **Buffer metadata** - GstMeta attached to buffers
4. **Pool configuration** - min/max buffers, alignment requirements

NVENC's `nvEncRegisterResource` may fail when it encounters buffers allocated with different characteristics than what it was initialized with.

### Potential Fixes (Future Work)

1. **Use a minimal waylanddisplaysrc for test pattern** - Run a trivial Wayland compositor that just displays a solid color, ensuring all sources use the same buffer allocation path

2. **Use CUDA-native test source** - Find or create a GStreamer element that generates test patterns directly in CUDA memory using the same allocator as waylanddisplaysrc

3. **Force buffer pool renegotiation on switch** - Make interpipesrc flush and renegotiate its buffer pool when switching `listen-to` sources

4. **Investigate GStreamer nvh264enc** - Determine if this is a bug in nvh264enc's handling of buffer pool transitions
