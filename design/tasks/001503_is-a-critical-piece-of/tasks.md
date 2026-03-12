# Implementation Tasks

## Problem 1: Messages Don't Get Finished

### Go API Fixes
- [x] In `flushAndClearStreamingContext` (`websocket_external_agent_sync.go`): verified — already forces DB write if `sctx.dirty == true` before clearing. Code is correct.
- [~] In `handleMessageCompleted`: after reloading interaction from DB, log a `WARN` if `response_message` is empty (indicates lost content)
- [x] Verify sequential processing of WebSocket messages guarantees `message_added` (final flush) is processed before `message_completed` — confirmed: `handleExternalAgentReceiver` processes messages sequentially in a single goroutine loop

### Frontend Fixes
- [~] In `streaming.tsx` `handleWebsocketEvent`: when `interaction_update` arrives with `state === "complete"`, clear `patchContentRef` for that interaction ID AND cancel pending RAF to prevent stale patches overriding final content
- [~] In `useLiveInteraction`: when `isComplete` becomes true, prioritize `initialInteraction.response_message` (React Query cache) over `lastKnownMessage`
- [~] Safety net: if `isComplete` is true and `message` is empty, trigger a session refetch

## Problem 2: Tool Calls Need Collapsing

### Frontend Implementation
- [~] Create `CollapsibleToolCall` component + `parseToolCallBlocks` parser function in `frontend/src/components/session/CollapsibleToolCall.tsx`
- [ ] Integrate into `InteractionInference.tsx` — split message by tool call blocks, render each via `<CollapsibleToolCall>` or `<Markdown>`
- [ ] Handle streaming: only collapse complete tool call blocks (must have header + status line)
- [ ] Apply same collapsing in `InteractionLiveStream.tsx` for live messages

## Problem 3: Session/Thread Switching Mess

### Frontend Fixes — streaming.tsx
- [~] In `handleWebsocketEvent`: add explicit session ID filtering — discard events for non-current sessions
- [~] Enhance `clearSessionData` to also clear `patchContentRef`, `patchPendingRef`, `stepInfos`
- [~] On `setCurrentSessionId` with new value: clear old session data before setting new ID

### Frontend Fixes — useLiveInteraction
- [~] Reset `lastKnownMessage` and `currentInteractionId` when `sessionId` changes

### Frontend Fixes — EmbeddedSessionView
- [~] Reset `hasInitiallyScrolled` ref when `sessionId` changes
- [~] Remove old session React Query cache on session switch to prevent stale flash

## Testing & Build
- [ ] `yarn build` passes in `frontend/`
- [ ] `go build ./pkg/server/` passes in `api/`
- [ ] Manual test: verify messages complete, tool calls collapse, session switching is clean