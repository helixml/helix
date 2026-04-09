# Requirements: Seamless Zed Reconnection After API Restart

## Context

When the Helix API restarts (deploy, crash, etc.), Zed is still running in the sandbox with an active Claude Code session. Zed's WebSocket reconnects automatically (1-30s exponential backoff), and the API rebuilds routing state from the database. However, several gaps cause the session to break or produce duplicate work rather than seamlessly continuing.

## User Stories

### US-1: In-flight response survives API restart
**As a** user chatting with the AI in Helix,
**I want** an in-progress AI response to continue seamlessly after an API restart,
**So that** I don't see the response restart from scratch or get duplicate processing.

**Acceptance Criteria:**
- If Zed completed a response for a `request_id` while the API was down, it sends the final result on reconnect instead of re-processing
- If Zed was mid-stream, the API resumes collecting from where it left off (or at minimum, doesn't duplicate the entire interaction)
- The frontend shows the completed/continuing response without a visible restart

### US-2: Messages sent during downtime are not lost
**As a** user who typed a message in the Helix chat while the API was briefly down,
**I want** that message to be delivered and processed once the API comes back,
**So that** I don't have to retype it.

**Acceptance Criteria:**
- Messages sent from the Helix frontend while the API WebSocket is down are retried on reconnection
- The interaction is created and routed correctly after reconnect

### US-3: No duplicate interactions after reconnect
**As a** user,
**I want** the API to not re-send a prompt that Zed already processed,
**So that** I don't see duplicate AI responses.

**Acceptance Criteria:**
- When `pickupWaitingInteraction` finds a waiting interaction after restart, the API first asks Zed if it already has a completed response for that `request_id`
- OR: Zed detects duplicate `chat_message` by `request_id` and replays the cached result instead of re-processing
- No duplicate interactions appear in the chat view

### US-4: Events are not silently dropped
**As a** developer,
**I want** Zed's outgoing WebSocket events to survive connection failures,
**So that** `message_added` / `message_completed` events sent during downtime are delivered on reconnect.

**Acceptance Criteria:**
- Events that fail to send over the WebSocket are re-queued, not dropped (currently a known limitation at `websocket_sync.rs:310-312`)
- Buffered events from the channel are delivered in order after reconnection
- The API handles out-of-order or delayed events gracefully (idempotent processing)
