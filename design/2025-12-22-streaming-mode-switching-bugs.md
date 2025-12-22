# Streaming Mode Switching Bugs Analysis

**Date:** 2025-12-22
**Component:** MoonlightStreamViewer.tsx, WebSocketStream

## Overview

User reported several issues with switching between streaming modes:
1. Switching from screenshot mode to WebSocket mode showed a blank screen (no video)
2. When viewing the same desktop in two tabs, issues occurred
3. In WebRTC/WebSocket mode, stats showed screenshots were still being fetched

This document analyzes the root causes and fixes for these issues.

## Architecture Context

MoonlightStreamViewer has two orthogonal mode dimensions:

1. **Streaming Mode** (`streamingMode`): Controls the transport protocol
   - `websocket`: WebSocket-based streaming (L7 HTTP compatible)
   - `webrtc`: WebRTC streaming (requires UDP)

2. **Quality Mode** (`qualityMode`): Controls video delivery within WebSocket mode
   - `high`: Video frames over WebSocket
   - `sse`: Video frames over SSE (Server-Sent Events)
   - `low`: Screenshot polling (HTTP GET images)

Key insight: `qualityMode` only applies when `streamingMode === 'websocket'`, but this isn't enforced in all code paths.

## Bug 1: Screenshot Polling Continues in WebRTC Mode

**Symptom:** User sees screenshots being fetched in stats while in WebRTC mode.

**Location:** `MoonlightStreamViewer.tsx:1587` and effect at line `1611`

**Root Cause:**
```typescript
// Line 1587 - Missing streamingMode check
const shouldPollScreenshots = qualityMode === 'low';

// Line 1611 - Effect only checks shouldPollScreenshots
useEffect(() => {
  if (!shouldPollScreenshots || !isConnected || !sessionId) {
    // cleanup...
    return;
  }
  // Start polling...
}, [shouldPollScreenshots, isConnected, sessionId]);
```

When switching from `websocket` + `low` to `webrtc`, the `qualityMode` stays `'low'` because there's no code to reset it. Since `shouldPollScreenshots` only checks `qualityMode`, screenshot polling continues.

**Fix:** Add `streamingMode === 'websocket'` check to `shouldPollScreenshots`.

## Bug 2: qualityMode Persists Across Streaming Mode Changes

**Symptom:** State bleeds across protocol switches.

**Location:** `MoonlightStreamViewer.tsx:859-873` (streaming mode effect)

**Root Cause:** When switching streaming modes, the effect triggers a reconnect but doesn't reset `qualityMode`:

```typescript
useEffect(() => {
  if (previousStreamingModeRef.current === streamingMode) return;
  // ...
  reconnectRef.current(1000, `Switching to ${modeLabel}...`);
  // qualityMode is NOT reset!
}, [streamingMode, sessionId]);
```

This causes:
- SSE EventSource may remain open briefly
- Screenshot polling continues in WebRTC mode
- Video control state is inconsistent

**Fix:** Reset `qualityMode` to `'high'` when switching streaming modes. Also add explicit cleanup of SSE resources.

## Bug 3: Blank Screen When Switching from 'low' to 'high' Mode

**Symptom:** User sees blank black screen after switching from screenshot mode to WebSocket video.

**Location:** Hot-switch effect at `MoonlightStreamViewer.tsx:883-1159`

**Root Cause:**
1. User is in `'low'` mode (screenshot polling active)
2. `isConnecting = false` (was set false after first screenshot loaded)
3. User switches to `'high'` mode
4. Hot-switch effect:
   - Calls `wsStream.setVideoEnabled(true)` - resets keyframe flag
   - Sets canvas: `wsStream.setCanvas(canvasRef.current)`
5. WS canvas becomes visible (`opacity: 1`)
6. BUT canvas is blank - first keyframe hasn't arrived yet!
7. No loading indicator since `isConnecting` is already false
8. User sees blank screen until keyframe arrives (can take 1-2 seconds)

**Fix:** Show a transient loading state while waiting for first video frame after mode switch.

## Bug 4: Missing Transition Loading State

**Symptom:** No feedback during mode switches.

**Location:** Various mode switching effects

**Root Cause:** The `isConnecting` state only controls the overlay during initial connection. After the first frame (screenshot or video), `isConnecting` is set to false and stays false during subsequent mode switches.

**Fix:** Add a separate `isTransitioning` state for mode switch transitions, or reset `isConnecting` during mode switches when switching TO a video mode.

## Bug 5: Multiple Instances Streaming Same Session

**Symptom:** Issues when opening two tabs viewing the same desktop.

**Root Cause:** Each tab creates its own:
- WebSocketStream with unique `componentInstanceIdRef`
- Separate WebSocket connections
- Separate screenshot polling

The server may not handle multiple concurrent video streams to the same session well, or the encoder may conflict.

**Note:** This is likely a server-side issue, not a frontend bug. The frontend correctly uses unique instance IDs.

