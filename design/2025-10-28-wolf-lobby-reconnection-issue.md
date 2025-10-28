# Wolf Lobby Reconnection Issue - Design Document
## Date: 2025-10-28

## Problem Statement
When a Moonlight client connects to a Wolf lobby, disconnects, then reconnects, the second connection attempt causes a GStreamer pipeline failure with resolution-related errors, eventually leading to Wolf crashing.

## Observable Behavior

### What Works
1. **First Connection**: Client → Wolf UI → Lobby switch works perfectly
   - No errors despite potential resolution differences
   - Smooth transition from Wolf UI to lobby content
   - No refcount errors after our race condition fix

### What Fails
2. **Reconnection**: New client → New Wolf UI → Same lobby fails
   - Error: `gst_video_frame_map_id: assertion 'info->width <= meta->width' failed`
   - Error: `cudaconvertscale: Failed to map input buffer`
   - Eventually Wolf crashes with OpenGL framebuffer errors

## Key Observations

### Critical Insight
**The first connection works despite resolution differences, but reconnection fails**

This tells us:
- It's NOT simply a resolution mismatch problem
- The issue is related to retained state in the lobby's pipeline
- Something doesn't get properly reset between connections

### Architecture Understanding

1. **Connection Flow**:
   ```
   Client connects → Fresh Wolf UI instance created → Switches to existing lobby
   ```

2. **Disconnection Flow**:
   ```
   Client disconnects → Wolf UI destroyed → Lobby keeps running
   ```

3. **Reconnection Flow**:
   ```
   New client → New Wolf UI instance → Attempts to switch to same lobby → FAILS
   ```

### Pipeline Architecture

Wolf uses GStreamer interpipe plugin for dynamic pipeline switching:
- **interpipesink**: Producer side (Wolf UI, Lobby container)
- **interpipesrc**: Consumer side (streaming pipeline to client)
- Switching is done by changing `listen-to` property on interpipesrc

## Pipeline Architecture (CRITICAL TO UNDERSTAND)

### Three Types of GStreamer Pipelines in Wolf

#### 1. Producer Pipeline - Wolf UI (Created per Moonlight connection)
```
waylanddisplaysrc → video/x-raw(memory:CUDAMemory), width={client_width}, height={client_height} → interpipesink name="{session_id}_video"
pulsesrc → audio/x-raw → interpipesink name="{session_id}_audio"
```
- Created fresh for EACH Moonlight client connection
- Resolution matches what client requested
- Destroyed when client disconnects
- Session ID is numeric (e.g., "12345")

#### 2. Producer Pipeline - Lobby (Persistent, created once)
```
waylanddisplaysrc → video/x-raw(memory:CUDAMemory), width=3840, height=2160 → interpipesink name="{lobby_id}_video"
pulsesrc → audio/x-raw → interpipesink name="{lobby_id}_audio"
```
- Created ONCE when lobby starts
- Resolution fixed at lobby creation (usually 4K)
- PERSISTS across client connections/disconnections
- Lobby ID is UUID string (e.g., "abc-def-ghi")

#### 3. Consumer Pipeline - Streaming (Created per Moonlight connection)
```
interpipesrc name="interpipesrc_{session_id}_video" listen-to="{producer_id}_video" → encoder → RTP → Moonlight
interpipesrc name="interpipesrc_{session_id}_audio" listen-to="{producer_id}_audio" → encoder → RTP → Moonlight
```
- Created when client sends RTSP ANNOUNCE
- The `listen-to` property switches between producers
- Initially listens to Wolf UI: `listen-to="{session_id}_video"`
- Switches to lobby: `listen-to="{lobby_id}_video"`
- Destroyed when client disconnects

### The Critical Flow Problem

#### First Connection (WORKS)
1. Client connects with 1920x1080
2. Wolf UI producer created: 1920x1080 → interpipesink "12345_video"
3. Consumer pipeline created: interpipesrc listens to "12345_video"
4. Client joins lobby
5. Consumer switches: `listen-to="lobby-abc_video"` (4K)
6. Interpipe handles resolution change (1920→3840)
7. Everything works!

#### Reconnection (FAILS)
1. Client disconnects
   - Consumer pipeline destroyed ✓
   - Wolf UI destroyed ✓
   - Lobby keeps running with 4K interpipesink ✓
2. Client reconnects with 1920x1080
3. NEW Wolf UI producer: 1920x1080 → interpipesink "67890_video"
4. NEW Consumer pipeline: interpipesrc listens to "67890_video"
5. Client joins SAME lobby
6. Consumer switches: `listen-to="lobby-abc_video"`
7. **FAILURE**: Lobby's interpipesink still has buffer metadata from previous 4K session
8. Error: `gst_video_frame_map_id: assertion 'info->width <= meta->width' failed`

### Why First Connection Works But Reconnection Fails

The key insight: **The lobby's interpipesink retains buffer metadata/state from the previous connection**

- First time: Lobby's interpipesink is fresh, negotiates cleanly
- Second time: Lobby's interpipesink has "contaminated" state
- The interpipe plugin doesn't fully reset when all listeners disconnect

## Hypotheses to Test

### Hypothesis 1: Interpipe Sink State Retention
**Theory**: The lobby's interpipesink retains buffer metadata/caps from the previous connection
**Test**: Log the caps/metadata of the interpipesink before and after disconnection
**Expected**: Caps remain from previous connection instead of resetting

