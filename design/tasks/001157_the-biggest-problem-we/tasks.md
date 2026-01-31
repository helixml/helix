# Implementation Tasks

## Phase 1: Persist WaitingInteractionID

- [x] Add `WaitingInteractionID` field to `SessionMetadata` struct in `api/pkg/types/session.go`
- [x] In `session_handlers.go:streamFromExternalAgent`, persist `WaitingInteractionID` to database when setting `sessionToWaitingInteraction`
- [x] In `websocket_external_agent_sync.go:handleExternalAgentSync`, restore `sessionToWaitingInteraction` from `session.Metadata.WaitingInteractionID` on reconnect
- [x] Clear `WaitingInteractionID` in metadata when interaction completes (in `handleMessageCompleted`)

## Phase 2: Persist RequestID Mapping

- [x] Add `LastRequestID` field to `SessionMetadata` struct (done in Phase 1)
- [x] Persist `LastRequestID` when storing `requestToSessionMapping` (done in Phase 1)
- [x] Restore `requestToSessionMapping` on agent reconnect (done in Phase 1)

## Phase 3: Handle Stale Requests

- [x] Add `RequestStartedAt` timestamp to `SessionMetadata` (done in Phase 1)
- [x] On reconnect, check if `WaitingInteractionID` is stale (>5 min old)
- [x] If stale, mark interaction as failed and clear mappings
- [x] Log warning when detecting stale requests

## Phase 4: Cleanup Dead Code

- [x] Verify `externalAgentSessionMapping` is unused (grep for writes)
- [x] Remove `externalAgentSessionMapping` field from `HelixAPIServer` struct
- [x] Remove initialization in `NewServer`
- [x] Remove any read references (also removed `externalAgentUserMapping` - same issue)

## Phase 5: Testing (User Verification Required)

- [ ] Manual test: start session, restart API, verify messages route correctly
- [ ] Verify logs show "Restored sessionToWaitingInteraction" message
- [ ] Test streaming request interrupted by restart times out with error (not hang)

Note: These tests require a running Helix environment with an active agent session.
Code changes are complete - ready for user to verify behavior.