# Helix ↔ Zed WebSocket Sync Architecture

## Date
2026-01-22

## Status
**Draft** - Investigating user message synchronization issues

## Problem Statement

Users reported that messages sent to the Zed agent aren't appearing in the Helix session view:

1. **User sends message from Helix UI** → message doesn't show in session view
2. **User types directly in Zed text box** → message doesn't show in session view
3. Only AI responses appear, not the user's original messages
4. Follow-up responses sometimes include parts of previous messages (duplication)
5. Large chunks of content get duplicated multiple times

---

## Architecture Overview

### System Components

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            HELIX FRONTEND                                   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ RobustPromptInput                                                    │   │
│  │  • localStorage buffer for offline resilience                        │   │
│  │  • Syncs to backend prompt_queue table                               │   │
│  │  • Supports interrupt/queue modes                                    │   │
│  └──────────────────────────────────┬──────────────────────────────────┘   │
│                                     │ HTTP POST /api/v1/sessions/chat      │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ EmbeddedSessionView                                                  │   │
│  │  • Polls session every 2s                                            │   │
│  │  • Receives WebSocket updates for real-time display                  │   │
│  │  • Renders Interaction components                                    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              HELIX API                                      │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ session_handlers.go                                                  │   │
│  │                                                                      │   │
│  │  startChatSessionHandler:                                            │   │
│  │    1. appendOrOverwrite() - Creates Interaction with PromptMessage   │   │
│  │    2. WriteInteractions() - Persists to database                     │   │
│  │    3. NotifyExternalAgentOfNewInteraction() - Sends with role=user   │   │
│  │    4. handleExternalAgentStreaming() - Routes to Zed                 │   │
│  │                                                                      │   │
│  │  streamFromExternalAgent():                                          │   │
│  │    • Sends chat_message command (NO role field)                      │   │
│  │    • Sets up request_id→session mapping                              │   │
│  │    • Waits for response stream                                       │   │
│  └──────────────────────────────────┬──────────────────────────────────┘   │
│                                     │                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ websocket_external_agent_sync.go                                     │   │
│  │                                                                      │   │
│  │  OUTGOING to Zed:                                                    │   │
│  │    • chat_message {acp_thread_id, message, request_id, agent_name}   │   │
│  │                                                                      │   │
│  │  INCOMING from Zed:                                                  │   │
│  │    • thread_created - Maps Zed thread to Helix session               │   │
│  │    • user_created_thread - Creates NEW session for user threads      │   │
│  │    • message_added - Creates/updates Interaction                     │   │
│  │    • message_completed - Marks interaction complete                  │   │
│  │    • agent_ready - Signals readiness for messages                    │   │
│  │                                                                      │   │
│  │  KEY DATA STRUCTURES:                                                │   │
│  │    contextMappings[ZedThreadID] → HelixSessionID                     │   │
│  │    sessionToWaitingInteraction[SessionID] → InteractionID            │   │
│  │    requestToSessionMapping[RequestID] → SessionID                    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                          WebSocket Connection
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         ZED DESKTOP (in sandbox)                            │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ websocket_sync.rs                                                    │   │
│  │                                                                      │   │
│  │  handle_chat_message():                                              │   │
│  │    • IGNORES messages with role="user" (echo prevention)             │   │
│  │    • Creates new thread OR sends to existing thread                  │   │
│  └──────────────────────────────────┬──────────────────────────────────┘   │
│                                     │                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ thread_service.rs                                                    │   │
│  │                                                                      │   │
│  │  create_new_thread_sync():                                           │   │
│  │    • Creates ACP thread                                              │   │
│  │    • Sends thread_created to Helix                                   │   │
│  │    • Marks entry as external-originated (no echo)                    │   │
│  │    • Sends message to AI agent                                       │   │
│  │                                                                      │   │
│  │  Thread Event Subscriptions:                                         │   │
│  │    • NewEntry → Sends MessageAdded(role=user) if NOT external        │   │
│  │    • EntryUpdated → Sends MessageAdded(role=assistant)               │   │
│  │    • Stopped → Sends MessageCompleted                                │   │
│  └──────────────────────────────────┬──────────────────────────────────┘   │
│                                     │                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ thread_view.rs (Agent Panel UI)                                      │   │
│  │                                                                      │   │
│  │  When user creates new thread with content:                          │   │
│  │    • Sends user_created_thread to Helix                              │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Message Flows