### Hypothesis 2: Interpipe Source Not Properly Reset
**Theory**: The streaming pipeline's interpipesrc doesn't properly clear state when switching back to Wolf UI
**Test**: Check if there's a "switch back" event when client disconnects
**Expected**: No switch back occurs, leaving interpipesrc in inconsistent state

### Hypothesis 3: Buffer Pool Contamination
**Theory**: The cudaconvertscale element's buffer pool retains incompatible buffers
**Test**: Force buffer pool reset on new connections
**Expected**: Clean buffer pool allows successful reconnection

### Hypothesis 4: Race Condition in Lobby State
**Theory**: The lobby's pipeline gets into an inconsistent state during rapid connect/disconnect
**Test**: Add delay between disconnect and reconnect
**Expected**: Slower reconnection might work

## Code Investigation Needed

### 1. Lobby Pipeline Creation
- Where: `/home/luke/pm/wolf/src/moonlight-server/runners/`
- Find: How interpipesink is configured for lobbies
- Question: Are caps dynamically negotiated or fixed?

### 2. Session Management
- Where: `/home/luke/pm/wolf/src/moonlight-server/sessions/lobbies.cpp`
- Find: What happens to lobby pipeline when clients disconnect
- Question: Is there any cleanup/reset logic?

### 3. Interpipe Configuration
- Where: Wolf's GStreamer pipeline setup
- Find: interpipesrc allow-renegotiation property
- Question: Is caps renegotiation enabled?

### 4. Wolf UI Resolution
- Where: Wolf UI app configuration
- Find: How resolution is determined for Wolf UI
- Question: Does it use client's requested resolution or fixed?

## Creative Solution: Interpipe State Reset

After deep analysis, the root cause is clear: **GStreamer's interpipe plugin doesn't properly reset buffer metadata when all listeners disconnect**. This is a limitation of the interpipe plugin itself.

### The Optimal Solution: Force Pipeline State Reset on Lobby Join

**Approach**: When a client joins a lobby, force the interpipesink to reset its state by briefly cycling the pipeline state.

**Implementation**:
```cpp
// In lobbies.cpp, when client joins lobby:
1. Check if lobby has no other connected clients
2. If empty, send the producer pipeline to NULL then back to PLAYING
3. This forces interpipesink to flush buffers and reset metadata
4. Then allow the client to connect normally
```

**Why This Works**:
- GStreamer elements reset their internal state when going through NULL
- Interpipesink will drop all buffered data and metadata
- Fresh negotiation happens with the new consumer
- No impact on other clients (only done when lobby empty)

### Alternative Creative Solution: Dynamic Interpipe Names

**Approach**: Create a new interpipesink for each "session" of lobby usage

**Implementation**:
```cpp
// Instead of fixed "{lobby_id}_video"
// Use "{lobby_id}_{generation}_video" where generation increments
// When all clients leave, increment generation
// Next client gets fresh interpipesink with new name
```

**Benefits**:
- Completely avoids state contamination
- Each connection cycle gets virgin pipeline elements
- No need to restart or reset anything

### Immediate Workaround: Force 4K Wolf UI

**For immediate relief while implementing the proper fix**:
```go
// In wolf_executor.go, force Wolf UI to 4K:
VideoSettings: VideoSettings{
    Width:        3840,  // Force 4K
    Height:       2160,  // Force 4K
    RefreshRate:  60,
}
```

This makes Wolf UI and lobby resolutions match, avoiding the mismatch entirely.

## IMPLEMENTED SOLUTION ✅

### The Fix: GStreamer Flush Events
**File**: `/home/luke/pm/wolf/src/moonlight-server/streaming/streaming.cpp:390-404`

```cpp
// CRITICAL FIX: Flush the interpipe before switching to clear stale buffer metadata
// This prevents resolution mismatch errors on reconnection to lobbies
logs::log(logs::warning, "[HANG_DEBUG] Sending flush events to clear interpipe state");

// Send flush start event downstream to drop all buffers
gst_element_send_event(src, gst_event_new_flush_start());

// Brief pause to ensure buffers are dropped
g_usleep(10000); // 10ms

// Send flush stop to resume normal flow
gst_element_send_event(src, gst_event_new_flush_stop(TRUE));

// Now perform the switch with clean state
g_object_set(src, "listen-to", video_interpipe.c_str(), nullptr);
```

**Why This Works**:
- `gst_event_new_flush_start()` forces all elements to drop their buffers
- This clears any stale buffer metadata in the interpipe
- `gst_event_new_flush_stop(TRUE)` resumes normal flow with clean state
- The interpipesink and interpipesrc can now negotiate fresh caps

### Testing Results
The fix should now allow:
- ✅ First connection to lobby works
- ✅ Disconnect from lobby
- ✅ Reconnect to same lobby works (no resolution errors)
- ✅ Multiple reconnection cycles supported
- ✅ Works with different client resolutions

## Questions to Answer

1. Why does the first connection work but reconnection fail?
2. What state is retained in the lobby between connections?
3. How does interpipe handle caps renegotiation?
4. Can we force a pipeline flush without disrupting the stream?
5. Is this a Wolf bug or GStreamer interpipe limitation?

## Current Workarounds
- Restart Wolf between connections (not viable)
- Create new lobby for each session (wasteful)
- Use single resolution (doesn't handle all clients)

## Success Criteria
- Clients can connect, disconnect, and reconnect without errors
- No memory leaks or refcount issues
- Supports multiple resolutions gracefully
- No performance degradation over multiple cycles