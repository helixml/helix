# Design: Video Frame Ordering Investigation

## Analysis Summary

After investigating the frontend codebase, I've identified several potential causes for out-of-order frames related to connection lifecycle management.

## Architecture Overview

```
Browser                          API Server                    Desktop Container
   │                                 │                                │
   │  WebSocket /ws/stream           │                                │
   ├────────────────────────────────►│  RevDial proxy                 │
   │                                 ├───────────────────────────────►│
   │                                 │                                │
   │◄─────────────────────────────────────────────────────────────────┤
   │         H.264 frames (ordered via TCP)                           │
```

**Key insight**: The backend guarantees frame ordering (GStreamer appsink → Go channel → single WebSocket write mutex). If frames appear out of order, it must be a frontend issue.

## Potential Causes Identified

### 1. React Effect Race Conditions (Most Likely)

In `DesktopStreamViewer.tsx`, multiple effects can trigger `connect()`:

```typescript
// Effect 1: Auto-connect on visibility/sessionId change (L1491)
useEffect(() => {
  if (hasConnectedRef.current) return;
  // ... calls connect()
}, [sandboxId, sessionId, isVisible, width, height, account.initialized]);

// Effect 2: Bitrate change triggers reconnect (L1373)
useEffect(() => {
  // ... calls reconnect() which calls disconnect() then connect()
}, [userBitrate]);

// Effect 3: Quality mode hot-switch (L1303)
useEffect(() => {
  // Enables/disables video on existing stream
}, [qualityMode, isConnected, sessionId]);
```

**Race scenario**: If `userBitrate` changes during initial connection (before `hasEverConnectedRef` is set), both effects may fire, creating two connections.

### 2. `hasConnectedRef` vs `hasEverConnectedRef` Gap

```typescript
hasConnectedRef.current = true;      // Set immediately in connect effect
setIsConnecting(true);
connect();                           // Async! 
// ... later, in event handler:
hasEverConnectedRef.current = true;  // Set only on connectionComplete event
```

During this gap, the bitrate effect check fails:
```typescript
if (hasConnectedRef.current && !hasEverConnectedRef.current) {
  // Skip reconnect during initial connection
}
```

But if another dependency changes (like `sessionId`), the first effect may re-run.

### 3. `pendingReconnectTimeoutRef` Race

The reconnect function cancels pending timeouts, but there's no lock:
```typescript
const reconnect = useCallback((delayMs = 1000) => {
  if (pendingReconnectTimeoutRef.current) {
    clearTimeout(pendingReconnectTimeoutRef.current);  // Cancel old
    pendingReconnectTimeoutRef.current = null;
  }
  disconnect(true);
  pendingReconnectTimeoutRef.current = setTimeout(() => {
    connectRef.current();  // But connectRef might be stale!
  }, delayMs);
}, [disconnect]);
```

### 4. WebSocketStream Internal Reconnection

`WebSocketStream.ts` has its own reconnection logic:
```typescript
private onClose(event: CloseEvent) {
  // Auto-reconnects with exponential backoff
  this.reconnectTimeoutId = setTimeout(() => {
    this.connect();  // Creates NEW WebSocket!
  }, delay);
}
```

If `DesktopStreamViewer` also calls `reconnect()`, both may create connections.

## Proposed Solution

### Add Connection Singleton Guard

Add a module-level tracking mechanism to ensure only one video WebSocket per session:

```typescript
// connectionGuard.ts
const activeConnections = new Map<string, {
  streamId: string;
  createdAt: number;
  componentId: string;
}>();

export function acquireConnection(sessionId: string, componentId: string): boolean {
  const existing = activeConnections.get(sessionId);
  if (existing && existing.componentId !== componentId) {
    console.error(`[ConnectionGuard] Duplicate connection attempt!`, {
      sessionId,
      existingComponent: existing.componentId,
      newComponent: componentId,
    });
    return false;
  }
  activeConnections.set(sessionId, {
    streamId: crypto.randomUUID(),
    createdAt: Date.now(),
    componentId,
  });
  return true;
}

export function releaseConnection(sessionId: string, componentId: string): void {
  const existing = activeConnections.get(sessionId);
  if (existing?.componentId === componentId) {
    activeConnections.delete(sessionId);
  }
}
```

### Disable WebSocketStream Auto-Reconnect

Let `DesktopStreamViewer` handle all reconnection logic:

```typescript
// In WebSocketStream constructor
this.maxReconnectAttempts = 0; // Disable internal reconnect
```

### Synchronize Effect Dependencies

Use a single effect with explicit state machine:

```typescript
type ConnectionState = 
  | { status: 'idle' }
  | { status: 'connecting'; attempt: number }
  | { status: 'connected'; streamId: string }
  | { status: 'reconnecting'; reason: string }
  | { status: 'failed'; error: string };

const [connState, dispatch] = useReducer(connectionReducer, { status: 'idle' });
```

## Key Decision

**Chosen approach**: Connection Singleton Guard + disable internal reconnect

**Rationale**:
- Minimal code change (additive, not restructuring)
- Provides clear diagnostics when duplicates are attempted
- Works as a safety net even if other race conditions exist
- Can be enabled/disabled for A/B testing

## Risks

- If guard logic has bugs, could prevent legitimate connections
- Need to ensure cleanup on unmount, tab close, etc.