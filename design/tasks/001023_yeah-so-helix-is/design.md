# Design: Boundary-Based Update Architecture

## Overview

Replace per-token full-state updates with boundary-based updates to achieve O(n) complexity instead of O(n²).

## Why Not Deltas?

The original delta-based approach assumed append-only streaming. This is wrong because:

1. **Parallel tool calls** - Zed can run multiple commands simultaneously with interleaved updates
2. **Status transitions** - Tool calls change from "pending" → "completed", modifying rendered content
3. **Non-monotonic updates** - Content can change anywhere in the message, not just at the end

Delta tracking would be complex, error-prone, and hard to debug.

## Architecture

### Current Flow (O(n²))

```
Every token arrives in Zed
    ↓
emit EntryUpdated(idx)
    ↓
Iterate ALL entries after last user message
    ↓
Serialize cumulative content (grows with N)
    ↓
WebSocket: send full content (on EVERY token)
    ↓
Helix: MessageProcessor.process(full text)
    ↓
Helix: react-markdown parses full text
```

### Proposed Flow (O(n))

```
Token arrives in Zed
    ↓
Append to current entry (no WebSocket send)
    ↓
... more tokens ...
    ↓
BOUNDARY EVENT (new entry, tool call status change, turn complete)
    ↓
Serialize current state
    ↓
WebSocket: send full content (only at boundaries)
    ↓
Helix: MessageProcessor.process(full text)
    ↓
Helix: react-markdown parses full text
```

## Key Insight

The conversation has natural **boundaries**:

1. **User message sent** - new turn begins
2. **Assistant text chunk complete** - LLM finished a text segment before tool call
3. **Tool call starts** - new tool call entry created
4. **Tool call completes** - status changes from pending to completed
5. **Turn complete** - agent stops, ready for next user input

Instead of sending updates on every token, send updates only at these boundaries.

## Key Changes

### 1. Zed Side: Boundary Detection

**Location**: `zed/crates/agent_ui/src/acp/thread_view.rs`

Change `handle_thread_event` to only send WebSocket updates on boundary events:

```rust
fn handle_thread_event(&mut self, thread: &Entity<AcpThread>, event: &AcpThreadEvent, ...) {
    match event {
        // BOUNDARY: New entry (user message, new tool call, new assistant chunk)
        AcpThreadEvent::NewEntry => {
            self.sync_entry_view(...);
            self.send_full_state_to_helix(thread, cx);  // Send update
        }
        
        // NOT a boundary: streaming token within existing entry
        AcpThreadEvent::EntryUpdated(index) => {
            self.sync_entry_view(...);
            
            // Only send if this is a STATUS change (tool call completed)
            // NOT for every streaming token
            if self.is_status_change(thread, *index, cx) {
                self.send_full_state_to_helix(thread, cx);
            }
        }
        
        // BOUNDARY: Turn complete
        AcpThreadEvent::Stopped => {
            self.send_full_state_to_helix(thread, cx);  // Final update
        }
        
        // ... other events
    }
}

fn is_status_change(&self, thread: &Entity<AcpThread>, index: usize, cx: &App) -> bool {
    // Check if tool call status changed (pending -> completed)
    // This is a boundary worth sending
    if let Some(AgentThreadEntry::ToolCall(tool_call)) = thread.read(cx).entries().get(index) {
        let current_status = tool_call.status;
        let previous_status = self.last_known_status.get(&index);
        if Some(&current_status) != previous_status {
            self.last_known_status.insert(index, current_status);
            return true;
        }
    }
    false
}
```

### 2. Track Tool Call Status

Add state to `AcpThreadView` to detect status transitions:

```rust
pub struct AcpThreadView {
    // ... existing fields
    
    /// Track last known tool call status to detect transitions
    last_tool_call_status: HashMap<usize, ToolCallStatus>,
}
```

### 3. No Protocol Changes Needed

The existing `MessageAdded` event works fine - we're just sending it less frequently.

## Boundary Events Summary

| Event | Send Update? | Rationale |
|-------|--------------|-----------|
| `NewEntry` (user message) | ✅ Yes | New turn started |
| `NewEntry` (assistant text) | ✅ Yes | New chunk of response |
| `NewEntry` (tool call) | ✅ Yes | Tool call started |
| `EntryUpdated` (streaming text) | ❌ No | Just more tokens |
| `EntryUpdated` (tool status change) | ✅ Yes | Tool completed |
| `Stopped` | ✅ Yes | Turn complete, final state |

## Performance Comparison

For a response with 1000 tokens, 3 tool calls:

| Metric | Current | Proposed |
|--------|---------|----------|
| WebSocket messages | 1000 | ~7 (user + 3 tool starts + 3 tool completes + stopped) |
| Total chars sent | ~500,000 | ~7,000 (7 × ~1000 chars final state) |
| React re-renders | 1000 | ~7 |

## Trade-offs

**Pros:**
- Simple implementation - just add conditions around existing send logic
- No protocol changes
- Dramatic reduction in updates (100x fewer)
- No complex delta tracking or sequence numbers