### Flow 1: User Sends Message from Helix UI (Robust Prompt)

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ HELIX FRONTEND                                                               │
│                                                                              │
│ 1. User types in RobustPromptInput                                          │
│ 2. Message stored in localStorage (offline resilience)                       │
│ 3. Synced to backend prompt_queue table                                      │
│ 4. HTTP POST /api/v1/sessions/chat                                           │
└──────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│ HELIX API: startChatSessionHandler                                           │
│                                                                              │
│ 5. appendOrOverwrite() creates Interaction:                                  │
│    {                                                                         │
│      ID: "int_xxx",                                                          │
│      SessionID: "ses_xxx",                                                   │
│      PromptMessage: "user's message",  ← USER MESSAGE STORED HERE            │
│      State: Waiting                                                          │
│    }                                                                         │
│                                                                              │
│ 6. WriteInteractions() persists to database                                  │
│                                                                              │
│ 7. NotifyExternalAgentOfNewInteraction() sends:                              │
│    chat_message { role: "user", message: "..." }                             │
│    ⚠️  Zed IGNORES this (role=user filtering)                                │
│                                                                              │
│ 8. handleExternalAgentStreaming() → streamFromExternalAgent() sends:         │
│    chat_message { acp_thread_id, message, request_id, agent_name }           │
│    ✅ Zed PROCESSES this (no role field)                                     │
└──────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│ ZED DESKTOP                                                                  │
│                                                                              │
│ 9. websocket_sync.rs receives chat_message                                   │
│    - If acp_thread_id is null: create_new_thread_sync()                      │
│    - If acp_thread_id exists: handle_follow_up_message()                     │
│                                                                              │
│ 10. thread_service.rs:                                                       │
│     - Creates/loads ACP thread                                               │
│     - Sends thread_created { acp_thread_id, request_id } to Helix            │
│     - Marks entry as external-originated (prevents echo)                     │
│     - Sends message to AI agent                                              │
│                                                                              │
│ 11. AI agent responds, triggers EntryUpdated events                          │
│     - Sends MessageAdded { role: "assistant", content: "..." }               │
└──────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│ HELIX API: handleMessageAdded (role=assistant)                               │
│                                                                              │
│ 12. Looks up session: contextMappings[acp_thread_id] → sessionID             │
│                                                                              │
│ 13. Finds interaction: sessionToWaitingInteraction[sessionID] → interactionID│
│                                                                              │
│ 14. Updates interaction.ResponseMessage with AI content                      │
│                                                                              │
│ 15. Publishes session update via WebSocket to frontend                       │
└──────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│ HELIX FRONTEND: EmbeddedSessionView                                          │
│                                                                              │
│ 16. Receives session update                                                  │
│                                                                              │
│ 17. Renders Interaction component:                                           │
│     - User bubble: interaction.prompt_message (from step 5)                  │
│     - AI bubble: interaction.response_message (from step 14)                 │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Key Insight**: User message is stored in step 5, BEFORE sending to Zed. The interaction should always have the PromptMessage set correctly.


### Flow 2: User Types Directly in Zed

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ ZED DESKTOP: Agent Panel UI                                                  │
│                                                                              │
│ 1. User types message in Zed's text input                                    │
│                                                                              │
│ 2. Entry added to thread → triggers NewEntry event                           │
│                                                                              │
│ 3. thread_view.rs: If new thread with entries, sends:                        │
│    user_created_thread { acp_thread_id, title }                              │
│                                                                              │
│ 4. thread_service.rs NewEntry handler: Entry NOT external-originated, sends: │
│    MessageAdded { acp_thread_id, role: "user", content: "..." }              │
└──────────────────────────────────────────────────────────────────────────────┘
                                      │
                        ┌─────────────┴─────────────┐
                        │                           │
                        ▼                           ▼
┌───────────────────────────────────┐ ┌────────────────────────────────────────┐
│ HELIX: handleUserCreatedThread    │ │ HELIX: handleMessageAdded (role=user)  │
│                                   │ │                                        │
│ Creates NEW session:              │ │ Looks up: contextMappings[acp_thread_id]│
│ {                                 │ │                                        │
│   ID: "ses_new",                  │ │ ⚠️  IF NOT FOUND:                       │
│   Metadata.ZedThreadID: "..."     │ │   return error → MESSAGE LOST!         │
│ }                                 │ │                                        │
│                                   │ │ If found: Creates Interaction           │
│ Sets contextMappings[threadID]    │ │ {                                       │
│                                   │ │   PromptMessage: content,               │
└───────────────────────────────────┘ │   State: Waiting                        │
                                      │ }                                        │
                                      └────────────────────────────────────────┘
