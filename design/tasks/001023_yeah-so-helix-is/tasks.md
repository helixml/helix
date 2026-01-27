# Implementation Tasks

## Phase 1: Zed Boundary-Based Updates (Highest Priority)

- [ ] Add `last_tool_call_status: HashMap<usize, ToolCallStatus>` to `AcpThreadView` in `thread_view.rs`
- [ ] Add `is_status_change()` helper to detect tool call status transitions (pending → completed)
- [ ] Modify `handle_thread_event` for `NewEntry`: always send WebSocket update (this is a boundary)
- [ ] Modify `handle_thread_event` for `EntryUpdated`: only send if `is_status_change()` returns true
- [ ] Ensure `Stopped`, `Error`, `Refusal` events also trigger a final WebSocket update
- [ ] Reset `last_tool_call_status` when new user message is sent (new turn)

## Phase 2: Helix Backend - Interaction-Only Updates

- [ ] Add new WebSocket event type `interaction_update` in `types/enums.go`
- [ ] Update `WebsocketEvent` struct to include optional `Interaction` field (single interaction)
- [ ] Modify `handleMessageAdded()` in `websocket_external_agent_sync.go` to send only the updated interaction
- [ ] Keep full session broadcast for `session_update` (initial load, reconnect, major state changes)
- [ ] Skip the `ListInteractions` DB query when only sending single interaction update

## Phase 3: Helix Frontend - Surgical Cache Updates

- [ ] Add handler for `interaction_update` event type in `streaming.tsx`
- [ ] Implement surgical React Query cache update (find and replace single interaction)
- [ ] Keep existing `session_update` handler for full session syncs
- [ ] Verify `InteractionLiveStream` works correctly with interaction-only updates

## Phase 4: Testing & Verification

- [ ] Test with long agent response (10+ tool calls) - UI should remain responsive
- [ ] Verify tool call status transitions are captured (pending → completed shows in Helix)
- [ ] Verify final state is always sent when turn completes
- [ ] Measure WebSocket message count and size before/after
- [ ] Test reconnection scenario - full state should sync correctly

## Edge Cases (if needed)

- [ ] Consider periodic updates for long-running tool calls (e.g., every 5s)
- [ ] Handle parallel tool calls - ensure all status changes are captured
- [ ] Add logging to track boundary events for debugging