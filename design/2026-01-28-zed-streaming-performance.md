# Zed Streaming Performance: Boundary-Based Update Architecture

**Source:** helix-specs commit c440920e0

---

## Requirements

### Problem Statement

Helix chat widget becomes unusably slow when rendering long agent responses. There are two O(n²) problems:

1. **Zed side**: On every token, the entire conversation is serialized and sent to Helix
2. **Helix side**: On every update, the entire session (all interactions) is broadcast to the frontend

### User Stories

1. **As a user**, I want to chat with agents without the UI becoming sluggish during long responses.
2. **As a user**, I want streaming responses to feel smooth and responsive, regardless of conversation length.

### Acceptance Criteria

1. Streaming a response with 10+ tool calls should not cause noticeable UI lag
2. CPU usage during streaming should remain relatively constant (not grow with message length)
3. WebSocket message count should be proportional to conversation structure (turns, tool calls), not token count
4. No visual regression - tool call status, completion state, and markdown rendering still work correctly

### Current Behavior (O(n²))

For each token received:
1. **Zed** emits `EntryUpdated` event
2. **Zed** iterates all entries after last user message, collecting cumulative content
3. **Zed** sends full cumulative content via WebSocket (`MessageAdded` event)
4. **Helix** receives full content, stores it
5. **Helix** `MessageProcessor.process()` runs regex transformations on full text
6. **Helix** `react-markdown` parses and renders full markdown

**Result**: Token N causes O(N) work → Total work = O(N²)

### Why Deltas Don't Work

The Zed display is **not append-only**:
- Parallel tool calls update interleaved content
- Tool calls transition from "pending" to "completed", changing rendered output
- Content can change anywhere, not just at the end

### Proposed Behavior (O(n))

#### Zed Side
Send updates only at **boundaries**:
- New entry created (user message, tool call, assistant text chunk)
- Tool call status changes (pending → completed)
- Turn completes (stopped, error, refusal)

#### Helix Side
Send only the updated interaction, not full session:
- New WebSocket event type `interaction_update` with single interaction
- Frontend surgically updates React Query cache instead of replacing full session
- Keep full session broadcast for initial load and reconnection

Trade-off: Less smooth character-by-character streaming, but dramatically better responsiveness.

### Constraints

- Must maintain compatibility with existing WebSocket protocol (minimal changes)
- Cannot break Helix sessions that don't use external agents (Zed)
- Acceptable to show chunky updates instead of smooth streaming

---

## Design

### Overview

Replace per-token full-state updates with boundary-based updates to achieve O(n) complexity instead of O(n²).

### Why Not Deltas?

The original delta-based approach assumed append-only streaming. This is wrong because:

1. **Parallel tool calls** - Zed can run multiple commands simultaneously with interleaved updates
2. **Status transitions** - Tool calls change from "pending" → "completed", modifying rendered content
3. **Non-monotonic updates** - Content can change anywhere in the message, not just at the end

Delta tracking would be complex, error-prone, and hard to debug.

### Architecture

#### Current Flow (O(n²))

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

#### Proposed Flow (O(n))

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

### Key Insight

The conversation has natural **boundaries**:

1. **User message sent** - new turn begins
2. **Assistant text chunk complete** - LLM finished a text segment before tool call
3. **Tool call starts** - new tool call entry created
4. **Tool call completes** - status changes from pending to completed
5. **Turn complete** - agent stops, ready for next user input

Instead of sending updates on every token, send updates only at these boundaries.

### Key Changes

#### 1. Zed Side: Boundary Detection

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

#### 2. Track Tool Call Status

Add state to `AcpThreadView` to detect status transitions:

```rust
pub struct AcpThreadView {
    // ... existing fields

    /// Track last known tool call status to detect transitions
    last_tool_call_status: HashMap<usize, ToolCallStatus>,
}
```

#### 3. No Protocol Changes Needed

The existing `MessageAdded` event works fine - we're just sending it less frequently.

### Boundary Events Summary

| Event | Send Update? | Rationale |
|-------|--------------|-----------|
| `NewEntry` (user message) | ✅ Yes | New turn started |
| `NewEntry` (assistant text) | ✅ Yes | New chunk of response |
| `NewEntry` (tool call) | ✅ Yes | Tool call started |
| `EntryUpdated` (streaming text) | ❌ No | Just more tokens |
| `EntryUpdated` (tool status change) | ✅ Yes | Tool completed |
| `Stopped` | ✅ Yes | Turn complete, final state |

### Performance Comparison

