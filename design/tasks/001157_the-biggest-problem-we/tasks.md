# Implementation Tasks

## Phase 1: Persist WaitingInteractionID

- [x] Add `WaitingInteractionID` field to `SessionMetadata` struct in `api/pkg/types/session.go`
- [~] In `session_handlers.go:streamFromExternalAgent`, persist `WaitingInteractionID` to database when setting `sessionToWaitingInteraction`
- [ ] In `websocket_external_agent_sync.go:handleExternalAgentSync`, restore `sessionToWaitingInteraction` from `session.Metadata.WaitingInteractionID` on reconnect
- [ ] Clear `WaitingInteractionID` in metadata when interaction completes (in `handleMessageCompleted`)

## Phase 2: Persist RequestID Mapping

- [ ] Add `LastRequestID` field to `SessionMetadata` struct
- [ ] Persist `LastRequestID` when storing `requestToSessionMapping`
- [ ] Restore `requestToSessionMapping` on agent reconnect

## Phase 3: Handle Stale Requests

- [ ] Add `RequestStartedAt` timestamp to `SessionMetadata`
- [ ] On reconnect, check if `WaitingInteractionID` is stale (>5 min old)
- [ ] If stale, mark interaction as failed and clear mappings
- [ ] Log warning when detecting stale requests

## Phase 4: Cleanup Dead Code

- [ ] Verify `externalAgentSessionMapping` is unused (grep for writes)
- [ ] Remove `externalAgentSessionMapping` field from `HelixAPIServer` struct
- [ ] Remove initialization in `NewServer`
- [ ] Remove any read references

## Phase 5: Testing

- [ ] Manual test: start session, restart API, verify messages route correctly
- [ ] Verify logs show "Restored sessionToWaitingInteraction" message
- [ ] Test streaming request interrupted by restart times out with error (not hang)