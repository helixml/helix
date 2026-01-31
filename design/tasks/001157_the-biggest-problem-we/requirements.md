# Requirements: Fix In-Memory Mappings Lost After Restart

## Problem Statement

After Helix API restart, the system loses the ability to send messages to connected agents. The root cause is that several critical in-memory mappings are not persisted or restored:

1. `responseChannels` / `doneChannels` / `errorChannels` - For streaming HTTP responses
2. `externalAgentWSManager.connections` - WebSocket connections to agents
3. `contextMappings` - Zed context_id → Helix session_id
4. `sessionToWaitingInteraction` - Helix session_id → interaction_id
5. `requestToSessionMapping` - request_id → session_id
6. `externalAgentSessionMapping` - External agent session_id → Helix session_id

## User Stories

### US1: Messages Route Correctly After Restart
As a user, I want my messages to reach the agent even if Helix was restarted while my session was active, so that I don't lose work or get stuck.

**Acceptance Criteria:**
- [ ] When agent WebSocket reconnects after API restart, message routing works immediately
- [ ] Existing sessions continue to function without user intervention
- [ ] No "no WebSocket connection found for session" errors for active sessions

### US2: Streaming Responses Resume
As a user, I want in-flight streaming responses to either complete or fail gracefully after restart, not hang forever.

**Acceptance Criteria:**
- [ ] Active streaming requests timeout with clear error (not hang)
- [ ] New requests work normally after reconnection

## Technical Requirements

### TR1: Restore contextMappings on Reconnect
- When agent WebSocket connects, restore `contextMappings` from `session.Metadata.ZedThreadID`
- This is partially implemented (line 332 in websocket_external_agent_sync.go) but may have gaps

### TR2: Restore sessionToWaitingInteraction
- Persist `waitingInteractionID` in session metadata when set
- Restore mapping when agent reconnects
- Clear stale mappings for completed interactions

### TR3: Handle Lost Response Channels
- When streaming request's response channel doesn't exist (lost due to restart), return error immediately
- Add logging to identify affected requests

## Out of Scope
- Persisting WebSocket connections across restarts (impossible - agents must reconnect)
- Replaying lost streaming chunks (agents must re-send)