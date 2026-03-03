# Implementation Tasks

## Backend (Go)

- [x] Add `requestID string` field to `streamingContext` struct in `websocket_external_agent_sync.go`
- [x] In `handleMessageAdded`, extract `request_id` from `syncMsg.Data` and store in streaming context on creation
- [x] Update `publishInteractionPatchToFrontend` signature to accept optional `requestID ...string` parameter
- [x] In `publishInteractionPatchToFrontend`, if `requestID` provided, look up commenter from `requestToCommenterMapping` and publish patch to commenter's queue
- [x] Pass `sctx.requestID` when calling `publishInteractionPatchToFrontend` from `handleMessageAdded`

## Testing

- [ ] Manual test: Submit comment as non-owner user, verify streaming response appears in real-time
- [ ] Manual test: Verify owner still sees streaming in main chat view
- [ ] Manual test: Verify comment queue still processes multiple comments sequentially
- [ ] Run existing Go tests: `go test -v ./api/pkg/server/... -run WebSocket`

## Verification

- [~] Build succeeds: `go build ./api/pkg/server/`
- [ ] Frontend build succeeds: `cd frontend && yarn build`
- [ ] No regressions in streaming performance (patches still sent, not full updates)