# Frontend Video Streaming Reconnect Loop Investigation

**Date:** 2026-04-12
**Status:** Investigation in progress
**Symptoms:** Video stream constantly reconnects during normal user interaction. Reported on iPad Safari, Chrome desktop, and other browsers. User is NOT changing settings — just watching/interacting with the desktop.

## Architecture Overview

Two layers of reconnection logic exist:

1. **WebSocketStream class** (`websocket-stream.ts`): Has its own internal reconnect with exponential backoff (1s, 2s, 4s... capped at 30s, max 10 attempts). Triggered by `onClose` event. Prevented by `this.closed = true` flag.

2. **DesktopStreamViewer component** (`DesktopStreamViewer.tsx`): Has a component-level `reconnect()` function that calls `disconnect(true)` → `setTimeout(connect())`. This creates an entirely NEW `WebSocketStream` instance (not reusing the old one).

The component also has multiple health checks that independently trigger reconnects:
- Frame health check (every 3s, 5s stall threshold)
- Visibility change handler (checks WS state when page becomes visible)
- WebSocketStream heartbeat (every 5s, 10s stall threshold)

## Hypotheses

### Hypothesis 1: Dual staleness detection causes reconnect fight

**Theory:** WebSocketStream's internal heartbeat (10s timeout) and the component's frame health check (5s timeout, 3s interval) both detect "staleness" independently. The component's check is MORE AGGRESSIVE (5s vs 10s). When the component detects staleness, it calls `reconnect()` which calls `disconnect(true)` → `stream.close()` → creates new `WebSocketStream`. But meanwhile, the OLD stream's `onClose` fires and dispatches a "disconnected" event. Even though the component checks `if (stream !== streamRef.current)`, there's a window where both are active.

**Test against code:**
- Component health check at line 1997: fires if `timeSinceWsData > FRAME_STALL_THRESHOLD_MS` (5000ms)
- WebSocketStream heartbeat at line 2135: fires if `elapsed > heartbeatTimeout` (10000ms)
- Component check runs every 3s. WebSocketStream check runs every 5s.

**Scenario:**
1. Network briefly stalls for 5 seconds (common on mobile networks)
2. At t=5s: Component health check fires → calls `reconnect(500ms)`
3. `reconnect()` calls `disconnect(true)` → sets `isExplicitlyClosingRef = true`, calls `stream.close()`
4. `stream.close()` sets `this.closed = true`, calls `ws.close()`
5. `onClose` fires → `this.closed` is true → dispatches "reconnectAborted"
6. Component receives "reconnectAborted" → checks `isExplicitlyClosingRef` → shows "Disconnected"
7. At t=5.5s: `connectRef.current()` fires from the setTimeout → creates new WebSocketStream
8. New WebSocketStream connects → but now `isExplicitlyClosingRef` is STILL TRUE from step 3
9. If the new stream encounters ANY issue, the "disconnected" handler at line 911 sees `isExplicitlyClosingRef = true` → shows "Disconnected" instead of "Reconnecting..."
10. User sees "Disconnected" overlay → stream appears broken

**Wait — `connect()` at line 629 resets `isExplicitlyClosingRef.current = false`.** So this particular race is handled.

**Verdict: Partially valid.** The dual detection doesn't directly cause a fight because the component's `disconnect(true)` properly closes the old stream. But the 5s component threshold is more aggressive than the 10s WebSocketStream threshold, meaning the component intervenes before WebSocketStream gets a chance to handle it. This is redundant but not the root cause.

### Hypothesis 2: VIDEO_START_TIMEOUT causes reconnect loop when GStreamer pipeline is slow

**Theory:** After `connectionComplete` event (line 782), a 15-second timeout starts (line 804). If the first video frame doesn't arrive within 15s, the component shows an error and sets `isConnected = false`. But it does NOT disconnect — it just shows an error. Meanwhile, the WebSocketStream is still connected and streaming (just no video frames yet because the GStreamer pipeline is starting). The component's frame health check then kicks in, sees `isConnected = false` (from the timeout handler), and returns early (line 1942 guard). So the timeout causes a permanent "error" state.

**Test against code:**
- Line 804-814: `videoStartTimeoutRef.current = setTimeout(() => { setError(...); setIsConnecting(false); setIsConnected(false); }, VIDEO_START_TIMEOUT_MS)`
- Line 840-843: The timeout is cleared when "videoStarted" event fires.

**But:** The timeout sets `setIsConnected(false)` but doesn't call `disconnect()`. The WebSocketStream keeps running, the stream keeps its heartbeat, and eventually frames may arrive. The component would be in an inconsistent state: error shown, but stream still connected internally.

**Verdict: Not the reconnect loop cause.** This would cause a stuck error state, not a reconnect loop.

### Hypothesis 3: connect() useCallback dependency on `account` causes reconnect on auth refresh