**Cons:**
- Less smooth streaming - user sees text appear in chunks, not character by character
- Slight delay before seeing tool call output (until completion)

**Why this is acceptable:**
- For long agent sessions, responsiveness matters more than smoothness
- Users care about seeing the structure (which tools ran, what happened) not individual characters
- Current behavior is "unusable" - chunky but responsive is a huge improvement

## Risks

1. **Missing final state**: If `Stopped` event is missed, UI might be stale
   - Mitigation: Also send on `Error`, `Refusal`, etc.

2. **Long-running tool calls**: User sees nothing until tool completes
   - Mitigation: Could add periodic updates (every 5s) for long-running tools
   - Or: Send update when tool output starts streaming, not just when complete

## Decision

Implement boundary-based updates:
1. Send on `NewEntry` (always)
2. Send on `EntryUpdated` only if tool call status changed
3. Send on `Stopped`, `Error`, `Refusal`
4. Remove sends from pure streaming token updates

This is a minimal change to `thread_view.rs` that should fix the O(n²) problem.

---

## Helix-Side Optimization: Full Session Broadcasts

### The Problem

Even after fixing the Zed side, there's a second O(n²) issue on the Helix side:

**Current flow in `websocket_external_agent_sync.go`:**
```go
// handleMessageAdded() - called on every message_added from Zed
func handleMessageAdded(...) {
    // ... update interaction in DB ...
    
    // Reload ENTIRE session with ALL interactions
    allInteractions, _ := store.ListInteractions(...)
    reloadedSession.Interactions = allInteractions
    
    // Broadcast ENTIRE session to frontend
    publishSessionUpdateToFrontend(reloadedSession, ...)
}
```

**What gets sent via WebSocket:**
```go
type WebsocketEvent struct {
    Type      string   `json:"type"`       // "session_update"
    SessionID string   `json:"session_id"`
    Session   *Session `json:"session"`    // Contains ALL interactions!
}

type Session struct {
    // ...
    Interactions []*Interaction `json:"interactions"`  // O(n) data
}
```

**Frontend receives:**
- Full session JSON with all interactions (could be 100+ for long conversations)
- React Query cache is updated: `queryClient.setQueryData(GET_SESSION_QUERY_KEY(sessionId), { data: parsedData.session })`
- This triggers re-render of components watching this query

### Why This Is Expensive

For a session with N interactions:
1. Backend: Load N interactions from DB on every update
2. Backend: Serialize N interactions to JSON
3. Network: Send N interactions over WebSocket
4. Frontend: Parse N interactions from JSON
5. Frontend: Update React Query cache with N interactions
6. Frontend: Re-render any components watching session data

Even if we reduce Zed→Helix updates to boundaries only, each boundary update still sends the full session.

### Proposed Fix: Interaction-Only Updates

Add a new WebSocket event type that sends only the updated interaction:

```go
// New event type
type WebsocketEvent struct {
    Type               string       `json:"type"`
    SessionID          string       `json:"session_id"`
    InteractionID      string       `json:"interaction_id"`
    Session            *Session     `json:"session,omitempty"`      // Full session (for initial load, reconnect)
    Interaction        *Interaction `json:"interaction,omitempty"` // Single interaction (for updates)
}
```

**Backend change:**
```go
func handleMessageAdded(...) {
    // ... update interaction in DB ...
    
    // Send ONLY the updated interaction, not full session
    event := &WebsocketEvent{
        Type:          "interaction_update",  // New type
        SessionID:     helixSessionID,
        InteractionID: targetInteraction.ID,
        Interaction:   targetInteraction,     // Just this one
    }
    publishToFrontend(event)
}
```

**Frontend change (streaming.tsx):**
```typescript
if (parsedData.type === "interaction_update" && parsedData.interaction) {
    // Update only the specific interaction in cache
    queryClient.setQueryData(
        GET_SESSION_QUERY_KEY(currentSessionId),
        (old: { data?: TypesSession }) => {
            if (!old?.data) return old;
            const interactions = [...(old.data.interactions || [])];
            const idx = interactions.findIndex(i => i.id === parsedData.interaction.id);
            if (idx >= 0) {
                interactions[idx] = parsedData.interaction;
            }
            return { data: { ...old.data, interactions } };
        }
    );
}
```

### Performance Comparison (Helix Side)

For a session with 50 interactions, updating the last one:

| Metric | Current (full session) | Proposed (interaction only) |
|--------|------------------------|----------------------------|
| DB queries | Load 50 interactions | Load 1 interaction |
| JSON size | ~50KB | ~1KB |
| React re-renders | Full session tree | Single interaction |

### Implementation Order

1. **Phase 1 (Zed side)**: Boundary-based updates - biggest impact, simplest change
2. **Phase 2 (Helix backend)**: Add `interaction_update` event type
3. **Phase 3 (Helix frontend)**: Handle `interaction_update` with surgical cache update

Phase 1 alone should make the UI usable. Phases 2-3 are further optimizations.