```

**Race Condition Identified**:
- `MessageAdded(role=user)` and `user_created_thread` are sent ~simultaneously from Zed
- If `MessageAdded` arrives BEFORE `user_created_thread`:
  - `contextMappings[acp_thread_id]` doesn't exist yet
  - `handleMessageAdded` returns error at line 877
  - **User message is LOST**
- Then `user_created_thread` arrives and creates the session/mapping
- Then `MessageAdded(role=assistant)` arrives and succeeds (mapping now exists)
- **Result**: AI response shows, but user's message is missing

---

## Observed Behavior Analysis

### Symptom 1: User message doesn't show in session view

**For messages from Helix UI**:
- The Interaction is created with PromptMessage in step 5 (before Zed is involved)
- This should work correctly
- If not showing, likely a frontend display issue or database write failure

**For messages from Zed**:
- Race condition causes MessageAdded to fail before contextMappings is set up
- User message is lost, only AI response is saved

### Symptom 2: Duplicated content

**Possible causes**:
1. Multiple code paths sending the same message
2. `NotifyExternalAgentOfNewInteraction` AND `streamFromExternalAgent` both fire
3. However, Zed ignores `role=user` messages, so only `streamFromExternalAgent` should work

### Symptom 3: "Zed agent panel doesn't load"

This was caused by the failed fix attempt (see below).

---

## Failed Fix Attempt

### The Attempted Solution

I modified `handleMessageAdded` to create a session on-the-fly when receiving a `role=user` message for an unknown thread:

```go
// In handleMessageAdded, after database fallback fails:
if role != "assistant" {
    // Create new session on-the-fly
    newSession := &types.Session{
        ID:             system.GenerateSessionID(),
        // ... copy config from agent session ...
    }
    apiServer.Controller.Options.Store.CreateSession(ctx, *newSession)
    // ... set up contextMappings ...
}
```

### Why It Was Wrong

1. **Violated the Architecture**: Each spec task creates ONE session. Messages should flow to that session. Creating sessions on-the-fly breaks this model.

2. **Wrong Session Type**: The on-the-fly session copied config from the "agent session" (external agent connection), not the actual spec task session. This created orphan sessions with wrong ownership.

3. **Caused Panel Load Failures**: The code had a bug where sessions were being created incorrectly, causing downstream errors.

4. **Treated Symptom, Not Cause**: The real issue is the race condition in event ordering from Zed, not a missing session creation path.

---

## Proposed Solution

### Option A: Ensure Event Ordering in Zed (Recommended)

Modify Zed's thread_view.rs to send `user_created_thread` BEFORE any `MessageAdded` events:

```rust
// In thread_view.rs, when thread becomes active with content:
// 1. Send user_created_thread FIRST
if entry_count > 0 {
    send_websocket_event(SyncEvent::UserCreatedThread { ... });
}
// 2. THEN set up event subscriptions that will send MessageAdded
```

**Pros**:
- Fixes the root cause
- No changes to Helix backend
- Clean event ordering

**Cons**:
- Requires Zed codebase changes


### Option B: Queue Messages in Helix Until Session Exists

In `handleMessageAdded`, if session not found for a user message:
1. Queue the message with (acp_thread_id, content, timestamp)
2. When `handleUserCreatedThread` creates the session, check queue
3. Process any queued messages for that thread

```go
// New data structure
pendingUserMessages map[string][]PendingMessage // acp_thread_id → messages

// In handleMessageAdded (role=user), if session not found:
apiServer.queuePendingMessage(contextID, content)
return nil // Don't return error

