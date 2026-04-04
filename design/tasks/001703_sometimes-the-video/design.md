# Design: Video Streaming Reconnect Loop Fix

## Problem Analysis

Based on code analysis, the reconnect loop issue likely stems from one of these scenarios:

### Scenario A: Counter Reset Race
The `reconnectAttempts` counter is reset in `onOpen()` (line 414). If a connection opens briefly then closes, the counter resets but the reconnection cycle continues - creating a loop where each reconnect "succeeds" briefly then fails.

```typescript
// websocket-stream.ts:414
private onOpen() {
  this.reconnectAttempts = 0  // Reset counter
  // ... but if connection fails shortly after, we start over
}
```

### Scenario B: Proxy Deduplication Interference
When a client reconnects with the same `client_id`, the API cancels the previous proxy (lines 1270-1278 in external_agent_handlers.go). If the old proxy is still mid-handshake when cancelled, it could cause immediate reconnection.

### Scenario C: Stale Detection During Valid Gap
The heartbeat timeout (10s) may fire during periods of legitimate low activity (static screens send ~1 frame/500ms keepalive), triggering unnecessary reconnection.

### Scenario D: Error Propagation Without Close Code
When the ResilientProxy encounters an error, it may close the client connection without a proper WebSocket close code, causing "error unknown" in stats.

## Recommended Investigation & Fix

### Phase 1: Add Diagnostic Logging

Add connection state logging to identify which scenario is occurring:

```typescript
// websocket-stream.ts - Enhanced close logging
private onClose(event: CloseEvent) {
  console.log("[WebSocketStream] Disconnected:", {
    code: event.code,
    reason: event.reason,
    wasClean: event.wasClean,
    reconnectAttempts: this.reconnectAttempts,
    timeSinceOpen: this.connected ? Date.now() - this.lastOpenTime : -1
  })
}
```

### Phase 2: Fix Connection Stability

**Fix A: Require Minimum Connection Duration**

Don't reset `reconnectAttempts` until connection has been stable for a minimum period (e.g., 2 seconds). This prevents rapid connect/disconnect loops from resetting the counter.

```typescript
// websocket-stream.ts
private connectionStabilityTimer: ReturnType<typeof setTimeout> | null = null

private onOpen() {
  this.connected = true
  // Don't reset attempts immediately - wait for stability
  this.connectionStabilityTimer = setTimeout(() => {
    this.reconnectAttempts = 0
    console.log("[WebSocketStream] Connection stabilized, reset reconnect counter")
  }, 2000)
}

private onClose(event: CloseEvent) {
  // Clear stability timer if connection drops before stabilizing
  if (this.connectionStabilityTimer) {
    clearTimeout(this.connectionStabilityTimer)
    this.connectionStabilityTimer = null
  }
  // ... existing close logic
}
```

**Fix B: Propagate Close Codes**

Ensure the backend sends proper WebSocket close codes when terminating connections:

```go
// resilient.go - Send proper close frame before terminating
func (p *ResilientProxy) closeWithCode(code int, reason string) {
    // Write close frame with code and reason
    closeMsg := websocket.FormatCloseMessage(code, reason)
    p.clientConn.Write(closeMsg)
    p.clientConn.Close()
}
```

**Fix C: Prevent Reconnect During Existing Reconnect**

Add guard to prevent multiple concurrent reconnection attempts:

```typescript
// websocket-stream.ts
private isReconnecting = false

private onClose(event: CloseEvent) {
  if (this.isReconnecting) {
    console.log("[WebSocketStream] Already reconnecting, ignoring close event")
    return
  }
  this.isReconnecting = true
  // ... reconnection logic
}

private onOpen() {
  this.isReconnecting = false
  // ...
}
```

## Key Files to Modify

| File | Changes |
|------|---------|
| `frontend/src/lib/helix-stream/stream/websocket-stream.ts` | Connection stability timer, reconnection guards, enhanced logging |
| `frontend/src/components/external-agent/DesktopStreamViewer.tsx` | Coordinate with WebSocketStream reconnection state |
| `api/pkg/proxy/resilient.go` | Send proper close codes, improve error propagation |
| `api/pkg/desktop/ws_stream.go` | Log close reasons for debugging |

## Testing Strategy

1. **Manual test**: Open stream, disconnect network briefly, verify single reconnection
2. **Stress test**: Rapidly toggle network connectivity, verify no infinite loops
3. **Long-duration test**: Stream for 30+ minutes, verify no spurious reconnections
4. **Multi-tab test**: Open same session in multiple tabs, verify proxy deduplication works

## Decision: Start with Diagnostics

Given the intermittent nature of the bug, implement Phase 1 (diagnostics) first to identify which scenario is actually occurring. The fix can then be targeted to the specific root cause.

## Implementation Notes (Post-Implementation)

### What was implemented

1. **Connection stability timer** (`websocket-stream.ts`): Added `connectionStabilityTimer`, `lastOpenTime`, and `connectionStabilized` properties. The `reconnectAttempts` counter is only reset after the connection has been stable for 2 seconds.

2. **Reconnection guard**: If `reconnectTimeoutId` is already set when `onClose` fires, we skip scheduling another reconnection. Also clear the timeout at the start of `connect()`.

3. **Enhanced close logging**: The `onClose` handler now logs an object with `code`, `reason`, `wasClean`, `connectionDurationMs`, `wasStabilized`, `reconnectAttempts`, and `explicitlyClosed`.

4. **Type update**: Added optional `code` property to the `disconnected` event in `websocket-stream.types.ts`.

### Backend close codes - Not applicable

The ResilientProxy (`api/pkg/proxy/resilient.go`) is a raw TCP proxy after the WebSocket upgrade. It doesn't understand WebSocket frames, so it can't send proper close codes. The frontend fixes handle this by being resilient to unknown close codes.

### Files modified

- `frontend/src/lib/helix-stream/stream/websocket-stream.ts` - Main fixes
- `frontend/src/lib/helix-stream/stream/websocket-stream.types.ts` - Type update for close code
