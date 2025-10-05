# Helix Protocol Update for New Zed WebSocket Spec

## Summary

Zed's WebSocket protocol has been updated to be fully stateless. Helix needs updates to match the new protocol defined in `/home/luke/pm/zed/WEBSOCKET_PROTOCOL_SPEC.md`.

## Key Changes:

### 1. Event Naming
- `context_created` → `thread_created`
- Field `context_id` → `acp_thread_id`
- All events now use `acp_thread_id` consistently

### 2. Removed Fields
- ❌ `helix_session_id` - NO LONGER SENT by Helix to Zed
- ❌ `session_id` - NO LONGER SENT by Zed to Helix
- ✅ `request_id` - NEW field for correlation

### 3. Stateless Design
- Zed NEVER stores external session IDs
- Helix maintains ALL mappings: `helix_session_id → acp_thread_id`
- Correlation via `request_id` instead of session_id

## Files to Update in Helix:

### A. api/pkg/server/websocket_external_agent_sync.go

#### 1. Add thread_created handler
```go
case "thread_created":  // NEW - was "context_created"
    return apiServer.handleThreadCreated(sessionID, syncMsg)
```

#### 2. Rename handleContextCreated → handleThreadCreated  
```go
func (apiServer *HelixAPIServer) handleThreadCreated(sessionID string, syncMsg *types.SyncMessage) error {
    // Extract fields with NEW names
    acpThreadID, ok := syncMsg.Data["acp_thread_id"].(string)  // Was: context_id
    if !ok {
        return fmt.Errorf("missing or invalid acp_thread_id")
    }
    
    requestID, ok := syncMsg.Data["request_id"].(string)  // NEW field
    if !ok {
        log.Warn().Msg("thread_created missing request_id")
    }
    
    log.Info().
        Str("acp_thread_id", acpThreadID).
        Str("request_id", requestID).
        Msg("📥 [HELIX] Received thread_created from Zed")
    
    // Find the session that initiated this request
    if requestID != "" {
        session := apiServer.findSessionByRequestID(requestID)
        if session != nil {
            // Store the mapping: Helix session → ACP thread
            session.Metadata.AcpThreadID = acpThreadID
            apiServer.Controller.Options.Store.UpdateSession(context.Background(), *session)
            
            log.Info().
                Str("helix_session_id", session.ID).
                Str("acp_thread_id", acpThreadID).
                Msg("✅ [HELIX] Stored acp_thread_id on session")
        }
    }
    
    return nil
}
```

#### 3. Update handleMessageAdded
```go
func (apiServer *HelixAPIServer) handleMessageAdded(sessionID string, syncMsg *types.SyncMessage) error {
    // Use acp_thread_id instead of context_id
    acpThreadID, ok := syncMsg.Data["acp_thread_id"].(string)
    if !ok {
        return fmt.Errorf("missing acp_thread_id")
    }
    
    // Find session by acp_thread_id
    session := apiServer.findSessionByAcpThreadID(acpThreadID)
    if session == nil {
        return fmt.Errorf("no session found for acp_thread_id: %s", acpThreadID)
    }
    
    // Rest of logic same...
    content, _ := syncMsg.Data["content"].(string)
    // Update interaction response but keep waiting state
}
```

#### 4. Update handleMessageCompleted
```go
func (apiServer *HelixAPIServer) handleMessageCompleted(sessionID string, syncMsg *types.SyncMessage) error {
    // Use acp_thread_id instead of context_id
    acpThreadID, ok := syncMsg.Data["acp_thread_id"].(string)
    if !ok {
        return fmt.Errorf("missing acp_thread_id")
    }
    
    requestID, _ := syncMsg.Data["request_id"].(string)
    
    // Find session by acp_thread_id
    session := apiServer.findSessionByAcpThreadID(acpThreadID)
    if session == nil {
        return fmt.Errorf("no session found for acp_thread_id: %s", acpThreadID)
    }
    
    // Mark interaction as complete
}
```

### B. api/pkg/types/types.go

Update session metadata:
```go
type SessionMetadata struct {
    // OLD
    ZedThreadID string `json:"zed_thread_id,omitempty"`  // Deprecated
    
    // NEW
    AcpThreadID string `json:"acp_thread_id,omitempty"`  // Zed ACP thread ID
}
```

### C. Add helper function
```go
func (apiServer *HelixAPIServer) findSessionByAcpThreadID(acpThreadID string) *types.Session {
    // Query sessions where metadata.acp_thread_id = acpThreadID
    sessions, _ := apiServer.Controller.Options.Store.GetSessions(context.Background(), &types.SessionFilter{
        // Filter by metadata
    })
    
    for _, session := range sessions {
        if session.Metadata.AcpThreadID == acpThreadID {
            return &session
        }
    }
    return nil
}
```

### D. Update outgoing chat_message
When Helix sends chat_message to Zed:
```go
// OLD
command := types.ExternalAgentCommand{
    Type: "chat_message",
    Data: map[string]interface{}{
        "helix_session_id": session.ID,           // REMOVE
        "zed_context_id":   session.Metadata.ZedThreadID,
        "message":          userMessage,
        "request_id":       requestID,
    },
}

// NEW  
command := types.ExternalAgentCommand{
    Type: "chat_message",
    Data: map[string]interface{}{
        "acp_thread_id": session.Metadata.AcpThreadID,  // Was: zed_context_id
        "message":       userMessage,
        "request_id":    requestID,                      // Correlation ID
    },
}
```

## Testing After Update:

1. Run `cargo test -p external_websocket_sync` in Zed (should pass)
2. Update Helix code as above
3. Run Helix tests
4. Test end-to-end:
   - Start Helix with WebSocket enabled
   - Start Zed with `external_websocket_sync` feature
   - Send message from Helix
   - Verify Zed creates thread and responds
   - Verify response streams back to Helix
   - Send follow-up message
   - Verify it goes to same thread

## Protocol Reference:

See `/home/luke/pm/zed/WEBSOCKET_PROTOCOL_SPEC.md` for complete spec.

## Implementation Status:

- ✅ Zed implementation complete (thread_service.rs, websocket_sync.rs)
- ✅ Zed tests pass (6/6 protocol tests)
- ⚠️ Helix needs updates (documented above)
- ⏳ End-to-end testing pending

