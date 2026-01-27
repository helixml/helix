# Implementation Tasks

## Phase 1: Zed Delta Tracking (Highest Impact)

- [ ] Add `StreamingState` struct to track last-sent content length per entry in `thread_view.rs`
- [ ] Modify `handle_thread_event` for `EntryUpdated` to compute delta instead of full content
- [ ] Add new `SyncEvent::MessageDelta` variant with `delta` and `offset` fields
- [ ] Send delta for streaming appends, full content for structure changes (new tool call, etc.)
- [ ] Reset tracking state when new user message is sent

## Phase 2: Helix WebSocket Handler

- [ ] Add handler for `message_delta` event type in `streaming.tsx`
- [ ] Accumulate deltas into existing message content (simple string append)
- [ ] Keep `message_added` handler for full-state syncs (reconnect, structure changes)
- [ ] Add sequence number validation to detect missed deltas

## Phase 3: Helix Frontend Optimization

- [ ] Throttle `MessageProcessor.process()` during streaming (500ms debounce)
- [ ] Skip expensive processing (citations, document IDs) during streaming
- [ ] Only apply full processing when streaming completes
- [ ] Profile and verify CPU usage is now O(n) not O(n²)

## Phase 4: Robustness

- [ ] Send full-state checkpoint every 100 deltas (or 10 seconds)
- [ ] Request full sync on WebSocket reconnect
- [ ] Handle content replacement (agent modifies earlier content) with full sync

## Verification

- [ ] Test with 10,000 token response - UI should remain responsive
- [ ] Measure WebSocket traffic before/after (should be ~100x reduction)
- [ ] Verify no visual regression in streaming cursor, thinking widgets, markdown
- [ ] Test reconnection scenario - should recover cleanly