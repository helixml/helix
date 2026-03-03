# Implementation Tasks

## Investigation & Debugging

- [x] Add debug logging to backend `getOrCreateStreamingContext()` to log interaction ID transitions
- [x] Add debug logging to frontend patch handler to log when interaction ID changes
- [x] Reproduce bug with browser DevTools Network tab open to capture WebSocket messages
- [x] Capture exact sequence of events: `interaction_patch` vs `interaction_update` timing
- [x] Check API logs for `message_completed` handling - look for "No waiting interaction found" warnings

## Backend Fixes - Streaming Context (helix/api/pkg/server/websocket_external_agent_sync.go)

- [x] Add `interactionID` field to `streamingContext` struct to track current interaction
- [x] Modify `getOrCreateStreamingContext()` to detect interaction transitions
- [x] Reset `previousContent` to `""` when interaction ID changes within same session
- [x] Flush dirty state for old interaction before switching to new interaction
- [x] Add logging for interaction transitions to aid debugging
- [ ] Write unit test `TestStreamingPatch_InteractionTransition` for interaction change scenario

## Backend Fixes - Interaction Completion (helix/api/pkg/server/websocket_external_agent_sync.go)

- [ ] In `handleMessageCompleted()`: change silent `return nil` to explicit error returns or ERROR-level logs
- [ ] Add fallback: if no `waiting` interaction found, look for any non-`complete` interaction
- [ ] Ensure `flushAndClearStreamingContext()` is called even when session mapping is found via DB fallback
- [ ] Add more robust session lookup when `contextMappings` is empty (API restart scenario)
- [ ] Write unit test `TestMessageCompleted_NoWaitingInteraction` for edge case handling

## Frontend Fixes (helix/frontend/src/contexts/streaming.tsx)

- [x] In patch handler: detect when incoming `interaction_id` differs from `currentResponses[sessionId].id`
- [x] When interaction transition detected, start with clean slate (don't spread old interaction data)
- [x] Clear `patchContentRef` for old interaction when new interaction starts streaming
- [ ] Consider delaying `currentResponses` overwrite until React Query cache is updated
- [ ] Ensure `interaction_update` for completed interaction doesn't clobber new interaction's streaming state

## Frontend Fixes (helix/frontend/src/hooks/useLiveInteraction.ts)

- [ ] Review `lastKnownMessage` preservation logic for interaction transitions
- [ ] Ensure old interaction content remains visible until new interaction has content
- [ ] Add handling for case where `initialInteraction` changes but `currentResponse` is stale

## Testing

- [ ] Run existing E2E test: `cd zed/crates/external_websocket_sync/e2e-test && ./run_docker_e2e.sh`
- [x] Manual test: "say hello" → wait → "list contents of /tmp" → verify no content loss
- [ ] Test rapid follow-ups: send second message before first completes
- [ ] Test tool calls that modify then restore content (backwards patches)
- [x] Run frontend build: `cd frontend && yarn build`
- [ ] Run backend tests: `go test -v -run TestStreamingPatch ./pkg/server/ -count=1`
- [ ] Run backend tests: `go test -v -run TestMessageCompleted ./pkg/server/ -count=1`

## Verification

- [x] Verify previous interaction response stays visible during follow-up streaming
- [x] Verify new interaction response appears correctly after tool calls
- [x] Verify no duplicate content or flashing
- [x] Check browser console for errors during interaction transitions
- [ ] Verify interaction state changes to `complete` after agent finishes (check DB or API response)
- [ ] Verify UI stops showing "in progress" spinner after completion
- [ ] Check API logs confirm `message_completed` was processed successfully (look for "Marked interaction as complete" log)

## Implementation Notes

### Root Cause
The streaming context was cached per-session but didn't track which interaction it was for. When a follow-up message created a new interaction, `getOrCreateStreamingContext()` returned the cached context with the OLD interaction, causing:
1. Patches computed against wrong `previousContent` (from old interaction)
2. Frontend spreading old interaction data into new interaction state

### Backend Fix (c6ad549f1)
- Added `interactionID` field to `streamingContext` struct
- In `getOrCreateStreamingContext()`: check `sessionToWaitingInteraction` mapping for expected interaction ID
- If mismatch detected: flush dirty state, reset `previousContent`, re-query DB for new interaction
- Update existing context instead of creating new one on transitions

### Frontend Fix (66d4ffe33)
- In patch handler: detect when `interactionId` differs from `currentResponses[sessionId].id`
- On transition: start with clean slate instead of spreading old interaction data
- Clear old interaction from `patchContentRef`
