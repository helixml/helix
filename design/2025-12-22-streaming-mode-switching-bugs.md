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

## Conclusion

The core issues stem from `qualityMode` not being properly scoped to `websocket` streaming mode. The fixes ensure:
1. Quality mode state is reset when switching streaming protocols
2. Resources (SSE, screenshots) are properly cleaned up
3. User gets visual feedback during mode transitions
4. State guards prevent cross-mode pollution