**Theory:** The `connect` function's dependency array (approximately line 1171) includes `account`. If the auth context refreshes (token renewal, session check), `account` changes → `connect` recreated → `connectRef.current` updated → but nothing directly calls connect again (the auto-connect effect at line 1847 has `hasConnectedRef` guard). However, `connect` changing causes `reconnect` to be recreated (since `reconnect` depends on `disconnect` which depends on `clearAllConnections`). This cascades to all effects that depend on `reconnect`.

**Test against code:**
- `connect` depends on: `[sessionId, hostId, appId, width, height, onConnectionChange, onError, helixApi, account, sandboxId, onClientIdCalculated, qualityMode, userBitrate]`
- `disconnect` depends on: `[clearAllConnections]`
- `reconnect` depends on: `[disconnect]`
- After my fix: visibility and health check effects no longer depend on `reconnect`

**Verdict: Not a root cause for reconnect loops** (the auto-connect guard prevents re-connection). But `account` in connect's deps is suspicious — any token refresh would recreate the function.

### Hypothesis 4: The `connect()` function closes the existing stream before opening a new one, and the close event races with the new connection

**Theory:** `connect()` at line 597-619 closes any existing stream FIRST. This calls `streamRef.current.close()` which sets `this.closed = true` on the WebSocketStream. Then it sets `streamRef.current = null`. Then at line 743, it creates a NEW WebSocketStream which calls `this.connect()` in its constructor. 

The OLD stream's `close()` triggers `ws.close()` asynchronously. The `onClose` event fires later and dispatches "disconnected" and "reconnectAborted" events. But by then, `streamRef.current` points to the NEW stream. The event listener was registered on the OLD stream at line 772 (`stream.addInfoListener(...)`). This listener closure captures `stream` (the OLD stream).

**At line 898:** `if (stream !== streamRef.current) { return; }` — this guard CORRECTLY filters out old stream events!

**Verdict: Not the root cause.** The stale stream event guard works correctly.

### Hypothesis 5: WebSocketStream heartbeat falsely detects stale on iPad when event loop is blocked

**Theory:** On iPad, heavy rendering (canvas compositing, WebCodecs decoding) can block the event loop. The heartbeat check runs every 5s but `lastMessageTime` is only updated in `onMessage()` which runs on the main thread. If the event loop is blocked for >10s (e.g., during a complex page interaction like scrolling, or a GC pause), the heartbeat check fires late and sees a stale `lastMessageTime`. It then closes the WebSocket, triggering reconnection.

**Test against code:**
- Line 2124-2148: Heartbeat interval checks `elapsed > this.heartbeatTimeout` (10000ms)
- Line 567: `this.lastMessageTime = Date.now()` — updated on every message
- Line 2128-2130: `if (!this.pageVisible) return` — skips check when page hidden

**But:** On iPad, the page might be "visible" but the JS event loop could still be heavily blocked by:
- Scroll animations
- Large DOM updates
- WebCodecs decoder processing
- GPU compositing delays

A 10-second event loop block is extreme but not impossible on constrained devices.

