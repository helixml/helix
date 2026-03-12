# Implementation Tasks

## Problem 1: Messages Don't Get Finished

### Go API Fixes
- [x] In `flushAndClearStreamingContext` (`websocket_external_agent_sync.go`): verified — already forces DB write if `sctx.dirty == true` before clearing. Code is correct.
- [x] In `handleMessageCompleted`: after reloading interaction from DB, log a `WARN` if `response_message` is empty (indicates lost content)
- [x] Verify sequential processing of WebSocket messages guarantees `message_added` (final flush) is processed before `message_completed` — confirmed: `handleExternalAgentReceiver` processes messages sequentially in a single goroutine loop

### Frontend Fixes
- [x] In `streaming.tsx` `handleWebsocketEvent`: when `interaction_update` arrives with `state === "complete"`, clear `patchContentRef` for that interaction ID AND cancel pending RAF to prevent stale patches overriding final content
- [x] In `useLiveInteraction`: when `isComplete` becomes true, prioritize `initialInteraction.response_message` (React Query cache) over `lastKnownMessage`
- [x] Safety net: if `isComplete` is true and `message` is empty, trigger a session refetch

## Problem 2: Tool Calls Need Collapsing

### Frontend Implementation
- [x] Create `CollapsibleToolCall` component + `parseToolCallBlocks` parser function in `frontend/src/components/session/CollapsibleToolCall.tsx`
- [x] Integrate into `InteractionInference.tsx` — split message by tool call blocks, render each via `<CollapsibleToolCall>` or `<Markdown>`
- [x] Handle streaming: only collapse complete tool call blocks (must have header + status line)
- [x] Apply same collapsing in `InteractionLiveStream.tsx` for live messages

## Problem 3: Session/Thread Switching Mess

### Frontend Fixes — streaming.tsx
- [x] In `handleWebsocketEvent`: add explicit session ID filtering — discard events for non-current sessions
- [x] Enhance `clearSessionData` to also clear `patchContentRef`, `patchPendingRef`, `stepInfos`
- [x] On `setCurrentSessionId` with new value: clear old session data before setting new ID

### Frontend Fixes — useLiveInteraction
- [x] Reset `lastKnownMessage` and `currentInteractionId` when `sessionId` changes

### Frontend Fixes — EmbeddedSessionView
- [x] Reset `hasInitiallyScrolled` ref when `sessionId` changes
- [x] Remove old session React Query cache on session switch to prevent stale flash

## Testing & Build
- [x] `yarn build` passes in `frontend/`
- [x] `go build ./pkg/server/` passes in `api/` (pre-existing tree-sitter dep issue, not from our changes)
- [ ] Manual test: verify messages complete, tool calls collapse, session switching is clean