For a response with 1000 tokens, 3 tool calls:

| Metric | Current | Proposed |
|--------|---------|----------|
| WebSocket messages | 1000 | ~7 (user + 3 tool starts + 3 tool completes + stopped) |
| Total chars sent | ~500,000 | ~7,000 (7 × ~1000 chars final state) |
| React re-renders | 1000 | ~7 |

### Trade-offs

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

### Risks

1. **Missing final state**: If `Stopped` event is missed, UI might be stale
   - Mitigation: Also send on `Error`, `Refusal`, etc.

2. **Long-running tool calls**: User sees nothing until tool completes
   - Mitigation: Could add periodic updates (every 5s) for long-running tools
   - Or: Send update when tool output starts streaming, not just when complete

### Decision

Implement boundary-based updates:
1. Send on `NewEntry` (always)
2. Send on `EntryUpdated` only if tool call status changed
3. Send on `Stopped`, `Error`, `Refusal`
4. Remove sends from pure streaming token updates

This is a minimal change to `thread_view.rs` that should fix the O(n²) problem.

---

### Helix-Side Optimization: Full Session Broadcasts

#### The Problem

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

#### Why This Is Expensive

For a session with N interactions:
1. Backend: Load N interactions from DB on every update
2. Backend: Serialize N interactions to JSON
3. Network: Send N interactions over WebSocket
4. Frontend: Parse N interactions from JSON
5. Frontend: Update React Query cache with N interactions
6. Frontend: Re-render any components watching session data

Even if we reduce Zed→Helix updates to boundaries only, each boundary update still sends the full session.

#### Proposed Fix: Interaction-Only Updates

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

#### Performance Comparison (Helix Side)

For a session with 50 interactions, updating the last one:

| Metric | Current (full session) | Proposed (interaction only) |
|--------|------------------------|----------------------------|
| DB queries | Load 50 interactions | Load 1 interaction |
| JSON size | ~50KB | ~1KB |
| React re-renders | Full session tree | Single interaction |

#### Implementation Order

1. **Phase 1 (Zed side)**: Boundary-based updates - biggest impact, simplest change
2. **Phase 2 (Helix backend)**: Add `interaction_update` event type
3. **Phase 3 (Helix frontend)**: Handle `interaction_update` with surgical cache update

Phase 1 alone should make the UI usable. Phases 2-3 are further optimizations.

---

## Implementation Tasks

### Phase 1: Zed Boundary-Based Updates (Highest Priority)

- [ ] Add `last_tool_call_status: HashMap<usize, ToolCallStatus>` to `AcpThreadView` in `thread_view.rs`
- [ ] Add `is_status_change()` helper to detect tool call status transitions (pending → completed)
- [ ] Modify `handle_thread_event` for `NewEntry`: always send WebSocket update (this is a boundary)
- [ ] Modify `handle_thread_event` for `EntryUpdated`: only send if `is_status_change()` returns true
- [ ] Ensure `Stopped`, `Error`, `Refusal` events also trigger a final WebSocket update
- [ ] Reset `last_tool_call_status` when new user message is sent (new turn)

### Phase 2: Helix Backend - Interaction-Only Updates

- [ ] Add new WebSocket event type `interaction_update` in `types/enums.go`
- [ ] Update `WebsocketEvent` struct to include optional `Interaction` field (single interaction)
- [ ] Modify `handleMessageAdded()` in `websocket_external_agent_sync.go` to send only the updated interaction
- [ ] Keep full session broadcast for `session_update` (initial load, reconnect, major state changes)
- [ ] Skip the `ListInteractions` DB query when only sending single interaction update

### Phase 3: Helix Frontend - Surgical Cache Updates

- [ ] Add handler for `interaction_update` event type in `streaming.tsx`
- [ ] Implement surgical React Query cache update (find and replace single interaction)
- [ ] Keep existing `session_update` handler for full session syncs
- [ ] Verify `InteractionLiveStream` works correctly with interaction-only updates

### Phase 4: Testing & Verification

- [ ] Test with long agent response (10+ tool calls) - UI should remain responsive
- [ ] Verify tool call status transitions are captured (pending → completed shows in Helix)
- [ ] Verify final state is always sent when turn completes
- [ ] Measure WebSocket message count and size before/after
- [ ] Test reconnection scenario - full state should sync correctly

### Edge Cases (if needed)

- [ ] Consider periodic updates for long-running tool calls (e.g., every 5s)
- [ ] Handle parallel tool calls - ensure all status changes are captured
- [ ] Add logging to track boundary events for debugging