**More realistic scenario:** The heartbeat check fires at t=0, everything fine. At t=5, the next check is scheduled. But the event loop is busy processing touch events + video frames. The check actually fires at t=8. `elapsed = 8000 - lastMessageTime`. If `lastMessageTime` was set at t=3 (last message), `elapsed = 5000ms` — fine. But if messages ALSO stopped arriving (because the WebSocket's receive buffer is full while JS is blocked), then `elapsed` could exceed 10s.

**Verdict: Plausible but requires very heavy event loop blocking.** The 10s threshold is generous. More likely a contributing factor than root cause.

### Hypothesis 6: CRITICAL — `connect()` is called while WebSocketStream's internal reconnect is pending

**Theory:** This is the most likely cause of the reconnect loop:

1. WebSocket drops (transient network issue, server-side timeout, etc.)
2. WebSocketStream's `onClose` fires → schedules internal reconnect with backoff (e.g., 1s delay)
3. WebSocketStream dispatches "disconnected" event → component receives it
4. Component's "disconnected" handler (line 918-927) sets `isConnecting=true`, shows "Reconnecting..."
5. 3 seconds later, component's frame health check fires (line 1946)
6. It checks `ws.readyState` — but the WebSocket is CLOSED (the old one), and the internal reconnect hasn't fired yet
7. Frame health check calls `reconnectRef.current(500ms)` (line 1966)
8. `reconnect()` calls `disconnect(true)` → `stream.close()` → sets `this.closed = true`
9. **THIS KILLS THE PENDING INTERNAL RECONNECT** — because `close()` at line 2251-2253 clears `reconnectTimeoutId`
10. `reconnect()` schedules `connect()` in 500ms
11. `connect()` creates a BRAND NEW WebSocketStream
12. But the server might not be ready / still thinks the old session is active → "AlreadyStreaming" or immediate disconnect
13. Cycle repeats

**The key issue:** The component's frame health check (3s interval, 5s threshold) is MORE AGGRESSIVE than WebSocketStream's internal backoff (1s, 2s, 4s, 8s...). After the first reconnect attempt, WebSocketStream's backoff increases, but the component's health check doesn't know about the backoff. It just sees a closed WebSocket and triggers a full reconnect (creating a new WebSocketStream), which resets the backoff counter.

**Test against code:**
- Line 1951-1962: health check gets `ws` from `streamRef.current` and checks `readyState`
- Line 1960: if CLOSED or CLOSING, calls reconnect
- But `streamRef.current` is the WebSocketStream OBJECT, not the underlying WebSocket
- `(stream as any).ws` accesses the private `ws` field
- During WebSocketStream's internal reconnect, `this.ws` is set to null at line 371 (`this.ws.close()`) then `this.ws = null`
- When `ws` is null, the health check at line 1951 gets `undefined` → the `if (ws && ...)` check at line 1952 fails → health check returns without action
- **BUT** if `ws` is not null but readyState is CLOSED (the old WebSocket object hasn't been cleared yet), the check fires

**Actually, looking more carefully at `connect()` in WebSocketStream:**
- Line 367-373: `if (this.ws) { this.ws.close(); this.ws = null; }` — this is called at the START of reconnection
- So during the backoff delay, `this.ws` IS null (it was cleared in `connect()` during the previous attempt... wait no. `connect()` is called when the reconnect timer fires, not when it's scheduled.

**Let me trace more carefully:**
1. WebSocket closes → `onClose()` fires
2. `this.connected = false` (line 477)
3. Old `this.ws` still exists (it's the WebSocket that fired the close event)
4. Backoff timer scheduled (line 525)
5. **During backoff wait:** `this.ws` is STILL the old (now-closed) WebSocket
6. Component health check runs → gets `this.ws` → checks `readyState` → it's `WebSocket.CLOSED`
7. Health check calls `reconnectRef.current()` → component-level reconnect
8. `disconnect(true)` → `stream.close()` → sets `this.closed = true`, `this.ws.close()` (no-op, already closed), `this.ws = null`
9. **The pending reconnect timeout in WebSocketStream is cleared by `close()` at line 2251-2253**
10. Component's setTimeout fires → `connect()` → creates new WebSocketStream
11. New WebSocketStream connects → but server may reject with AlreadyStreaming
12. Even if it connects, the NEXT time there's a transient drop, the same cycle repeats
13. Over time, this creates hundreds of WebSocketStream instances

**This is the root cause.** The component's frame health check doesn't know that WebSocketStream has a pending internal reconnect. It sees a closed WebSocket, panics, and creates a brand new stream — killing the pending reconnect and its backoff.

**Verdict: ROOT CAUSE CONFIRMED.** The component-level health check competes with WebSocketStream's internal reconnection. The health check is more aggressive and "wins" by closing the stream (setting `this.closed = true`), which cancels the internal reconnect. This creates a new stream every 3-8 seconds instead of letting WebSocketStream's backoff work.

### Hypothesis 7: Parent component sandbox state polling causes unmount/remount

**Theory:** `ExternalAgentDesktopViewer` polls sandbox state every 3 seconds (line 36: `refetchInterval: 3000`). If `sandboxState` oscillates between "running" and other states, `DesktopStreamViewer` could unmount and remount.

**Test against code:**
- Line 139-140: `hasEverBeenRunning` state prevents unmount once running
- Line 447: `if (isStarting && !hasEverBeenRunning)` — only unmounts before first run
- Line 493: `if (isPaused && !hasEverBeenRunning)` — same guard
- After first run, the stream viewer stays mounted regardless of state changes

**Verdict: Not a cause during normal use.** The `hasEverBeenRunning` guard prevents this. Only affects first connection before stream starts.

## Root Cause

**Hypothesis 6 is the root cause.** The component's frame health check (`checkFrameHealth`, every 3s) sees a CLOSED WebSocket during WebSocketStream's internal reconnect backoff period, and pre-emptively creates a brand new stream, killing the pending backoff. This cycle repeats indefinitely because:

1. Each new WebSocketStream starts with `reconnectAttempts = 0` (fresh instance)
2. The component's health check runs every 3s — faster than any backoff
3. Creating a new stream while the server still has the old session active triggers race conditions

## Fix Required

The frame health check should NOT trigger a component-level reconnect while WebSocketStream has a pending internal reconnect. The check should distinguish between:
- **WebSocket is closed AND no reconnect pending** → yes, component should reconnect (stream is dead)
- **WebSocket is closed AND internal reconnect pending** → no, let WebSocketStream handle it

Options:
1. **Expose reconnect state:** Add `isReconnecting()` method to WebSocketStream that returns true when `reconnectTimeoutId` is not null
2. **Remove component-level reconnect from health check:** Only check decoder state, not WebSocket state (WebSocketStream already handles WS reconnection)
3. **Coordinate:** Have health check only act on stale connections, not closed ones (closed = WebSocketStream handles it; stale = no messages for N seconds while WS appears open)

Option 3 is cleanest — the health check should only detect the case where the WebSocket APPEARS connected (`readyState === OPEN`) but is actually dead (no messages flowing). This is the exact case WebSocketStream's heartbeat doesn't cover fast enough (10s vs 5s). For CLOSED WebSocket states, WebSocketStream's own `onClose → reconnect` logic should be authoritative.