// In handleUserCreatedThread, after creating session:
apiServer.processPendingMessages(acpThreadID, newSessionID)
```

**Pros**:
- Handles race condition gracefully
- No Zed changes required
- Messages are never lost

**Cons**:
- Added complexity in Helix
- Need timeout/cleanup for orphaned queued messages


### Option C: Use Existing Thread→Session Mapping

The spec task's session already has `Metadata.ZedThreadID` set when the thread is first created. For NEW threads initiated by the user in Zed:

1. Zed sends `user_created_thread`
2. `handleUserCreatedThread` creates a new child session
3. `MessageAdded` should then find the mapping

The race condition means we need to ensure `handleUserCreatedThread` always runs first, or implement Option B's queuing.


### Recommendation: Implement Option B

Option B (queuing) is the most robust because:
1. Network delays can always cause race conditions
2. We can't guarantee event ordering over WebSocket
3. A small queue adds minimal complexity
4. Provides guaranteed message delivery

---

## Code Simplification Opportunities

### 1. Remove Duplicate Notification Path

Currently two paths send messages to Zed:
- `NotifyExternalAgentOfNewInteraction` → sends with `role: "user"` → Zed ignores
- `streamFromExternalAgent` → sends without role → Zed processes

**Proposal**: Remove `NotifyExternalAgentOfNewInteraction` for zed_external sessions, or don't send `role` field. Having both paths is confusing and wasteful.


### 2. Unify Session Lookup

Three places look up sessions by ZedThreadID:
- `handleMessageAdded` with fallback to DB
- `handleUserCreatedThread`
- Connection handler on reconnect

**Proposal**: Create a single `getOrWaitForSession(threadID)` helper that:
- Checks contextMappings
- Falls back to database
- Optionally waits briefly for pending session creation


### 3. Consolidate Interaction Mapping

Multiple mappings track interaction state:
- `sessionToWaitingInteraction[sessionID] → interactionID`
- `requestToSessionMapping[requestID] → sessionID`
- `requestToCommenterMapping[requestID] → commenterID`

**Proposal**: Create a unified `RequestContext` struct:
```go
type RequestContext struct {
    SessionID     string
    InteractionID string
    CommenterID   string  // optional
    CreatedAt     time.Time
}
requestContexts map[string]*RequestContext // requestID → context
```

---

## Requirements Checklist

Any solution must support:

- [x] Robust prompt architecture with localStorage buffering
- [x] Backend queue with interrupt/queue modes
- [x] Tight Helix session ↔ Zed thread mapping
- [x] Responses go to correct interaction (not just latest)
- [ ] User messages never lost due to race conditions
- [ ] No duplicate messages
- [ ] Clean event ordering

---

## Next Steps

1. **Investigate frontend**: Verify that interactions with PromptMessage are actually being displayed correctly
2. **Add logging**: Log when contextMappings lookup fails to confirm race condition theory
3. **Implement Option B**: Add message queue for pending user messages
4. **Test thoroughly**: Both Helix UI and Zed direct input paths
5. **Simplify**: Remove duplicate notification path

---

## Duplicate Code Paths Analysis

### The Two Methods That Send Messages to Zed

There are **two different methods** that send messages to the Zed external agent:

#### 1. NotifyExternalAgentOfNewInteraction (websocket_external_agent_sync.go:786)

```go
func (apiServer *HelixAPIServer) NotifyExternalAgentOfNewInteraction(sessionID string, interaction *types.Interaction) error {
    // ...
    commandData := map[string]interface{}{
        "message":    interaction.PromptMessage,
        "role":       "user",                    // ← INCLUDES role: "user"
        "request_id": interaction.ID,            // ← Uses interaction.ID
    }
    // ...
}
```

**Called from**: `startChatSessionHandler` at line 363, for ALL new interactions
**When**: After `WriteInteractions()` persists the interaction to database
**Zed behavior**: **IGNORED** - Zed filters out messages with `role: "user"`

#### 2. streamFromExternalAgent (session_handlers.go:1183)

```go
func (s *HelixAPIServer) streamFromExternalAgent(...) error {
    requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())  // ← Generated ID

    command := types.ExternalAgentCommand{
        Type: "chat_message",
        Data: map[string]interface{}{
            "acp_thread_id": session.Metadata.ZedThreadID,
            "message":       userMessage,
            "request_id":    requestID,           // ← Different ID!
            "agent_name":    agentName,
            // NOTE: NO role field
        },
    }
    // ...
}
```

**Called from**: `handleExternalAgentStreaming` at line 1179
**When**: After routing through `handleBlockingSession` or `handleStreamingSession`
**Zed behavior**: **PROCESSED** - No role field, so not filtered

### Flow Diagram

```
startChatSessionHandler
        │
        ▼
