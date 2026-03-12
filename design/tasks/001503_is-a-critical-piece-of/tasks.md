# Implementation Tasks

## Problem 1: Messages Don't Get Finished

### Investigation / Reproduction
- [ ] Add structured logging in Go API `flushAndClearStreamingContext` to log content length before and after flush, and whether `dirty` flag was true
- [ ] Add structured logging in Go API `handleMessageCompleted` to compare `response_message` length after DB reload vs what was last seen in streaming context
- [ ] Add a frontend debug mode (console log) in `useLiveInteraction` that logs: message length at each state transition (streaming → lastKnownMessage → completed cache)
- [ ] Reproduce the bug: send a long message to agent, watch sidebar, confirm truncation. Check Go API logs for content length discrepancy between last `message_added` and final DB state

### Go API Fixes
- [ ] In `flushAndClearStreamingContext` (`websocket_external_agent_sync.go`): add guard to force DB write if `sctx.dirty == true` before clearing the context — verify this path actually executes
- [ ] In `handleMessageCompleted`: after reloading interaction from DB, log a `WARN` if `response_message` is empty or shorter than 50 chars (likely indicates lost content)
- [ ] Verify that the sequential processing of WebSocket messages guarantees `message_added` (final flush) is fully processed before `message_completed` runs — check if there's any goroutine dispatch that could reorder them

### Frontend Fixes
- [ ] In `streaming.tsx` `handleWebsocketEvent`: when `interaction_update` arrives with `state === "complete"`, clear `patchContentRef` for that interaction ID to prevent stale patches from overriding final content
- [ ] In `useLiveInteraction`: when `isComplete` becomes true, prioritize reading `response_message` from `initialInteraction` (React Query cache, which gets updated by `interaction_update`) over `lastKnownMessage` from streaming
- [ ] Add a safety net: if `isComplete` is true and `message` is empty, trigger a refetch of the session to recover the final content from DB

## Problem 2: Tool Calls Need Collapsing

### Frontend Implementation
- [ ] Create `CollapsibleToolCall` React component in `frontend/src/components/session/CollapsibleToolCall.tsx` — renders collapsed summary line (icon + tool name + status) with expand/collapse toggle
- [ ] Write a parser function `parseToolCallBlocks(text: string)` that splits response text into segments: regular markdown and tool call blocks. Detect pattern: `**Tool Call: <name>**\nStatus: <status>\n\n<content>` (terminated by next `**Tool Call:` or end of string)
- [ ] Integrate parser into `InteractionInference.tsx`: before passing `message` to `<Markdown>`, split into segments and render tool call segments via `<CollapsibleToolCall>` and regular segments via `<Markdown>`
- [ ] Handle streaming edge case: only collapse tool call blocks that have both the header line AND a `Status:` line. Incomplete blocks (still streaming) render as raw markdown
- [ ] Style the collapsed tool call: small font, muted color, left border accent, hover effect for expand. Match existing UI theme (MUI + light/dark mode)
- [ ] Apply same collapsing in `InteractionLiveStream.tsx` for live streaming messages (not just completed ones)

## Problem 3: Session/Thread Switching Mess

### Frontend Fixes — streaming.tsx
- [ ] In `handleWebsocketEvent`: add explicit session ID check — for `session_update`, verify `parsedData.session?.id === currentSessionId`; for `interaction_update` and `interaction_patch`, verify the interaction belongs to current session (may need to track session→interaction mapping or add session_id to patch events)
- [ ] Enhance `clearSessionData` to also clear: `patchContentRef.current` (clear all entries), `patchPendingRef.current = false`, `stepInfos` for the old session
- [ ] When `setCurrentSessionId` is called with a new value, call `clearSessionData` for the OLD session ID before setting the new one (ensure clean transition)

### Frontend Fixes — useLiveInteraction
- [ ] Add `sessionId` to the dependency array of the effect that resets `lastKnownMessage` and `currentInteractionId` — when sessionId changes, reset everything to prevent stale content leaking across sessions
- [ ] When `sessionId` changes, explicitly set `lastKnownMessage` to empty string and `currentInteractionId` to undefined before processing new data

### Frontend Fixes — EmbeddedSessionView
- [ ] When `sessionId` prop changes, remove the old session's React Query cache entry (`queryClient.removeQueries`) to prevent flash of stale content
- [ ] Reset `hasInitiallyScrolled` ref when `sessionId` changes so the new session gets scrolled to bottom on load

## Testing

- [ ] Manual test: send a long agent message (500+ words), verify full content appears in sidebar after completion
- [ ] Manual test: trigger a multi-tool-call response, verify tool calls are collapsed and expandable
- [ ] Manual test: switch between 3 different sessions rapidly, verify no stale content appears
- [ ] Manual test: switch session while a message is actively streaming, verify old stream stops and new session loads cleanly
- [ ] Verify `yarn build` passes in `frontend/` after all changes
- [ ] Verify `go build ./pkg/server/` passes in `api/` after Go changes