## Fixes Applied

### Fix 1: Guard Screenshot Polling with Streaming Mode

```typescript
// Before
const shouldPollScreenshots = qualityMode === 'low';

// After
const shouldPollScreenshots = qualityMode === 'low' && streamingMode === 'websocket';
```

### Fix 2: Reset qualityMode on Streaming Mode Change

```typescript
useEffect(() => {
  if (previousStreamingModeRef.current === streamingMode) return;

  const prevMode = previousStreamingModeRef.current;
  const newMode = streamingMode;
  previousStreamingModeRef.current = newMode;

  // NEW: Reset quality mode to default when switching streaming modes
  // Prevents state bleeding (e.g., screenshot polling in WebRTC mode)
  if (qualityMode !== 'high') {
    console.log('[MoonlightStreamViewer] Resetting qualityMode to high for streaming mode switch');
    setQualityMode('high');
    previousQualityModeRef.current = 'high';
  }

  // NEW: Explicitly clean up SSE resources before reconnecting
  if (sseEventSourceRef.current) {
    sseEventSourceRef.current.close();
    sseEventSourceRef.current = null;
  }
  // ...rest of effect
}, [streamingMode, sessionId, qualityMode]);
```

### Fix 3: Show Loading State During Mode Transitions

Add transition state management:
```typescript
// When switching TO 'high' mode from 'low' or 'sse', show loading overlay
// until first video frame arrives (via videoStarted event)
if (newMode === 'high') {
  console.log('[MoonlightStreamViewer] Enabling WS video for high mode');
  setIsConnecting(true);  // Show overlay while waiting for first frame
  setStatus('Switching to video stream...');
  wsStream.setVideoEnabled(true);
  if (canvasRef.current) {
    wsStream.setCanvas(canvasRef.current);
  }
}
```

### Fix 4: Properly Gate Video Control Effect

Ensure the video control effect only runs for websocket mode:
```typescript
useEffect(() => {
  const stream = streamRef.current;
  if (!stream || !(stream instanceof WebSocketStream) || !isConnected) {
    return;
  }

  // ADDED: Only apply quality mode changes in websocket streaming mode
  if (streamingMode !== 'websocket') {
    return;
  }

  // ...rest of effect
}, [qualityMode, isConnected, streamingMode]);
```

## Testing Checklist

1. **Screenshot → WebSocket Video Switch:**
   - [ ] Shows loading overlay during transition
   - [ ] Video appears when first frame arrives
   - [ ] Screenshot polling stops immediately
   - [ ] Stats show no screenshot fetches

2. **WebSocket → WebRTC Switch:**
   - [ ] qualityMode resets to 'high'
   - [ ] No screenshot polling in WebRTC mode
   - [ ] SSE EventSource is closed
   - [ ] WebRTC video starts correctly

3. **SSE → High Quality Switch:**
   - [ ] SSE EventSource closes
   - [ ] WS video enables
   - [ ] Canvas shows video

4. **Multiple Tabs:**
   - [ ] Each tab gets independent stream
   - [ ] Closing one tab doesn't affect other
   - [ ] Stats are tab-specific

## Additional Hardening: Preventing Duplicate Streams

A second round of fixes addressed the root cause of "out-of-order frames" - duplicate streams writing to the same canvas.

### Fix 5: Prevent Rendering After WebSocketStream.close()

Added `this.closed` check in `renderVideoFrame()`:
```typescript
private renderVideoFrame(frame: VideoFrame) {
  // CRITICAL: Prevent rendering after stream is closed
  if (this.closed) {
    frame.close()
    return
  }
  // ... rest of rendering
}
```

### Fix 6: Clear Canvas References in WebSocketStream.close()

Clear canvas references FIRST in `close()` before any async cleanup:
```typescript
close() {
  this.closed = true
  // CRITICAL: Clear canvas references FIRST
  this.canvas = null
  this.canvasCtx = null
  // ... rest of cleanup
}
```

### Fix 7: Stream Cleanup at Start of connect()

Added belt-and-suspenders cleanup at the START of `connect()`:
```typescript
const connect = useCallback(async () => {
  // CRITICAL: Close any existing stream FIRST
  if (streamRef.current) {
    streamRef.current.close();
    streamRef.current = null;
  }
  // Also clean up SSE resources...
  // ... rest of connect
})
```

### Fix 8: SSE Decoder Output Guards

Added decoder identity check to SSE video decoder output callbacks:
```typescript
const decoder = new VideoDecoder({
  output: (frame: VideoFrame) => {
    // CRITICAL: Check if this decoder is still the active one
    if (sseVideoDecoderRef.current !== decoder) {
      frame.close();
      return;
    }
    // ... render frame
  }
})
```

## Root Cause: Duplicate Stream Race Conditions

