# Design: Fix In-Memory Mappings Lost After Restart

## Problem Analysis

The Helix API server maintains several in-memory maps for routing messages between HTTP clients and WebSocket-connected agents:

```
┌─────────────────┐      ┌─────────────────────┐      ┌─────────────────┐
│  HTTP Client    │──────│   Helix API         │──────│   Agent (Zed)   │
│  (browser)      │      │   (in-memory maps)  │      │   (WebSocket)   │
└─────────────────┘      └─────────────────────┘      └─────────────────┘
                                   │
                         ┌─────────┴─────────┐
                         │ RESTART = LOST    │
                         │ - contextMappings │
                         │ - responseChannels│
                         │ - sessionToWaiting│
                         └───────────────────┘
```

When the API restarts:
1. WebSocket connections drop (expected - agents reconnect)
2. In-memory maps are empty (problem - routing breaks)
3. Agent reconnects but API can't route messages to it

## Current State

**Partially Working:**
- `contextMappings` (Zed thread → Helix session) is restored from `session.Metadata.ZedThreadID` on reconnect (line 332 of websocket_external_agent_sync.go)

**Not Working:**
- `sessionToWaitingInteraction` - never persisted, never restored
- `requestToSessionMapping` - never persisted, never restored  
- `externalAgentSessionMapping` - initialized empty, never populated (unused?)
- `responseChannels` - HTTP Go channels (cannot be persisted, must timeout gracefully)

## Solution: Persist to Session Metadata

### Key Insight
Session metadata is already persisted in the database and restored when agents reconnect. We should extend this pattern.

### Design

1. **Add Fields to Session Metadata**
```go
type SessionMetadata struct {
    // Existing
    ZedThreadID string
    // New
    WaitingInteractionID string  // The interaction currently waiting for response
    LastRequestID        string  // Most recent request_id (for reconnect routing)
}
```

2. **Persist on Set**
When we set `sessionToWaitingInteraction[sessionID] = interactionID`, also:
```go
session.Metadata.WaitingInteractionID = interactionID
store.UpdateSession(ctx, session)
```

3. **Restore on Reconnect**
In `handleExternalAgentSync`, after restoring `contextMappings`:
```go
if session.Metadata.WaitingInteractionID != "" {
    apiServer.sessionToWaitingInteraction[helixSessionID] = session.Metadata.WaitingInteractionID
}
```

4. **Clear on Complete**
When interaction completes, clear both in-memory and persisted:
```go
delete(apiServer.sessionToWaitingInteraction, sessionID)
session.Metadata.WaitingInteractionID = ""
store.UpdateSession(ctx, session)
```

### Response Channels (Cannot Persist)

Go channels cannot be persisted. When API restarts during streaming:

**Current Behavior:** Request hangs until 90s timeout

**Proposed Behavior:** 
- Add `RequestStartedAt` to session metadata
- On reconnect, if `WaitingInteractionID` exists and `RequestStartedAt > 5 minutes ago`, mark interaction as failed
- HTTP streaming code should check for stale requests and return error immediately

## Affected Files

| File | Change |
|------|--------|
| `api/pkg/types/session.go` | Add `WaitingInteractionID` to SessionMetadata |
| `api/pkg/server/session_handlers.go` | Persist WaitingInteractionID when set |
| `api/pkg/server/websocket_external_agent_sync.go` | Restore mappings on reconnect, clear on complete |

## Decision: Keep externalAgentSessionMapping?

This map is initialized but never written to. Options:
1. **Remove it** - dead code
2. **Implement it** - for non-ses_* agent IDs

Recommendation: Remove it. Current code uses `ses_*` prefixed session IDs directly.

## Testing Strategy

1. Start session, send message, verify response
2. Restart API while session active
3. Wait for agent reconnect
4. Send new message, verify routing works
5. Check logs for "Restored contextMappings" and new "Restored sessionToWaitingInteraction" messages