┌─────────────────────────────────────────────────────────────────┐
│ WriteInteractions() - persist to DB                             │
└─────────────────────────────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────────┐
│ NotifyExternalAgentOfNewInteraction()           [Line 363]      │
│                                                                 │
│ Sends: { role: "user", message, request_id: interaction.ID }    │
│                                                                 │
│ ⚠️  ZED IGNORES THIS (role=user filtering in websocket_sync.rs) │
└─────────────────────────────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────────┐
│ handleBlockingSession / handleStreamingSession                  │
│        │                                                        │
│        ▼                                                        │
│ if AgentType == "zed_external":                                 │
│     handleExternalAgentStreaming()                              │
└─────────────────────────────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────────┐
│ streamFromExternalAgent()                       [Line 1179]     │
│                                                                 │
│ Sends: { acp_thread_id, message, request_id: "req_<ts>" }       │
│                                                                 │
│ ✅ ZED PROCESSES THIS (no role field)                           │
└─────────────────────────────────────────────────────────────────┘
```

### Comparison Table

| Aspect | NotifyExternalAgentOfNewInteraction | streamFromExternalAgent |
|--------|-------------------------------------|-------------------------|
| **Called At** | Line 363 | Line 1179 |
| **Defined At** | websocket_external_agent_sync.go:786 | session_handlers.go:1183 |
| **Message Has role** | Yes (`role: "user"`) | No |
| **request_id Source** | `interaction.ID` | `"req_<timestamp>"` |
| **Zed Behavior** | **IGNORED** | **PROCESSED** |
| **Response Handling** | Fire-and-forget | Waits for streaming response |
| **Purpose** | Unknown/Legacy? | Actually routes message |

### Problems This Causes

1. **Wasted traffic**: Every message is sent twice over WebSocket
2. **Inconsistent request_ids**: Two different IDs for the same logical message
3. **Confusion**: Unclear why both methods exist
4. **Fragile**: If Zed ever stops filtering `role=user`, messages would duplicate

### Recommendation

Either:
- **Remove** `NotifyExternalAgentOfNewInteraction` call for zed_external sessions, OR
- **Remove** the `role: "user"` field so both paths work (but then need to dedupe)

---

## Interaction Mapping Collision (Critical Bug)

### The Problem: Single-Value Map Overwrite

The `sessionToWaitingInteraction` map stores only ONE interaction per session:

```go
sessionToWaitingInteraction map[string]string  // session_id → interaction_id
```

**Multiple places write to this map:**
1. `streamFromExternalAgent` (session_handlers.go:1256)
2. `sendQueuedPromptToSession` (websocket_external_agent_sync.go:1991)
3. Session creation (websocket_external_agent_sync.go:774)

**The bug manifests when:**
1. User sends Message 1 → creates Interaction A → `map[session] = A`
2. User sends Message 2 before A completes → creates Interaction B → `map[session] = B` (overwrites A!)
3. Response for A arrives from Zed
4. `handleMessageAdded` looks up `map[session]` → gets B (WRONG!)
5. AI response for Message 1 is saved to Interaction B instead of A

### Visual Representation

```
TIME →

t1: User sends "What is 2+2?"
    └─ Creates interaction int_001
    └─ sessionToWaitingInteraction["ses_xxx"] = "int_001"

t2: User sends "What is 3+3?" (before t1 completes)
    └─ Creates interaction int_002
    └─ sessionToWaitingInteraction["ses_xxx"] = "int_002"  ← OVERWRITES!

t3: Zed responds to "What is 2+2?" with "4"
    └─ handleMessageAdded looks up sessionToWaitingInteraction["ses_xxx"]
    └─ Gets "int_002" (WRONG - should be int_001!)
    └─ Saves "4" to int_002 instead of int_001

RESULT:
- int_001: PromptMessage="What is 2+2?", ResponseMessage=""  ← Missing response!
- int_002: PromptMessage="What is 3+3?", ResponseMessage="4" ← Wrong response!
```

### Related Commit

This issue was partially addressed in commit `65a3a4d8e`:
```
feat: fix queue prompt processing to create interactions and track responses

- Fix sendQueuedPromptToSession to create interaction before sending
  so agent responses have somewhere to go
- CRITICAL: Store the mapping so handleMessageAdded can find this interaction
```

The commit added the `sessionToWaitingInteraction` write in `sendQueuedPromptToSession`, but this made the collision problem worse when combined with `streamFromExternalAgent`.

### Proposed Fix

Change from single-value map to multi-value tracking using `request_id`:

```go
// Option A: Use request_id as key instead of session_id
requestToInteraction map[string]string  // request_id → interaction_id

// Option B: Store list of pending interactions per session
sessionToPendingInteractions map[string][]string  // session_id → []interaction_id