The "out-of-order frames" issue was caused by:
1. Old stream's decoder having frames queued
2. New stream created and starts rendering
3. Old decoder's output callback fires and renders to same canvas
4. Frames from two different streams interleaved on same canvas

This could happen during:
- Reconnections
- Quality mode switches
- Streaming mode switches (WebSocket ↔ WebRTC)

The fixes ensure that:
1. Old streams are always closed before new ones are created
2. Closed streams cannot render to the canvas (double-checked via `this.closed` AND cleared canvas refs)
3. SSE decoders verify they're still the active decoder before rendering

## Connection Registry Debug Tool

Added a debug registry to track all active streaming connections. This helps developers verify that connections are being properly created and destroyed.

### Connection Types Tracked

| Type | Description |
|------|-------------|
| `websocket-stream` | WebSocket connection for input and signaling |
| `websocket-video-enabled` | WebSocket video stream is active |
| `sse-video` | SSE EventSource for video frames |
| `screenshot-polling` | HTTP polling for screenshots |
| `webrtc-stream` | WebRTC peer connection |

### Valid Connection Combinations

- `[websocket-stream, websocket-video-enabled]` - WebSocket high quality mode
- `[websocket-stream, sse-video]` - WebSocket + SSE video mode
- `[websocket-stream, screenshot-polling]` - WebSocket + screenshot mode
- `[webrtc-stream]` - WebRTC mode

### Registration Points

1. **websocket-stream**: `connectionComplete` event in info listener
2. **websocket-video-enabled**: `videoStarted` event in info listener
3. **sse-video**: First keyframe received in SSE video handler
4. **screenshot-polling**: Screenshot polling effect start
5. **webrtc-stream**: `onCanPlay` event on video element

### Unregistration Points

1. **disconnect()**: Calls `clearAllConnections()` to clear all registrations
2. **connect()**: Calls `clearAllConnections()` at start (belt-and-suspenders)
3. **Hot-switch teardown**: Unregisters old video source before enabling new one
4. **SSE stop event**: Unregisters SSE video connection
5. **SSE decoder error**: Unregisters SSE video connection before reconnect
6. **Screenshot polling cleanup**: Effect cleanup unregisters connection

### Validation

The `validateConnectionState()` function checks for invalid combinations:
- Both WebSocket and WebRTC streams active simultaneously
- Multiple video sources active simultaneously
- Video source without transport layer

Invalid states are logged to console with `[StreamRegistry] INVALID:` prefix.

### Display

The registry is visible in "Stats for Nerds" panel with:
- List of active connection types
- Warning indicator (⚠️ TOO MANY!) if more than 2 connections active

## Mode Transition Analysis

Comprehensive review of all possible mode switching paths:

### Quality Mode Transitions (within WebSocket streaming)

| Transition | Teardown | Setup | Loading Overlay |
|------------|----------|-------|-----------------|
| high → sse | Disable WS video, unregister | Open SSE EventSource | ✓ Shows "Switching to SSE stream..." |
| high → low | Disable WS video, unregister | Enable screenshot polling | ✓ Shows "Switching to screenshots..." |
| sse → high | Close SSE EventSource/decoder, unregister | Enable WS video | ✓ Shows "Switching to video stream..." |
| sse → low | Close SSE EventSource/decoder, unregister | Enable screenshot polling | ✓ Shows "Switching to screenshots..." |
| low → high | Effect cleanup stops polling | Enable WS video | ✓ Shows "Switching to video stream..." |
| low → sse | Effect cleanup stops polling | Open SSE EventSource | ✓ Shows "Switching to SSE stream..." |

### Streaming Mode Transitions

| Transition | Actions |
|------------|---------|
| websocket → webrtc | Reset qualityMode to 'high', close SSE resources, unregister SSE, full reconnect |
| webrtc → websocket | Full reconnect (qualityMode already 'high') |

### Bugs Fixed in This Review

1. **Missing loading overlay for low mode transitions**
   - When switching TO 'low' mode, canvas becomes transparent immediately
   - Screenshot takes time to arrive → user saw black screen
   - Fix: Added `setIsConnecting(true)` in 'low' mode setup

2. **SSE registry unregistration missing in streaming mode switch**
   - When switching streaming modes while in SSE quality mode
   - SSE resources were closed but connection not unregistered
   - Fix: Added `unregisterConnection(currentSseVideoIdRef.current)` after SSE cleanup

## Conclusion

The core issues stem from `qualityMode` not being properly scoped to `websocket` streaming mode. The fixes ensure:
1. Quality mode state is reset when switching streaming protocols
2. Resources (SSE, screenshots) are properly cleaned up
3. User gets visual feedback during mode transitions
4. State guards prevent cross-mode pollution
5. **Duplicate streams are prevented by closing old resources before creating new ones**
6. **Rendering is blocked after close() is called via multiple guard checks**
7. **Connection registry provides visibility into active connections for debugging**
8. **All mode transitions show loading overlays to prevent black screen gaps**
