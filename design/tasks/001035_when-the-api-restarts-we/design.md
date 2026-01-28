# Design: WebSocket Session State Sync After API Restart

## Overview

When the API restarts, the embedded NATS server loses all subscriptions. Although the frontend's `ReconnectingWebSocket` successfully reconnects, any messages published during the disconnect window are lost. This design addresses the issue by sending the current session state immediately when a WebSocket connection is established.

## Architecture

### Current Flow (Broken)

```
1. API restart → NATS server restarts → subscriptions lost
2. Frontend WebSocket reconnects → new NATS subscription created
3. Messages published during gap → LOST (never delivered)
4. User sees stale state until next event triggers publish
```

### Proposed Flow (Fixed)

```
1. API restart → NATS server restarts → subscriptions lost
2. Frontend WebSocket reconnects → new NATS subscription created
3. Server immediately fetches session from DB
4. Server publishes current session state to new WebSocket
5. User sees current state immediately, no missed updates
```

## Implementation

### Location

`helix/api/pkg/server/websocket_server_user.go` - the user WebSocket handler

### Changes

**After WebSocket upgrade and NATS subscription setup:**

1. Fetch the session from the database using the `session_id` query parameter
2. Create a `WebsocketEvent` with type `session_update` containing the full session state
3. Send this event over the WebSocket connection before entering the message loop

### Code Pattern

```go
// After subscription is set up, send current session state
session, err := apiServer.Store.GetSession(r.Context(), sessionID)
if err == nil && session != nil {
    // Include interactions for full state
    interactions, _, _ := apiServer.Store.ListInteractions(r.Context(), &types.ListInteractionsQuery{
        SessionID: sessionID,
    })
    if interactions != nil {
        session.Interactions = interactions
    }
    
    // Send initial state to client
    event := &types.WebsocketEvent{
        Type:      types.WebsocketEventSessionUpdate,
        SessionID: sessionID,
        Owner:     user.ID,
        Session:   session,
    }
    if payload, err := json.Marshal(event); err == nil {
        conn.WriteMessage(websocket.TextMessage, payload)
    }
}
```

## Key Decisions

### Why send full session state?

- Simple and reliable - guarantees client is in sync
- Reuses existing `session_update` event type - no frontend changes needed
- Interactions are included so streaming responses are visible

### Why not use JetStream (persistent messaging)?

- Would require significant architectural changes
- Overkill for this use case - we just need eventual consistency
- Session state is already in PostgreSQL, no need to duplicate in NATS

### Why not buffer messages on API side?

- Complexity of managing buffers across restarts
- Would still lose messages during actual server downtime
- DB-based state sync is more robust

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Session lookup adds latency | Single indexed query, ~1ms typical |
| Large sessions slow WebSocket connect | Pagination possible in future; most sessions are small |
| Frontend receives duplicate state | Already handles idempotently (React reconciliation) |

## Testing

1. **Manual test**: Start session, observe streaming, restart API, verify UI updates
2. **Automated**: Add test that connects WebSocket and verifies initial `session_update` received
3. **Edge cases**: Test with non-existent session, session with many interactions