// Option C: Include request_id in Zed response messages
// Zed would echo back the request_id so we can correlate
```

**Recommended: Option C** - Have Zed echo back `request_id` in `message_added` events, then use `requestToInteraction` mapping for precise correlation.

---

## ROOT CAUSE FOUND: Cumulative Content Bug in thread_view.rs

**Date**: 2026-01-22 (confirmed via diagnostic logs)

### The Bug

In `crates/agent_ui/src/acp/thread_view.rs` (lines 1771-1808), when handling `EntryUpdated` events:

```rust
// IMPORTANT: We send ALL entries' content (cumulative) on every update.
let mut cumulative_content = String::new();
for entry in thread_read.entries().iter() {
    let entry_content = match entry {
        acp_thread::AgentThreadEntry::AssistantMessage(msg) => {
            Some(msg.content_only(cx))
        }
        acp_thread::AgentThreadEntry::ToolCall(tool_call) => {
            Some(tool_call.to_markdown(cx))
        }
        acp_thread::AgentThreadEntry::UserMessage(_) => None,
    };
    if let Some(content) = entry_content {
        cumulative_content.push_str(&content);  // Concatenates ALL entries!
    }
}

// Use constant message_id "response" so it always overwrites
external_websocket_sync::send_websocket_event(
    external_websocket_sync::SyncEvent::MessageAdded {
        acp_thread_id,
        message_id: "response".to_string(),  // <-- Always "response"!
        content: cumulative_content,          // <-- ALL entries concatenated!
        ...
    }
)
```

### Why This Happens

1. On EVERY `EntryUpdated` event, the code collects **ALL non-user entries** from the entire thread
2. This includes entries from ALL previous interactions (tool calls, assistant messages)
3. It sends this cumulative content with constant `message_id: "response"` to always overwrite
4. Helix receives this and stores it as the response for the CURRENT interaction

### Example Flow

```
t1: Initial prompt → AI responds with tool calls + assistant message
    Thread entries: [UserMessage, ToolCall, ToolCall, AssistantMessage]
    Cumulative content sent: "tool call 1... tool call 2... response text"
    Stored in int_001.ResponseMessage ✓ (correct)

t2: User sends "test 1"
    Thread entries: [UserMessage, ToolCall, ToolCall, AssistantMessage, UserMessage]
    Creates int_002 with PromptMessage="test 1"

t3: AI responds to "test 1"
    Thread entries: [UserMessage, ToolCall, ToolCall, AssistantMessage, UserMessage, AssistantMessage]
    Cumulative content sent: "tool call 1... tool call 2... first response... test 1 response"
                              ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
                              BUG: Includes ALL previous non-user entries!
    Stored in int_002.ResponseMessage ✗ (wrong - contains all previous content!)

t4: User sends "test 2"
    Similar bug - cumulative content grows with each interaction
```

### Evidence from Logs

API logs show incoming WebSocket messages with:
- `message_id: "response"` (constant, not entry index)
- `content` containing ALL previous tool calls and responses concatenated

### Why This Was Added

The comment says:
> "This ensures Helix always has the complete current state, even if entries update out of order (e.g., entry 1 streaming while entry 2 is added)."

This was meant to fix streaming issues where entry updates arrive out of order. But it breaks multi-turn conversations because it doesn't distinguish between interactions.

### The Fix

Change the cumulative content logic to only include entries AFTER the last UserMessage:

```rust
// Find the last UserMessage index
let last_user_idx = thread_read.entries().iter()
    .rposition(|e| matches!(e, acp_thread::AgentThreadEntry::UserMessage(_)))
    .unwrap_or(0);

// Only collect entries AFTER the last UserMessage
let mut cumulative_content = String::new();
for (idx, entry) in thread_read.entries().iter().enumerate() {
    if idx <= last_user_idx {
        continue;  // Skip entries before/at the last user message
    }
    // ... rest of collection logic
}
```

This ensures each interaction only receives the AI response content for THAT turn, not all previous turns.

---

## Files Involved

### Helix API
- `api/pkg/server/session_handlers.go` - HTTP handlers, interaction creation
- `api/pkg/server/websocket_external_agent_sync.go` - WebSocket event handlers

### Helix Frontend
- `frontend/src/components/common/RobustPromptInput.tsx` - Prompt buffering
- `frontend/src/components/session/EmbeddedSessionView.tsx` - Session display
- `frontend/src/components/session/Interaction.tsx` - Message rendering

### Zed
- `crates/external_websocket_sync/src/websocket_sync.rs` - WebSocket handler
- `crates/external_websocket_sync/src/thread_service.rs` - Thread management
- `crates/agent_ui/src/acp/thread_view.rs` - Agent panel UI (**ROOT CAUSE**)
