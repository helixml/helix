# Implementation Tasks

## Phase 1: Zed Boundary-Based Updates

- [ ] Add `last_tool_call_status: HashMap<usize, ToolCallStatus>` to `AcpThreadView` in `thread_view.rs`
- [ ] Add `is_status_change()` helper to detect tool call status transitions (pending → completed)
- [ ] Modify `handle_thread_event` for `NewEntry`: always send WebSocket update (this is a boundary)
- [ ] Modify `handle_thread_event` for `EntryUpdated`: only send if `is_status_change()` returns true
- [ ] Ensure `Stopped`, `Error`, `Refusal` events also trigger a final WebSocket update
- [ ] Reset `last_tool_call_status` when new user message is sent (new turn)

## Phase 2: Testing & Verification

- [ ] Test with long agent response (10+ tool calls) - UI should remain responsive
- [ ] Verify tool call status transitions are captured (pending → completed shows in Helix)
- [ ] Verify final state is always sent when turn completes
- [ ] Test reconnection scenario - full state should sync correctly
- [ ] Measure WebSocket message count before/after (should be ~100x reduction)

## Phase 3: Edge Cases (if needed)

- [ ] Consider periodic updates for long-running tool calls (e.g., every 5s)
- [ ] Handle parallel tool calls - ensure all status changes are captured
- [ ] Add logging to track boundary events for debugging

## Out of Scope (for now)

- No WebSocket protocol changes needed
- No Helix frontend changes needed
- No delta tracking or sequence numbers